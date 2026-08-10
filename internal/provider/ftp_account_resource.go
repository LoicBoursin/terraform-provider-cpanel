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

	"terraform-provider-cpanel/internal/cpanel/ftp"
)

var (
	_ resource.Resource                = &ftpAccountResource{}
	_ resource.ResourceWithConfigure   = &ftpAccountResource{}
	_ resource.ResourceWithImportState = &ftpAccountResource{}
)

func NewFTPAccountResource() resource.Resource {
	return &ftpAccountResource{}
}

type ftpAccountResource struct {
	client *ftp.Client
}

func (r *ftpAccountResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ftp_account"
}

func (r *ftpAccountResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel FTP account, password, home directory, and disk quota.",
		MarkdownDescription: "Manages a cPanel FTP account, password, home directory, and disk quota.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Required:            true,
				Description:         "The complete FTP login in user@domain form.",
				MarkdownDescription: "The complete FTP login in `user@domain` form.",
				Validators:          ftpAccountValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The FTP account password. Configure it together with password_version. Terraform never stores it in plan or state artifacts.",
				MarkdownDescription: "The FTP account password. Configure it together with `password_version`. Terraform never stores it in plan or state artifacts.",
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
			"home_directory": schema.StringAttribute{
				Required:            true,
				Description:         "The FTP account home directory relative to the cPanel account home.",
				MarkdownDescription: "The FTP account home directory relative to the cPanel account home.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 1024),
				},
			},
			"quota_mib": schema.Int64Attribute{
				Required:            true,
				Description:         "The disk quota in MiB. Use 0 for unlimited when the cPanel account permits it.",
				MarkdownDescription: "The disk quota in MiB. Use `0` for unlimited when the cPanel account permits it.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"delete_on_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether deleting the Terraform resource also deletes the FTP account. " +
					"cPanel does not expose an immutable account identity, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the FTP account. Defaults to `false`. " +
					"cPanel does not expose an immutable account identity, so enabling this option accepts that a same-name replacement with the same observable home directory and quota cannot be distinguished from the managed account.",
			},
			"delete_home_directory": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Whether deleting the FTP account also deletes its home directory and contents. This option has no effect unless delete_on_destroy is true.",
				MarkdownDescription: "Whether deleting the FTP account also deletes its home directory and contents. Defaults to `false` and has no effect unless `delete_on_destroy` is `true`.",
			},
		},
	}
}

func (r *ftpAccountResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state FTPAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, _, err := splitFTPAccountUsername(state.Username.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid FTP account username in state", err.Error())
		return
	}

	account, err := r.client.GetAccount(ctx, state.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account", err.Error())
		return
	}
	if account == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account quota", err.Error())
		return
	}
	state.HomeDirectory = types.StringValue(account.RelativeDirectory)
	state.QuotaMiB = types.Int64Value(quotaMiB)
	if state.DeleteOnDestroy.IsNull() || state.DeleteOnDestroy.IsUnknown() {
		state.DeleteOnDestroy = types.BoolValue(false)
	}
	if state.DeleteHomeDirectory.IsNull() || state.DeleteHomeDirectory.IsUnknown() {
		state.DeleteHomeDirectory = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ftpAccountResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan FTPAccountResourceModel
	var config FTPAccountResourceModel
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
			"Missing FTP account password configuration",
			"The password and password_version attributes must both be known when creating an FTP account. They may both be omitted only after importing an existing account.",
		)
		return
	}

	user, domain, err := validateFTPAccountUsername(
		ctx,
		r.client,
		plan.Username.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid FTP account username", err.Error())
		return
	}
	if err := validateFTPHomeDirectory(plan.HomeDirectory.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid FTP account home directory", err.Error())
		return
	}

	existing, err := r.client.GetAccount(ctx, plan.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"FTP account already exists",
			fmt.Sprintf(
				"FTP account %q already exists. Import it instead of replacing its password implicitly.",
				plan.Username.ValueString(),
			),
		)
		return
	}

	if err := r.client.CreateAccount(
		ctx,
		user,
		domain,
		config.Password.ValueString(),
		plan.HomeDirectory.ValueString(),
		plan.QuotaMiB.ValueInt64(),
	); err != nil {
		detail := sensitiveMutationError(
			err,
			"FTP account creation",
		).Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			created, recoveryErr := r.client.GetAccount(
				ctx,
				plan.Username.ValueString(),
			)
			switch {
			case recoveryErr != nil:
				detail += fmt.Sprintf(
					". Terraform could not determine whether the account was created because the follow-up read failed: %v",
					recoveryErr,
				)
			case created != nil:
				detail = fmt.Sprintf(
					"FTP account %q may have been created, but Terraform cannot verify its password after an ambiguous response and refuses to adopt or delete it automatically. Inspect the account in cPanel and import it if it is intended to remain.",
					plan.Username.ValueString(),
				)
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create FTP account",
			detail,
		)
		return
	}

	if err := r.verifyAccount(
		ctx,
		plan.Username.ValueString(),
		plan.HomeDirectory.ValueString(),
		plan.QuotaMiB.ValueInt64(),
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify FTP account",
			fmt.Sprintf(
				"%v. Terraform will not delete the account or its home directory because its password cannot be attributed safely after creation. Inspect the account in cPanel and import it if it is intended to remain.",
				err,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ftpAccountResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan FTPAccountResourceModel
	var state FTPAccountResourceModel
	var config FTPAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := validateFTPAccountUsername(
		ctx,
		r.client,
		plan.Username.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid FTP account username", err.Error())
		return
	}
	if err := validateFTPHomeDirectory(plan.HomeDirectory.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid FTP account home directory", err.Error())
		return
	}
	current, err := r.client.GetAccount(ctx, plan.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"FTP account no longer exists",
			"Refresh the Terraform state before updating the FTP account.",
		)
		return
	}
	currentQuotaMiB, err := current.QuotaMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account quota", err.Error())
		return
	}
	originalObservable := ftpAccountObservable{
		HomeDirectory: state.HomeDirectory.ValueString(),
		QuotaMiB:      state.QuotaMiB.ValueInt64(),
	}
	if current.RelativeDirectory != originalObservable.HomeDirectory ||
		currentQuotaMiB != originalObservable.QuotaMiB {
		resp.Diagnostics.AddError(
			"FTP account changed during update",
			"The remote FTP account home directory or quota no longer matches Terraform state. Refresh and review the drift before retrying.",
		)
		return
	}

	homeChanged := !plan.HomeDirectory.Equal(state.HomeDirectory)
	quotaChanged := !plan.QuotaMiB.Equal(state.QuotaMiB)
	passwordChanged := !plan.PasswordVersion.Equal(state.PasswordVersion)
	if passwordChanged && plan.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown FTP account password version",
			"The password_version attribute must be known when Terraform applies an FTP account password change.",
		)
		return
	}
	if passwordChanged &&
		!plan.PasswordVersion.IsNull() &&
		(config.Password.IsNull() || config.Password.IsUnknown()) {
		resp.Diagnostics.AddError(
			"Missing FTP account password",
			"A known password must be configured when password_version changes to a non-null value.",
		)
		return
	}

	if homeChanged {
		if err := r.client.SetHomeDirectory(
			ctx,
			user,
			domain,
			plan.HomeDirectory.ValueString(),
		); err != nil {
			var rollbackErr error
			if !cPanelMutationErrorIsDeterministic(err) {
				rollbackErr = r.restoreFTPAccountIfCurrentMatches(
					ctx,
					plan.Username.ValueString(),
					user,
					domain,
					originalObservable,
					ftpAccountObservable{
						HomeDirectory: plan.HomeDirectory.ValueString(),
						QuotaMiB:      originalObservable.QuotaMiB,
					},
				)
			}
			resp.Diagnostics.AddError(
				"Unable to update FTP account home directory",
				ftpMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}
	if quotaChanged {
		if err := r.client.SetQuota(ctx, user, domain, plan.QuotaMiB.ValueInt64()); err != nil {
			expectedAfterHome := originalObservable
			if homeChanged {
				expectedAfterHome.HomeDirectory = plan.HomeDirectory.ValueString()
			}
			expectedAfterQuota := expectedAfterHome
			expectedAfterQuota.QuotaMiB = plan.QuotaMiB.ValueInt64()
			expectedStates := []ftpAccountObservable{expectedAfterHome}
			if !cPanelMutationErrorIsDeterministic(err) {
				expectedStates = append(expectedStates, expectedAfterQuota)
			}
			rollbackErr := r.restoreFTPAccountIfCurrentMatches(
				ctx,
				plan.Username.ValueString(),
				user,
				domain,
				originalObservable,
				expectedStates...,
			)
			resp.Diagnostics.AddError(
				"Unable to update FTP account quota",
				ftpMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}
	if passwordChanged && !plan.PasswordVersion.IsNull() {
		if err := r.client.SetPassword(ctx, user, domain, config.Password.ValueString()); err != nil {
			rollbackErr := r.restoreFTPAccountIfCurrentMatches(
				ctx,
				plan.Username.ValueString(),
				user,
				domain,
				originalObservable,
				ftpAccountObservable{
					HomeDirectory: plan.HomeDirectory.ValueString(),
					QuotaMiB:      plan.QuotaMiB.ValueInt64(),
				},
			)
			resp.Diagnostics.AddError(
				"Unable to update FTP account password",
				ftpPasswordMutationErrorDetail(
					sensitiveMutationError(
						err,
						"FTP account password update",
					),
					rollbackErr,
				),
			)
			return
		}
	}

	if err := r.verifyAccount(
		ctx,
		plan.Username.ValueString(),
		plan.HomeDirectory.ValueString(),
		plan.QuotaMiB.ValueInt64(),
	); err != nil {
		rollbackErr := r.restoreFTPAccountIfCurrentMatches(
			ctx,
			plan.Username.ValueString(),
			user,
			domain,
			originalObservable,
			ftpAccountObservable{
				HomeDirectory: plan.HomeDirectory.ValueString(),
				QuotaMiB:      plan.QuotaMiB.ValueInt64(),
			},
		)
		resp.Diagnostics.AddError(
			"Unable to verify FTP account",
			ftpAccountVerificationErrorDetail(
				err,
				rollbackErr,
				passwordChanged,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ftpAccountResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state FTPAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"FTP account preserved",
			fmt.Sprintf(
				"Terraform removed FTP account %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Username.ValueString(),
			),
		)
		return
	}

	user, domain, err := splitFTPAccountUsername(state.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid FTP account username in state", err.Error())
		return
	}

	account, err := r.client.GetAccount(ctx, state.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account", err.Error())
		return
	}
	if account == nil {
		return
	}
	matchesState, err := ftpAccountMatchesState(*account, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify FTP account identity",
			err.Error(),
		)
		return
	}
	if !matchesState {
		resp.Diagnostics.AddError(
			"Refusing to delete changed FTP account",
			fmt.Sprintf(
				"FTP account %q no longer matches the home directory and quota stored in Terraform state. Refresh and review the replacement before retrying.",
				state.Username.ValueString(),
			),
		)
		return
	}

	deleteErr := r.client.DeleteAccount(
		ctx,
		user,
		domain,
		state.DeleteHomeDirectory.ValueBool(),
	)
	remaining, verifyErr := r.client.GetAccount(
		ctx,
		state.Username.ValueString(),
	)
	if verifyErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify FTP account deletion",
			fmt.Sprintf(
				"Delete response: %v. Follow-up read failed: %v",
				deleteErr,
				verifyErr,
			),
		)
		return
	}
	if remaining == nil {
		return
	}
	remainingMatchesState, matchErr := ftpAccountMatchesState(
		*remaining,
		state,
	)
	if matchErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify FTP account deletion",
			matchErr.Error(),
		)
		return
	}
	if !remainingMatchesState {
		resp.Diagnostics.AddError(
			"FTP account replacement preserved",
			fmt.Sprintf(
				"FTP account %q exists after the deletion attempt but no longer matches the account stored in Terraform state. Terraform will not delete the replacement or its home directory.",
				state.Username.ValueString(),
			),
		)
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete FTP account",
			"Could not delete FTP account: "+deleteErr.Error(),
		)
		return
	}
	resp.Diagnostics.AddError(
		"Unable to verify FTP account deletion",
		fmt.Sprintf(
			"FTP account %q still exists after cPanel reported a successful deletion.",
			state.Username.ValueString(),
		),
	)
}

func ftpAccountMatchesState(
	account ftp.Account,
	state FTPAccountResourceModel,
) (bool, error) {
	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		return false, fmt.Errorf("read FTP account quota: %w", err)
	}

	return account.Login == state.Username.ValueString() &&
		account.RelativeDirectory == state.HomeDirectory.ValueString() &&
		quotaMiB == state.QuotaMiB.ValueInt64(), nil
}

func (r *ftpAccountResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("username"), req.ID)...,
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
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("delete_home_directory"), false)...,
	)
}

func (r *ftpAccountResource) Configure(
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

	client, ok := providerData["ftp"].(*ftp.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected FTP Client Type",
			fmt.Sprintf("Expected *ftp.Client, got: %T.", providerData["ftp"]),
		)
		return
	}

	r.client = client
}

func (r *ftpAccountResource) verifyAccount(
	ctx context.Context,
	username string,
	expectedHomeDirectory string,
	expectedQuotaMiB int64,
) error {
	account, err := r.client.GetAccount(ctx, username)
	if err != nil {
		return fmt.Errorf("read FTP account after mutation: %w", err)
	}
	if account == nil {
		return fmt.Errorf("FTP account %q was not found after mutation", username)
	}
	if account.RelativeDirectory != expectedHomeDirectory {
		return fmt.Errorf(
			"FTP account %q has home directory %q; expected %q",
			username,
			account.RelativeDirectory,
			expectedHomeDirectory,
		)
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		return err
	}
	if quotaMiB != expectedQuotaMiB {
		return fmt.Errorf(
			"FTP account %q has quota %d MiB; expected %d MiB",
			username,
			quotaMiB,
			expectedQuotaMiB,
		)
	}

	return nil
}

type ftpAccountObservable struct {
	HomeDirectory string
	QuotaMiB      int64
}

func (r *ftpAccountResource) restoreFTPAccountIfCurrentMatches(
	ctx context.Context,
	username string,
	user string,
	domain string,
	target ftpAccountObservable,
	expectedCurrent ...ftpAccountObservable,
) error {
	account, err := r.client.GetAccount(ctx, username)
	if err != nil {
		return fmt.Errorf("read FTP account before restore: %w", err)
	}
	if account == nil {
		return fmt.Errorf("FTP account disappeared before restore")
	}
	currentQuotaMiB, err := account.QuotaMiB()
	if err != nil {
		return err
	}
	current := ftpAccountObservable{
		HomeDirectory: account.RelativeDirectory,
		QuotaMiB:      currentQuotaMiB,
	}
	if current == target {
		return nil
	}
	matchesTransition := false
	for _, expected := range expectedCurrent {
		if current == expected {
			matchesTransition = true
			break
		}
	}
	if !matchesTransition {
		return fmt.Errorf(
			"refuse to restore the previous FTP account because the current home directory or quota no longer matches the Terraform transition",
		)
	}
	if current.QuotaMiB != target.QuotaMiB {
		if err := r.client.SetQuota(ctx, user, domain, target.QuotaMiB); err != nil {
			return fmt.Errorf("restore previous quota: %w", err)
		}
	}
	if current.HomeDirectory != target.HomeDirectory {
		if err := r.client.SetHomeDirectory(
			ctx,
			user,
			domain,
			target.HomeDirectory,
		); err != nil {
			return fmt.Errorf("restore previous home directory: %w", err)
		}
	}

	return r.verifyAccount(
		ctx,
		username,
		target.HomeDirectory,
		target.QuotaMiB,
	)
}

func ftpMutationErrorDetail(mutationErr, rollbackErr error) string {
	if rollbackErr == nil {
		return mutationErr.Error()
	}

	return fmt.Sprintf(
		"%v. Automatic rollback also failed: %v",
		mutationErr,
		rollbackErr,
	)
}

func ftpPasswordMutationErrorDetail(
	mutationErr error,
	rollbackErr error,
) string {
	detail := mutationErr.Error() +
		". Terraform will not reapply the previous password because cPanel does not expose enough state to attribute the current password safely."
	if rollbackErr != nil {
		detail += fmt.Sprintf(
			" Restoring the previous home directory and quota also failed: %v",
			rollbackErr,
		)
	}

	return detail
}

func ftpAccountVerificationErrorDetail(
	verificationErr error,
	rollbackErr error,
	passwordChanged bool,
) string {
	detail := verificationErr.Error()
	if passwordChanged {
		detail += ". Terraform will not reapply the previous password because cPanel does not expose enough state to attribute the current password safely."
	}
	if rollbackErr != nil {
		detail += fmt.Sprintf(
			" Restoring the previous home directory and quota also failed: %v",
			rollbackErr,
		)
	}

	return detail
}
