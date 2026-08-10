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
	_ resource.Resource                = &addonDomainResource{}
	_ resource.ResourceWithConfigure   = &addonDomainResource{}
	_ resource.ResourceWithImportState = &addonDomainResource{}
)

func NewAddonDomainResource() resource.Resource {
	return &addonDomainResource{}
}

type addonDomainResource struct {
	client *cpaneldomain.Client
}

func (r *addonDomainResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_addon_domain"
}

func (r *addonDomainResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel addon domain, its internal subdomain, and document root.",
		MarkdownDescription: "Manages a cPanel addon domain, its internal subdomain, and document root.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The addon domain name.",
				MarkdownDescription: "The addon domain name.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"internal_subdomain": schema.StringAttribute{
				Required:            true,
				Description:         "The internal subdomain label to create under the cPanel account main domain.",
				MarkdownDescription: "The internal subdomain label to create under the cPanel account main domain.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 63),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"root_domain": schema.StringAttribute{
				Computed:            true,
				Description:         "The cPanel account main domain that owns the internal subdomain.",
				MarkdownDescription: "The cPanel account main domain that owns the internal subdomain.",
			},
			"full_subdomain": schema.StringAttribute{
				Computed:            true,
				Description:         "The complete internal subdomain created for the addon domain.",
				MarkdownDescription: "The complete internal subdomain created for the addon domain.",
			},
			"domain_key": schema.StringAttribute{
				Computed:            true,
				Description:         "The cPanel domain key used to identify the addon domain internally.",
				MarkdownDescription: "The cPanel domain key used to identify the addon domain internally.",
			},
			"document_root": schema.StringAttribute{
				Required:            true,
				Description:         "The document root relative to the cPanel account home.",
				MarkdownDescription: "The document root relative to the cPanel account home. cPanel preserves its contents when the addon domain is deleted.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 1024),
				},
			},
			"delete_document_root": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Retained for compatibility. The addon domain resource preserves document roots because it cannot prove filesystem ownership; manage owned directory deletion with cpanel_filesystem_directory.",
				MarkdownDescription: "Retained for compatibility. The addon domain resource preserves document roots because it cannot prove filesystem ownership; manage owned directory deletion with `cpanel_filesystem_directory`.",
			},
		},
	}
}

func (r *addonDomainResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state AddonDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	addonDomain, err := r.client.GetAddonDomain(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read addon domain", err.Error())
		return
	}
	if addonDomain == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	setAddonDomainRemoteState(&state, addonDomain)
	if state.DeleteDocumentRoot.IsNull() || state.DeleteDocumentRoot.IsUnknown() {
		state.DeleteDocumentRoot = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *addonDomainResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan AddonDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainName(plan.Domain.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid addon domain", err.Error())
		return
	}
	if err := validateAddonDomainInternalSubdomain(
		plan.InternalSubdomain.ValueString(),
	); err != nil {
		resp.Diagnostics.AddError("Invalid addon domain internal subdomain", err.Error())
		return
	}
	if err := validateDomainDocumentRoot(plan.DocumentRoot.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid addon domain document root", err.Error())
		return
	}

	existing, err := r.client.GetAddonDomain(ctx, plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read addon domain", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Addon domain already exists",
			fmt.Sprintf(
				"Addon domain %q already exists in cPanel. Import it instead of taking ownership implicitly.",
				plan.Domain.ValueString(),
			),
		)
		return
	}

	createErr := r.client.CreateAddonDomain(
		ctx,
		plan.Domain.ValueString(),
		plan.InternalSubdomain.ValueString(),
		plan.DocumentRoot.ValueString(),
	)
	if createErr != nil {
		if cPanelMutationErrorIsDeterministic(createErr) {
			resp.Diagnostics.AddError(
				"Unable to create addon domain",
				"Could not create addon domain: "+createErr.Error(),
			)
			return
		}

		addonDomain, reconcileErr := r.verifyAddonDomain(
			ctx,
			plan.Domain.ValueString(),
			plan.InternalSubdomain.ValueString(),
			plan.DocumentRoot.ValueString(),
		)
		if reconcileErr != nil {
			resp.Diagnostics.AddError(
				"Unable to reconcile addon domain creation",
				fmt.Sprintf(
					"%s Reconciliation also failed: %v",
					createMutationErrorDetail(
						createErr,
						"addon domain creation",
						fmt.Sprintf("addon domain %q", plan.Domain.ValueString()),
					),
					reconcileErr,
				),
			)
			return
		}

		setAddonDomainRemoteState(&plan, addonDomain)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	addonDomain, err := r.verifyAddonDomain(
		ctx,
		plan.Domain.ValueString(),
		plan.InternalSubdomain.ValueString(),
		plan.DocumentRoot.ValueString(),
	)
	if err != nil {
		rollbackErr := r.rollbackAddonDomainCreate(ctx, &plan)
		resp.Diagnostics.AddError(
			"Unable to verify addon domain",
			domainMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	setAddonDomainRemoteState(&plan, addonDomain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *addonDomainResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan AddonDomainResourceModel
	var state AddonDomainResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainDocumentRoot(plan.DocumentRoot.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid addon domain document root", err.Error())
		return
	}

	if !plan.DocumentRoot.Equal(state.DocumentRoot) {
		current, err := r.client.GetAddonDomain(
			ctx,
			state.Domain.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read addon domain", err.Error())
			return
		}
		if current == nil {
			resp.Diagnostics.AddError(
				"Addon domain no longer exists",
				"Refresh the Terraform state before updating the addon domain.",
			)
			return
		}
		if !addonDomainsEqual(
			current,
			addonDomainFromResourceModel(state),
		) {
			resp.Diagnostics.AddError(
				"Addon domain changed during update",
				"The remote addon domain no longer matches Terraform state, so the provider refuses to overwrite its document root. Refresh and review the drift before retrying.",
			)
			return
		}
		if err := r.client.SetSubdomainDocumentRoot(
			ctx,
			state.InternalSubdomain.ValueString(),
			state.RootDomain.ValueString(),
			plan.DocumentRoot.ValueString(),
		); err != nil {
			resp.Diagnostics.AddError(
				"Unable to update addon domain document root",
				"Could not update addon domain document root: "+err.Error(),
			)
			return
		}

		addonDomain, err := r.verifyAddonDomain(
			ctx,
			plan.Domain.ValueString(),
			plan.InternalSubdomain.ValueString(),
			plan.DocumentRoot.ValueString(),
		)
		if err != nil {
			rollbackErr := r.restoreAddonDomainDocumentRoot(
				ctx,
				plan.Domain.ValueString(),
				plan.DocumentRoot.ValueString(),
				state.DocumentRoot.ValueString(),
			)
			resp.Diagnostics.AddError(
				"Unable to verify addon domain",
				domainMutationErrorDetail(err, rollbackErr),
			)
			return
		}
		setAddonDomainRemoteState(&plan, addonDomain)
	} else {
		plan.RootDomain = state.RootDomain
		plan.FullSubdomain = state.FullSubdomain
		plan.DomainKey = state.DomainKey
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *addonDomainResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state AddonDomainResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	addonDomain, err := r.client.GetAddonDomain(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read addon domain", err.Error())
		return
	}
	if addonDomain == nil {
		r.warnAddonDomainDocumentRootPreserved(resp, &state)
		return
	}
	if addonDomain.InternalSubdomain != state.InternalSubdomain.ValueString() ||
		addonDomain.BaseDirectory != state.DocumentRoot.ValueString() {
		resp.Diagnostics.AddError(
			"Refusing to delete addon domain",
			"The current addon domain no longer matches the Terraform-managed internal subdomain and document root.",
		)
		return
	}
	if err := r.deleteAddonDomainAndVerify(ctx, addonDomain); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete addon domain",
			err.Error(),
		)
		return
	}

	r.warnAddonDomainDocumentRootPreserved(resp, &state)
}

func (r *addonDomainResource) ImportState(
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

func (r *addonDomainResource) Configure(
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

func (r *addonDomainResource) verifyAddonDomain(
	ctx context.Context,
	domain string,
	expectedInternalSubdomain string,
	expectedDocumentRoot string,
) (*cpaneldomain.AddonDomain, error) {
	addonDomain, err := r.client.GetAddonDomain(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("read addon domain after mutation: %w", err)
	}
	if addonDomain == nil {
		return nil, fmt.Errorf("addon domain %q was not found after mutation", domain)
	}
	if addonDomain.InternalSubdomain != expectedInternalSubdomain {
		return nil, fmt.Errorf(
			"addon domain %q has internal subdomain %q; expected %q",
			domain,
			addonDomain.InternalSubdomain,
			expectedInternalSubdomain,
		)
	}
	if addonDomain.BaseDirectory != expectedDocumentRoot {
		return nil, fmt.Errorf(
			"addon domain %q has document root %q; expected %q",
			domain,
			addonDomain.BaseDirectory,
			expectedDocumentRoot,
		)
	}

	return addonDomain, nil
}

func (r *addonDomainResource) rollbackAddonDomainCreate(
	ctx context.Context,
	plan *AddonDomainResourceModel,
) error {
	addonDomain, err := r.client.GetAddonDomain(ctx, plan.Domain.ValueString())
	if err != nil {
		return fmt.Errorf("read addon domain: %w", err)
	}
	if addonDomain == nil {
		return nil
	}
	if addonDomain.InternalSubdomain !=
		plan.InternalSubdomain.ValueString() ||
		addonDomain.BaseDirectory != plan.DocumentRoot.ValueString() {
		return fmt.Errorf(
			"refuse to roll back addon domain %q because its current configuration does not match the attempted creation",
			plan.Domain.ValueString(),
		)
	}
	return r.deleteAddonDomainAndVerify(ctx, addonDomain)
}

func (r *addonDomainResource) deleteAddonDomainAndVerify(
	ctx context.Context,
	expected *cpaneldomain.AddonDomain,
) error {
	mutationErr := r.client.DeleteAddonDomain(
		ctx,
		expected.Domain,
		expected.DomainKey,
	)
	current, readErr := r.client.GetAddonDomain(ctx, expected.Domain)
	if readErr != nil {
		return errors.Join(
			mutationErr,
			fmt.Errorf("read addon domain after deletion: %w", readErr),
		)
	}
	if current == nil {
		return nil
	}

	if !addonDomainsEqual(current, expected) {
		return errors.Join(
			mutationErr,
			fmt.Errorf(
				"addon domain %q changed during deletion; refusing to treat the replacement as deleted",
				expected.Domain,
			),
		)
	}

	return errors.Join(
		mutationErr,
		fmt.Errorf(
			"addon domain %q still exists after deletion",
			expected.Domain,
		),
	)
}

func (r *addonDomainResource) warnAddonDomainDocumentRootPreserved(
	resp *resource.DeleteResponse,
	state *AddonDomainResourceModel,
) {
	if !state.DeleteDocumentRoot.ValueBool() {
		return
	}

	resp.Diagnostics.AddWarning(
		"Preserving addon domain document root",
		fmt.Sprintf(
			"Document root %q was preserved because this resource has no private filesystem ownership proof. Manage the directory with cpanel_filesystem_directory when Terraform should delete it.",
			state.DocumentRoot.ValueString(),
		),
	)
}

func addonDomainsEqual(
	left *cpaneldomain.AddonDomain,
	right *cpaneldomain.AddonDomain,
) bool {
	return left != nil &&
		right != nil &&
		left.Domain == right.Domain &&
		left.InternalSubdomain == right.InternalSubdomain &&
		left.RootDomain == right.RootDomain &&
		left.FullSubdomain == right.FullSubdomain &&
		left.DomainKey == right.DomainKey &&
		left.BaseDirectory == right.BaseDirectory
}

func addonDomainFromResourceModel(
	model AddonDomainResourceModel,
) *cpaneldomain.AddonDomain {
	return &cpaneldomain.AddonDomain{
		Domain:            model.Domain.ValueString(),
		DomainKey:         model.DomainKey.ValueString(),
		InternalSubdomain: model.InternalSubdomain.ValueString(),
		RootDomain:        model.RootDomain.ValueString(),
		FullSubdomain:     model.FullSubdomain.ValueString(),
		BaseDirectory:     model.DocumentRoot.ValueString(),
	}
}

func (r *addonDomainResource) restoreAddonDomainDocumentRoot(
	ctx context.Context,
	domain string,
	expectedCurrent string,
	target string,
) error {
	addonDomain, err := r.client.GetAddonDomain(ctx, domain)
	if err != nil {
		return fmt.Errorf("read addon domain before document-root restore: %w", err)
	}
	if addonDomain == nil {
		return fmt.Errorf(
			"refuse to restore addon domain document root because %q no longer exists",
			domain,
		)
	}
	if addonDomain.BaseDirectory == target {
		return nil
	}
	if addonDomain.BaseDirectory != expectedCurrent {
		return fmt.Errorf(
			"refuse to restore addon domain document root because the current value no longer matches the Terraform transition",
		)
	}

	return r.client.SetSubdomainDocumentRoot(
		ctx,
		addonDomain.InternalSubdomain,
		addonDomain.RootDomain,
		target,
	)
}

func setAddonDomainRemoteState(
	state *AddonDomainResourceModel,
	addonDomain *cpaneldomain.AddonDomain,
) {
	state.InternalSubdomain = types.StringValue(addonDomain.InternalSubdomain)
	state.RootDomain = types.StringValue(addonDomain.RootDomain)
	state.FullSubdomain = types.StringValue(addonDomain.FullSubdomain)
	state.DomainKey = types.StringValue(addonDomain.DomainKey)
	state.DocumentRoot = types.StringValue(addonDomain.BaseDirectory)
}
