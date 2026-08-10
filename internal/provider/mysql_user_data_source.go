package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

var (
	_ datasource.DataSource              = &mySQLUserDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLUserDataSource{}
)

func NewMySQLUserDataSource() datasource.DataSource {
	return &mySQLUserDataSource{}
}

type mySQLUserDataSource struct {
	client *mysql.Client
}

func (d *mySQLUserDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mysql_user"
}

func (d *mySQLUserDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel MySQL or MariaDB user by name.",
		MarkdownDescription: "Looks up a cPanel MySQL or MariaDB user by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database user name, including the cPanel database prefix.",
				MarkdownDescription: "The database user name, including the cPanel database prefix.",
				Validators:          mySQLNameValidators(),
			},
		},
	}
}

func (d *mySQLUserDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config MySQLUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	if err := validateMySQLAccountName(ctx, d.client, name, mySQLUserName); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL user name", err.Error())
		return
	}

	exists, err := d.client.UserExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL user", err.Error())
		return
	}
	if !exists {
		resp.Diagnostics.AddError(
			"MySQL user not found",
			fmt.Sprintf("No MySQL user named %q exists.", name),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *mySQLUserDataSource) Configure(
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
