package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &postgreSQLUserDataSource{}
	_ datasource.DataSourceWithConfigure = &postgreSQLUserDataSource{}
)

// NewPostgreSQLUserDataSource is a helper function to simplify the provider implementation.
func NewPostgreSQLUserDataSource() datasource.DataSource {
	return &postgreSQLUserDataSource{}
}

// postgreSQLUserDataSource is the data source implementation.
type postgreSQLUserDataSource struct {
	client *postgresql.Client
}

// Configure adds the provider configured client to the data source.
func (d *postgreSQLUserDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected map[string]interface{}, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	postgresqlClient, ok := providerData["postgresql"].(*postgresql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected PostgreSQL Client Type",
			fmt.Sprintf("Expected *postgresql.Client, got: %T. Please report this issue to the provider developers.", providerData["postgresql"]),
		)
		return
	}

	d.client = postgresqlClient
}

// Metadata returns the data source type name.
func (d *postgreSQLUserDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgresql_user"
}

// Schema defines the schema for the data source.
func (d *postgreSQLUserDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel PostgreSQL user by name.",
		MarkdownDescription: "Looks up a cPanel PostgreSQL user by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The PostgreSQL user name, including the cPanel account prefix.",
				MarkdownDescription: "The PostgreSQL user name, including the cPanel account prefix.",
				Validators:          postgreSQLNameValidators(),
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *postgreSQLUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PostgreSQLUserDataSourceModel

	// Read Terraform configuration data into the state
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}
	if err := validatePostgreSQLAccountName(d.client.Auth.Username, config.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Invalid PostgreSQL user name",
			err.Error(),
		)
		return
	}

	users, err := d.client.GetUsers(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to Read PostgreSQL user: %s", err),
			err.Error(),
		)
		return
	}

	state := PostgreSQLUserAPIToDataSourceModel(users, config.Name.ValueString())

	if state == nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to Read PostgreSQL user from name: %s", config.Name),
			"",
		)
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
