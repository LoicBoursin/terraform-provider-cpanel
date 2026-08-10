package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

var (
	_ resource.Resource                = &subdomainResource{}
	_ resource.ResourceWithConfigure   = &subdomainResource{}
	_ resource.ResourceWithImportState = &subdomainResource{}
)

func NewSubdomainResource() resource.Resource {
	return &subdomainResource{}
}

type subdomainResource struct {
	client *cpaneldomain.Client
}

func (r *subdomainResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_subdomain"
}

func (r *subdomainResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel web subdomain and its document root.",
		MarkdownDescription: "Manages a cPanel web subdomain and its document root.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The complete subdomain name under an existing main or addon domain.",
				MarkdownDescription: "The complete subdomain name under an existing main or addon domain.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subdomain": schema.StringAttribute{
				Computed:            true,
				Description:         "The subdomain portion before the root domain.",
				MarkdownDescription: "The subdomain portion before the root domain.",
			},
			"root_domain": schema.StringAttribute{
				Computed:            true,
				Description:         "The existing main or addon domain that owns the subdomain.",
				MarkdownDescription: "The existing main or addon domain that owns the subdomain.",
			},
			"document_root": schema.StringAttribute{
				Required:            true,
				Description:         "The document root relative to the cPanel account home.",
				MarkdownDescription: "The document root relative to the cPanel account home. cPanel preserves its contents when the subdomain is deleted.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 1024),
				},
			},
			"delete_document_root": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Retained for compatibility. The subdomain resource preserves document roots because it cannot prove filesystem ownership; manage owned directory deletion with cpanel_filesystem_directory.",
				MarkdownDescription: "Retained for compatibility. The subdomain resource preserves document roots because it cannot prove filesystem ownership; manage owned directory deletion with `cpanel_filesystem_directory`.",
			},
		},
	}
}

func (r *subdomainResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state SubdomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subdomain, err := r.client.GetSubdomain(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read subdomain", err.Error())
		return
	}
	if subdomain == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Subdomain = types.StringValue(subdomain.Subdomain)
	state.RootDomain = types.StringValue(subdomain.RootDomain)
	state.DocumentRoot = types.StringValue(subdomain.BaseDirectory)
	if state.DeleteDocumentRoot.IsNull() || state.DeleteDocumentRoot.IsUnknown() {
		state.DeleteDocumentRoot = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *subdomainResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan SubdomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subdomain, rootDomain, err := resolveSubdomainParts(
		ctx,
		r.client,
		plan.Domain.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid subdomain", err.Error())
		return
	}
	if err := validateDomainDocumentRoot(plan.DocumentRoot.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid subdomain document root", err.Error())
		return
	}

	existing, err := r.client.GetSubdomain(ctx, plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read subdomain", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Subdomain already exists",
			fmt.Sprintf(
				"Subdomain %q already exists in cPanel. Import it instead of taking ownership implicitly.",
				plan.Domain.ValueString(),
			),
		)
		return
	}

	createErr := r.client.CreateSubdomain(
		ctx,
		subdomain,
		rootDomain,
		plan.DocumentRoot.ValueString(),
	)
	if createErr != nil {
		if cPanelMutationErrorIsDeterministic(createErr) {
			resp.Diagnostics.AddError(
				"Unable to create subdomain",
				"Could not create subdomain: "+createErr.Error(),
			)
			return
		}

		reconcileErr := r.verifySubdomain(
			ctx,
			plan.Domain.ValueString(),
			plan.DocumentRoot.ValueString(),
		)
		if reconcileErr != nil {
			resp.Diagnostics.AddError(
				"Unable to reconcile subdomain creation",
				fmt.Sprintf(
					"%s Reconciliation also failed: %v",
					createMutationErrorDetail(
						createErr,
						"subdomain creation",
						fmt.Sprintf("subdomain %q", plan.Domain.ValueString()),
					),
					reconcileErr,
				),
			)
			return
		}

		plan.Subdomain = types.StringValue(subdomain)
		plan.RootDomain = types.StringValue(rootDomain)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	if err := r.verifySubdomain(
		ctx,
		plan.Domain.ValueString(),
		plan.DocumentRoot.ValueString(),
	); err != nil {
		rollbackErr := r.rollbackSubdomainCreate(
			ctx,
			plan.Domain.ValueString(),
			plan.DocumentRoot.ValueString(),
		)
		resp.Diagnostics.AddError(
			"Unable to verify subdomain",
			domainMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	plan.Subdomain = types.StringValue(subdomain)
	plan.RootDomain = types.StringValue(rootDomain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *subdomainResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan SubdomainResourceModel
	var state SubdomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subdomain, rootDomain, err := resolveSubdomainParts(
		ctx,
		r.client,
		plan.Domain.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid subdomain", err.Error())
		return
	}
	if err := validateDomainDocumentRoot(plan.DocumentRoot.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid subdomain document root", err.Error())
		return
	}

	if !plan.DocumentRoot.Equal(state.DocumentRoot) {
		current, err := r.client.GetSubdomain(
			ctx,
			state.Domain.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read subdomain", err.Error())
			return
		}
		if current == nil {
			resp.Diagnostics.AddError(
				"Subdomain no longer exists",
				"Refresh the Terraform state before updating the subdomain.",
			)
			return
		}
		if !subdomainsEqual(current, subdomainFromResourceModel(state)) {
			resp.Diagnostics.AddError(
				"Subdomain changed during update",
				"The remote subdomain no longer matches Terraform state, so the provider refuses to overwrite its document root. Refresh and review the drift before retrying.",
			)
			return
		}
		if err := r.client.SetSubdomainDocumentRoot(
			ctx,
			subdomain,
			rootDomain,
			plan.DocumentRoot.ValueString(),
		); err != nil {
			resp.Diagnostics.AddError(
				"Unable to update subdomain document root",
				"Could not update subdomain document root: "+err.Error(),
			)
			return
		}

		if err := r.verifySubdomain(
			ctx,
			plan.Domain.ValueString(),
			plan.DocumentRoot.ValueString(),
		); err != nil {
			rollbackErr := r.restoreSubdomainDocumentRoot(
				ctx,
				plan.Domain.ValueString(),
				plan.DocumentRoot.ValueString(),
				state.DocumentRoot.ValueString(),
			)
			resp.Diagnostics.AddError(
				"Unable to verify subdomain",
				domainMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	plan.Subdomain = types.StringValue(subdomain)
	plan.RootDomain = types.StringValue(rootDomain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *subdomainResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state SubdomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subdomain, err := r.client.GetSubdomain(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read subdomain", err.Error())
		return
	}
	if subdomain == nil {
		r.warnSubdomainDocumentRootPreserved(resp, &state)
		return
	}
	if subdomain.BaseDirectory != state.DocumentRoot.ValueString() {
		resp.Diagnostics.AddError(
			"Refusing to delete subdomain",
			"The current subdomain document root no longer matches the Terraform-managed value.",
		)
		return
	}
	if err := r.deleteSubdomainAndVerify(ctx, subdomain); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete subdomain",
			err.Error(),
		)
		return
	}

	r.warnSubdomainDocumentRootPreserved(resp, &state)
}

func (r *subdomainResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("delete_document_root"), false)...,
	)
}

func (r *subdomainResource) Configure(
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

	client, ok := providerData["domain"].(*cpaneldomain.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Domain Client Type",
			fmt.Sprintf("Expected *domain.Client, got: %T.", providerData["domain"]),
		)
		return
	}

	r.client = client
}

func (r *subdomainResource) verifySubdomain(
	ctx context.Context,
	domain string,
	expectedDocumentRoot string,
) error {
	subdomain, err := r.client.GetSubdomain(ctx, domain)
	if err != nil {
		return fmt.Errorf("read subdomain after mutation: %w", err)
	}
	if subdomain == nil {
		return fmt.Errorf("subdomain %q was not found after mutation", domain)
	}
	if subdomain.BaseDirectory != expectedDocumentRoot {
		return fmt.Errorf(
			"subdomain %q has document root %q; expected %q",
			domain,
			subdomain.BaseDirectory,
			expectedDocumentRoot,
		)
	}

	return nil
}

func (r *subdomainResource) rollbackSubdomainCreate(
	ctx context.Context,
	domain string,
	expectedDocumentRoot string,
) error {
	subdomain, err := r.client.GetSubdomain(ctx, domain)
	if err != nil {
		return fmt.Errorf("read subdomain before rollback: %w", err)
	}
	if subdomain == nil {
		return nil
	}
	if subdomain.BaseDirectory != expectedDocumentRoot {
		return fmt.Errorf(
			"refuse to roll back subdomain %q because its current document root does not match the attempted creation",
			domain,
		)
	}
	return r.deleteSubdomainAndVerify(ctx, subdomain)
}

func (r *subdomainResource) deleteSubdomainAndVerify(
	ctx context.Context,
	expected *cpaneldomain.Subdomain,
) error {
	mutationErr := r.client.DeleteSubdomain(ctx, expected.Domain)
	current, readErr := r.client.GetSubdomain(ctx, expected.Domain)
	if readErr != nil {
		return errors.Join(
			mutationErr,
			fmt.Errorf("read subdomain after deletion: %w", readErr),
		)
	}
	if current == nil {
		return nil
	}

	if !subdomainsEqual(current, expected) {
		return errors.Join(
			mutationErr,
			fmt.Errorf(
				"subdomain %q changed during deletion; refusing to treat the replacement as deleted",
				expected.Domain,
			),
		)
	}

	return errors.Join(
		mutationErr,
		fmt.Errorf("subdomain %q still exists after deletion", expected.Domain),
	)
}

func (r *subdomainResource) warnSubdomainDocumentRootPreserved(
	resp *resource.DeleteResponse,
	state *SubdomainResourceModel,
) {
	if !state.DeleteDocumentRoot.ValueBool() {
		return
	}

	resp.Diagnostics.AddWarning(
		"Preserving subdomain document root",
		fmt.Sprintf(
			"Document root %q was preserved because this resource has no private filesystem ownership proof. Manage the directory with cpanel_filesystem_directory when Terraform should delete it.",
			state.DocumentRoot.ValueString(),
		),
	)
}

func subdomainsEqual(
	left *cpaneldomain.Subdomain,
	right *cpaneldomain.Subdomain,
) bool {
	return left != nil &&
		right != nil &&
		left.Domain == right.Domain &&
		left.Subdomain == right.Subdomain &&
		left.RootDomain == right.RootDomain &&
		left.BaseDirectory == right.BaseDirectory
}

func subdomainFromResourceModel(
	model SubdomainResourceModel,
) *cpaneldomain.Subdomain {
	return &cpaneldomain.Subdomain{
		Domain:        model.Domain.ValueString(),
		Subdomain:     model.Subdomain.ValueString(),
		RootDomain:    model.RootDomain.ValueString(),
		BaseDirectory: model.DocumentRoot.ValueString(),
	}
}

func (r *subdomainResource) restoreSubdomainDocumentRoot(
	ctx context.Context,
	domain string,
	expectedCurrent string,
	target string,
) error {
	subdomain, err := r.client.GetSubdomain(ctx, domain)
	if err != nil {
		return fmt.Errorf("read subdomain before document-root restore: %w", err)
	}
	if subdomain == nil {
		return fmt.Errorf(
			"refuse to restore subdomain document root because %q no longer exists",
			domain,
		)
	}
	if subdomain.BaseDirectory == target {
		return nil
	}
	if subdomain.BaseDirectory != expectedCurrent {
		return fmt.Errorf(
			"refuse to restore subdomain document root because the current value no longer matches the Terraform transition",
		)
	}

	return r.client.SetSubdomainDocumentRoot(
		ctx,
		subdomain.Subdomain,
		subdomain.RootDomain,
		target,
	)
}

func domainMutationErrorDetail(mutationErr, rollbackErr error) string {
	if rollbackErr == nil {
		return mutationErr.Error() + ". The previous remote state was restored."
	}

	return fmt.Sprintf(
		"%v. Automatic rollback also failed: %v",
		mutationErr,
		rollbackErr,
	)
}
