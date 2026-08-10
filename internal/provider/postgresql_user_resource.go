package provider

import (
	"context"
	"errors"
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

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

var (
	_ resource.Resource                 = &postgreSQLUserResource{}
	_ resource.ResourceWithConfigure    = &postgreSQLUserResource{}
	_ resource.ResourceWithImportState  = &postgreSQLUserResource{}
	_ resource.ResourceWithUpgradeState = &postgreSQLUserResource{}
)

func NewPostgreSQLUserResource() resource.Resource {
	return &postgreSQLUserResource{}
}

type postgreSQLUserClient interface {
	CreateUser(
		context.Context,
		postgresql.UserCreateModel,
	) (*postgresql.UserDataSourceModel, error)
	DeleteUser(
		context.Context,
		postgresql.UserDeleteModel,
	) (*postgresql.UserDataSourceModel, error)
	SetPassword(
		context.Context,
		postgresql.UserSetPasswordModel,
	) (*postgresql.UserDataSourceModel, error)
	UserExists(context.Context, string) (bool, error)
}

type postgreSQLUserResource struct {
	client          postgreSQLUserClient
	accountUsername string
}

func (r *postgreSQLUserResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_postgresql_user"
}

func (r *postgreSQLUserResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel PostgreSQL user and its password.",
		MarkdownDescription: "Manages a cPanel PostgreSQL user and its password.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The PostgreSQL user name, including the cPanel account prefix. Changing it replaces the resource.",
				MarkdownDescription: "The PostgreSQL user name, including the cPanel account prefix. Changing it replaces the resource.",
				Validators:          postgreSQLNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The PostgreSQL user password. Configure it together with password_version. Terraform never stores it in plan or state artifacts.",
				MarkdownDescription: "The PostgreSQL user password. Configure it together with `password_version`. Terraform never stores it in plan or state artifacts.",
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
				Description: "Whether deleting the Terraform resource also deletes the PostgreSQL user. " +
					"cPanel does not expose an immutable user identity, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the PostgreSQL user. Defaults to `false`. " +
					"cPanel does not expose an immutable user identity, so enabling this option accepts that a same-name replacement cannot be distinguished from the managed user.",
			},
		},
		Version: 1,
	}
}

func (r *postgreSQLUserResource) UpgradeState(
	_ context.Context,
) map[int64]resource.StateUpgrader {
	priorSchema := schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
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
				var prior postgreSQLUserModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				resp.Diagnostics.Append(resp.State.Set(
					ctx,
					&PostgreSQLUserModel{
						Name:            prior.Name,
						Password:        types.StringNull(),
						PasswordVersion: types.Int64Null(),
						DeleteOnDestroy: types.BoolValue(false),
					},
				)...)
			},
		},
	}
}

func (r *postgreSQLUserResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state PostgreSQLUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	exists, err := r.client.UserExists(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read PostgreSQL user",
			"Could not read PostgreSQL users: "+err.Error(),
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

func (r *postgreSQLUserResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan PostgreSQLUserModel
	var config PostgreSQLUserModel
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
			"Missing PostgreSQL user password configuration",
			"The password and password_version attributes must both be known when creating a PostgreSQL user. They may both be omitted only after importing an existing user.",
		)
		return
	}

	name := plan.Name.ValueString()
	if err := validatePostgreSQLAccountName(r.accountUsername, name); err != nil {
		resp.Diagnostics.AddError("Invalid PostgreSQL user name", err.Error())
		return
	}

	exists, err := r.client.UserExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read PostgreSQL user", err.Error())
		return
	}
	if exists {
		resp.Diagnostics.AddError(
			"PostgreSQL user already exists",
			fmt.Sprintf(
				"PostgreSQL user %q already exists. Import it instead of replacing its password implicitly.",
				name,
			),
		)
		return
	}

	_, mutationErr := r.client.CreateUser(
		ctx,
		postgresql.UserCreateModel{
			Name:     name,
			Password: config.Password.ValueString(),
		},
	)
	if mutationErr != nil {
		if postgreSQLMutationErrorIsDeterministic(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to create PostgreSQL user",
				sensitiveMutationError(
					mutationErr,
					"PostgreSQL user creation",
				).Error(),
			)
			return
		}

		exists, readErr := r.client.UserExists(ctx, name)
		detail := sensitiveMutationError(
			mutationErr,
			"PostgreSQL user creation",
		).Error()
		switch {
		case readErr != nil:
			detail += ". Terraform could not determine whether the user was created; inspect cPanel before retrying."
		case exists:
			detail += ". A user with this name now exists, but Terraform did not adopt or delete it because its password cannot be verified. Inspect it and import it if appropriate."
		default:
			detail += ". cPanel did not expose a user with this name after the ambiguous response."
		}
		resp.Diagnostics.AddError("Unable to create PostgreSQL user", detail)
		return
	}

	if err := r.verifyUser(ctx, name); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify PostgreSQL user",
			fmt.Sprintf(
				"%v. Terraform did not delete the user because its password and creation identity cannot be verified; inspect cPanel and import it if it exists.",
				err,
			),
		)
		return
	}

	stateDiagnostics := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Unable to store PostgreSQL user state",
			fmt.Sprintf(
				"Terraform created PostgreSQL user %q but could not write its state. Terraform left the user untouched because its password cannot be verified; inspect cPanel and import it.",
				name,
			),
		)
	}
}

func (r *postgreSQLUserResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan PostgreSQLUserModel
	var state PostgreSQLUserModel
	var config PostgreSQLUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldName := state.Name.ValueString()
	newName := plan.Name.ValueString()
	if err := validatePostgreSQLAccountName(r.accountUsername, newName); err != nil {
		resp.Diagnostics.AddError("Invalid PostgreSQL user name", err.Error())
		return
	}
	if oldName != newName {
		resp.Diagnostics.AddError(
			"Unexpected in-place PostgreSQL user rename",
			"PostgreSQL user names require resource replacement because cPanel does not expose an immutable user identity.",
		)
		return
	}

	currentExists, err := r.client.UserExists(ctx, oldName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read PostgreSQL user", err.Error())
		return
	}
	if !currentExists {
		resp.Diagnostics.AddError(
			"PostgreSQL user no longer exists",
			"Refresh the Terraform state before updating the PostgreSQL user.",
		)
		return
	}

	passwordChanged := !plan.PasswordVersion.Equal(state.PasswordVersion)
	if passwordChanged && plan.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown PostgreSQL user password version",
			"The password_version attribute must be known when Terraform applies a PostgreSQL user password change.",
		)
		return
	}
	if passwordChanged &&
		!plan.PasswordVersion.IsNull() &&
		(config.Password.IsNull() || config.Password.IsUnknown()) {
		resp.Diagnostics.AddError(
			"Missing PostgreSQL user password",
			"A known password must be configured when password_version changes to a non-null value.",
		)
		return
	}
	if passwordChanged && !plan.PasswordVersion.IsNull() {
		if _, err := r.client.SetPassword(
			ctx,
			postgresql.UserSetPasswordModel{
				User:     oldName,
				Password: config.Password.ValueString(),
			},
		); err != nil {
			detail := sensitiveMutationError(
				err,
				"PostgreSQL user password update",
			).Error()
			if !postgreSQLMutationErrorIsDeterministic(err) {
				detail += ". cPanel may have applied the password change; retrying the same configured password is safe."
			}
			resp.Diagnostics.AddError(
				"Unable to update PostgreSQL user password",
				detail,
			)
			return
		}
	}

	if err := r.verifyUser(ctx, oldName); err != nil {
		detail := err.Error()
		if passwordChanged {
			detail += ". Terraform did not restore the previous password because PostgreSQL password changes cannot be verified remotely."
		}
		resp.Diagnostics.AddError(
			"Unable to verify PostgreSQL user update",
			detail,
		)
		return
	}

	stateDiagnostics := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Unable to store PostgreSQL user state",
			fmt.Sprintf(
				"PostgreSQL user %q was updated but Terraform could not write its state. Terraform did not restore the previous name or password because password changes cannot be verified remotely.",
				newName,
			),
		)
	}
}

func (r *postgreSQLUserResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state PostgreSQLUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"PostgreSQL user preserved",
			fmt.Sprintf(
				"Terraform removed PostgreSQL user %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Name.ValueString(),
			),
		)
		return
	}

	name := state.Name.ValueString()
	exists, err := r.client.UserExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read PostgreSQL user", err.Error())
		return
	}
	if !exists {
		return
	}

	_, mutationErr := r.client.DeleteUser(
		ctx,
		postgresql.UserDeleteModel{Name: name},
	)
	if mutationErr != nil {
		if postgreSQLMutationErrorIsDeterministic(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to delete PostgreSQL user",
				mutationErr.Error(),
			)
			return
		}

		exists, recoveryErr := r.client.UserExists(ctx, name)
		if recoveryErr != nil || exists {
			if recoveryErr == nil {
				recoveryErr = fmt.Errorf(
					"PostgreSQL user %q still exists",
					name,
				)
			}
			resp.Diagnostics.AddError(
				"Unable to reconcile PostgreSQL user deletion",
				postgreSQLAmbiguousMutationErrorDetail(
					mutationErr,
					"PostgreSQL user deletion",
					recoveryErr,
				),
			)
			return
		}

		return
	}

	exists, err = r.client.UserExists(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify PostgreSQL user deletion",
			err.Error(),
		)
		return
	}
	if exists {
		resp.Diagnostics.AddError(
			"Unable to verify PostgreSQL user deletion",
			fmt.Sprintf("PostgreSQL user %q still exists after deletion.", name),
		)
	}
}

func (r *postgreSQLUserResource) ImportState(
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

func (r *postgreSQLUserResource) Configure(
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
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["postgresql"].(*postgresql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected PostgreSQL Client Type",
			fmt.Sprintf(
				"Expected *postgresql.Client, got: %T.",
				providerData["postgresql"],
			),
		)
		return
	}

	r.client = client
	r.accountUsername = client.Auth.Username
}

func (r *postgreSQLUserResource) verifyUser(
	ctx context.Context,
	name string,
) error {
	exists, err := r.client.UserExists(ctx, name)
	if err != nil {
		return fmt.Errorf("read PostgreSQL users after mutation: %w", err)
	}
	if !exists {
		return fmt.Errorf("PostgreSQL user %q was not found after mutation", name)
	}

	return nil
}

func postgreSQLMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError
	if errors.As(err, &apiError) {
		return true
	}

	var httpError *cpanelapi.HTTPError

	return errors.As(err, &httpError) &&
		httpError.StatusCode >= 400 &&
		httpError.StatusCode < 500
}

func postgreSQLAmbiguousMutationErrorDetail(
	mutationErr error,
	operation string,
	recoveryErr error,
) string {
	return fmt.Sprintf(
		"%v. Read-only recovery could not confirm the requested remote state: %v",
		sensitiveMutationError(mutationErr, operation),
		recoveryErr,
	)
}
