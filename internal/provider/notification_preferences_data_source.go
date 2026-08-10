package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelcontact "terraform-provider-cpanel/internal/cpanel/contactinformation"
)

var (
	_ datasource.DataSource              = &notificationPreferencesDataSource{}
	_ datasource.DataSourceWithConfigure = &notificationPreferencesDataSource{}
)

func NewNotificationPreferencesDataSource() datasource.DataSource {
	return &notificationPreferencesDataSource{}
}

type notificationPreferencesDataSource struct {
	client *cpanelcontact.Client
}

func (d *notificationPreferencesDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_notification_preferences"
}

func (d *notificationPreferencesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the cPanel account's notification preferences.",
		MarkdownDescription: "Reads the cPanel account's notification preferences and their localized descriptions.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
			},
			"preferences": schema.MapAttribute{
				ElementType:         types.BoolType,
				Computed:            true,
				Description:         "Every notification preference name and whether it is enabled.",
				MarkdownDescription: "Every notification preference name and whether it is enabled.",
			},
			"descriptions": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "Localized descriptions keyed by notification preference name.",
				MarkdownDescription: "Localized descriptions keyed by notification preference name.",
			},
		},
	}
}

func (d *notificationPreferencesDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	preferences, err := d.client.GetNotificationPreferences(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel notification preferences",
			err.Error(),
		)
		return
	}

	model, diagnostics := notificationPreferencesToDataSourceModel(
		ctx,
		*preferences,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, model)...)
}

func (d *notificationPreferencesDataSource) Configure(
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

	client, ok := providerData["contactinformation"].(*cpanelcontact.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected ContactInformation Client Type",
			fmt.Sprintf(
				"Expected *contactinformation.Client, got: %T.",
				providerData["contactinformation"],
			),
		)
		return
	}

	d.client = client
}
