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
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database name.",
				MarkdownDescription: "The database name.",
			},
			"users": schema.ListAttribute{
				ElementType:         types.StringType,
				Description:         "The database users.",
				MarkdownDescription: "The database users.",
				Optional:            true,
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *postgreSQLDatabaseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PostgreSQLDatabaseModel

	// Read Terraform configuration data into the state
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	databases, err := d.client.GetDatabases()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to Read PostgreSQL databases: %s", err),
			err.Error(),
		)
		return
	}

	state := PostgreSQLDatabaseAPIToModel(databases, config.Name.ValueString())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
