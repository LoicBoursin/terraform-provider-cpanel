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
	_ datasource.DataSource              = &mySQLDatabasesDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLDatabasesDataSource{}
)

func NewMySQLDatabasesDataSource() datasource.DataSource {
	return &mySQLDatabasesDataSource{}
}

type mySQLDatabasesDataSource struct {
	client *mysql.Client
}

type MySQLDatabasesDataSourceModel struct {
	Databases types.List `tfsdk:"databases"`
}

func (d *mySQLDatabasesDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_mysql_databases"
}

func (d *mySQLDatabasesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel MySQL or MariaDB database-name inventory.",
		MarkdownDescription: "Reads the complete cPanel MySQL or MariaDB database-name inventory through `Mysql::list_databases`.",
		Attributes: map[string]schema.Attribute{
			"databases": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The complete database names in sorted order.",
				MarkdownDescription: "The complete database names in sorted order.",
			},
		},
	}
}

func (d *mySQLDatabasesDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	databases, err := d.client.ListDatabaseNames(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read MySQL database inventory",
			err.Error(),
		)
		return
	}

	state, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		databases,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(
		ctx,
		&MySQLDatabasesDataSourceModel{Databases: state},
	)...)
}

func (d *mySQLDatabasesDataSource) Configure(
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
