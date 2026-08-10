package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

var (
	_ resource.Resource                = &domainAliasResource{}
	_ resource.ResourceWithConfigure   = &domainAliasResource{}
	_ resource.ResourceWithImportState = &domainAliasResource{}
)

func NewDomainAliasResource() resource.Resource {
	return &domainAliasResource{}
}

type domainAliasResource struct {
	client *cpaneldomain.Client
}

func (r *domainAliasResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_domain_alias"
}

func (r *domainAliasResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel alias of the account main domain.",
		MarkdownDescription: "Manages a cPanel alias of the account main domain.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The alias domain name.",
				MarkdownDescription: "The alias domain name.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_domain": schema.StringAttribute{
				Computed:            true,
				Description:         "The cPanel account main domain targeted by the alias.",
				MarkdownDescription: "The cPanel account main domain targeted by the alias.",
			},
			"document_root": schema.StringAttribute{
				Computed:            true,
				Description:         "The shared main-domain document root relative to the cPanel account home.",
				MarkdownDescription: "The shared main-domain document root relative to the cPanel account home. Terraform never deletes this shared directory.",
			},
		},
	}
}

func (r *domainAliasResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DomainAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainAlias, err := r.client.GetDomainAlias(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read domain alias", err.Error())
		return
	}
	if domainAlias == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mainDomain, err := r.client.GetMainDomain(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel main domain", err.Error())
		return
	}
	state.TargetDomain = types.StringValue(mainDomain)
	state.DocumentRoot = types.StringValue(domainAlias.BaseDirectory)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *domainAliasResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DomainAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainName(plan.Domain.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid domain alias", err.Error())
		return
	}

	mainDomain, err := r.client.GetMainDomain(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel main domain", err.Error())
		return
	}
	if plan.Domain.ValueString() == mainDomain {
		resp.Diagnostics.AddError(
			"Invalid domain alias",
			"The cPanel main domain cannot alias itself.",
		)
		return
	}

	existing, err := r.client.GetDomainAlias(ctx, plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read domain alias", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Domain alias already exists",
			fmt.Sprintf(
				"Domain alias %q already exists. Import it instead of taking ownership implicitly.",
				plan.Domain.ValueString(),
			),
		)
		return
	}

	if err := r.client.CreateDomainAlias(ctx, plan.Domain.ValueString()); err != nil {
		detail := "Could not create domain alias: " + err.Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			existing, readErr := r.client.GetDomainAlias(
				ctx,
				plan.Domain.ValueString(),
			)
			switch {
			case readErr != nil:
				detail += ". Terraform could not determine whether the alias was created; inspect cPanel before retrying."
			case existing != nil:
				detail += ". An alias with this name now exists, but Terraform did not adopt or delete it because the ambiguous creation cannot be attributed safely. Inspect it and import it if appropriate."
			default:
				detail += ". cPanel did not expose an alias with this name after the ambiguous response."
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create domain alias",
			detail,
		)
		return
	}

	domainAlias, err := r.verifyDomainAlias(ctx, plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify domain alias",
			fmt.Sprintf(
				"%v. Terraform left the remote alias untouched because cPanel does not expose a stable alias identifier; inspect cPanel and import it if it exists.",
				err,
			),
		)
		return
	}

	plan.TargetDomain = types.StringValue(mainDomain)
	plan.DocumentRoot = types.StringValue(domainAlias.BaseDirectory)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainAliasResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DomainAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainAlias, err := r.verifyDomainAlias(ctx, plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to verify domain alias", err.Error())
		return
	}
	mainDomain, err := r.client.GetMainDomain(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel main domain", err.Error())
		return
	}

	plan.TargetDomain = types.StringValue(mainDomain)
	plan.DocumentRoot = types.StringValue(domainAlias.BaseDirectory)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainAliasResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DomainAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainAlias, err := r.client.GetDomainAlias(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read domain alias", err.Error())
		return
	}
	if domainAlias == nil {
		return
	}

	mainDomain, err := r.client.GetMainDomain(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel main domain", err.Error())
		return
	}
	if !state.DocumentRoot.IsNull() &&
		!state.DocumentRoot.IsUnknown() &&
		domainAlias.BaseDirectory != state.DocumentRoot.ValueString() {
		resp.Diagnostics.AddError(
			"Domain alias changed outside Terraform",
			fmt.Sprintf(
				"Refusing to delete domain alias %q because its document root is now %q instead of %q.",
				state.Domain.ValueString(),
				domainAlias.BaseDirectory,
				state.DocumentRoot.ValueString(),
			),
		)
		return
	}
	if !state.TargetDomain.IsNull() &&
		!state.TargetDomain.IsUnknown() &&
		mainDomain != state.TargetDomain.ValueString() {
		resp.Diagnostics.AddError(
			"Domain alias changed outside Terraform",
			fmt.Sprintf(
				"Refusing to delete domain alias %q because the cPanel main domain is now %q instead of %q.",
				state.Domain.ValueString(),
				mainDomain,
				state.TargetDomain.ValueString(),
			),
		)
		return
	}

	deleteErr := r.client.DeleteDomainAlias(ctx, state.Domain.ValueString())
	remainingAlias, readErr := r.client.GetDomainAlias(ctx, state.Domain.ValueString())
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete domain alias",
			domainAliasDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if remainingAlias == nil {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete domain alias",
			"Could not delete domain alias: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to delete domain alias",
		fmt.Sprintf(
			"cPanel reported success but domain alias %q still exists.",
			state.Domain.ValueString(),
		),
	)
}

func (r *domainAliasResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("domain"), req, resp)
}

func (r *domainAliasResource) Configure(
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

func (r *domainAliasResource) verifyDomainAlias(
	ctx context.Context,
	domain string,
) (*cpaneldomain.DomainAlias, error) {
	domainAlias, err := r.client.GetDomainAlias(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("read domain alias after mutation: %w", err)
	}
	if domainAlias == nil {
		return nil, fmt.Errorf("domain alias %q was not found after mutation", domain)
	}

	return domainAlias, nil
}

func domainAliasDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the domain alias is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete domain alias: %v. Terraform also could not verify whether the alias still exists: %v",
		deleteErr,
		readErr,
	)
}
