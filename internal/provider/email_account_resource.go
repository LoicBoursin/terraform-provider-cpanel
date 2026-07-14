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

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailAccountResource{}
	_ resource.ResourceWithConfigure   = &emailAccountResource{}
	_ resource.ResourceWithImportState = &emailAccountResource{}
)

func NewEmailAccountResource() resource.Resource {
	return &emailAccountResource{}
}

type emailAccountResource struct {
	client *cpanelmail.Client
}

func (r *emailAccountResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_account"
}

func (r *emailAccountResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel email account, password, and disk quota.",
		MarkdownDescription: "Manages a cPanel email account, password, and disk quota.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:            true,
				Description:         "The complete email address on a domain owned by the cPanel account.",
				MarkdownDescription: "The complete email address on a domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The email account password. Configure it together with password_version. Terraform never stores it in plan or state artifacts.",
				MarkdownDescription: "The email account password. Configure it together with `password_version`. Terraform never stores it in plan or state artifacts.",
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
				Description: "Whether deleting the Terraform resource also deletes the email account. " +
					"cPanel does not expose an immutable account identity, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the email account. Defaults to `false`. " +
					"cPanel does not expose an immutable account identity, so enabling this option accepts that a same-address replacement with the same observable quota cannot be distinguished from the managed account.",
			},
		},
	}
}

func (r *emailAccountResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state EmailAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := splitEmailAccountAddress(state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid email account address in state", err.Error())
		return
	}

	account, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account", err.Error())
		return
	}
	if account == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account quota", err.Error())
		return
	}
	state.QuotaMiB = types.Int64Value(quotaMiB)
	if state.DeleteOnDestroy.IsNull() || state.DeleteOnDestroy.IsUnknown() {
		state.DeleteOnDestroy = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailAccountResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan EmailAccountResourceModel
	var config EmailAccountResourceModel
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
			"Missing email account password configuration",
			"The password and password_version attributes must both be known when creating an email account. They may both be omitted only after importing an existing account.",
		)
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		plan.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid email account address", err.Error())
		return
	}

	existing, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Email account already exists",
			fmt.Sprintf(
				"Email account %q already exists. Import it instead of replacing its password implicitly.",
				plan.Email.ValueString(),
			),
		)
		return
	}

	if err := r.client.CreateAccount(
		ctx,
		user,
		domain,
		config.Password.ValueString(),
		plan.QuotaMiB.ValueInt64(),
	); err != nil {
		detail := sensitiveMutationError(
			err,
			"email account creation",
		).Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			created, recoveryErr := r.client.GetAccount(ctx, user, domain)
			switch {
			case recoveryErr != nil:
				detail += fmt.Sprintf(
					". Terraform could not determine whether the account was created because the follow-up read failed: %v",
					recoveryErr,
				)
			case created != nil:
				detail = fmt.Sprintf(
					"Email account %q may have been created, but Terraform cannot verify its password after an ambiguous response and refuses to adopt or delete it automatically. Inspect the account in cPanel and import it if it is intended to remain.",
					plan.Email.ValueString(),
				)
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create email account",
			detail,
		)
		return
	}

	if err := r.verifyAccount(ctx, user, domain, plan.QuotaMiB.ValueInt64()); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify email account",
			fmt.Sprintf(
				"%v. Terraform will not delete the account because its password cannot be attributed safely after creation. Inspect the account in cPanel and import it if it is intended to remain.",
				err,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAccountResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan EmailAccountResourceModel
	var state EmailAccountResourceModel
	var config EmailAccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		plan.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid email account address", err.Error())
		return
	}
	current, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Email account no longer exists",
			"Refresh the Terraform state before updating the email account.",
		)
		return
	}
	currentQuotaMiB, err := current.QuotaMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account quota", err.Error())
		return
	}
	if currentQuotaMiB != state.QuotaMiB.ValueInt64() {
		resp.Diagnostics.AddError(
			"Email account changed during update",
			"The remote email account quota no longer matches Terraform state. Refresh and review the drift before retrying.",
		)
		return
	}

	quotaChanged := !plan.QuotaMiB.Equal(state.QuotaMiB)
	passwordChanged := !plan.PasswordVersion.Equal(state.PasswordVersion)
	if passwordChanged && plan.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown email account password version",
			"The password_version attribute must be known when Terraform applies an email account password change.",
		)
		return
	}
	if passwordChanged &&
		!plan.PasswordVersion.IsNull() &&
		(config.Password.IsNull() || config.Password.IsUnknown()) {
		resp.Diagnostics.AddError(
			"Missing email account password",
			"A known password must be configured when password_version changes to a non-null value.",
		)
		return
	}

	if quotaChanged {
		if err := r.client.SetQuota(ctx, user, domain, plan.QuotaMiB.ValueInt64()); err != nil {
			var rollbackErr error
			if !cPanelMutationErrorIsDeterministic(err) {
				rollbackErr = r.restoreEmailQuotaIfCurrentMatches(
					ctx,
					user,
					domain,
					plan.QuotaMiB.ValueInt64(),
					state.QuotaMiB.ValueInt64(),
				)
			}
			resp.Diagnostics.AddError(
				"Unable to update email account quota",
				emailMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	if passwordChanged && !plan.PasswordVersion.IsNull() {
		if err := r.client.SetPassword(ctx, user, domain, config.Password.ValueString()); err != nil {
			var rollbackErr error
			if quotaChanged {
				rollbackErr = r.restoreEmailQuotaIfCurrentMatches(
					ctx,
					user,
					domain,
					plan.QuotaMiB.ValueInt64(),
					state.QuotaMiB.ValueInt64(),
				)
			}
			resp.Diagnostics.AddError(
				"Unable to update email account password",
				emailPasswordMutationErrorDetail(
					sensitiveMutationError(
						err,
						"email account password update",
					),
					rollbackErr,
				),
			)
			return
		}
	}

	if err := r.verifyAccount(ctx, user, domain, plan.QuotaMiB.ValueInt64()); err != nil {
		var rollbackErr error
		if quotaChanged {
			rollbackErr = r.restoreEmailQuotaIfCurrentMatches(
				ctx,
				user,
				domain,
				plan.QuotaMiB.ValueInt64(),
				state.QuotaMiB.ValueInt64(),
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify email account",
			emailAccountVerificationErrorDetail(
				err,
				rollbackErr,
				passwordChanged,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAccountResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state EmailAccountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"Email account preserved",
			fmt.Sprintf(
				"Terraform removed email account %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Email.ValueString(),
			),
		)
		return
	}

	user, domain, err := splitEmailAccountAddress(state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid email account address in state", err.Error())
		return
	}

	account, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account", err.Error())
		return
	}
	if account == nil {
		return
	}
	matchesState, err := emailAccountMatchesState(*account, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify email account identity",
			err.Error(),
		)
		return
	}
	if !matchesState {
		resp.Diagnostics.AddError(
			"Refusing to delete changed email account",
			fmt.Sprintf(
				"Email account %q no longer matches the quota stored in Terraform state. Refresh and review the replacement before retrying.",
				state.Email.ValueString(),
			),
		)
		return
	}

	deleteErr := r.client.DeleteAccount(ctx, user, domain)
	remaining, verifyErr := r.client.GetAccount(ctx, user, domain)
	if verifyErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify email account deletion",
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
	remainingMatchesState, matchErr := emailAccountMatchesState(
		*remaining,
		state,
	)
	if matchErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify email account deletion",
			matchErr.Error(),
		)
		return
	}
	if !remainingMatchesState {
		resp.Diagnostics.AddError(
			"Email account replacement preserved",
			fmt.Sprintf(
				"Email account %q exists after the deletion attempt but no longer matches the account stored in Terraform state. Terraform will not delete the replacement.",
				state.Email.ValueString(),
			),
		)
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete email account",
			"Could not delete email account: "+deleteErr.Error(),
		)
		return
	}
	resp.Diagnostics.AddError(
		"Unable to verify email account deletion",
		fmt.Sprintf(
			"Email account %q still exists after cPanel reported a successful deletion.",
			state.Email.ValueString(),
		),
	)
}

func emailAccountMatchesState(
	account cpanelmail.Account,
	state EmailAccountResourceModel,
) (bool, error) {
	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		return false, fmt.Errorf("read email account quota: %w", err)
	}

	return account.Email == state.Email.ValueString() &&
		quotaMiB == state.QuotaMiB.ValueInt64(), nil
}

func (r *emailAccountResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("email"), req.ID)...,
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

func (r *emailAccountResource) Configure(
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

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf("Expected *email.Client, got: %T.", providerData["email"]),
		)
		return
	}

	r.client = client
}

func (r *emailAccountResource) verifyAccount(
	ctx context.Context,
	user string,
	domain string,
	expectedQuotaMiB int64,
) error {
	account, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		return fmt.Errorf("read email account after mutation: %w", err)
	}
	if account == nil {
		return fmt.Errorf("email account %q was not found after mutation", user+"@"+domain)
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		return err
	}
	if quotaMiB != expectedQuotaMiB {
		return fmt.Errorf(
			"email account %q has quota %d MiB; expected %d MiB",
			user+"@"+domain,
			quotaMiB,
			expectedQuotaMiB,
		)
	}

	return nil
}

func (r *emailAccountResource) restoreEmailQuotaIfCurrentMatches(
	ctx context.Context,
	user string,
	domain string,
	expectedCurrentMiB int64,
	targetMiB int64,
) error {
	account, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		return fmt.Errorf("read email account before quota restore: %w", err)
	}
	if account == nil {
		return fmt.Errorf("email account disappeared before quota restore")
	}
	currentMiB, err := account.QuotaMiB()
	if err != nil {
		return err
	}
	if currentMiB == targetMiB {
		return nil
	}
	if currentMiB != expectedCurrentMiB {
		return fmt.Errorf(
			"refuse to restore previous quota because the current value no longer matches the Terraform transition",
		)
	}
	if err := r.client.SetQuota(ctx, user, domain, targetMiB); err != nil {
		return fmt.Errorf("restore previous quota: %w", err)
	}

	return r.verifyAccount(ctx, user, domain, targetMiB)
}

func emailMutationErrorDetail(mutationErr, rollbackErr error) string {
	if rollbackErr == nil {
		return mutationErr.Error()
	}

	return fmt.Sprintf(
		"%v. Automatic rollback also failed: %v",
		mutationErr,
		rollbackErr,
	)
}

func emailPasswordMutationErrorDetail(
	mutationErr error,
	rollbackErr error,
) string {
	detail := mutationErr.Error() +
		". Terraform will not reapply the previous password because cPanel does not expose enough state to attribute the current password safely."
	if rollbackErr != nil {
		detail += fmt.Sprintf(
			" Restoring the previous quota also failed: %v",
			rollbackErr,
		)
	}

	return detail
}

func emailAccountVerificationErrorDetail(
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
			" Restoring the previous quota also failed: %v",
			rollbackErr,
		)
	}

	return detail
}
