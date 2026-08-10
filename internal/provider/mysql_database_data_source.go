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
	_ datasource.DataSource              = &mySQLDatabaseDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLDatabaseDataSource{}
)

func NewMySQLDatabaseDataSource() datasource.DataSource {
	return &mySQLDatabaseDataSource{}
}

type mySQLDatabaseDataSource struct {
	client *mysql.Client
}

func (d *mySQLDatabaseDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mysql_database"
}

func (d *mySQLDatabaseDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel MySQL or MariaDB database and its privileged users.",
		MarkdownDescription: "Looks up a cPanel MySQL or MariaDB database and its privileged users.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database name, including the cPanel database prefix.",
				MarkdownDescription: "The database name, including the cPanel database prefix.",
				Validators:          mySQLNameValidators(),
			},
			"users": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The MySQL or MariaDB users that currently have access to the database.",
				MarkdownDescription: "The MySQL or MariaDB users that currently have access to the database.",
			},
		},
	}
}

func (d *mySQLDatabaseDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config MySQLDatabaseDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	if err := validateMySQLAccountName(ctx, d.client, name, mySQLDatabaseName); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL database name", err.Error())
		return
	}

	databases, err := d.client.ListDatabases(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL databases", err.Error())
		return
	}

	state, diagnostics := MySQLDatabaseAPIToModel(ctx, databases, name)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state == nil {
		resp.Diagnostics.AddError(
			"MySQL database not found",
			fmt.Sprintf("No MySQL database named %q exists.", name),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (d *mySQLDatabaseDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected map[string]interface{}, got: %T.", req.ProviderData),
		)
		return
	}

	client, ok := providerData["mysql"].(*mysql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected MySQL Client Type",
			fmt.Sprintf("Expected *mysql.Client, got: %T.", providerData["mysql"]),
		)
		return
	}

	d.client = client
}
