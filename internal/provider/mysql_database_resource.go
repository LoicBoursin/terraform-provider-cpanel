package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

var (
	_ resource.Resource                = &mySQLDatabaseResource{}
	_ resource.ResourceWithConfigure   = &mySQLDatabaseResource{}
	_ resource.ResourceWithImportState = &mySQLDatabaseResource{}
)

func NewMySQLDatabaseResource() resource.Resource {
	return &mySQLDatabaseResource{}
}

type mySQLDatabaseResource struct {
	client *mysql.Client
}

func (r *mySQLDatabaseResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mysql_database"
}

func (r *mySQLDatabaseResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Manages a cPanel MySQL or MariaDB database and the users granted all privileges. " +
			"Privilege grants and revocations are non-atomic; after a partial failure, inspect cPanel and re-import the database before retrying.",
		MarkdownDescription: "Manages a cPanel MySQL or MariaDB database and the users granted all privileges. " +
			"Privilege grants and revocations are non-atomic; after a partial failure, inspect cPanel and re-import the database before retrying.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database name, including the cPanel database prefix.",
				MarkdownDescription: "The database name, including the cPanel database prefix.",
				Validators:          mySQLNameValidators(),
			},
			"users": schema.SetAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The MySQL or MariaDB users that receive all privileges on the database.",
				MarkdownDescription: "The MySQL or MariaDB users that receive all privileges on the database.",
				Validators: []validator.Set{
					setvalidator.NoNullValues(),
					setvalidator.ValueStringsAre(mySQLNameValidators()...),
				},
			},
			"delete_on_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether deleting the Terraform resource also deletes the database. " +
					"cPanel does not expose an immutable database identity, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the database. Defaults to `false`. " +
					"cPanel does not expose an immutable database identity, so enabling this option accepts that a same-name replacement with the same grants cannot be distinguished from the managed database.",
			},
		},
	}
}

func (r *mySQLDatabaseResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state MySQLDatabaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	databases, err := r.client.ListDatabases(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read MySQL database",
			"Could not read MySQL databases: "+err.Error(),
		)
		return
	}

	refreshedDatabase, diagnostics := MySQLDatabaseAPIToModel(
		ctx,
		databases,
		state.Name.ValueString(),
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if refreshedDatabase == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Name = refreshedDatabase.Name
	state.Users = refreshedDatabase.Users
	if state.DeleteOnDestroy.IsNull() || state.DeleteOnDestroy.IsUnknown() {
		state.DeleteOnDestroy = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mySQLDatabaseResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan MySQLDatabaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	databaseName := plan.Name.ValueString()
	if err := validateMySQLAccountName(ctx, r.client, databaseName, mySQLDatabaseName); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL database name", err.Error())
		return
	}

	users, diagnostics := databaseUserNames(ctx, plan.Users)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.validateDatabaseUsers(ctx, users); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL database users", err.Error())
		return
	}

	exists, err := r.databaseExists(ctx, databaseName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL database", err.Error())
		return
	}
	if exists {
		resp.Diagnostics.AddError(
			"MySQL database already exists",
			fmt.Sprintf(
				"MySQL database %q already exists. Import it instead of taking ownership implicitly.",
				databaseName,
			),
		)
		return
	}

	if err := r.client.CreateDatabase(ctx, databaseName); err != nil {
		detail := "Could not create MySQL database: " + err.Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			exists, readErr := r.databaseExists(ctx, databaseName)
			switch {
			case readErr != nil:
				detail += ". Terraform could not determine whether the database was created; inspect cPanel before retrying."
			case exists:
				detail += ". A database with this name now exists, but Terraform did not adopt or delete it because the ambiguous creation cannot be attributed safely. Inspect it and import it if appropriate."
			default:
				detail += ". cPanel did not expose a database with this name after the ambiguous response."
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create MySQL database",
			detail,
		)
		return
	}

	for _, user := range users {
		if err := r.client.GrantAllPrivileges(ctx, user, databaseName); err != nil {
			resp.Diagnostics.AddError(
				"Unable to grant MySQL database privileges",
				fmt.Sprintf(
					"%v. Terraform left database %q intact because it may already contain data; inspect cPanel, complete the grants manually, or import the database before retrying.",
					err,
					databaseName,
				),
			)
			return
		}
	}

	if err := r.verifyDatabaseState(ctx, databaseName, users); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify MySQL database",
			fmt.Sprintf(
				"%v. Terraform left database %q intact because it may already contain data; inspect cPanel and import it before retrying.",
				err,
				databaseName,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mySQLDatabaseResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan MySQLDatabaseModel
	var state MySQLDatabaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldName := state.Name.ValueString()
	newName := plan.Name.ValueString()
	if err := validateMySQLAccountName(ctx, r.client, newName, mySQLDatabaseName); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL database name", err.Error())
		return
	}

	desiredUsers, diagnostics := databaseUserNames(ctx, plan.Users)
	resp.Diagnostics.Append(diagnostics...)
	currentUsers, diagnostics := databaseUserNames(ctx, state.Users)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.validateDatabaseUsers(ctx, desiredUsers); err != nil {
		resp.Diagnostics.AddError("Invalid MySQL database users", err.Error())
		return
	}
	if err := r.verifyDatabaseState(ctx, oldName, currentUsers); err != nil {
		resp.Diagnostics.AddError(
			"MySQL database changed outside Terraform",
			fmt.Sprintf(
				"Refusing to update the MySQL database because its current grants no longer match Terraform state: %v. Refresh and review the drift before retrying.",
				err,
			),
		)
		return
	}

	renamed := oldName != newName
	if renamed {
		currentExists, err := r.databaseExists(ctx, oldName)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read MySQL database",
				err.Error(),
			)
			return
		}
		if !currentExists {
			resp.Diagnostics.AddError(
				"MySQL database no longer exists",
				"Refresh the Terraform state before updating the MySQL database.",
			)
			return
		}
		targetExists, err := r.databaseExists(ctx, newName)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read target MySQL database",
				err.Error(),
			)
			return
		}
		if targetExists {
			resp.Diagnostics.AddError(
				"MySQL database name already exists",
				fmt.Sprintf("MySQL database %q already exists.", newName),
			)
			return
		}
	}

	addedUsers := stringSetDifference(desiredUsers, currentUsers)
	removedUsers := stringSetDifference(currentUsers, desiredUsers)

	for _, user := range addedUsers {
		if err := r.client.GrantAllPrivileges(ctx, user, oldName); err != nil {
			resp.Diagnostics.AddError(
				"Unable to grant MySQL database privileges",
				databaseUpdateFailureDetail(err, oldName),
			)
			return
		}
	}

	for _, user := range removedUsers {
		userExists, err := r.client.UserExists(ctx, user)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to verify MySQL database user",
				databaseUpdateFailureDetail(err, oldName),
			)
			return
		}
		if !userExists {
			continue
		}

		if err := r.client.RevokeAccess(ctx, user, oldName); err != nil {
			resp.Diagnostics.AddError(
				"Unable to revoke MySQL database privileges",
				databaseUpdateFailureDetail(err, oldName),
			)
			return
		}
	}

	if err := r.verifyDatabaseState(ctx, oldName, desiredUsers); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify MySQL database",
			databaseUpdateFailureDetail(err, oldName),
		)
		return
	}

	if renamed {
		if renameErr := r.client.RenameDatabase(
			ctx,
			oldName,
			newName,
		); renameErr != nil {
			if cPanelMutationErrorIsDeterministic(renameErr) {
				resp.Diagnostics.AddError(
					"Unable to rename MySQL database",
					"Could not rename MySQL database: "+renameErr.Error(),
				)
				return
			}

			applied, recoveryErr := r.reconcileDatabaseRename(
				ctx,
				oldName,
				newName,
				desiredUsers,
			)
			if recoveryErr != nil {
				resp.Diagnostics.AddError(
					"Unable to reconcile MySQL database rename",
					fmt.Sprintf(
						"%v. Read-only recovery could not attribute the new database safely: %v",
						renameErr,
						recoveryErr,
					),
				)
				return
			}
			if !applied {
				resp.Diagnostics.AddError(
					"Unable to rename MySQL database",
					"Could not rename MySQL database: "+renameErr.Error(),
				)
				return
			}
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.verifyDatabaseState(ctx, newName, desiredUsers); err != nil {
			resp.Diagnostics.AddWarning(
				"Unable to verify renamed MySQL database",
				fmt.Sprintf(
					"The rename to %q was applied after the desired grants were verified on %q, but the final read failed: %v. Refresh the resource before making another change.",
					newName,
					oldName,
					err,
				),
			)
		}
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mySQLDatabaseResource) reconcileDatabaseRename(
	ctx context.Context,
	oldName string,
	newName string,
	expectedUsers []string,
) (bool, error) {
	oldExists, err := r.databaseExists(ctx, oldName)
	if err != nil {
		return false, fmt.Errorf("read old MySQL database: %w", err)
	}
	newExists, err := r.databaseExists(ctx, newName)
	if err != nil {
		return false, fmt.Errorf("read new MySQL database: %w", err)
	}
	if !oldExists && newExists {
		if err := r.verifyDatabaseState(ctx, newName, expectedUsers); err != nil {
			return false, fmt.Errorf(
				"verify renamed MySQL database: %w",
				err,
			)
		}

		return true, nil
	}

	return reconcileMySQLDatabaseRenamePresence(oldExists, newExists)
}

func reconcileMySQLDatabaseRenamePresence(
	oldExists bool,
	newExists bool,
) (bool, error) {
	return reconcileRenamePresence(oldExists, newExists)
}

func (r *mySQLDatabaseResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state MySQLDatabaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"MySQL database preserved",
			fmt.Sprintf(
				"Terraform removed MySQL database %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Name.ValueString(),
			),
		)
		return
	}

	name := state.Name.ValueString()
	expectedUsers, diagnostics := databaseUserNames(ctx, state.Users)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}

	exists, err := r.databaseExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MySQL database", err.Error())
		return
	}
	if !exists {
		return
	}
	if err := r.verifyDatabaseState(ctx, name, expectedUsers); err != nil {
		resp.Diagnostics.AddError(
			"Refusing to delete MySQL database",
			fmt.Sprintf(
				"The remote MySQL database no longer matches Terraform state: %v. Refresh and review the drift before retrying.",
				err,
			),
		)
		return
	}

	deleteErr := r.client.DeleteDatabase(ctx, name)
	exists, readErr := r.databaseExists(ctx, name)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete MySQL database",
			mySQLDatabaseDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if !exists {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete MySQL database",
			"Could not delete MySQL database: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to delete MySQL database",
		fmt.Sprintf("cPanel reported success but MySQL database %q still exists.", name),
	)
}

func (r *mySQLDatabaseResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("delete_on_destroy"), false)...,
	)
}

func (r *mySQLDatabaseResource) Configure(
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

func (r *mySQLDatabaseResource) validateDatabaseUsers(
	ctx context.Context,
	users []string,
) error {
	for _, user := range users {
		if err := validateMySQLAccountName(ctx, r.client, user, mySQLUserName); err != nil {
			return fmt.Errorf("invalid user %q: %w", user, err)
		}

		exists, err := r.client.UserExists(ctx, user)
		if err != nil {
			return fmt.Errorf("verify user %q: %w", user, err)
		}
		if !exists {
			return fmt.Errorf("MySQL user %q does not exist", user)
		}
	}

	return nil
}

func (r *mySQLDatabaseResource) databaseExists(
	ctx context.Context,
	name string,
) (bool, error) {
	databases, err := r.client.ListDatabases(ctx)
	if err != nil {
		return false, err
	}

	for _, database := range databases.Data {
		if database.Database == name {
			return true, nil
		}
	}

	return false, nil
}

func (r *mySQLDatabaseResource) verifyDatabaseState(
	ctx context.Context,
	name string,
	expectedUsers []string,
) error {
	databases, err := r.client.ListDatabases(ctx)
	if err != nil {
		return fmt.Errorf("read MySQL databases after mutation: %w", err)
	}

	for _, database := range databases.Data {
		if database.Database != name {
			continue
		}

		actualUsers := append([]string(nil), database.Users...)
		sortedExpectedUsers := append([]string(nil), expectedUsers...)
		slices.Sort(actualUsers)
		slices.Sort(sortedExpectedUsers)
		if !slices.Equal(actualUsers, sortedExpectedUsers) {
			return fmt.Errorf(
				"MySQL database %q grants access to %v; expected %v",
				name,
				actualUsers,
				sortedExpectedUsers,
			)
		}

		for _, user := range expectedUsers {
			privileges, err := r.client.GetPrivileges(ctx, user, name)
			if err != nil {
				return fmt.Errorf("read privileges for %q: %w", user, err)
			}
			if !slices.Contains(privileges, "ALL PRIVILEGES") {
				return fmt.Errorf(
					"MySQL user %q has privileges %v on %q; expected ALL PRIVILEGES",
					user,
					privileges,
					name,
				)
			}
		}

		return nil
	}

	return fmt.Errorf("MySQL database %q was not found after mutation", name)
}

func mySQLDatabaseDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the MySQL database is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete MySQL database: %v. Terraform also could not verify whether the database still exists: %v",
		deleteErr,
		readErr,
	)
}
