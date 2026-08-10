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
	_ datasource.DataSource              = &domainsDataSource{}
	_ datasource.DataSourceWithConfigure = &domainsDataSource{}
)

func NewDomainsDataSource() datasource.DataSource {
	return &domainsDataSource{}
}

type domainsDataSource struct {
	client *cpaneldomain.Client
}

type DomainsDataSourceModel struct {
	Domains []DomainInventoryModel `tfsdk:"domains"`
}

type DomainInventoryModel struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

func (d *domainsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_domains"
}

func (d *domainsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel web-domain inventory.",
		MarkdownDescription: "Reads the complete cPanel web-domain inventory from `DomainInfo::list_domains`, normalized and sorted by domain name.",
		Attributes: map[string]schema.Attribute{
			"domains": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The account domains sorted by normalized name.",
				MarkdownDescription: "The account domains sorted by normalized name.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							Description:         "The normalized domain name.",
							MarkdownDescription: "The normalized domain name.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							Description:         "The cPanel domain category: main, addon, subdomain, or alias.",
							MarkdownDescription: "The cPanel domain category: `main`, `addon`, `subdomain`, or `alias`.",
						},
					},
				},
			},
		},
	}
}

func (d *domainsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	domains, err := d.client.ListDomains(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel domain inventory",
			err.Error(),
		)
		return
	}

	model := DomainsDataSourceModel{
		Domains: make([]DomainInventoryModel, 0, len(domains)),
	}
	for _, domain := range domains {
		model.Domains = append(model.Domains, DomainInventoryModel{
			Name: types.StringValue(domain.Name),
			Type: types.StringValue(string(domain.Type)),
		})
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *domainsDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}
	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
	}
	client, ok := providerData["domain"].(*cpaneldomain.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Domain Client Type",
			fmt.Sprintf(
				"Expected *domain.Client, got: %T.",
				providerData["domain"],
			),
		)
		return
	}
	d.client = client
}
