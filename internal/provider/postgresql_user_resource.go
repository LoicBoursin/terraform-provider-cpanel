package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"strings"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
	"time"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &postgreSQLUserResource{}
	_ resource.ResourceWithConfigure   = &postgreSQLUserResource{}
	_ resource.ResourceWithImportState = &postgreSQLUserResource{}
)

// NewPostgreSQLUserResource is a helper function to simplify the provider implementation.
func NewPostgreSQLUserResource() resource.Resource {
	return &postgreSQLUserResource{}
}

// postgreSQLUserResource is the resource implementation.
type postgreSQLUserResource struct {
	client *postgresql.Client
}

// Metadata returns the resource type name.
func (r *postgreSQLUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgresql_user"
}

// Schema defines the schema for the resource.
func (r *postgreSQLUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The user name.",
				MarkdownDescription: "The user name.",
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				Description:         "The user password.",
				MarkdownDescription: "The user password.",
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *postgreSQLUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from plan
	var state PostgreSQLUserModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read users
	postgreSQLUserDataSource, err := r.client.GetUsers()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting users",
			"Could not get users, unexpected error: "+err.Error(),
		)
		return
	}

	var currentUser string

	for _, user := range postgreSQLUserDataSource.Data {
		if user != state.Name.ValueString() {
			continue
		}

		currentUser = user
	}

	state.Name = types.StringValue(currentUser)

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *postgreSQLUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan PostgreSQLUserModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request parameters from plan
	var user postgresql.UserCreateModel
	user.Name = plan.Name.ValueString()
	user.Password = plan.Password.ValueString()

	// Create new database
	postgreSQLUserDataSourceModel, err := r.client.CreateUser(user)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating user",
			"Could not create user, unexpected error: "+err.Error(),
		)
		return
	}
	if postgreSQLUserDataSourceModel.Status != 1 {
		resp.Diagnostics.AddError(
			"Error creating user",
			"Could not create user, got errors: ["+strings.Join(postgreSQLUserDataSourceModel.Errors, ", ")+"]",
		)
		return
	}

	plan.Name = types.StringValue(user.Name)
	plan.Password = types.StringValue(user.Password)
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC3339))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *postgreSQLUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan PostgreSQLUserModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PostgreSQLUserModel

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request parameters from plan
	var userRename postgresql.UserRenameModel
	userRename.OldName = state.Name.ValueString()
	userRename.NewName = plan.Name.ValueString()
	userRename.Password = plan.Password.ValueString()

	var userSetPassword postgresql.UserSetPasswordModel
	userSetPassword.User = plan.Name.ValueString()
	userSetPassword.Password = plan.Password.ValueString()

	// Update user
	var postgreSQLUserDataSourceModel *postgresql.UserDataSourceModel
	var err error

	if userRename.OldName != userRename.NewName {
		postgreSQLUserDataSourceModel, err = r.client.RenameUser(userRename)
	} else {
		postgreSQLUserDataSourceModel, err = r.client.SetPassword(userSetPassword)
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating user",
			"Could not updating user, unexpected error: "+err.Error(),
		)
		return
	}

	if postgreSQLUserDataSourceModel.Status != 1 {
		resp.Diagnostics.AddError(
			"Error updating user",
			"Could not update user, got errors: ["+strings.Join(postgreSQLUserDataSourceModel.Errors, ", ")+"]",
		)
		return
	}

	plan.Name = types.StringValue(userRename.NewName)
	plan.Password = types.StringValue(userRename.Password)
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC3339))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *postgreSQLUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state PostgreSQLUserModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var user postgresql.UserDeleteModel
	user.Name = state.Name.ValueString()

	// Delete existing user
	postgreSQLUserDataSourceModel, err := r.client.DeleteUser(user)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting user",
			"Could not deleting user, unexpected error: "+err.Error(),
		)
		return
	}
	if postgreSQLUserDataSourceModel.Status != 1 {
		resp.Diagnostics.AddError(
			"Error deleting user",
			"Could not deleting user, got errors: ["+strings.Join(postgreSQLUserDataSourceModel.Errors, ", ")+"]",
		)
		return
	}
}

func (r *postgreSQLUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// Configure adds the provider configured client to the resource.
func (r *postgreSQLUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
