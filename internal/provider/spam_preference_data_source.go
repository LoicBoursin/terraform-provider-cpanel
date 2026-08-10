package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelspam "terraform-provider-cpanel/internal/cpanel/spamassassin"
)

var (
	_ datasource.DataSource              = &spamPreferenceDataSource{}
	_ datasource.DataSourceWithConfigure = &spamPreferenceDataSource{}
)

func NewSpamPreferenceDataSource() datasource.DataSource {
	return &spamPreferenceDataSource{}
}

type spamPreferenceDataSource struct {
	client *cpanelspam.Client
}

func (d *spamPreferenceDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_spam_preference"
}

func (d *spamPreferenceDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads one documented cPanel SpamAssassin user preference.",
		MarkdownDescription: "Reads one documented cPanel SpamAssassin user preference. The data source never exposes arbitrary custom SpamAssassin configuration.",
		Attributes: map[string]schema.Attribute{
			"preference": schema.StringAttribute{
				Required:            true,
				Description:         "The supported SpamAssassin preference name.",
				MarkdownDescription: "The supported SpamAssassin preference name: `required_score`, `score`, `whitelist_from`, or `blacklist_from`.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						cpanelspam.SupportedPreferenceNames()...,
					),
				},
			},
			"values": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The preference values sorted and represented as an unordered set.",
				MarkdownDescription: "The preference values represented as an unordered set. This set is empty when the preference is not configured.",
			},
			"configured": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the preference is explicitly configured.",
				MarkdownDescription: "Whether the preference is explicitly configured for the account.",
			},
		},
	}
}

func (d *spamPreferenceDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config SpamPreferenceDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	preference, err := d.client.GetPreference(
		ctx,
		config.Preference.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}

	model, diagnostics := spamPreferenceToDataSourceModel(ctx, *preference)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, model)...)
}

func (d *spamPreferenceDataSource) Configure(
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

	client, ok := providerData["spamassassin"].(*cpanelspam.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SpamAssassin Client Type",
			fmt.Sprintf(
				"Expected *spamassassin.Client, got: %T.",
				providerData["spamassassin"],
			),
		)
		return
	}

	d.client = client
}
