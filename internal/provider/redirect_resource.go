package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

var (
	_ resource.Resource                = &redirectResource{}
	_ resource.ResourceWithConfigure   = &redirectResource{}
	_ resource.ResourceWithImportState = &redirectResource{}
)

func NewRedirectResource() resource.Resource {
	return &redirectResource{}
}

type redirectResource struct {
	client *cpanelredirect.Client
}

func (r *redirectResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_redirect"
}

func (r *redirectResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one cPanel HTTP redirect rule.",
		MarkdownDescription: "Manages one cPanel HTTP redirect rule.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel account domain that owns the redirect.",
				MarkdownDescription: "The cPanel account domain that owns the redirect.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("/"),
				Description:         "The absolute URL path to redirect.",
				MarkdownDescription: "The absolute URL path to redirect. Defaults to `/`.",
				Validators:          redirectSourceValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				Description:         "The absolute HTTP or HTTPS destination URL.",
				MarkdownDescription: "The absolute HTTP or HTTPS destination URL.",
				Validators:          redirectDestinationValidators(),
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(cpanelredirect.TypePermanent),
				Description:         "Whether the redirect is permanent or temporary.",
				MarkdownDescription: "Whether the redirect is `permanent` (`301`) or `temporary` (`302`).",
				Validators:          redirectTypeValidators(),
			},
			"www_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(cpanelredirect.WWWModeBoth),
				Description:         "Whether the redirect matches both www and non-www requests, or only requests without www.",
				MarkdownDescription: "Whether the redirect matches `both` www and non-www requests, or only requests `without` www. cPanel does not expose enough read metadata to manage its separate www-only mode safely.",
				Validators:          redirectWWWModeValidators(),
			},
			"wildcard": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Whether matching files below the source path keep their relative path below the destination.",
				MarkdownDescription: "Whether matching files below the source path keep their relative path below the destination.",
			},
			"status_code": schema.Int64Attribute{
				Computed:            true,
				Description:         "The HTTP status code reported by cPanel.",
				MarkdownDescription: "The HTTP status code reported by cPanel.",
			},
			"document_root": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute document root whose .htaccess file contains the redirect.",
				MarkdownDescription: "The absolute document root whose `.htaccess` file contains the redirect.",
			},
			"kind": schema.StringAttribute{
				Computed:            true,
				Description:         "The redirect directive kind reported by cPanel.",
				MarkdownDescription: "The redirect directive kind reported by cPanel.",
			},
		},
	}
}

func (r *redirectResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state RedirectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	redirect, err := r.client.Get(
		ctx,
		state.Domain.ValueString(),
		state.Source.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read redirect", err.Error())
		return
	}
	if redirect == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(applyRedirectToResourceModel(&state, *redirect)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *redirectResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan RedirectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := redirectDefinitionFromResourceModel(plan)
	if err := validateRedirectDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid redirect", err.Error())
		return
	}

	existing, err := r.client.Get(ctx, definition.Domain, definition.Source)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read redirect", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Redirect already exists",
			fmt.Sprintf(
				"A redirect for %s%s already exists. Import it instead of replacing it implicitly.",
				definition.Domain,
				definition.Source,
			),
		)
		return
	}

	if err := r.client.Add(ctx, definition); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create redirect",
			createMutationErrorDetail(
				err,
				"redirect creation",
				fmt.Sprintf(
					"the redirect for %s%s",
					definition.Domain,
					definition.Source,
				),
			),
		)
		return
	}

	redirect, err := r.verifyRedirect(ctx, definition)
	if err != nil {
		rollbackErr := r.rollbackCreatedRedirect(ctx, definition)
		resp.Diagnostics.AddError(
			"Unable to verify redirect",
			redirectMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(applyRedirectToResourceModel(&plan, *redirect)...)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.rollbackCreatedRedirect(ctx, definition)
		resp.Diagnostics.AddError(
			"Unable to decode redirect",
			redirectMutationErrorDetail(
				fmt.Errorf("convert redirect metadata"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *redirectResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan RedirectResourceModel
	var state RedirectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.Get(
		ctx,
		state.Domain.ValueString(),
		state.Source.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read redirect", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Redirect no longer exists",
			"Refresh the Terraform state before updating the redirect.",
		)
		return
	}

	oldDefinition := redirectDefinitionFromAPI(*current)
	stateDefinition := redirectDefinitionFromResourceModel(state)
	if oldDefinition != stateDefinition {
		resp.Diagnostics.AddError(
			"Unable to replace redirect",
			"The remote redirect no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}
	newDefinition := redirectDefinitionFromResourceModel(plan)
	if err := validateRedirectDefinition(newDefinition); err != nil {
		resp.Diagnostics.AddError("Invalid redirect", err.Error())
		return
	}

	if err := r.deleteRedirectForReplacement(
		ctx,
		oldDefinition,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to replace redirect",
			err.Error(),
		)
		return
	}

	if err := r.client.Add(ctx, newDefinition); err != nil {
		rollbackErr := r.restoreRedirect(
			ctx,
			newDefinition,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to replace redirect",
			redirectMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	redirect, err := r.verifyRedirect(ctx, newDefinition)
	if err != nil {
		rollbackErr := r.restoreRedirect(
			ctx,
			newDefinition,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to verify redirect replacement",
			redirectMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(applyRedirectToResourceModel(&plan, *redirect)...)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.restoreRedirect(
			ctx,
			newDefinition,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to decode redirect replacement",
			redirectMutationErrorDetail(
				fmt.Errorf("convert redirect metadata"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *redirectResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state RedirectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain := state.Domain.ValueString()
	source := state.Source.ValueString()
	existing, err := r.client.Get(ctx, domain, source)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read redirect", err.Error())
		return
	}
	if existing == nil {
		return
	}
	expected := redirectDefinitionFromResourceModel(state)
	if redirectDefinitionFromAPI(*existing) != expected {
		resp.Diagnostics.AddError(
			"Unable to delete redirect",
			"The remote redirect no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.deleteRedirectForReplacement(ctx, expected); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete redirect",
			err.Error(),
		)
	}
}

func (r *redirectResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	domain, source, found := strings.Cut(req.ID, "|")
	if !found || domain == "" || source == "" || strings.Contains(source, "|") {
		resp.Diagnostics.AddError(
			"Invalid redirect import identifier",
			"Expected an identifier in the form domain|/source-path.",
		)
		return
	}
	if err := validateDomainName(domain); err != nil {
		resp.Diagnostics.AddError("Invalid redirect import domain", err.Error())
		return
	}
	if err := validateRedirectSource(source); err != nil {
		resp.Diagnostics.AddError("Invalid redirect import source", err.Error())
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("domain"), domain)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("source"), source)...,
	)
}

func (r *redirectResource) Configure(
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
			fmt.Sprintf("Expected map[string]interface{}, got: %T.", req.ProviderData),
		)
		return
	}

	client, ok := providerData["redirect"].(*cpanelredirect.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Redirect Client Type",
			fmt.Sprintf(
				"Expected *redirect.Client, got: %T.",
				providerData["redirect"],
			),
		)
		return
	}

	r.client = client
}

func (r *redirectResource) verifyRedirect(
	ctx context.Context,
	expected cpanelredirect.Definition,
) (*cpanelredirect.Redirect, error) {
	redirect, err := r.client.Get(ctx, expected.Domain, expected.Source)
	if err != nil {
		return nil, fmt.Errorf("read redirect after mutation: %w", err)
	}
	if redirect == nil {
		return nil, fmt.Errorf(
			"redirect for %s%s was not found after mutation",
			expected.Domain,
			expected.Source,
		)
	}

	actual := redirectDefinitionFromAPI(*redirect)
	if actual != expected {
		return nil, fmt.Errorf(
			"redirect for %s%s does not match the requested definition",
			expected.Domain,
			expected.Source,
		)
	}

	expectedStatusCode := "301"
	if expected.Type == cpanelredirect.TypeTemporary {
		expectedStatusCode = "302"
	}
	if redirect.StatusCode != expectedStatusCode {
		return nil, fmt.Errorf(
			"redirect for %s%s has status %q; expected %s",
			expected.Domain,
			expected.Source,
			redirect.StatusCode,
			expectedStatusCode,
		)
	}

	return redirect, nil
}

func (r *redirectResource) restoreRedirect(
	ctx context.Context,
	replacement cpanelredirect.Definition,
	original cpanelredirect.Definition,
) error {
	current, err := r.client.Get(
		ctx,
		replacement.Domain,
		replacement.Source,
	)
	if err != nil {
		return fmt.Errorf("read replacement redirect before rollback: %w", err)
	}
	if current == nil {
		return fmt.Errorf(
			"refuse to restore the previous redirect because the attempted replacement is absent",
		)
	}

	currentDefinition := redirectDefinitionFromAPI(*current)
	if currentDefinition == original {
		return nil
	}
	if currentDefinition != replacement {
		return fmt.Errorf(
			"refuse to restore the previous redirect because the current redirect no longer matches the attempted replacement",
		)
	}

	if err := r.deleteRedirectForReplacement(ctx, replacement); err != nil {
		return fmt.Errorf("delete replacement redirect: %w", err)
	}

	addErr := r.client.Add(ctx, original)
	restored, readErr := r.client.Get(
		ctx,
		original.Domain,
		original.Source,
	)
	if readErr != nil {
		if addErr != nil {
			return fmt.Errorf(
				"restore previous redirect: %v; read it after restoration: %w",
				addErr,
				readErr,
			)
		}

		return fmt.Errorf("read redirect after restoration: %w", readErr)
	}
	if restored != nil && redirectDefinitionFromAPI(*restored) == original {
		return nil
	}
	if restored != nil {
		return fmt.Errorf(
			"redirect for %s%s changed concurrently while restoring the previous definition",
			original.Domain,
			original.Source,
		)
	}
	if addErr != nil {
		return fmt.Errorf("restore previous redirect: %w", addErr)
	}

	return fmt.Errorf(
		"cPanel reported success but the previous redirect for %s%s was not restored",
		original.Domain,
		original.Source,
	)
}

func (r *redirectResource) rollbackCreatedRedirect(
	ctx context.Context,
	attempted cpanelredirect.Definition,
) error {
	current, err := r.client.Get(
		ctx,
		attempted.Domain,
		attempted.Source,
	)
	if err != nil {
		return fmt.Errorf("read created redirect before rollback: %w", err)
	}
	if current == nil {
		return nil
	}
	if redirectDefinitionFromAPI(*current) != attempted {
		return fmt.Errorf(
			"refuse to roll back redirect creation because the current redirect no longer matches the attempted definition",
		)
	}

	return r.deleteRedirectForReplacement(ctx, attempted)
}

func (r *redirectResource) deleteRedirectForReplacement(
	ctx context.Context,
	original cpanelredirect.Definition,
) error {
	deleteErr := r.client.Delete(ctx, original.Domain, original.Source)
	current, readErr := r.client.Get(ctx, original.Domain, original.Source)
	if readErr != nil {
		if deleteErr != nil {
			return fmt.Errorf(
				"delete redirect: %v; read it after deletion: %w",
				deleteErr,
				readErr,
			)
		}

		return fmt.Errorf("read redirect after deletion: %w", readErr)
	}
	if current == nil {
		return nil
	}

	currentDefinition := redirectDefinitionFromAPI(*current)
	if currentDefinition != original {
		return fmt.Errorf(
			"redirect for %s%s changed concurrently while it was being deleted; refusing to continue the replacement",
			original.Domain,
			original.Source,
		)
	}
	if deleteErr != nil {
		return fmt.Errorf(
			"delete redirect for %s%s: %w",
			original.Domain,
			original.Source,
			deleteErr,
		)
	}

	return fmt.Errorf(
		"cPanel reported success but redirect for %s%s still exists",
		original.Domain,
		original.Source,
	)
}

func validateRedirectDefinition(definition cpanelredirect.Definition) error {
	if err := validateDomainName(definition.Domain); err != nil {
		return fmt.Errorf("invalid redirect domain: %w", err)
	}
	if err := validateRedirectSource(definition.Source); err != nil {
		return err
	}
	if err := validateRedirectDestination(definition.Destination); err != nil {
		return err
	}
	if definition.Type != cpanelredirect.TypePermanent &&
		definition.Type != cpanelredirect.TypeTemporary {
		return fmt.Errorf("redirect type must be permanent or temporary")
	}
	if definition.WWWMode != cpanelredirect.WWWModeBoth &&
		definition.WWWMode != cpanelredirect.WWWModeWithout {
		return fmt.Errorf("redirect www_mode must be both or without")
	}

	return nil
}

func redirectMutationErrorDetail(primaryError, rollbackError error) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to roll back the redirect change: %v",
		primaryError,
		rollbackError,
	)
}
