package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
)

var (
	_ resource.Resource                = &localeResource{}
	_ resource.ResourceWithConfigure   = &localeResource{}
	_ resource.ResourceWithImportState = &localeResource{}
)

func NewLocaleResource() resource.Resource {
	return &localeResource{}
}

type localeResourceClient interface {
	GetCurrent(context.Context) (*cpanellocale.Locale, error)
	Set(context.Context, string) (*cpanellocale.Locale, error)
}

type localeResource struct {
	client localeResourceClient
}

func (r *localeResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_locale"
}

func (r *localeResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages the display locale for the cPanel account.",
		MarkdownDescription: "Manages the display locale for the cPanel account. Removing the resource restores the locale observed when Terraform first took ownership.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"locale": schema.StringAttribute{
				Required:            true,
				Description:         "The abbreviated locale name returned by cPanel.",
				MarkdownDescription: "The abbreviated locale name returned by cPanel `Locale::list_locales`.",
				Validators:          localeCodeValidators(),
			},
			"restore_locale": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale that Terraform restores when the resource is removed.",
				MarkdownDescription: "The locale that Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale name translated into the current cPanel locale.",
				MarkdownDescription: "The locale name translated into the current cPanel locale.",
			},
			"local_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale name written in its own language.",
				MarkdownDescription: "The locale name written in its own language.",
			},
			"direction": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale text direction.",
				MarkdownDescription: "The locale text direction: `ltr` or `rtl`.",
			},
			"encoding": schema.StringAttribute{
				Computed:            true,
				Description:         "The character encoding reported by cPanel.",
				MarkdownDescription: "The character encoding reported by cPanel.",
			},
		},
	}
}

func (r *localeResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state LocaleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetCurrent(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel locale", err.Error())
		return
	}
	if state.RestoreLocale.IsNull() ||
		state.RestoreLocale.IsUnknown() ||
		state.RestoreLocale.ValueString() == "" {
		state.RestoreLocale = types.StringValue(current.Code)
	}
	applyLocaleToResourceModel(&state, *current)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *localeResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan LocaleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	original, err := r.client.GetCurrent(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel locale", err.Error())
		return
	}

	updated, err := r.transition(ctx, *original, plan.Locale.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure cPanel locale", err.Error())
		return
	}

	plan.RestoreLocale = types.StringValue(original.Code)
	applyLocaleToResourceModel(&plan, *updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() && original.Code != updated.Code {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			updated.Code,
			original.Code,
		); rollbackErr != nil {
			resp.Diagnostics.AddError(
				"Unable to restore cPanel locale",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *localeResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan LocaleResourceModel
	var state LocaleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetCurrent(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel locale", err.Error())
		return
	}
	if current.Code != state.Locale.ValueString() {
		resp.Diagnostics.AddError(
			"cPanel locale changed during update",
			"The current cPanel locale no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}
	updated, err := r.transition(ctx, *current, plan.Locale.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update cPanel locale", err.Error())
		return
	}

	if state.RestoreLocale.IsNull() ||
		state.RestoreLocale.IsUnknown() ||
		state.RestoreLocale.ValueString() == "" {
		plan.RestoreLocale = types.StringValue(current.Code)
	} else {
		plan.RestoreLocale = state.RestoreLocale
	}
	applyLocaleToResourceModel(&plan, *updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() && current.Code != updated.Code {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			updated.Code,
			current.Code,
		); rollbackErr != nil {
			resp.Diagnostics.AddError(
				"Unable to restore cPanel locale",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *localeResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state LocaleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.RestoreLocale.IsNull() ||
		state.RestoreLocale.IsUnknown() ||
		state.RestoreLocale.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Unable to restore cPanel locale",
			"The resource state does not contain the locale that preceded Terraform management.",
		)
		return
	}

	if err := r.restoreIfCurrentMatches(
		ctx,
		state.Locale.ValueString(),
		state.RestoreLocale.ValueString(),
	); err != nil {
		resp.Diagnostics.AddError("Unable to restore cPanel locale", err.Error())
	}
}

func (r *localeResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if req.ID != cpanellocale.AccountIdentity {
		resp.Diagnostics.AddError(
			"Invalid cPanel locale import identifier",
			fmt.Sprintf(
				"Use %q to import the account locale.",
				cpanellocale.AccountIdentity,
			),
		)
		return
	}

	current, err := r.client.GetCurrent(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to import cPanel locale",
			err.Error(),
		)
		return
	}

	state := LocaleResourceModel{
		RestoreLocale: types.StringValue(current.Code),
	}
	applyLocaleToResourceModel(&state, *current)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *localeResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["locale"].(*cpanellocale.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Locale Client Type",
			fmt.Sprintf(
				"Expected *locale.Client, got: %T.",
				providerData["locale"],
			),
		)
		return
	}

	r.client = client
}

func (r *localeResource) transition(
	ctx context.Context,
	original cpanellocale.Locale,
	target string,
) (*cpanellocale.Locale, error) {
	if err := cpanellocale.ValidateCode(target); err != nil {
		return nil, err
	}
	if original.Code == target {
		return &original, nil
	}

	updated, err := r.client.Set(ctx, target)
	if err != nil {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			target,
			original.Code,
		)
		return nil, errors.New(localeMutationErrorDetail(err, rollbackErr))
	}

	return updated, nil
}

func (r *localeResource) restoreIfCurrentMatches(
	ctx context.Context,
	expectedCurrent string,
	target string,
) error {
	if err := cpanellocale.ValidateCode(expectedCurrent); err != nil {
		return err
	}
	if err := cpanellocale.ValidateCode(target); err != nil {
		return err
	}

	current, err := r.client.GetCurrent(ctx)
	if err != nil {
		return fmt.Errorf("read cPanel locale before restore: %w", err)
	}
	if current.Code == target {
		return nil
	}
	if current.Code != expectedCurrent {
		return fmt.Errorf(
			"refuse to restore cPanel locale because the current locale %q no longer matches the Terraform transition",
			current.Code,
		)
	}
	restored, err := r.client.Set(ctx, target)
	if err != nil {
		return fmt.Errorf("restore previous cPanel locale: %w", err)
	}
	if restored.Code != target {
		return fmt.Errorf(
			"cPanel locale is %q after restore; expected %q",
			restored.Code,
			target,
		)
	}

	return nil
}

func localeMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel locale: %v",
		primaryError,
		rollbackError,
	)
}
