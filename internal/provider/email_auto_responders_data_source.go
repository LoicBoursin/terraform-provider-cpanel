package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailAutoRespondersDataSource{}
	_ datasource.DataSourceWithConfigure = &emailAutoRespondersDataSource{}
)

func NewEmailAutoRespondersDataSource() datasource.DataSource {
	return &emailAutoRespondersDataSource{}
}

type emailAutoRespondersDataSource struct {
	client *cpanelmail.Client
}

func (d *emailAutoRespondersDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName +
		"_email_auto_responders"
}

func (d *emailAutoRespondersDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel email autoresponder address inventory.",
		MarkdownDescription: "Reads every cPanel mail domain through `Email::list_mail_domains`, then reads its autoresponder addresses through `Email::list_auto_responders` without exposing message content, sender, charset, interval, or schedule metadata.",
		Attributes: map[string]schema.Attribute{
			"addresses": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The complete autoresponder addresses in sorted order.",
				MarkdownDescription: "The complete autoresponder addresses in sorted order.",
			},
		},
	}
}

func (d *emailAutoRespondersDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	addresses, err := d.client.ListAutoResponderAddresses(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel autoresponder inventory",
			err.Error(),
		)
		return
	}
	state, diagnostics := emailInventoryStringList(ctx, addresses)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(
		ctx,
		&EmailAddressInventoryDataSourceModel{Addresses: state},
	)...)
}

func (d *emailAutoRespondersDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	d.client = configureEmailInventoryClient(request, response)
}
