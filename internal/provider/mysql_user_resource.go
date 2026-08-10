package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

var (
	_ resource.Resource                = &mySQLUserResource{}
	_ resource.ResourceWithConfigure   = &mySQLUserResource{}
	_ resource.ResourceWithImportState = &mySQLUserResource{}
)

func NewMySQLUserResource() resource.Resource {
	return &mySQLUserResource{}
}

type mySQLUserResource struct {
	client *mysql.Client
}

func (r *mySQLUserResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mysql_user"
}

func (r *mySQLUserResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel MySQL or MariaDB user and its password.",
		MarkdownDescription: "Manages a cPanel MySQL or MariaDB user and its password.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database user name, including the cPanel database prefix. Changing it replaces the resource.",
				MarkdownDescription: "The database user name, including the cPanel database prefix. Changing it replaces the resource.",
				Validators:          mySQLNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The database user password. Configure it together with password_version. Terraform never stores it in plan or state artifacts.",
				MarkdownDescription: "The database user password. Configure it together with `password_version`. Terraform never stores it in plan or state artifacts.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(
						path.MatchRoot("password_version"),
					),
				},
			},
			"password_version": schema.Int64Attribute{
				Optional:            true,
				Description:         "A positive version that triggers an in-place password update when changed. It must be configured together with password.",
				MarkdownDescription: "A positive version that triggers an in-place password update when changed. It must be configured together with `password`.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(
						path.MatchRoot("password"),
					),
				},
			},
			"delete_on_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether deleting the Terraform resource also deletes the database user. " +
					"cPanel does not expose an immutable user identity, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the database user. Defaults to `false`. " +
					"cPanel does not expose an immutable user identity, so enabling this option accepts that a same-name replacement cannot be distinguished from the managed user.",
			},
		},
	}
}

func (r *mySQLUserResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state MySQLUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	exists, err := r.client.UserExists(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read MySQL user",
			"Could not read MySQL users: "+err.Error(),
		)
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}
	if state.DeleteOnDestroy.IsNull() || state.DeleteOnDestroy.IsUnknown() {
		state.DeleteOnDestroy = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mySQLUserResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan MySQLUserModel
	var config MySQLUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Password.IsNull() ||
		config.Password.IsUnknown() ||
		config.PasswordVersion.IsNull() ||
		config.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Missing MySQL user password configuration",
			"The password and password_version attributes must both be known when creating a MySQL user. They may both be omitted only after importing an existing user.",
		)
		return
	}

	name := plan.Name.ValueString()
	if err := validateMySQLAccountName(ctx, r.client, name, mySQLUserName); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL user name", err.Error())
		return
	}

	exists, err := r.client.UserExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL user", err.Error())
		return
	}
	if exists {
		resp.Diagnostics.AddError(
			"MySQL user already exists",
			fmt.Sprintf(
				"MySQL user %q already exists. Import it instead of replacing its password implicitly.",
				name,
			),
		)
		return
	}

	if err := r.client.CreateUser(ctx, name, config.Password.ValueString()); err != nil {
		detail := sensitiveMutationError(err, "MySQL user creation").Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			exists, readErr := r.client.UserExists(ctx, name)
			switch {
			case readErr != nil:
				detail += ". Terraform could not determine whether the user was created; inspect cPanel before retrying."
			case exists:
				detail += ". A user with this name now exists, but Terraform did not adopt or delete it because its password cannot be verified. Inspect it and import it if appropriate."
			default:
				detail += ". cPanel did not expose a user with this name after the ambiguous response."
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create MySQL user",
			detail,
		)
		return
	}

	if err := r.verifyUser(ctx, name); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify MySQL user",
			fmt.Sprintf(
				"%v. Terraform did not delete the user because its password and creation identity cannot be verified; inspect cPanel and import it if it exists.",
				err,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mySQLUserResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan MySQLUserModel
	var state MySQLUserModel
	var config MySQLUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldName := state.Name.ValueString()
	newName := plan.Name.ValueString()
	if err := validateMySQLAccountName(ctx, r.client, newName, mySQLUserName); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL user name", err.Error())
		return
	}
	if oldName != newName {
		resp.Diagnostics.AddError(
			"Unexpected in-place MySQL user rename",
			"MySQL user names require resource replacement because cPanel does not expose an immutable user identity.",
		)
		return
	}

	currentExists, err := r.client.UserExists(ctx, oldName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL user", err.Error())
		return
	}
	if !currentExists {
		resp.Diagnostics.AddError(
			"MySQL user no longer exists",
			"Refresh the Terraform state before updating the MySQL user.",
		)
		return
	}

	passwordChanged := !plan.PasswordVersion.Equal(state.PasswordVersion)
	if passwordChanged && plan.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown MySQL user password version",
			"The password_version attribute must be known when Terraform applies a MySQL user password change.",
		)
		return
	}
	if passwordChanged &&
		!plan.PasswordVersion.IsNull() &&
		(config.Password.IsNull() || config.Password.IsUnknown()) {
		resp.Diagnostics.AddError(
			"Missing MySQL user password",
			"A known password must be configured when password_version changes to a non-null value.",
		)
		return
	}
	if passwordChanged && !plan.PasswordVersion.IsNull() {
		if err := r.client.SetPassword(ctx, oldName, config.Password.ValueString()); err != nil {
			detail := sensitiveMutationError(
				err,
				"MySQL user password update",
			).Error()
			if !cPanelMutationErrorIsDeterministic(err) {
				detail += ". The remote password may already match the requested value; Terraform did not reapply the previous password."
			}
			resp.Diagnostics.AddError(
				"Unable to update MySQL user password",
				detail,
			)
			return
		}
	}

	if err := r.verifyUser(ctx, oldName); err != nil {
		detail := err.Error()
		if passwordChanged {
			detail += ". Terraform did not reapply the previous password because password changes cannot be verified remotely."
		}
		resp.Diagnostics.AddError(
			"Unable to verify MySQL user",
			detail,
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mySQLUserResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state MySQLUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"MySQL user preserved",
			fmt.Sprintf(
				"Terraform removed MySQL user %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Name.ValueString(),
			),
		)
		return
	}

	name := state.Name.ValueString()
	exists, err := r.client.UserExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL user", err.Error())
		return
	}
	if !exists {
		return
	}

	deleteErr := r.client.DeleteUser(ctx, name)
	exists, readErr := r.client.UserExists(ctx, name)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete MySQL user",
			mySQLUserDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if !exists {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete MySQL user",
			"Could not delete MySQL user: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to delete MySQL user",
		fmt.Sprintf("cPanel reported success but MySQL user %q still exists.", name),
	)
}

func (r *mySQLUserResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(
			ctx,
			path.Root("password_version"),
			types.Int64Null(),
		)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("delete_on_destroy"), false)...,
	)
}

func (r *mySQLUserResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
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

	r.client = client
}

func (r *mySQLUserResource) verifyUser(ctx context.Context, name string) error {
	exists, err := r.client.UserExists(ctx, name)
	if err != nil {
		return fmt.Errorf("read MySQL users after mutation: %w", err)
	}
	if !exists {
		return fmt.Errorf("MySQL user %q was not found after mutation", name)
	}

	return nil
}

func mySQLUserDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the MySQL user is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete MySQL user: %v. Terraform also could not verify whether the user still exists: %v",
		deleteErr,
		readErr,
	)
}
