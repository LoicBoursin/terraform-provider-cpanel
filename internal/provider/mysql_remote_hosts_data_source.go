package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

var (
	_ datasource.DataSource              = &mySQLRemoteHostsDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLRemoteHostsDataSource{}
)

func NewMySQLRemoteHostsDataSource() datasource.DataSource {
	return &mySQLRemoteHostsDataSource{}
}

type mySQLRemoteHostsDataSource struct {
	client *mysql.Client
}

type MySQLRemoteHostsDataSourceModel struct {
	Hosts types.List `tfsdk:"hosts"`
}

func (d *mySQLRemoteHostsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_mysql_remote_hosts"
}

func (d *mySQLRemoteHostsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel remote MySQL host inventory.",
		MarkdownDescription: "Reads the complete cPanel remote MySQL host inventory through `MysqlFE::listhosts`.",
		Attributes: map[string]schema.Attribute{
			"hosts": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The normalized remote MySQL hosts in sorted order.",
				MarkdownDescription: "The normalized remote MySQL hosts in sorted order.",
			},
		},
	}
}

func (d *mySQLRemoteHostsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	hosts, err := d.client.ListRemoteHostNames(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read remote MySQL host inventory",
			err.Error(),
		)
		return
	}

	state, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		hosts,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(
		ctx,
		&MySQLRemoteHostsDataSourceModel{Hosts: state},
	)...)
}

func (d *mySQLRemoteHostsDataSource) Configure(
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

	client, ok := providerData["mysql"].(*mysql.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected MySQL Client Type",
			fmt.Sprintf(
				"Expected *mysql.Client, got: %T.",
				providerData["mysql"],
			),
		)
		return
	}

	d.client = client
}
