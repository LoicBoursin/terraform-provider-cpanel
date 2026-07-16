package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailRoutingsDataSource{}
	_ datasource.DataSourceWithConfigure = &emailRoutingsDataSource{}
)

func NewEmailRoutingsDataSource() datasource.DataSource {
	return &emailRoutingsDataSource{}
}

type emailRoutingsDataSource struct {
	client *cpanelmail.Client
}

func (d *emailRoutingsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_routings"
}

func (d *emailRoutingsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel email routing inventory.",
		MarkdownDescription: "Reads every cPanel mail domain and its configured local-delivery mode through `Email::list_mxs` without exposing MX entries or detected routing metadata.",
		Attributes: map[string]schema.Attribute{
			"routings": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The email routing definitions sorted by domain.",
				MarkdownDescription: "The email routing definitions sorted by domain.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"domain": schema.StringAttribute{
							Computed:            true,
							Description:         "The normalized mail domain.",
							MarkdownDescription: "The normalized mail domain.",
						},
						"mode": schema.StringAttribute{
							Computed:            true,
							Description:         "The configured routing mode: auto, local, backup, or remote.",
							MarkdownDescription: "The configured routing mode: `auto`, `local`, `backup`, or `remote`.",
						},
					},
				},
			},
		},
	}
}

func (d *emailRoutingsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	definitions, err := d.client.ListRoutingDefinitions(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email routing inventory",
			err.Error(),
		)
		return
	}
	model := EmailRoutingsDataSourceModel{
		Routings: make([]EmailRoutingInventoryModel, 0, len(definitions)),
	}
	for _, definition := range definitions {
		model.Routings = append(
			model.Routings,
			EmailRoutingInventoryModel{
				Domain: types.StringValue(definition.Domain),
				Mode:   types.StringValue(string(definition.Mode)),
			},
		)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *emailRoutingsDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	d.client = configureEmailInventoryClient(request, response)
}
