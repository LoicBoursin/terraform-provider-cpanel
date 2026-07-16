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
	_ datasource.DataSource              = &mySQLRestrictionsDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLRestrictionsDataSource{}
)

func NewMySQLRestrictionsDataSource() datasource.DataSource {
	return &mySQLRestrictionsDataSource{}
}

type mySQLRestrictionsDataSource struct {
	client *mysql.Client
}

type MySQLRestrictionsDataSourceModel struct {
	Prefix                types.String `tfsdk:"prefix"`
	MaxDatabaseNameLength types.Int64  `tfsdk:"max_database_name_length"`
	MaxUsernameLength     types.Int64  `tfsdk:"max_username_length"`
}

func (d *mySQLRestrictionsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_mysql_restrictions"
}

func (d *mySQLRestrictionsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the cPanel MySQL or MariaDB account naming restrictions.",
		MarkdownDescription: "Reads the cPanel MySQL or MariaDB account naming restrictions through `Mysql::get_restrictions`.",
		Attributes: map[string]schema.Attribute{
			"prefix": schema.StringAttribute{
				Computed:            true,
				Description:         "The prefix cPanel applies to database and database user names.",
				MarkdownDescription: "The prefix cPanel applies to database and database user names, or an empty string when prefixing is disabled.",
			},
			"max_database_name_length": schema.Int64Attribute{
				Computed:            true,
				Description:         "The maximum complete database-name length reported by cPanel.",
				MarkdownDescription: "The maximum complete database-name length reported by cPanel.",
			},
			"max_username_length": schema.Int64Attribute{
				Computed:            true,
				Description:         "The maximum complete database username length reported by cPanel.",
				MarkdownDescription: "The maximum complete database username length reported by cPanel.",
			},
		},
	}
}

func (d *mySQLRestrictionsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	restrictions, err := d.client.GetRestrictions(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read MySQL restrictions",
			err.Error(),
		)
		return
	}

	model := MySQLRestrictionsDataSourceModel{
		Prefix: types.StringValue(restrictions.Prefix),
		MaxDatabaseNameLength: types.Int64Value(
			int64(restrictions.MaxDatabaseNameLength),
		),
		MaxUsernameLength: types.Int64Value(
			int64(restrictions.MaxUsernameLength),
		),
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *mySQLRestrictionsDataSource) Configure(
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
