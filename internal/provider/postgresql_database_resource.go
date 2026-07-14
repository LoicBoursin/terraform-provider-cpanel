package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

var (
	_ resource.Resource                 = &postgreSQLDatabaseResource{}
	_ resource.ResourceWithConfigure    = &postgreSQLDatabaseResource{}
	_ resource.ResourceWithImportState  = &postgreSQLDatabaseResource{}
	_ resource.ResourceWithUpgradeState = &postgreSQLDatabaseResource{}
)

func NewPostgreSQLDatabaseResource() resource.Resource {
	return &postgreSQLDatabaseResource{}
}

type postgreSQLDatabaseResource struct {
	client *postgresql.Client
}

func (r *postgreSQLDatabaseResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_postgresql_database"
}

func (r *postgreSQLDatabaseResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "Manages a cPanel PostgreSQL database and the users granted all privileges. " +
			"Privilege grants and revocations are non-atomic; after a partial failure, inspect cPanel and re-import the database before retrying.",
		MarkdownDescription: "Manages a cPanel PostgreSQL database and the users granted all privileges. " +
			"Privilege grants and revocations are non-atomic; after a partial failure, inspect cPanel and re-import the database before retrying.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The database name, including the cPanel account prefix.",
				MarkdownDescription: "The database name, including the cPanel account prefix.",
				Validators:          postgreSQLNameValidators(),
			},
			"users": schema.SetAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The PostgreSQL users that receive all privileges on the database.",
				MarkdownDescription: "The PostgreSQL users that receive all privileges on the database.",
				Validators: []validator.Set{
					setvalidator.NoNullValues(),
					setvalidator.ValueStringsAre(postgreSQLNameValidators()...),
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
		Version: 1,
	}
}

func (r *postgreSQLDatabaseResource) UpgradeState(
	_ context.Context,
) map[int64]resource.StateUpgrader {
	priorSchema := schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true},
			"users": schema.ListAttribute{
				ElementType: types.StringType,
				Required:    true,
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}

	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &priorSchema,
			StateUpgrader: func(
				ctx context.Context,
				req resource.UpgradeStateRequest,
				resp *resource.UpgradeStateResponse,
			) {
				var prior postgreSQLDatabaseModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				users := types.SetNull(types.StringType)
				switch {
				case prior.Users.IsUnknown():
					users = types.SetUnknown(types.StringType)
				case !prior.Users.IsNull():
					var diagnostics diag.Diagnostics
					users, diagnostics = types.SetValue(
						types.StringType,
						prior.Users.Elements(),
					)
					resp.Diagnostics.Append(diagnostics...)
					if resp.Diagnostics.HasError() {
						return
					}
				}

				resp.Diagnostics.Append(resp.State.Set(
					ctx,
					&PostgreSQLDatabaseModel{
						Name:            prior.Name,
						Users:           users,
						DeleteOnDestroy: types.BoolValue(false),
					},
				)...)
			},
		},
	}
}

func (r *postgreSQLDatabaseResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state PostgreSQLDatabaseModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	databases, err := r.client.GetDatabases(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read PostgreSQL database",
			"Could not read PostgreSQL databases: "+err.Error(),
		)
		return
	}

	refreshedDatabase, diagnostics := PostgreSQLDatabaseAPIToModel(
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

func (r *postgreSQLDatabaseResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan PostgreSQLDatabaseModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	databaseName := plan.Name.ValueString()
	if err := validatePostgreSQLAccountName(r.client.Auth.Username, databaseName); err != nil {
		resp.Diagnostics.AddError("Invalid PostgreSQL database name", err.Error())
		return
	}

	users, diagnostics := databaseUserNames(ctx, plan.Users)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.validateDatabaseUsers(ctx, users); err != nil {
		resp.Diagnostics.AddError("Invalid PostgreSQL database users", err.Error())
		return
	}

	exists, err := r.databaseExists(ctx, databaseName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read PostgreSQL database", err.Error())
		return
	}
	if exists {
		resp.Diagnostics.AddError(
			"PostgreSQL database already exists",
			fmt.Sprintf(
				"PostgreSQL database %q already exists. Import it instead of taking ownership implicitly.",
				databaseName,
			),
		)
		return
	}

	if _, err := r.client.CreateDatabase(
		ctx,
		postgresql.DatabaseCreateModel{Name: databaseName},
	); err != nil {
		detail := "Could not create PostgreSQL database: " + err.Error()
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
			"Unable to create PostgreSQL database",
			detail,
		)
		return
	}

	for _, user := range users {
		if _, err := r.client.GrantAllPrivileges(
			ctx,
			postgresql.UserGrantAllPrivilegesModel{
				Database: databaseName,
				User:     user,
			},
		); err != nil {
			resp.Diagnostics.AddError(
				"Unable to grant PostgreSQL database privileges",
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
			"Unable to verify PostgreSQL database",
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

func (r *postgreSQLDatabaseResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan PostgreSQLDatabaseModel
	var state PostgreSQLDatabaseModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldName := state.Name.ValueString()
	newName := plan.Name.ValueString()
	if err := validatePostgreSQLAccountName(r.client.Auth.Username, newName); err != nil {
		resp.Diagnostics.AddError("Invalid PostgreSQL database name", err.Error())
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
		resp.Diagnostics.AddError("Invalid PostgreSQL database users", err.Error())
		return
	}
	if err := r.verifyDatabaseState(ctx, oldName, currentUsers); err != nil {
		resp.Diagnostics.AddError(
			"PostgreSQL database changed outside Terraform",
			fmt.Sprintf(
				"Refusing to update the PostgreSQL database because its current grants no longer match Terraform state: %v. Refresh and review the drift before retrying.",
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
				"Unable to read PostgreSQL database",
				err.Error(),
			)
			return
		}
		if !currentExists {
			resp.Diagnostics.AddError(
				"PostgreSQL database no longer exists",
				"Refresh the Terraform state before updating the PostgreSQL database.",
			)
			return
		}
		targetExists, err := r.databaseExists(ctx, newName)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to read target PostgreSQL database",
				err.Error(),
			)
			return
		}
		if targetExists {
			resp.Diagnostics.AddError(
				"PostgreSQL database name already exists",
				fmt.Sprintf("PostgreSQL database %q already exists.", newName),
			)
			return
		}
	}

	addedUsers := stringSetDifference(desiredUsers, currentUsers)
	removedUsers := stringSetDifference(currentUsers, desiredUsers)

	for _, user := range addedUsers {
		if _, err := r.client.GrantAllPrivileges(
			ctx,
			postgresql.UserGrantAllPrivilegesModel{
				Database: oldName,
				User:     user,
			},
		); err != nil {
			resp.Diagnostics.AddError(
				"Unable to grant PostgreSQL database privileges",
				databaseUpdateFailureDetail(err, oldName),
			)
			return
		}
	}

	for _, user := range removedUsers {
		userExists, err := r.client.UserExists(ctx, user)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to verify PostgreSQL database user",
				databaseUpdateFailureDetail(err, oldName),
			)
			return
		}
		if !userExists {
			continue
		}

		if _, err := r.client.RevokeAllPrivileges(
			ctx,
			postgresql.UserRevokeAllPrivilegesModel{
				Database: oldName,
				User:     user,
			},
		); err != nil {
			resp.Diagnostics.AddError(
				"Unable to revoke PostgreSQL database privileges",
				databaseUpdateFailureDetail(err, oldName),
			)
			return
		}
	}
	if err := r.verifyDatabaseState(ctx, oldName, desiredUsers); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify PostgreSQL database",
			databaseUpdateFailureDetail(err, oldName),
		)
		return
	}

	if renamed {
		if _, renameErr := r.client.UpdateDatabase(
			ctx,
			postgresql.DatabaseUpdateModel{
				OldName: oldName,
				NewName: newName,
			},
		); renameErr != nil {
			if cPanelMutationErrorIsDeterministic(renameErr) {
				resp.Diagnostics.AddError(
					"Unable to rename PostgreSQL database",
					"Could not rename PostgreSQL database: "+renameErr.Error(),
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
					"Unable to reconcile PostgreSQL database rename",
					fmt.Sprintf(
						"%v. Read-only recovery could not determine whether the rename was applied: %v",
						renameErr,
						recoveryErr,
					),
				)
				return
			}
			if !applied {
				resp.Diagnostics.AddError(
					"Unable to rename PostgreSQL database",
					"Could not rename PostgreSQL database: "+renameErr.Error(),
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
				"Unable to verify renamed PostgreSQL database",
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

func (r *postgreSQLDatabaseResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state PostgreSQLDatabaseModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"PostgreSQL database preserved",
			fmt.Sprintf(
				"Terraform removed PostgreSQL database %q from state without deleting it from cPanel because delete_on_destroy is false.",
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
		resp.Diagnostics.AddError(
			"Unable to read PostgreSQL database",
			err.Error(),
		)
		return
	}
	if !exists {
		return
	}
	if err := r.verifyDatabaseState(ctx, name, expectedUsers); err != nil {
		resp.Diagnostics.AddError(
			"Refusing to delete PostgreSQL database",
			fmt.Sprintf(
				"The remote PostgreSQL database no longer matches Terraform state: %v. Refresh and review the drift before retrying.",
				err,
			),
		)
		return
	}

	deleteErr := r.deleteDatabase(ctx, name)
	exists, readErr := r.databaseExists(ctx, name)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete PostgreSQL database",
			postgreSQLDatabaseDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if !exists {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete PostgreSQL database",
			"Could not delete PostgreSQL database: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to delete PostgreSQL database",
		fmt.Sprintf(
			"cPanel reported success but PostgreSQL database %q still exists.",
			name,
		),
	)
}

func (r *postgreSQLDatabaseResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("delete_on_destroy"), false)...,
	)
}

func (r *postgreSQLDatabaseResource) Configure(
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
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T. Please report this issue to the provider developers.",
				req.ProviderData,
			),
		)
		return
	}

	postgreSQLClient, ok := providerData["postgresql"].(*postgresql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected PostgreSQL Client Type",
			fmt.Sprintf(
				"Expected *postgresql.Client, got: %T. Please report this issue to the provider developers.",
				providerData["postgresql"],
			),
		)
		return
	}

	r.client = postgreSQLClient
}

func databaseUserNames(ctx context.Context, users types.Set) ([]string, diag.Diagnostics) {
	var names []string
	diagnostics := users.ElementsAs(ctx, &names, false)
	slices.Sort(names)

	return names, diagnostics
}

func (r *postgreSQLDatabaseResource) validateDatabaseUsers(
	ctx context.Context,
	users []string,
) error {
	for _, user := range users {
		if err := validatePostgreSQLAccountName(r.client.Auth.Username, user); err != nil {
			return fmt.Errorf("invalid user %q: %w", user, err)
		}

		exists, err := r.client.UserExists(ctx, user)
		if err != nil {
			return fmt.Errorf("verify user %q: %w", user, err)
		}
		if !exists {
			return fmt.Errorf("PostgreSQL user %q does not exist", user)
		}
	}

	return nil
}

func (r *postgreSQLDatabaseResource) deleteDatabase(
	ctx context.Context,
	name string,
) error {
	_, err := r.client.DeleteDatabase(
		ctx,
		postgresql.DatabaseDeleteModel{Name: name},
	)

	return err
}

func (r *postgreSQLDatabaseResource) databaseExists(
	ctx context.Context,
	name string,
) (bool, error) {
	databases, err := r.client.GetDatabases(ctx)
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

func (r *postgreSQLDatabaseResource) reconcileDatabaseRename(
	ctx context.Context,
	oldName string,
	newName string,
	expectedUsers []string,
) (bool, error) {
	oldExists, err := r.databaseExists(ctx, oldName)
	if err != nil {
		return false, fmt.Errorf("read old PostgreSQL database: %w", err)
	}
	newExists, err := r.databaseExists(ctx, newName)
	if err != nil {
		return false, fmt.Errorf("read new PostgreSQL database: %w", err)
	}
	if !oldExists && newExists {
		if err := r.verifyDatabaseState(ctx, newName, expectedUsers); err != nil {
			return false, fmt.Errorf(
				"verify renamed PostgreSQL database: %w",
				err,
			)
		}

		return true, nil
	}

	return reconcileRenamePresence(oldExists, newExists)
}

func (r *postgreSQLDatabaseResource) verifyDatabaseState(
	ctx context.Context,
	name string,
	expectedUsers []string,
) error {
	databases, err := r.client.GetDatabases(ctx)
	if err != nil {
		return fmt.Errorf("read PostgreSQL databases after mutation: %w", err)
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
				"PostgreSQL database %q grants access to %v; expected %v",
				name,
				actualUsers,
				sortedExpectedUsers,
			)
		}

		return nil
	}

	return fmt.Errorf("PostgreSQL database %q was not found after mutation", name)
}

func stringSetDifference(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}

	var difference []string
	for _, value := range left {
		if _, ok := rightSet[value]; !ok {
			difference = append(difference, value)
		}
	}
	slices.Sort(difference)

	return difference
}

func databaseUpdateFailureDetail(mutationErr error, databaseName string) string {
	return fmt.Sprintf(
		"%v. Terraform did not attempt an automatic rollback because cPanel database mutations are non-atomic and the current database may contain concurrent grants or data. Inspect database %q and re-import it before retrying.",
		mutationErr,
		databaseName,
	)
}

func postgreSQLDatabaseDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the PostgreSQL database is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete PostgreSQL database: %v. Terraform also could not verify whether the database still exists: %v",
		deleteErr,
		readErr,
	)
}
