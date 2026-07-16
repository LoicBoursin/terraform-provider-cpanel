package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelresourceusage "terraform-provider-cpanel/internal/cpanel/resourceusage"
)

var (
	_ datasource.DataSource              = &resourceUsageDataSource{}
	_ datasource.DataSourceWithConfigure = &resourceUsageDataSource{}
)

func NewResourceUsageDataSource() datasource.DataSource {
	return &resourceUsageDataSource{}
}

type resourceUsageDataSource struct {
	client *cpanelresourceusage.Client
}

type ResourceUsageDataSourceModel struct {
	Account types.String               `tfsdk:"account"`
	Metrics []ResourceUsageMetricModel `tfsdk:"metrics"`
}

type ResourceUsageMetricModel struct {
	Description types.String `tfsdk:"description"`
	Error       types.String `tfsdk:"error"`
	Formatter   types.String `tfsdk:"formatter"`
	ID          types.String `tfsdk:"id"`
	Maximum     types.String `tfsdk:"maximum"`
	Usage       types.String `tfsdk:"usage"`
}

func (d *resourceUsageDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_resource_usage"
}

func (d *resourceUsageDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the cPanel account resource usage inventory.",
		MarkdownDescription: "Reads the cPanel account resource usage inventory without exposing cPanel navigation URLs or unrelated account data. Numeric and numeric-string values are returned as strings so the data source preserves the precision reported by cPanel.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
			},
			"metrics": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The resource usage metrics sorted by identifier.",
				MarkdownDescription: "The resource usage metrics sorted by identifier.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "The stable cPanel metric identifier.",
							MarkdownDescription: "The stable cPanel metric identifier.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							Description:         "The localized metric description.",
							MarkdownDescription: "The localized metric description.",
						},
						"usage": schema.StringAttribute{
							Computed:            true,
							Description:         "The current usage exactly as a decimal or integer string.",
							MarkdownDescription: "The current usage exactly as a decimal or integer string.",
						},
						"maximum": schema.StringAttribute{
							Computed:            true,
							Description:         "The reported maximum as a decimal or integer string, or null when cPanel reports no maximum.",
							MarkdownDescription: "The reported maximum as a decimal or integer string, or `null` when cPanel reports no maximum.",
						},
						"formatter": schema.StringAttribute{
							Computed:            true,
							Description:         "The cPanel display formatter, when reported.",
							MarkdownDescription: "The cPanel display formatter, when reported, such as `format_bytes`.",
						},
						"error": schema.StringAttribute{
							Computed:            true,
							Description:         "The metric-specific collection error, when reported.",
							MarkdownDescription: "The metric-specific collection error, when reported.",
						},
					},
				},
			},
		},
	}
}

func (d *resourceUsageDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	metrics, err := d.client.Get(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel resource usage",
			err.Error(),
		)
		return
	}

	model := ResourceUsageDataSourceModel{
		Account: types.StringValue(cpanelresourceusage.AccountIdentity),
		Metrics: make([]ResourceUsageMetricModel, 0, len(metrics)),
	}
	for _, metric := range metrics {
		metricModel := ResourceUsageMetricModel{
			Description: types.StringValue(metric.Description),
			Error:       resourceUsageNullableStringValue(metric.Error),
			Formatter:   resourceUsageNullableStringValue(metric.Formatter),
			ID:          types.StringValue(metric.ID),
			Maximum:     resourceUsageNullableStringValue(metric.Maximum),
			Usage:       types.StringValue(metric.Usage),
		}
		model.Metrics = append(model.Metrics, metricModel)
	}

	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *resourceUsageDataSource) Configure(
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

	client, ok := providerData["resourceusage"].(*cpanelresourceusage.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Usage Client Type",
			fmt.Sprintf(
				"Expected *resourceusage.Client, got: %T.",
				providerData["resourceusage"],
			),
		)
		return
	}

	d.client = client
}

func resourceUsageNullableStringValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}

	return types.StringValue(*value)
}
