package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

var (
	_ resource.Resource                = &modSecurityDomainResource{}
	_ resource.ResourceWithConfigure   = &modSecurityDomainResource{}
	_ resource.ResourceWithImportState = &modSecurityDomainResource{}
)

func NewModSecurityDomainResource() resource.Resource {
	return &modSecurityDomainResource{}
}

type modSecurityDomainResource struct {
	client *modsecurity.Client
}

func (r *modSecurityDomainResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_modsecurity_domain"
}

func (r *modSecurityDomainResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages the ModSecurity status for an existing domain in a cPanel account.",
		MarkdownDescription: "Manages the ModSecurity status for an existing domain in a cPanel account. Removing the resource re-enables ModSecurity and preserves the domain.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The existing account domain to manage.",
				MarkdownDescription: "The existing account domain to manage.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether ModSecurity must be enabled for the domain.",
				MarkdownDescription: "Whether ModSecurity must be enabled for the domain.",
			},
			"domain_type": schema.StringAttribute{
				Computed:            true,
				Description:         "The cPanel domain type.",
				MarkdownDescription: "The cPanel domain type: `main` or `sub`.",
			},
			"dependencies": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "Related domains that cPanel reports as affected by changes to this domain.",
				MarkdownDescription: "Related domains that cPanel reports as affected by changes to this domain.",
			},
			"affected_domains": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The managed domain and every related domain that cPanel reports as affected.",
				MarkdownDescription: "The managed domain and every related domain that cPanel reports as affected.",
			},
			"search_hint": schema.StringAttribute{
				Computed:            true,
				Description:         "The search hint reported by cPanel for related domains.",
				MarkdownDescription: "The search hint reported by cPanel for related domains.",
			},
		},
	}
}

func (r *modSecurityDomainResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ModSecurityDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiDomain, err := r.client.Get(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read ModSecurity domain",
			err.Error(),
		)
		return
	}
	if apiDomain == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(
		applyModSecurityDomainToResourceModel(ctx, &state, *apiDomain)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *modSecurityDomainResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan ModSecurityDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := modSecurityDomainDefinitionFromResourceModel(plan)
	if err := validateModSecurityDomainDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid ModSecurity domain", err.Error())
		return
	}

	current, err := r.client.Get(ctx, definition.Domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read ModSecurity domain",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"ModSecurity domain not found",
			fmt.Sprintf(
				"Domain %q must exist in the cPanel ModSecurity inventory before Terraform can manage it.",
				definition.Domain,
			),
		)
		return
	}

	if err := r.reconcile(ctx, *current, definition.Enabled); err != nil {
		resp.Diagnostics.AddError(
			"Unable to configure ModSecurity domain",
			err.Error(),
		)
		return
	}

	apiDomain, err := r.verify(ctx, definition)
	if err != nil {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			definition.Domain,
			definition.Enabled,
			current.Enabled,
		)
		resp.Diagnostics.AddError(
			"Unable to verify ModSecurity domain",
			modSecurityMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyModSecurityDomainToResourceModel(ctx, &plan, *apiDomain)...,
	)
	if resp.Diagnostics.HasError() {
		if current.Enabled != definition.Enabled {
			_ = r.restoreIfCurrentMatches(
				ctx,
				definition.Domain,
				definition.Enabled,
				current.Enabled,
			)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *modSecurityDomainResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan ModSecurityDomainResourceModel
	var state ModSecurityDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := modSecurityDomainDefinitionFromResourceModel(plan)
	if err := validateModSecurityDomainDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid ModSecurity domain", err.Error())
		return
	}

	current, err := r.client.Get(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read ModSecurity domain",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"ModSecurity domain no longer exists",
			"Refresh the Terraform state before updating the ModSecurity status.",
		)
		return
	}
	if !modSecurityDomainMatchesResourceModel(*current, state) {
		resp.Diagnostics.AddError(
			"ModSecurity domain changed during update",
			"The remote ModSecurity status no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.reconcile(ctx, *current, definition.Enabled); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update ModSecurity domain",
			err.Error(),
		)
		return
	}

	apiDomain, err := r.verify(ctx, definition)
	if err != nil {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			definition.Domain,
			definition.Enabled,
			current.Enabled,
		)
		resp.Diagnostics.AddError(
			"Unable to verify ModSecurity domain update",
			modSecurityMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyModSecurityDomainToResourceModel(ctx, &plan, *apiDomain)...,
	)
	if resp.Diagnostics.HasError() {
		if current.Enabled != definition.Enabled {
			_ = r.restoreIfCurrentMatches(
				ctx,
				definition.Domain,
				definition.Enabled,
				current.Enabled,
			)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *modSecurityDomainResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state ModSecurityDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain := state.Domain.ValueString()
	current, err := r.client.Get(ctx, domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read ModSecurity domain",
			err.Error(),
		)
		return
	}
	if current == nil {
		return
	}
	if current.Enabled != state.Enabled.ValueBool() {
		resp.Diagnostics.AddError(
			"Unable to reset ModSecurity domain",
			"The remote ModSecurity status no longer matches Terraform state, so the provider refuses to change it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.reconcile(ctx, *current, true); err != nil {
		resp.Diagnostics.AddError(
			"Unable to reset ModSecurity domain",
			err.Error(),
		)
		return
	}
	if _, err := r.verify(
		ctx,
		modsecurity.Definition{Domain: domain, Enabled: true},
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify ModSecurity domain reset",
			err.Error(),
		)
	}
}

func (r *modSecurityDomainResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	definition := modsecurity.Definition{Domain: req.ID}
	if err := validateModSecurityDomainDefinition(definition); err != nil {
		resp.Diagnostics.AddError(
			"Invalid ModSecurity domain import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...,
	)
}

func (r *modSecurityDomainResource) Configure(
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

	client, ok := providerData["modsecurity"].(*modsecurity.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected ModSecurity Client Type",
			fmt.Sprintf(
				"Expected *modsecurity.Client, got: %T.",
				providerData["modsecurity"],
			),
		)
		return
	}

	r.client = client
}

func (r *modSecurityDomainResource) reconcile(
	ctx context.Context,
	current modsecurity.Domain,
	enabled bool,
) error {
	if current.Enabled == enabled {
		return nil
	}
	if _, err := r.client.SetEnabled(
		ctx,
		[]string{current.Domain},
		enabled,
	); err != nil {
		if cPanelMutationErrorIsDeterministic(err) {
			return err
		}

		observed, reconcileErr := r.client.Get(ctx, current.Domain)
		if reconcileErr != nil {
			return errors.New(modSecurityMutationErrorDetail(
				err,
				fmt.Errorf(
					"read ModSecurity domain after ambiguous mutation: %w",
					reconcileErr,
				),
			))
		}
		if observed != nil && observed.Enabled == enabled {
			return nil
		}

		return err
	}

	return nil
}

func (r *modSecurityDomainResource) verify(
	ctx context.Context,
	expected modsecurity.Definition,
) (*modsecurity.Domain, error) {
	apiDomain, err := r.client.Get(ctx, expected.Domain)
	if err != nil {
		return nil, fmt.Errorf("read ModSecurity domain after mutation: %w", err)
	}
	if apiDomain == nil {
		return nil, fmt.Errorf(
			"domain %q was not found after ModSecurity mutation",
			expected.Domain,
		)
	}
	if apiDomain.Enabled != expected.Enabled {
		return nil, fmt.Errorf(
			"domain %q ModSecurity enabled state is %t; expected %t",
			expected.Domain,
			apiDomain.Enabled,
			expected.Enabled,
		)
	}

	return apiDomain, nil
}

func (r *modSecurityDomainResource) restoreIfCurrentMatches(
	ctx context.Context,
	domain string,
	expectedCurrent bool,
	target bool,
) error {
	current, err := r.client.Get(ctx, domain)
	if err != nil {
		return fmt.Errorf("read ModSecurity domain before restore: %w", err)
	}
	if current == nil {
		return fmt.Errorf(
			"domain %q no longer exists during ModSecurity restore",
			domain,
		)
	}
	if current.Enabled == target {
		return nil
	}
	if current.Enabled != expectedCurrent {
		return fmt.Errorf(
			"refuse to restore previous ModSecurity state because the current status no longer matches the Terraform transition",
		)
	}
	if _, err := r.client.SetEnabled(
		ctx,
		[]string{domain},
		target,
	); err != nil {
		return fmt.Errorf("restore previous ModSecurity state: %w", err)
	}
	if _, err := r.verify(
		ctx,
		modsecurity.Definition{
			Domain:  domain,
			Enabled: target,
		},
	); err != nil {
		return fmt.Errorf("verify restored ModSecurity state: %w", err)
	}

	return nil
}

func modSecurityMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous ModSecurity state: %v",
		primaryError,
		rollbackError,
	)
}
