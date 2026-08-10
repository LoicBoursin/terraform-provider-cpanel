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
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailMailingListResource{}
	_ resource.ResourceWithConfigure   = &emailMailingListResource{}
	_ resource.ResourceWithImportState = &emailMailingListResource{}
)

func NewEmailMailingListResource() resource.Resource {
	return &emailMailingListResource{}
}

type emailMailingListClient interface {
	ChangeMailingListPassword(context.Context, string, string) error
	CreateMailingList(context.Context, string, string, string, bool) error
	DeleteMailingList(context.Context, string) error
	GetMailingList(
		context.Context,
		string,
		string,
	) (*cpanelmail.MailingList, error)
	ListMailDomains(context.Context) ([]string, error)
	LockMailingList(string) func()
	SetMailingListPrivacyOptions(
		context.Context,
		string,
		cpanelmail.MailingListPrivacyOptions,
	) error
}

type emailMailingListResource struct {
	client emailMailingListClient
}

func (r *emailMailingListResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_mailing_list"
}

func (r *emailMailingListResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one cPanel Mailman mailing list.",
		MarkdownDescription: "Manages one cPanel Mailman mailing list. Password and privacy changes use cPanel's in-place update APIs, preserving Mailman subscribers, archives, and settings that are outside Terraform state.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Required:            true,
				Description:         "The complete mailing list address on a mail domain owned by the cPanel account.",
				MarkdownDescription: "The complete mailing list address on a mail domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The Mailman administrator password. Configure it together with password_version. Terraform never stores it in plan or state artifacts.",
				MarkdownDescription: "The Mailman administrator password. Configure it together with `password_version`. Terraform never stores it in plan or state artifacts.",
				Validators: append(
					emailMailingListPasswordValidators(),
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
				Description: "Whether deleting the Terraform resource also deletes the Mailman list. " +
					"Mailman subscribers, archives, and settings are outside Terraform state, so this destructive behavior requires explicit opt-in.",
				MarkdownDescription: "Whether deleting the Terraform resource also deletes the Mailman list. Defaults to `false`. " +
					"Mailman subscribers, archives, and settings are outside Terraform state, so enabling this option accepts their deletion.",
			},
			"advertised": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether the Mailman directory page displays the list.",
				MarkdownDescription: "Whether the Mailman directory page displays the list. Changes are applied in place.",
			},
			"archive_private": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether the mailing list archive is private.",
				MarkdownDescription: "Whether the mailing list archive is private. Changes are applied in place.",
			},
			"subscribe_policy": schema.Int64Attribute{
				Required:            true,
				Description:         "The Mailman subscription policy code.",
				MarkdownDescription: "The Mailman subscription policy code: `1` allows confirmed subscriptions, while `2` and `3` require administrator approval. Changes are applied in place.",
				Validators: []validator.Int64{
					int64validator.OneOf(1, 2, 3),
				},
			},
			"private": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether every Mailman privacy setting is private.",
				MarkdownDescription: "Whether every Mailman privacy setting is private. The value is `true` only when the list is not advertised, its archive is private, and its subscription policy requires administrator approval.",
			},
			"list_id": schema.StringAttribute{
				Computed:            true,
				Description:         "The internal Mailman list identifier returned by cPanel.",
				MarkdownDescription: "The internal Mailman list identifier returned by cPanel.",
			},
			"administrators": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The mailing list administrator addresses reported by cPanel.",
				MarkdownDescription: "The mailing list administrator addresses reported by cPanel.",
			},
			"human_disk_used": schema.StringAttribute{
				Computed:            true,
				Description:         "The human-readable mailing list disk usage reported by cPanel.",
				MarkdownDescription: "The human-readable mailing list disk usage reported by cPanel.",
			},
		},
	}
}

func (r *emailMailingListResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state EmailMailingListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := validateEmailMailingListAddress(
		state.Address.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list address in state", err.Error())
		return
	}

	unlock := r.client.LockMailingList(state.Address.ValueString())
	defer unlock()

	mailingList, err := r.client.GetMailingList(
		ctx,
		state.Address.ValueString(),
		domain,
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mailing list", err.Error())
		return
	}
	if mailingList == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(
		applyMailingListToResourceModel(ctx, &state, *mailingList)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailMailingListResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan EmailMailingListResourceModel
	var config EmailMailingListResourceModel
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
			"Missing mailing list password configuration",
			"The password and password_version attributes must both be known when creating a mailing list. They may both be omitted only after importing an existing list.",
		)
		return
	}

	desiredPrivacy, err := mailingListPrivacyFromResourceModel(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list privacy settings", err.Error())
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		plan.Address.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list address", err.Error())
		return
	}

	unlock := r.client.LockMailingList(plan.Address.ValueString())
	defer unlock()

	existing, err := r.client.GetMailingList(
		ctx,
		plan.Address.ValueString(),
		domain,
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mailing list", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Mailing list already exists",
			fmt.Sprintf(
				"Mailing list %q already exists. Import it instead of taking ownership implicitly.",
				plan.Address.ValueString(),
			),
		)
		return
	}

	created, rollbackAllowed, err := r.createAndVerify(
		ctx,
		plan.Address.ValueString(),
		user,
		domain,
		config.Password.ValueString(),
	)
	if err != nil {
		detail := err.Error()
		var rollbackErr error
		if rollbackAllowed && created != nil {
			rollbackErr = r.rollbackCreatedMailingList(
				ctx,
				plan.Address.ValueString(),
				domain,
				created.ID,
				mailingListPrivacy(*created),
			)
			detail = emailMutationErrorDetail(err, rollbackErr)
		}
		resp.Diagnostics.AddError(
			"Unable to create mailing list",
			detail,
		)
		return
	}

	if !mailingListPrivacyEqual(mailingListPrivacy(*created), desiredPrivacy) {
		configured, configureErr := r.setPrivacyAndVerify(
			ctx,
			plan.Address.ValueString(),
			domain,
			created.ID,
			desiredPrivacy,
		)
		if configureErr == nil {
			created = configured
		} else {
			rollbackErr := r.rollbackCreatedMailingList(
				ctx,
				plan.Address.ValueString(),
				domain,
				created.ID,
				mailingListPrivacy(*created),
				desiredPrivacy,
			)
			resp.Diagnostics.AddError(
				"Unable to configure mailing list privacy",
				emailMutationErrorDetail(configureErr, rollbackErr),
			)
			return
		}
	}

	resp.Diagnostics.Append(
		applyMailingListToResourceModel(ctx, &plan, *created)...,
	)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.rollbackCreatedMailingList(
			ctx,
			plan.Address.ValueString(),
			domain,
			created.ID,
			desiredPrivacy,
		)
		resp.Diagnostics.AddError(
			"Unable to store mailing list state",
			emailMutationErrorDetail(
				errors.New("convert mailing list state"),
				rollbackErr,
			),
		)
		return
	}

	stateDiagnostics := resp.State.Set(ctx, &plan)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreatedMailingList(
			ctx,
			plan.Address.ValueString(),
			domain,
			created.ID,
			desiredPrivacy,
		)
		resp.Diagnostics.Append(stateDiagnostics...)
		resp.Diagnostics.AddError(
			"Unable to store mailing list state",
			emailMutationErrorDetail(
				errors.New("write Terraform state"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(stateDiagnostics...)
}

func (r *emailMailingListResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan EmailMailingListResourceModel
	var state EmailMailingListResourceModel
	var config EmailMailingListResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	passwordChanged := !plan.PasswordVersion.Equal(state.PasswordVersion)
	if passwordChanged && plan.PasswordVersion.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unknown mailing list password version",
			"The password_version attribute must be known when Terraform applies a mailing list password change.",
		)
		return
	}
	if passwordChanged &&
		!plan.PasswordVersion.IsNull() &&
		(config.Password.IsNull() || config.Password.IsUnknown()) {
		resp.Diagnostics.AddError(
			"Missing mailing list password",
			"A known password must be configured when password_version changes to a non-null value.",
		)
		return
	}

	desiredPrivacy, err := mailingListPrivacyFromResourceModel(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list privacy settings", err.Error())
		return
	}

	_, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		plan.Address.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list address", err.Error())
		return
	}

	unlock := r.client.LockMailingList(plan.Address.ValueString())
	defer unlock()

	current, err := r.client.GetMailingList(
		ctx,
		plan.Address.ValueString(),
		domain,
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mailing list", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Mailing list no longer exists",
			"Refresh the Terraform state before retrying so Terraform can recreate the missing mailing list.",
		)
		return
	}
	if err := verifyMailingListIdentity(state, *current); err != nil {
		resp.Diagnostics.AddError("Mailing list identity changed", err.Error())
		return
	}

	statePrivacy, err := mailingListPrivacyFromResourceModel(state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid mailing list privacy state",
			err.Error(),
		)
		return
	}
	previousPrivacy := mailingListPrivacy(*current)
	if !mailingListPrivacyEqual(previousPrivacy, statePrivacy) {
		resp.Diagnostics.AddError(
			"Mailing list privacy changed during update",
			"The remote mailing list privacy no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}
	privacyChanged := !mailingListPrivacyEqual(
		previousPrivacy,
		desiredPrivacy,
	)
	updated := current
	if privacyChanged {
		updated, err = r.setPrivacyAndVerify(
			ctx,
			plan.Address.ValueString(),
			domain,
			current.ID,
			desiredPrivacy,
		)
		if err != nil {
			rollbackErr := r.rollbackUpdatedPrivacy(
				ctx,
				plan.Address.ValueString(),
				domain,
				current.ID,
				previousPrivacy,
				desiredPrivacy,
			)
			resp.Diagnostics.AddError(
				"Unable to update mailing list privacy",
				emailMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	if passwordChanged && !plan.PasswordVersion.IsNull() {
		if err := r.client.ChangeMailingListPassword(
			ctx,
			plan.Address.ValueString(),
			config.Password.ValueString(),
		); err != nil {
			var rollbackErr error
			if privacyChanged {
				rollbackErr = r.rollbackUpdatedPrivacy(
					ctx,
					plan.Address.ValueString(),
					domain,
					current.ID,
					previousPrivacy,
					desiredPrivacy,
				)
			}
			resp.Diagnostics.AddError(
				"Unable to update mailing list password",
				mailingListPasswordMutationErrorDetail(
					mailingListSensitiveMutationError(
						err,
						"password update",
					),
					rollbackErr,
					privacyChanged,
				),
			)
			return
		}
	}

	resp.Diagnostics.Append(
		applyMailingListToResourceModel(ctx, &plan, *updated)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	stateDiagnostics := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Unable to store updated mailing list state",
			"cPanel accepted the mailing list update, but Terraform could not persist the refreshed state. Refresh the resource before retrying.",
		)
	}
}

func (r *emailMailingListResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state EmailMailingListResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeleteOnDestroy.ValueBool() {
		resp.Diagnostics.AddWarning(
			"Mailing list preserved",
			fmt.Sprintf(
				"Terraform removed mailing list %q from state without deleting it from cPanel because delete_on_destroy is false.",
				state.Address.ValueString(),
			),
		)
		return
	}

	address := state.Address.ValueString()
	domain, err := validateEmailMailingListAddress(address)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list address in state", err.Error())
		return
	}

	unlock := r.client.LockMailingList(address)
	defer unlock()

	existing, err := r.client.GetMailingList(ctx, address, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mailing list", err.Error())
		return
	}
	if existing == nil {
		return
	}
	if err := verifyMailingListIdentity(state, *existing); err != nil {
		resp.Diagnostics.AddError(
			"Mailing list identity changed",
			err.Error(),
		)
		return
	}

	mutationErr := r.client.DeleteMailingList(ctx, address)
	verificationErr := r.verifyMailingListAbsent(ctx, address, domain)
	if verificationErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify mailing list deletion",
			errors.Join(mutationErr, verificationErr).Error(),
		)
	}
}

func (r *emailMailingListResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if _, err := validateEmailMailingListAddress(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid mailing list import identifier", err.Error())
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("address"), req.ID)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("delete_on_destroy"), false)...,
	)
}

func (r *emailMailingListResource) Configure(
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

func (r *emailMailingListResource) createAndVerify(
	ctx context.Context,
	address string,
	user string,
	domain string,
	password string,
) (*cpanelmail.MailingList, bool, error) {
	mutationErr := r.client.CreateMailingList(
		ctx,
		user,
		domain,
		password,
		true,
	)
	safeMutationErr := mailingListSensitiveMutationError(
		mutationErr,
		"creation",
	)
	rollbackAllowed := mutationErr == nil

	actual, readErr := r.client.GetMailingList(ctx, address, domain)
	if readErr != nil {
		return nil, rollbackAllowed, errors.Join(
			safeMutationErr,
			fmt.Errorf("read mailing list after creation: %w", readErr),
		)
	}
	if actual == nil {
		if mutationErr != nil {
			return nil, false, safeMutationErr
		}

		return nil, true, fmt.Errorf(
			"mailing list %q was not found after creation",
			address,
		)
	}
	if mutationErr != nil {
		if mailingListMutationErrorIsDeterministic(mutationErr) {
			return actual, false, errors.Join(
				safeMutationErr,
				fmt.Errorf(
					"refusing to adopt mailing list %q after cPanel rejected the creation request; import the address if the list is intended to remain",
					address,
				),
			)
		}

		return actual, false, errors.Join(
			safeMutationErr,
			fmt.Errorf(
				"cPanel may have created mailing list %q despite the failed response; Terraform will not adopt or delete it automatically, so inspect it and import the address if it is the intended list",
				address,
			),
		)
	}
	if !actual.Private {
		return actual, true, fmt.Errorf(
			"mailing list %q was not created with private initial settings",
			address,
		)
	}

	return actual, true, nil
}

func (r *emailMailingListResource) setPrivacyAndVerify(
	ctx context.Context,
	address string,
	domain string,
	expectedID string,
	desired cpanelmail.MailingListPrivacyOptions,
) (*cpanelmail.MailingList, error) {
	mutationErr := r.client.SetMailingListPrivacyOptions(
		ctx,
		address,
		desired,
	)
	actual, readErr := r.client.GetMailingList(ctx, address, domain)
	if readErr != nil {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf("read mailing list after privacy update: %w", readErr),
		)
	}
	if actual == nil {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf(
				"mailing list %q was not found after privacy update",
				address,
			),
		)
	}
	if actual.ID != expectedID {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf(
				"mailing list %q returned internal identifier %q instead of %q after privacy update",
				address,
				actual.ID,
				expectedID,
			),
		)
	}
	if !mailingListPrivacyEqual(mailingListPrivacy(*actual), desired) {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf(
				"mailing list %q returned unexpected privacy settings after update",
				address,
			),
		)
	}
	if mailingListMutationErrorIsDeterministic(mutationErr) {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf(
				"refusing to accept mailing list %q privacy after cPanel rejected the update request",
				address,
			),
		)
	}

	return actual, nil
}

func verifyMailingListIdentity(
	state EmailMailingListResourceModel,
	actual cpanelmail.MailingList,
) error {
	address := state.Address.ValueString()
	if actual.Address != address {
		return fmt.Errorf(
			"refusing to mutate mailing list %q because cPanel returned address %q",
			address,
			actual.Address,
		)
	}
	if !state.ListID.IsNull() &&
		!state.ListID.IsUnknown() &&
		state.ListID.ValueString() != actual.ID {
		return fmt.Errorf(
			"refusing to mutate mailing list %q because cPanel returned internal identifier %q instead of state identifier %q",
			address,
			actual.ID,
			state.ListID.ValueString(),
		)
	}

	return nil
}

func (r *emailMailingListResource) verifyMailingListAbsent(
	ctx context.Context,
	address string,
	domain string,
) error {
	mailingList, err := r.client.GetMailingList(ctx, address, domain)
	if err != nil {
		return fmt.Errorf("read mailing list after deletion: %w", err)
	}
	if mailingList != nil {
		return fmt.Errorf("mailing list %q still exists after deletion", address)
	}

	return nil
}

func (r *emailMailingListResource) rollbackCreatedMailingList(
	ctx context.Context,
	address string,
	domain string,
	expectedID string,
	allowedPrivacy ...cpanelmail.MailingListPrivacyOptions,
) error {
	mailingList, err := r.client.GetMailingList(ctx, address, domain)
	if err != nil {
		return fmt.Errorf("inspect mailing list before rollback: %w", err)
	}
	if mailingList == nil {
		return nil
	}
	if mailingList.ID != expectedID {
		return fmt.Errorf(
			"refusing rollback deletion because mailing list %q returned internal identifier %q instead of %q",
			address,
			mailingList.ID,
			expectedID,
		)
	}

	actualPrivacy := mailingListPrivacy(*mailingList)
	privacyMatches := false
	for _, allowed := range allowedPrivacy {
		if mailingListPrivacyEqual(actualPrivacy, allowed) {
			privacyMatches = true
			break
		}
	}
	if !privacyMatches {
		return fmt.Errorf(
			"refusing rollback deletion because mailing list %q privacy no longer matches the attempted creation",
			address,
		)
	}

	mutationErr := r.client.DeleteMailingList(ctx, address)
	verificationErr := r.verifyMailingListAbsent(ctx, address, domain)
	if verificationErr == nil {
		return nil
	}
	if mutationErr == nil {
		return verificationErr
	}

	return errors.Join(
		fmt.Errorf("delete mailing list during rollback: %w", mutationErr),
		verificationErr,
	)
}

func (r *emailMailingListResource) rollbackUpdatedPrivacy(
	ctx context.Context,
	address string,
	domain string,
	expectedID string,
	previous cpanelmail.MailingListPrivacyOptions,
	attempted cpanelmail.MailingListPrivacyOptions,
) error {
	actual, err := r.client.GetMailingList(ctx, address, domain)
	if err != nil {
		return fmt.Errorf("read mailing list before privacy rollback: %w", err)
	}
	if actual == nil {
		return fmt.Errorf(
			"mailing list %q no longer exists during privacy rollback",
			address,
		)
	}
	if actual.ID != expectedID {
		return fmt.Errorf(
			"refusing privacy rollback because mailing list %q returned internal identifier %q instead of %q",
			address,
			actual.ID,
			expectedID,
		)
	}

	actualPrivacy := mailingListPrivacy(*actual)
	if mailingListPrivacyEqual(actualPrivacy, previous) {
		return nil
	}
	if !mailingListPrivacyEqual(actualPrivacy, attempted) {
		return fmt.Errorf(
			"refusing privacy rollback because mailing list %q no longer matches the attempted update",
			address,
		)
	}

	mutationErr := r.client.SetMailingListPrivacyOptions(
		ctx,
		address,
		previous,
	)
	restored, readErr := r.client.GetMailingList(ctx, address, domain)
	if readErr != nil {
		return errors.Join(
			mutationErr,
			fmt.Errorf("read mailing list after privacy rollback: %w", readErr),
		)
	}
	if restored == nil {
		return errors.Join(
			mutationErr,
			fmt.Errorf(
				"mailing list %q no longer exists after privacy rollback",
				address,
			),
		)
	}
	if restored.ID != expectedID ||
		!mailingListPrivacyEqual(mailingListPrivacy(*restored), previous) {
		return errors.Join(
			mutationErr,
			fmt.Errorf(
				"mailing list %q did not return to its previous privacy settings",
				address,
			),
		)
	}

	return nil
}

func mailingListPrivacyFromResourceModel(
	model EmailMailingListResourceModel,
) (cpanelmail.MailingListPrivacyOptions, error) {
	if model.Advertised.IsNull() ||
		model.Advertised.IsUnknown() ||
		model.ArchivePrivate.IsNull() ||
		model.ArchivePrivate.IsUnknown() ||
		model.SubscribePolicy.IsNull() ||
		model.SubscribePolicy.IsUnknown() {
		return cpanelmail.MailingListPrivacyOptions{}, fmt.Errorf(
			"advertised, archive_private, and subscribe_policy must be known",
		)
	}

	options := cpanelmail.MailingListPrivacyOptions{
		Advertised:      model.Advertised.ValueBool(),
		ArchivePrivate:  model.ArchivePrivate.ValueBool(),
		SubscribePolicy: model.SubscribePolicy.ValueInt64(),
	}
	if options.SubscribePolicy < 1 || options.SubscribePolicy > 3 {
		return cpanelmail.MailingListPrivacyOptions{}, fmt.Errorf(
			"subscribe_policy must be 1, 2, or 3",
		)
	}

	return options, nil
}

func mailingListPrivacy(
	mailingList cpanelmail.MailingList,
) cpanelmail.MailingListPrivacyOptions {
	return cpanelmail.MailingListPrivacyOptions{
		Advertised:      mailingList.Advertised,
		ArchivePrivate:  mailingList.ArchivePrivate,
		SubscribePolicy: mailingList.SubscribePolicy,
	}
}

func mailingListPrivacyEqual(
	left cpanelmail.MailingListPrivacyOptions,
	right cpanelmail.MailingListPrivacyOptions,
) bool {
	return left == right
}

func mailingListMutationErrorIsDeterministic(err error) bool {
	if err == nil {
		return false
	}

	var apiError *cpanelapi.APIError
	if errors.As(err, &apiError) {
		return true
	}

	var httpError *cpanelapi.HTTPError

	return errors.As(err, &httpError) &&
		httpError.StatusCode >= 400 &&
		httpError.StatusCode < 500
}

func mailingListSensitiveMutationError(
	err error,
	operation string,
) error {
	if err == nil {
		return nil
	}

	var apiError *cpanelapi.APIError
	if errors.As(err, &apiError) {
		return fmt.Errorf(
			"cPanel rejected the mailing list %s request",
			operation,
		)
	}

	var httpError *cpanelapi.HTTPError
	if errors.As(err, &httpError) {
		return fmt.Errorf(
			"cPanel returned HTTP status %d for the mailing list %s request",
			httpError.StatusCode,
			operation,
		)
	}

	return fmt.Errorf(
		"the cPanel mailing list %s request failed or returned an ambiguous response",
		operation,
	)
}

func mailingListPasswordMutationErrorDetail(
	mutationErr error,
	rollbackErr error,
	privacyChanged bool,
) string {
	detail := fmt.Sprintf(
		"%v. cPanel may have applied the password change; retrying the same configured password is safe.",
		mutationErr,
	)
	if !privacyChanged {
		return detail
	}
	if rollbackErr == nil {
		return detail + " The mailing list privacy settings were restored."
	}

	return fmt.Sprintf(
		"%s Automatic privacy rollback also failed: %v",
		detail,
		rollbackErr,
	)
}
