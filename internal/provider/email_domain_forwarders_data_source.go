package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailDomainForwardersDataSource{}
	_ datasource.DataSourceWithConfigure = &emailDomainForwardersDataSource{}
)

func NewEmailDomainForwardersDataSource() datasource.DataSource {
	return &emailDomainForwardersDataSource{}
}

type emailDomainForwardersDataSource struct {
	client *cpanelmail.Client
}

func (d *emailDomainForwardersDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName +
		"_email_domain_forwarders"
}

func (d *emailDomainForwardersDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel email domain forwarder inventory.",
		MarkdownDescription: "Reads the complete cPanel email domain forwarder inventory through `Email::list_domain_forwarders`.",
		Attributes: map[string]schema.Attribute{
			"forwarders": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The domain forwarders sorted by source domain.",
				MarkdownDescription: "The domain forwarders sorted by source domain.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"domain": schema.StringAttribute{
							Computed:            true,
							Description:         "The normalized source domain.",
							MarkdownDescription: "The normalized source domain.",
						},
						"destination": schema.StringAttribute{
							Computed:            true,
							Description:         "The normalized destination domain.",
							MarkdownDescription: "The normalized destination domain.",
						},
					},
				},
			},
		},
	}
}

func (d *emailDomainForwardersDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	forwarders, err := d.client.ListDomainForwarders(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email domain forwarder inventory",
			err.Error(),
		)
		return
	}
	model := EmailDomainForwardersDataSourceModel{
		Forwarders: make(
			[]EmailDomainForwarderInventoryModel,
			0,
			len(forwarders),
		),
	}
	for _, forwarder := range forwarders {
		model.Forwarders = append(
			model.Forwarders,
			EmailDomainForwarderInventoryModel{
				Domain:      types.StringValue(forwarder.Domain),
				Destination: types.StringValue(forwarder.Destination),
			},
		)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *emailDomainForwardersDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	d.client = configureEmailInventoryClient(request, response)
}
