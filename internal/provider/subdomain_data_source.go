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
	_ datasource.DataSource              = &subdomainDataSource{}
	_ datasource.DataSourceWithConfigure = &subdomainDataSource{}
)

func NewSubdomainDataSource() datasource.DataSource {
	return &subdomainDataSource{}
}

type subdomainDataSource struct {
	client *cpaneldomain.Client
}

func (d *subdomainDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_subdomain"
}

func (d *subdomainDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel web subdomain and its document root.",
		MarkdownDescription: "Looks up a cPanel web subdomain and its document root.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The complete subdomain name.",
				MarkdownDescription: "The complete subdomain name.",
				Validators:          domainNameValidators(),
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
				Computed:            true,
				Description:         "The document root relative to the cPanel account home.",
				MarkdownDescription: "The document root relative to the cPanel account home.",
			},
		},
	}
}

func (d *subdomainDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config SubdomainDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainName(config.Domain.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid subdomain", err.Error())
		return
	}

	subdomain, err := d.client.GetSubdomain(ctx, config.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read subdomain", err.Error())
		return
	}
	if subdomain == nil {
		resp.Diagnostics.AddError(
			"Subdomain not found",
			fmt.Sprintf("No subdomain named %q exists.", config.Domain.ValueString()),
		)
		return
	}

	state := SubdomainDataSourceModel{
		Domain:       types.StringValue(subdomain.Domain),
		Subdomain:    types.StringValue(subdomain.Subdomain),
		RootDomain:   types.StringValue(subdomain.RootDomain),
		DocumentRoot: types.StringValue(subdomain.BaseDirectory),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *subdomainDataSource) Configure(
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
