package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

var (
	_ datasource.DataSource              = &modSecurityDomainDataSource{}
	_ datasource.DataSourceWithConfigure = &modSecurityDomainDataSource{}
)

func NewModSecurityDomainDataSource() datasource.DataSource {
	return &modSecurityDomainDataSource{}
}

type modSecurityDomainDataSource struct {
	client *modsecurity.Client
}

func (d *modSecurityDomainDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_modsecurity_domain"
}

func (d *modSecurityDomainDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up the ModSecurity status for a domain in a cPanel account.",
		MarkdownDescription: "Looks up the ModSecurity status for a domain in a cPanel account.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The account domain to look up.",
				MarkdownDescription: "The account domain to look up.",
				Validators:          domainNameValidators(),
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether ModSecurity is enabled for the domain.",
				MarkdownDescription: "Whether ModSecurity is enabled for the domain.",
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

func (d *modSecurityDomainDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config ModSecurityDomainDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := modsecurity.Definition{Domain: config.Domain.ValueString()}
	if err := validateModSecurityDomainDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid ModSecurity domain", err.Error())
		return
	}

	apiDomain, err := d.client.Get(ctx, definition.Domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read ModSecurity domain",
			err.Error(),
		)
		return
	}
	if apiDomain == nil {
		resp.Diagnostics.AddError(
			"ModSecurity domain not found",
			fmt.Sprintf(
				"Domain %q is not present in the cPanel ModSecurity inventory.",
				definition.Domain,
			),
		)
		return
	}

	model, diagnostics := modSecurityDomainToDataSourceModel(ctx, *apiDomain)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (d *modSecurityDomainDataSource) Configure(
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

	d.client = client
}
