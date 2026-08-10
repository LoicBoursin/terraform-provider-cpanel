package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailAccountsDataSource{}
	_ datasource.DataSourceWithConfigure = &emailAccountsDataSource{}
)

func NewEmailAccountsDataSource() datasource.DataSource {
	return &emailAccountsDataSource{}
}

type emailAccountsDataSource struct {
	client *cpanelmail.Client
}

func (d *emailAccountsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_accounts"
}

func (d *emailAccountsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel email account address inventory.",
		MarkdownDescription: "Reads the complete cPanel virtual email account address inventory through `Email::list_pops` without exposing suspension metadata or the cPanel system account.",
		Attributes: map[string]schema.Attribute{
			"addresses": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The complete email account addresses in sorted order.",
				MarkdownDescription: "The complete email account addresses in sorted order.",
			},
		},
	}
}

func (d *emailAccountsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	addresses, err := d.client.ListAccountAddresses(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email account inventory",
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

func (d *emailAccountsDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	d.client = configureEmailInventoryClient(request, response)
}
