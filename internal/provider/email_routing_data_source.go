package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailRoutingDataSource{}
	_ datasource.DataSourceWithConfigure = &emailRoutingDataSource{}
)

func NewEmailRoutingDataSource() datasource.DataSource {
	return &emailRoutingDataSource{}
}

type emailRoutingDataSource struct {
	client *cpanelmail.Client
}

func (d *emailRoutingDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_routing"
}

func (d *emailRoutingDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up the cPanel email routing mode for an account domain.",
		MarkdownDescription: "Looks up the cPanel email routing mode for an account domain. This describes cPanel's local delivery configuration and does not manage MX DNS records.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The existing account domain to look up.",
				MarkdownDescription: "The existing account domain to look up.",
				Validators:          domainNameValidators(),
			},
			"mode": schema.StringAttribute{
				Computed:            true,
				Description:         "The configured cPanel email routing mode.",
				MarkdownDescription: "The configured cPanel email routing mode: `auto`, `local`, `backup`, or `remote`.",
			},
			"detected_mode": schema.StringAttribute{
				Computed:            true,
				Description:         "The effective routing mode detected from the highest-priority mail exchanger.",
				MarkdownDescription: "The effective routing mode detected from the highest-priority mail exchanger.",
			},
			"primary_exchanger": schema.StringAttribute{
				Computed:            true,
				Description:         "The highest-priority mail exchanger reported by cPanel.",
				MarkdownDescription: "The highest-priority mail exchanger reported by cPanel.",
			},
		},
	}
}

func (d *emailRoutingDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config EmailRoutingDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	definition := cpanelmail.RoutingDefinition{
		Domain: config.Domain.ValueString(),
		Mode:   cpanelmail.RoutingModeAuto,
	}
	if err := validateEmailRoutingDefinition(definition); err != nil {
		response.Diagnostics.AddError("Invalid cPanel email routing", err.Error())
		return
	}

	unlock := d.client.LockRoutingDomain(definition.Domain)
	defer unlock()

	routing, err := d.client.GetRouting(ctx, definition.Domain)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email routing",
			err.Error(),
		)
		return
	}
	if routing == nil {
		response.Diagnostics.AddError(
			"cPanel email routing not found",
			fmt.Sprintf(
				"Domain %q is not present in the cPanel mail routing inventory.",
				definition.Domain,
			),
		)
		return
	}

	response.Diagnostics.Append(
		response.State.Set(ctx, emailRoutingToDataSourceModel(*routing))...,
	)
}

func (d *emailRoutingDataSource) Configure(
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

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf(
				"Expected *email.Client, got: %T.",
				providerData["email"],
			),
		)
		return
	}

	d.client = client
}
