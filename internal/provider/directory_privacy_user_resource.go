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

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

var (
	_ resource.Resource                = &directoryPrivacyUserResource{}
	_ resource.ResourceWithConfigure   = &directoryPrivacyUserResource{}
	_ resource.ResourceWithImportState = &directoryPrivacyUserResource{}
)

func NewDirectoryPrivacyUserResource() resource.Resource {
	return &directoryPrivacyUserResource{}
}

type directoryPrivacyUserResource struct {
	client *directoryprivacy.Client
}

func (r *directoryPrivacyUserResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_directory_privacy_user"
}

func (r *directoryPrivacyUserResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one authorized user for a cPanel Directory Privacy password file.",
		MarkdownDescription: "Manages one authorized user for a cPanel Directory Privacy password file.",
		Attributes: map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				Required:            true,
				Description:         "The directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path relative to the cPanel account home.",
				Validators:          directoryIndexDirectoryValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"username": schema.StringAttribute{
				Required:            true,
				Description:         "The username authorized to access the directory.",
				MarkdownDescription: "The username authorized to access the directory.",
				Validators:          directoryPrivacyUsernameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The user's HTTP Basic authentication password. Configure it together with password_version. Terraform never stores it in plan or state artifacts.",
				MarkdownDescription: "The user's HTTP Basic authentication password. Configure it together with `password_version`. Terraform never stores it in plan or state artifacts.",
				Validators: append(
					directoryPrivacyUserPasswordValidators(),
					stringvalidator.AlsoRequires(
						path.MatchRoot("password_version"),
					),
				),
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
				Description: "Whether deleting the Terraform resource also deletes the authorized user. " +
					"cPanel does not expose an immutable user identity, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the authorized user. Defaults to `false`. " +
					"cPanel does not expose an immutable user identity, so enabling this option accepts that a same-name replacement cannot be distinguished from the managed user.",
			},
			"absolute_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
		},
	}
}

func (r *directoryPrivacyUserResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DirectoryPrivacyUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiUser, err := r.client.GetUser(
		ctx,
		state.Directory.ValueString(),
		state.Username.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Directory Privacy user",
			err.Error(),
		)
		return
	}
	if apiUser == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyDirectoryPrivacyUserToResourceModel(&state, *apiUser)
	if state.DeleteOnDestroy.IsNull() || state.DeleteOnDestroy.IsUnknown() {
		state.DeleteOnDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *directoryPrivacyUserResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DirectoryPrivacyUserResourceModel
	var config DirectoryPrivacyUserResourceModel
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
			"Missing Directory Privacy user password configuration",
			"The password and password_version attributes must both be known when creating a Directory Privacy user. They may both be omitted only after importing an existing user.",
		)
		return
	}

	definition := directoryPrivacyUserDefinitionFromResourceModel(
		plan,
		config.Password.ValueString(),
	)
	if err := validateDirectoryPrivacyUserDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid Directory Privacy user", err.Error())
		return
	}

	privacy, err := r.client.Get(ctx, definition.Directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory privacy",
			err.Error(),
		)
		return
	}
	if privacy == nil {
		resp.Diagnostics.AddError(
			"Directory not found",
			fmt.Sprintf(
				"Directory %q does not exist in the cPanel account.",
				definition.Directory,
			),
		)
		return
	}
	if !privacy.Protected {
		resp.Diagnostics.AddError(
			"Directory Privacy is disabled",
			fmt.Sprintf(
				"Directory %q must be protected before adding authorized users.",
				definition.Directory,
			),
		)
		return
	}

	current, err := r.client.GetUser(
		ctx,
		definition.Directory,
		definition.Username,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Directory Privacy user",
			err.Error(),
		)
		return
	}
	if current != nil {
		resp.Diagnostics.AddError(
			"Directory Privacy user already exists",
			fmt.Sprintf(
				"User %q already exists for directory %q. Import it instead of replacing its password implicitly.",
				definition.Username,
				definition.Directory,
			),
		)
		return
	}

	if err := r.client.AddUser(ctx, definition); err != nil {
		detail := sensitiveMutationError(
			err,
			"Directory Privacy user creation",
		).Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			created, recoveryErr := r.client.GetUser(
				ctx,
				definition.Directory,
				definition.Username,
			)
			switch {
			case recoveryErr != nil:
				detail += fmt.Sprintf(
					". Terraform could not determine whether the user was created because the follow-up read failed: %v",
					recoveryErr,
				)
			case created != nil:
				detail = fmt.Sprintf(
					"User %q may have been created for directory %q, but Terraform cannot verify its password after an ambiguous response and refuses to adopt or delete it automatically. Inspect the user in cPanel and import it if it is intended to remain.",
					definition.Username,
					definition.Directory,
				)
			}
		}
		resp.Diagnostics.AddError(
			"Unable to add Directory Privacy user",
			detail,
		)
		return
	}

	apiUser, err := r.verifyUser(
		ctx,
		definition.Directory,
		definition.Username,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify Directory Privacy user",
			fmt.Sprintf(
				"%v. Terraform will not delete the user because its password cannot be attributed safely after creation. Inspect the user in cPanel and import it if it is intended to remain.",
				err,
			),
		)
		return
	}

	applyDirectoryPrivacyUserToResourceModel(&plan, *apiUser)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *directoryPrivacyUserResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DirectoryPrivacyUserResourceModel
	var state DirectoryPrivacyUserResourceModel
	var config DirectoryPrivacyUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	passwordChanged := !plan.PasswordVersion.Equal(state.PasswordVersion)
	if passwordChanged && plan.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown Directory Privacy user password version",
			"The password_version attribute must be known when Terraform applies a Directory Privacy user password change.",
		)
		return
	}
	if passwordChanged &&
		!plan.PasswordVersion.IsNull() &&
		(config.Password.IsNull() || config.Password.IsUnknown()) {
		resp.Diagnostics.AddError(
			"Missing Directory Privacy user password",
			"A known password must be configured when password_version changes to a non-null value.",
		)
		return
	}

	definition := directoryPrivacyUserDefinitionFromResourceModel(
		plan,
		config.Password.ValueString(),
	)
	if err := validateDirectoryPrivacyUserDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid Directory Privacy user", err.Error())
		return
	}

	current, err := r.client.GetUser(
		ctx,
		state.Directory.ValueString(),
		state.Username.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Directory Privacy user",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Directory Privacy user no longer exists",
			"Refresh the Terraform state before updating the Directory Privacy user.",
		)
		return
	}

	if passwordChanged && !plan.PasswordVersion.IsNull() {
		if err := r.client.AddUser(ctx, definition); err != nil {
			resp.Diagnostics.AddError(
				"Unable to update Directory Privacy user password",
				fmt.Sprintf(
					"%v. Terraform will not reapply the previous password because cPanel does not expose enough state to distinguish the attempted password from a concurrent change.",
					sensitiveMutationError(
						err,
						"Directory Privacy user password update",
					),
				),
			)
			return
		}
	}

	apiUser, err := r.verifyUser(
		ctx,
		definition.Directory,
		definition.Username,
	)
	if err != nil {
		detail := err.Error()
		if passwordChanged {
			detail += ". Terraform will not reapply the previous password because cPanel does not expose enough state to attribute the current password safely."
		}
		resp.Diagnostics.AddError(
			"Unable to verify Directory Privacy user update",
			detail,
		)
		return
	}

	applyDirectoryPrivacyUserToResourceModel(&plan, *apiUser)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *directoryPrivacyUserResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DirectoryPrivacyUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"Directory Privacy user preserved",
			fmt.Sprintf(
				"Terraform removed Directory Privacy user %q for directory %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Username.ValueString(),
				state.Directory.ValueString(),
			),
		)
		return
	}

	directory := state.Directory.ValueString()
	username := state.Username.ValueString()
	current, err := r.client.GetUser(ctx, directory, username)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Directory Privacy user",
			err.Error(),
		)
		return
	}
	if current == nil {
		return
	}

	deleteErr := r.client.DeleteUser(ctx, directory, username)
	remaining, readErr := r.client.GetUser(ctx, directory, username)
	if readErr != nil {
		detail := "Could not verify the Directory Privacy user deletion: " +
			readErr.Error()
		if deleteErr != nil {
			detail = fmt.Sprintf(
				"Could not delete Directory Privacy user: %v. %s",
				deleteErr,
				detail,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify Directory Privacy user deletion",
			detail,
		)
		return
	}
	if remaining == nil {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Directory Privacy user",
			deleteErr.Error(),
		)
		return
	}
	resp.Diagnostics.AddError(
		"Unable to verify Directory Privacy user deletion",
		fmt.Sprintf(
			"User %q still exists for directory %q after deletion.",
			username,
			directory,
		),
	)
}

func (r *directoryPrivacyUserResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	directory, username, err := parseDirectoryPrivacyUserImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Directory Privacy user import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("directory"), directory)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("username"), username)...,
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

func (r *directoryPrivacyUserResource) Configure(
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

	client, ok := providerData["directoryprivacy"].(*directoryprivacy.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Directory Privacy Client Type",
			fmt.Sprintf(
				"Expected *directoryprivacy.Client, got: %T.",
				providerData["directoryprivacy"],
			),
		)
		return
	}

	r.client = client
}

func (r *directoryPrivacyUserResource) verifyUser(
	ctx context.Context,
	directory string,
	username string,
) (*directoryprivacy.User, error) {
	apiUser, err := r.client.GetUser(ctx, directory, username)
	if err != nil {
		return nil, fmt.Errorf("read Directory Privacy users after mutation: %w", err)
	}
	if apiUser == nil {
		return nil, fmt.Errorf(
			"user %q was not found for directory %q after mutation",
			username,
			directory,
		)
	}

	return apiUser, nil
}
