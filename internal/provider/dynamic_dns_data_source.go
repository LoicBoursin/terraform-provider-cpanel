package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/ddns"
)

var (
	_ datasource.DataSource              = &dynamicDNSDataSource{}
	_ datasource.DataSourceWithConfigure = &dynamicDNSDataSource{}
)

func NewDynamicDNSDataSource() datasource.DataSource {
	return &dynamicDNSDataSource{}
}

type dynamicDNSDataSource struct {
	client *ddns.Client
}

func (d *dynamicDNSDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_dynamic_dns"
}

func (d *dynamicDNSDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel Dynamic DNS domain and its webcall metadata.",
		MarkdownDescription: "Looks up a cPanel Dynamic DNS domain and its sensitive webcall metadata.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The complete Dynamic DNS domain.",
				MarkdownDescription: "The complete Dynamic DNS domain.",
				Validators:          domainNameValidators(),
			},
			"description": schema.StringAttribute{
				Computed:            true,
				Description:         "The human-readable description of the Dynamic DNS domain.",
				MarkdownDescription: "The human-readable description of the Dynamic DNS domain.",
			},
			"webcall_id": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The secret identifier used by the Dynamic DNS update endpoint.",
				MarkdownDescription: "The secret identifier used by the Dynamic DNS update endpoint.",
			},
			"webcall_url": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The HTTPS URL that a router or device calls to update the domain addresses.",
				MarkdownDescription: "The HTTPS URL that a router or device calls to update the domain addresses.",
			},
			"created_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The Dynamic DNS domain creation time as a Unix timestamp.",
				MarkdownDescription: "The Dynamic DNS domain creation time as a Unix timestamp.",
			},
			"ipv4": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The IPv4 addresses currently associated with the Dynamic DNS domain.",
				MarkdownDescription: "The IPv4 addresses currently associated with the Dynamic DNS domain.",
			},
			"ipv6": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The IPv6 addresses currently associated with the Dynamic DNS domain.",
				MarkdownDescription: "The IPv6 addresses currently associated with the Dynamic DNS domain.",
			},
			"last_run_times": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.Int64Type,
				Description:         "The most recent webcall execution times as Unix timestamps.",
				MarkdownDescription: "The most recent webcall execution times as Unix timestamps.",
			},
			"last_update_time": schema.Int64Attribute{
				Computed:            true,
				Description:         "The most recent address-change time as a Unix timestamp.",
				MarkdownDescription: "The most recent address-change time as a Unix timestamp.",
			},
		},
	}
}

func (d *dynamicDNSDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config DynamicDNSDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := d.client.Get(ctx, config.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Dynamic DNS domain", err.Error())
		return
	}
	if domain == nil {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain not found",
			fmt.Sprintf(
				"No Dynamic DNS domain named %q exists.",
				config.Domain.ValueString(),
			),
		)
		return
	}

	state, diagnostics := dynamicDNSToDataSourceModel(ctx, *domain)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (d *dynamicDNSDataSource) Configure(
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

	client, ok := providerData["ddns"].(*ddns.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Dynamic DNS Client Type",
			fmt.Sprintf(
				"Expected *ddns.Client, got: %T.",
				providerData["ddns"],
			),
		)
		return
	}

	d.client = client
}
