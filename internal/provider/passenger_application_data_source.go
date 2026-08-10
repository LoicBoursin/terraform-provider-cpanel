package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/passenger"
)

var (
	_ datasource.DataSource              = &passengerApplicationDataSource{}
	_ datasource.DataSourceWithConfigure = &passengerApplicationDataSource{}
)

func NewPassengerApplicationDataSource() datasource.DataSource {
	return &passengerApplicationDataSource{}
}

type passengerApplicationDataSource struct {
	client *passenger.Client
}

func (d *passengerApplicationDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_passenger_application"
}

func (d *passengerApplicationDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one cPanel Passenger application registration.",
		MarkdownDescription: "Looks up one cPanel Passenger application registration.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The Passenger application name.",
				MarkdownDescription: "The Passenger application name.",
				Validators:          passengerApplicationNameValidators(),
			},
			"path": schema.StringAttribute{
				Computed:            true,
				Description:         "The application directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized application directory relative to the cPanel account home.",
			},
			"domain": schema.StringAttribute{
				Computed:            true,
				Description:         "The account domain that serves the application.",
				MarkdownDescription: "The account domain that serves the application.",
			},
			"base_uri": schema.StringAttribute{
				Computed:            true,
				Description:         "The URL path that mounts the application on its domain.",
				MarkdownDescription: "The normalized URL path that mounts the application on its domain.",
			},
			"deployment_mode": schema.StringAttribute{
				Computed:            true,
				Description:         "The Passenger deployment mode.",
				MarkdownDescription: "The Passenger deployment mode: `production` or `development`.",
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel generates the web-server configuration for the application.",
				MarkdownDescription: "Whether cPanel generates the web-server configuration for the application.",
			},
			"environment_variables": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Sensitive:           true,
				Description:         "The application environment variables.",
				MarkdownDescription: "The application environment variables. Values are stored as sensitive Terraform state.",
			},
			"absolute_path": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute application path reported by cPanel.",
				MarkdownDescription: "The absolute application path reported by cPanel.",
			},
			"dependency_commands": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The dependency installation commands reported by cPanel.",
				MarkdownDescription: "The dependency installation commands reported by cPanel for `gem`, `npm`, and `pip`. The provider never executes them.",
			},
			"nodejs": schema.StringAttribute{
				Computed:            true,
				Description:         "The Node.js executable reported by cPanel, when present.",
				MarkdownDescription: "The Node.js executable reported by cPanel, when present.",
			},
			"python": schema.StringAttribute{
				Computed:            true,
				Description:         "The Python executable reported by cPanel, when present.",
				MarkdownDescription: "The Python executable reported by cPanel, when present.",
			},
			"ruby": schema.StringAttribute{
				Computed:            true,
				Description:         "The Ruby executable reported by cPanel, when present.",
				MarkdownDescription: "The Ruby executable reported by cPanel, when present.",
			},
		},
	}
}

func (d *passengerApplicationDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config PassengerApplicationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	if err := passenger.ValidateName(name); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Passenger application name",
			err.Error(),
		)
		return
	}
	application, err := d.client.Get(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Passenger application",
			err.Error(),
		)
		return
	}
	if application == nil {
		resp.Diagnostics.AddError(
			"Passenger application not found",
			fmt.Sprintf(
				"Passenger application %q is not registered in cPanel.",
				name,
			),
		)
		return
	}

	model, diagnostics := passengerApplicationToDataSourceModel(
		ctx,
		*application,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (d *passengerApplicationDataSource) Configure(
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

	client, ok := providerData["passenger"].(*passenger.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Passenger Client Type",
			fmt.Sprintf(
				"Expected *passenger.Client, got: %T.",
				providerData["passenger"],
			),
		)
		return
	}

	d.client = client
}
