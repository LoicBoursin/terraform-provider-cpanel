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
	_ datasource.DataSource              = &mySQLUsersDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLUsersDataSource{}
)

func NewMySQLUsersDataSource() datasource.DataSource {
	return &mySQLUsersDataSource{}
}

type mySQLUsersDataSource struct {
	client *mysql.Client
}

type MySQLUsersDataSourceModel struct {
	Users types.List `tfsdk:"users"`
}

func (d *mySQLUsersDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_mysql_users"
}

func (d *mySQLUsersDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel MySQL or MariaDB user-name inventory.",
		MarkdownDescription: "Reads the complete cPanel MySQL or MariaDB user-name inventory through `Mysql::list_users`.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The complete database user names in sorted order.",
				MarkdownDescription: "The complete database user names in sorted order.",
			},
		},
	}
}

func (d *mySQLUsersDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	users, err := d.client.ListUserNames(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read MySQL user inventory",
			err.Error(),
		)
		return
	}

	state, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		users,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(
		ctx,
		&MySQLUsersDataSourceModel{Users: state},
	)...)
}

func (d *mySQLUsersDataSource) Configure(
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
