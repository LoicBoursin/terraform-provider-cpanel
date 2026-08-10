package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailMailingListsDataSource{}
	_ datasource.DataSourceWithConfigure = &emailMailingListsDataSource{}
)

func NewEmailMailingListsDataSource() datasource.DataSource {
	return &emailMailingListsDataSource{}
}

type emailMailingListsDataSource struct {
	client *cpanelmail.Client
}

func (d *emailMailingListsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName +
		"_email_mailing_lists"
}

func (d *emailMailingListsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel Mailman mailing list address inventory.",
		MarkdownDescription: "Reads the complete cPanel Mailman mailing list address inventory through `Email::list_lists` without exposing administrator, privacy, or disk-usage metadata.",
		Attributes: map[string]schema.Attribute{
			"addresses": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The complete mailing list addresses in sorted order.",
				MarkdownDescription: "The complete mailing list addresses in sorted order.",
			},
		},
	}
}

func (d *emailMailingListsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	addresses, err := d.client.ListMailingListAddresses(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel mailing list inventory",
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

func (d *emailMailingListsDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	d.client = configureEmailInventoryClient(request, response)
}
