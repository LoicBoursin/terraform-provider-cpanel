package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
)

var (
	_ datasource.DataSource              = &logSettingsDataSource{}
	_ datasource.DataSourceWithConfigure = &logSettingsDataSource{}
)

func NewLogSettingsDataSource() datasource.DataSource {
	return &logSettingsDataSource{}
}

type logSettingsDataSource struct {
	client *cpanellogmanager.Client
}

func (d *logSettingsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_log_settings"
}

func (d *logSettingsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the cPanel account's raw access log archival settings.",
		MarkdownDescription: "Reads the cPanel account's raw access log archival settings.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
			},
			"archive_logs": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel archives processed access logs in the account home directory.",
				MarkdownDescription: "Whether cPanel archives processed access logs in the account home directory.",
			},
			"prune_archives": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel removes the previous month's archived logs.",
				MarkdownDescription: "Whether cPanel removes the previous month's archived logs.",
			},
			"retention_days": schema.Int64Attribute{
				Computed:            true,
				Description:         "The configured retention period. -1 means the server default, 0 means indefinitely, and positive values are days.",
				MarkdownDescription: "The configured retention period. `-1` means the server default, `0` means indefinitely, and positive values are days.",
			},
			"effective_retention_days": schema.Int64Attribute{
				Computed:            true,
				Description:         "The effective number of retention days reported by cPanel.",
				MarkdownDescription: "The effective number of retention days reported by cPanel. `0` means indefinitely.",
			},
			"using_default_retention": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the account uses the server-wide retention period.",
				MarkdownDescription: "Whether the account uses the server-wide retention period.",
			},
		},
	}
}

func (d *logSettingsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	settings, err := d.client.Get(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel log settings",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		response.State.Set(
			ctx,
			logSettingsToDataSourceModel(*settings),
		)...,
	)
}

func (d *logSettingsDataSource) Configure(
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

	client, ok := providerData["logmanager"].(*cpanellogmanager.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected LogManager Client Type",
			fmt.Sprintf(
				"Expected *logmanager.Client, got: %T.",
				providerData["logmanager"],
			),
		)
		return
	}

	d.client = client
}
