package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

var (
	_ datasource.DataSource              = &domainAliasDataSource{}
	_ datasource.DataSourceWithConfigure = &domainAliasDataSource{}
)

func NewDomainAliasDataSource() datasource.DataSource {
	return &domainAliasDataSource{}
}

type domainAliasDataSource struct {
	client *cpaneldomain.Client
}

func (d *domainAliasDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_domain_alias"
}

func (d *domainAliasDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel alias of the account main domain.",
		MarkdownDescription: "Looks up a cPanel alias of the account main domain.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The alias domain name.",
				MarkdownDescription: "The alias domain name.",
				Validators:          domainNameValidators(),
			},
			"target_domain": schema.StringAttribute{
				Computed:            true,
				Description:         "The cPanel account main domain targeted by the alias.",
				MarkdownDescription: "The cPanel account main domain targeted by the alias.",
			},
			"document_root": schema.StringAttribute{
				Computed:            true,
				Description:         "The shared main-domain document root relative to the cPanel account home.",
				MarkdownDescription: "The shared main-domain document root relative to the cPanel account home.",
			},
		},
	}
}

func (d *domainAliasDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config DomainAliasModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainName(config.Domain.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid domain alias", err.Error())
		return
	}

	domainAlias, err := d.client.GetDomainAlias(ctx, config.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read domain alias", err.Error())
		return
	}
	if domainAlias == nil {
		resp.Diagnostics.AddError(
			"Domain alias not found",
			fmt.Sprintf("No domain alias named %q exists.", config.Domain.ValueString()),
		)
		return
	}

	mainDomain, err := d.client.GetMainDomain(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel main domain", err.Error())
		return
	}

	state := DomainAliasModel{
		Domain:       types.StringValue(domainAlias.Domain),
		TargetDomain: types.StringValue(mainDomain),
		DocumentRoot: types.StringValue(domainAlias.BaseDirectory),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *domainAliasDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
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

	d.client = client
}
