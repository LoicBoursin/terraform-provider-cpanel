package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &postgreSQLDatabaseDataSource{}
	_ datasource.DataSourceWithConfigure = &postgreSQLDatabaseDataSource{}
)

// NewPostgreSQLDatabaseDataSource is a helper function to simplify the provider implementation.
func NewPostgreSQLDatabaseDataSource() datasource.DataSource {
	return &postgreSQLDatabaseDataSource{}
}

// postgreSQLDatabaseDataSource is the data source implementation.
type postgreSQLDatabaseDataSource struct {
	client *postgresql.Client
}

// Configure adds the provider configured client to the data source.
func (d *postgreSQLDatabaseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *postgreSQLDatabaseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgresql_database"
}

// Schema defines the schema for the data source.
func (d *postgreSQLDatabaseDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel PostgreSQL database and its privileged users.",
		MarkdownDescription: "Looks up a cPanel PostgreSQL database and its privileged users.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The PostgreSQL database name, including the cPanel account prefix.",
				MarkdownDescription: "The PostgreSQL database name, including the cPanel account prefix.",
				Validators:          postgreSQLNameValidators(),
			},
			"users": schema.SetAttribute{
				ElementType:         types.StringType,
				Description:         "The PostgreSQL users that currently have all privileges on the database.",
				MarkdownDescription: "The PostgreSQL users that currently have all privileges on the database.",
				Computed:            true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *postgreSQLDatabaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PostgreSQLDatabaseDataSourceModel

	// Read Terraform configuration data into the state
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}
	if err := validatePostgreSQLAccountName(d.client.Auth.Username, config.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Invalid PostgreSQL database name",
			err.Error(),
		)
		return
	}

	databases, err := d.client.GetDatabases(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to Read PostgreSQL databases: %s", err),
			err.Error(),
		)
		return
	}

	state, diagnostics := PostgreSQLDatabaseAPIToModel(ctx, databases, config.Name.ValueString())
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state == nil {
		resp.Diagnostics.AddError(
			"PostgreSQL database not found",
			fmt.Sprintf("No PostgreSQL database named %q exists.", config.Name.ValueString()),
		)
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
