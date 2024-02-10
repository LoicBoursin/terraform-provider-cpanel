package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"slices"
	"strings"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
	"terraform-provider-cpanel/internal/utils"
	"time"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &postgreSQLDatabaseResource{}
	_ resource.ResourceWithConfigure   = &postgreSQLDatabaseResource{}
	_ resource.ResourceWithImportState = &postgreSQLDatabaseResource{}
)

// NewPostgreSQLDatabaseResource is a helper function to simplify the provider implementation.
func NewPostgreSQLDatabaseResource() resource.Resource {
	return &postgreSQLDatabaseResource{}
}

// postgreSQLDatabaseResource is the resource implementation.
type postgreSQLDatabaseResource struct {
	client *postgresql.Client
}

// Metadata returns the resource type name.
func (r *postgreSQLDatabaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgresql_database"
}

// Schema defines the schema for the resource.
func (r *postgreSQLDatabaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database name.",
				MarkdownDescription: "The database name.",
			},
			"users": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The database users.",
				MarkdownDescription: "The database users.",
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *postgreSQLDatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from plan
	var state PostgreSQLDatabaseModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read databases
	databases, err := r.client.GetDatabases()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting databases",
			"Could not get databases, unexpected error: "+err.Error(),
		)
		return
	}

	state = *PostgreSQLDatabaseAPIToModel(databases, state.Name.ValueString())

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *postgreSQLDatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan PostgreSQLDatabaseModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request parameters from plan
	var database postgresql.DatabaseCreateModel
	database.Name = plan.Name.ValueString()

	// Create new database
	postgreSQLDatabaseDataSourceModel, err := r.client.CreateDatabase(database)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating database",
			"Could not create database, unexpected error: "+err.Error(),
		)
		return
	}
	if postgreSQLDatabaseDataSourceModel.Status != 1 {
		resp.Diagnostics.AddError(
			"Error creating database",
			"Could not create database, got errors: ["+strings.Join(postgreSQLDatabaseDataSourceModel.Errors, ", ")+"]",
		)
		return
	}

	var users []types.String

	for _, user := range plan.Users {
		var grantAllPrivileges postgresql.UserGrantAllPrivilegesModel
		grantAllPrivileges.Database = database.Name
		grantAllPrivileges.User = user.ValueString()
		postgresqlUserDataSourceModel, err := r.client.GrantAllPrivileges(grantAllPrivileges)

		if err != nil {
			resp.Diagnostics.AddError(
				"Error granting all privileges",
				"Could not grant all privileges, unexpected error: "+err.Error(),
			)
			return
		}

		if postgresqlUserDataSourceModel.Status != 1 {
			resp.Diagnostics.AddError(
				"Error granting all privileges",
				"Could not grant all privileges, got errors: ["+strings.Join(postgresqlUserDataSourceModel.Errors, ", ")+"]",
			)
			return
		}

		users = append(users, user)
	}

	plan.Name = types.StringValue(database.Name)
	plan.Users = users
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC3339))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *postgreSQLDatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan PostgreSQLDatabaseModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PostgreSQLDatabaseModel

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that users are unique
	if len(plan.Users) != len(utils.SliceUniqueTypesString(plan.Users)) {
		resp.Diagnostics.AddError(
			"Duplicate users",
			"Users must be unique",
		)
		return
	}

	// Generate API request parameters from plan
	var database postgresql.DatabaseUpdateModel
	database.OldName = state.Name.ValueString()
	database.NewName = plan.Name.ValueString()

	// Update database
	if database.OldName != database.NewName {
		postgreSQLDatabaseDataSourceModel, err := r.client.UpdateDatabase(database)

		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating database",
				"Could not updating database, unexpected error: "+err.Error(),
			)
			return
		}

		if postgreSQLDatabaseDataSourceModel.Status != 1 {
			resp.Diagnostics.AddError(
				"Error updating database",
				"Could not update database, got errors: ["+strings.Join(postgreSQLDatabaseDataSourceModel.Errors, ", ")+"]",
			)
			return
		}
	}

	var users = []types.String{}

	for _, user := range plan.Users {
		users = append(users, user)

		userExists, _ := r.client.UserExists(user.ValueString())

		if !userExists {
			resp.Diagnostics.AddError(
				"User does not exist",
				fmt.Sprintf("User does not exist: %s. Create a postgreSQL user resource first.", user.ValueString()),
			)
			return
		}

		if slices.Contains(state.Users, user) {
			continue
		}

		var grantAllPrivileges postgresql.UserGrantAllPrivilegesModel
		grantAllPrivileges.Database = database.NewName
		grantAllPrivileges.User = user.ValueString()
		postgresqlUserDataSourceModel, err := r.client.GrantAllPrivileges(grantAllPrivileges)

		if err != nil {
			resp.Diagnostics.AddError(
				"Error granting all privileges",
				"Could not grant all privileges, unexpected error: "+err.Error(),
			)
			return
		}

		if postgresqlUserDataSourceModel.Status != 1 {
			resp.Diagnostics.AddError(
				"Error granting all privileges",
				"Could not grant all privileges, got errors: ["+strings.Join(postgresqlUserDataSourceModel.Errors, ", ")+"]",
			)
			return
		}
	}

	for _, user := range state.Users {
		userExists, _ := r.client.UserExists(user.ValueString())

		if slices.Contains(plan.Users, user) || !userExists {
			continue
		}

		var revokeAllPrivileges postgresql.UserRevokeAllPrivilegesModel
		revokeAllPrivileges.Database = database.NewName
		revokeAllPrivileges.User = user.ValueString()
		postgresqlUserDataSourceModel, err := r.client.RevokeAllPrivileges(revokeAllPrivileges)

		if err != nil {
			resp.Diagnostics.AddError(
				"Error revoking all privileges",
				"Could not revoke all privileges, unexpected error: "+err.Error(),
			)
			return
		}

		if postgresqlUserDataSourceModel.Status != 1 {
			resp.Diagnostics.AddError(
				"Error revoking all privileges",
				"Could not revoke all privileges, got errors: ["+strings.Join(postgresqlUserDataSourceModel.Errors, ", ")+"]",
			)
			return
		}
	}

	plan.Name = types.StringValue(database.NewName)
	plan.Users = users
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC3339))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *postgreSQLDatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state PostgreSQLDatabaseModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var database postgresql.DatabaseDeleteModel
	database.Name = state.Name.ValueString()

	// Delete existing database
	postgreSQLDatabaseDataSourceModel, err := r.client.DeleteDatabase(database)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting database",
			"Could not deleting database, unexpected error: "+err.Error(),
		)
		return
	}
	if postgreSQLDatabaseDataSourceModel.Status != 1 {
		resp.Diagnostics.AddError(
			"Error deleting database",
			"Could not deleting database, got errors: ["+strings.Join(postgreSQLDatabaseDataSourceModel.Errors, ", ")+"]",
		)
		return
	}
}

func (r *postgreSQLDatabaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// Configure adds the provider configured client to the resource.
func (r *postgreSQLDatabaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
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

	r.client = postgresqlClient
}
