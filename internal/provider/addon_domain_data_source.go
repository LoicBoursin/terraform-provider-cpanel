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
	_ datasource.DataSource              = &addonDomainDataSource{}
	_ datasource.DataSourceWithConfigure = &addonDomainDataSource{}
)

func NewAddonDomainDataSource() datasource.DataSource {
	return &addonDomainDataSource{}
}

type addonDomainDataSource struct {
	client *cpaneldomain.Client
}

func (d *addonDomainDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_addon_domain"
}

func (d *addonDomainDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel addon domain and its internal subdomain and document root.",
		MarkdownDescription: "Looks up a cPanel addon domain and its internal subdomain and document root.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The addon domain name.",
				MarkdownDescription: "The addon domain name.",
				Validators:          domainNameValidators(),
			},
			"internal_subdomain": schema.StringAttribute{
				Computed:            true,
				Description:         "The internal subdomain label created under the cPanel account main domain.",
				MarkdownDescription: "The internal subdomain label created under the cPanel account main domain.",
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
				Computed:            true,
				Description:         "The document root relative to the cPanel account home.",
				MarkdownDescription: "The document root relative to the cPanel account home.",
			},
		},
	}
}

func (d *addonDomainDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config AddonDomainDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainName(config.Domain.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid addon domain", err.Error())
		return
	}

	addonDomain, err := d.client.GetAddonDomain(ctx, config.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read addon domain", err.Error())
		return
	}
	if addonDomain == nil {
		resp.Diagnostics.AddError(
			"Addon domain not found",
			fmt.Sprintf("No addon domain named %q exists.", config.Domain.ValueString()),
		)
		return
	}

	state := addonDomainAPIToDataSourceModel(addonDomain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *addonDomainDataSource) Configure(
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

func addonDomainAPIToDataSourceModel(
	addonDomain *cpaneldomain.AddonDomain,
) AddonDomainDataSourceModel {
	return AddonDomainDataSourceModel{
		Domain:            types.StringValue(addonDomain.Domain),
		InternalSubdomain: types.StringValue(addonDomain.InternalSubdomain),
		RootDomain:        types.StringValue(addonDomain.RootDomain),
		FullSubdomain:     types.StringValue(addonDomain.FullSubdomain),
		DomainKey:         types.StringValue(addonDomain.DomainKey),
		DocumentRoot:      types.StringValue(addonDomain.BaseDirectory),
	}
}
