package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelboxtrapper "terraform-provider-cpanel/internal/cpanel/boxtrapper"
)

var (
	_ datasource.DataSource              = &boxTrapperSettingsDataSource{}
	_ datasource.DataSourceWithConfigure = &boxTrapperSettingsDataSource{}
)

func NewBoxTrapperSettingsDataSource() datasource.DataSource {
	return &boxTrapperSettingsDataSource{}
}

type boxTrapperSettingsDataSource struct {
	client *cpanelboxtrapper.Client
}

func (d *boxTrapperSettingsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_boxtrapper_settings"
}

func (d *boxTrapperSettingsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up BoxTrapper status and configuration for an existing cPanel mail account.",
		MarkdownDescription: "Looks up BoxTrapper status and configuration for an existing cPanel mail account. The account can be a complete mailbox address or the cPanel system username.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Required:            true,
				Description:         "The existing mailbox address or cPanel system username.",
				MarkdownDescription: "The existing mailbox address or cPanel system username.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 254),
				},
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether BoxTrapper is enabled for the account.",
				MarkdownDescription: "Whether BoxTrapper is enabled for the account.",
			},
			"enable_auto_whitelist": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether BoxTrapper automatically allowlists addresses that the account contacts.",
				MarkdownDescription: "Whether BoxTrapper automatically allowlists addresses that the account contacts.",
			},
			"from_addresses": schema.StringAttribute{
				Computed:            true,
				Description:         "The comma-separated sender addresses BoxTrapper associates with the account.",
				MarkdownDescription: "The comma-separated sender addresses BoxTrapper associates with the account.",
			},
			"from_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The optional sender name reported by cPanel.",
				MarkdownDescription: "The optional sender name reported by cPanel. This is null when no name is configured.",
			},
			"queue_days": schema.Int64Attribute{
				Computed:            true,
				Description:         "The number of days BoxTrapper retains logs and queued messages.",
				MarkdownDescription: "The number of days BoxTrapper retains logs and queued messages.",
			},
			"spam_score": schema.Float64Attribute{
				Computed:            true,
				Description:         "The SpamAssassin score at which BoxTrapper bypasses challenge verification.",
				MarkdownDescription: "The SpamAssassin score at which BoxTrapper bypasses challenge verification.",
			},
			"whitelist_by_association": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether BoxTrapper allowlists recipients associated with an allowlisted sender.",
				MarkdownDescription: "Whether BoxTrapper allowlists recipients associated with an allowlisted sender.",
			},
		},
	}
}

func (d *boxTrapperSettingsDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config BoxTrapperSettingsDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	account := config.Account.ValueString()
	if err := validateBoxTrapperAccount(account); err != nil {
		response.Diagnostics.AddError("Invalid BoxTrapper account", err.Error())
		return
	}

	settings, err := d.client.Get(ctx, account)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	if settings == nil {
		response.Diagnostics.AddError(
			"cPanel BoxTrapper account not found",
			fmt.Sprintf(
				"Account %q is not available to BoxTrapper on this cPanel account.",
				account,
			),
		)
		return
	}

	response.Diagnostics.Append(
		response.State.Set(
			ctx,
			boxTrapperSettingsToDataSourceModel(*settings),
		)...,
	)
}

func (d *boxTrapperSettingsDataSource) Configure(
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

	client, ok := providerData["boxtrapper"].(*cpanelboxtrapper.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected BoxTrapper Client Type",
			fmt.Sprintf(
				"Expected *boxtrapper.Client, got: %T.",
				providerData["boxtrapper"],
			),
		)
		return
	}

	d.client = client
}
