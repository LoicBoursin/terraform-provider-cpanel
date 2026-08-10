package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailAccountSuspensionResource{}
	_ resource.ResourceWithConfigure   = &emailAccountSuspensionResource{}
	_ resource.ResourceWithImportState = &emailAccountSuspensionResource{}
)

func NewEmailAccountSuspensionResource() resource.Resource {
	return &emailAccountSuspensionResource{}
}

type emailAccountSuspensionClient interface {
	emailMailDomainClient
	GetAccount(
		context.Context,
		string,
		string,
	) (*cpanelmail.Account, error)
	SetLoginSuspended(context.Context, string, bool) error
	SetIncomingSuspended(context.Context, string, bool) error
	SetOutgoingSuspended(context.Context, string, bool) error
}

type emailAccountSuspensionResource struct {
	client emailAccountSuspensionClient
}

func (r *emailAccountSuspensionResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_account_suspension"
}

func (r *emailAccountSuspensionResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages login, incoming-mail, and outgoing-mail suspension for an existing cPanel email account.",
		MarkdownDescription: "Manages login, incoming-mail, and outgoing-mail suspension for an existing cPanel email account. Removing the resource unsuspends all three managed restrictions without deleting the mailbox or releasing held mail.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:            true,
				Description:         "The complete existing email account address.",
				MarkdownDescription: "The complete existing email account address.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"login_suspended": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether cPanel must suspend mailbox login.",
				MarkdownDescription: "Whether cPanel must suspend mailbox login.",
			},
			"incoming_suspended": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether cPanel must reject incoming mail for the account.",
				MarkdownDescription: "Whether cPanel must reject incoming mail for the account.",
			},
			"outgoing_suspended": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether cPanel must reject outgoing mail for the account.",
				MarkdownDescription: "Whether cPanel must reject outgoing mail for the account.",
			},
			"outgoing_held": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether outgoing mail is held in the Exim queue. This provider reports but does not manage held mail.",
				MarkdownDescription: "Whether outgoing mail is held in the Exim queue. This provider reports but does not manage held mail.",
			},
			"has_suspended": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports any suspension or outgoing-mail hold for the account.",
				MarkdownDescription: "Whether cPanel reports any suspension or outgoing-mail hold for the account.",
			},
		},
	}
}

func (r *emailAccountSuspensionResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state EmailAccountSuspensionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := r.getAccount(ctx, state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read email account suspension",
			err.Error(),
		)
		return
	}
	if account == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	if err := applyEmailAccountSuspensionsToResourceModel(
		&state,
		*account,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode email account suspension",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailAccountSuspensionResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan EmailAccountSuspensionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := emailAccountSuspensionDefinitionFromResourceModel(plan)
	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		definition.Email,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid email account address",
			err.Error(),
		)
		return
	}

	current, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read email account suspension",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Email account not found",
			fmt.Sprintf(
				"Email account %q must exist before Terraform can manage its suspension state.",
				definition.Email,
			),
		)
		return
	}

	updated, err := r.transition(
		ctx,
		*current,
		definition.Statuses,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to configure email account suspension",
			err.Error(),
		)
		return
	}

	if err := applyEmailAccountSuspensionsToResourceModel(
		&plan,
		*updated,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode email account suspension",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAccountSuspensionResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan EmailAccountSuspensionResourceModel
	var state EmailAccountSuspensionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := emailAccountSuspensionDefinitionFromResourceModel(plan)
	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		definition.Email,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid email account address",
			err.Error(),
		)
		return
	}

	current, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read email account suspension",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Email account no longer exists",
			"Refresh the Terraform state before updating the suspension state.",
		)
		return
	}
	currentStatuses, err := current.Suspensions()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode email account suspension",
			err.Error(),
		)
		return
	}
	stateDefinition := emailAccountSuspensionDefinitionFromResourceModel(state)
	if !managedEmailAccountSuspensionsEqual(
		currentStatuses,
		stateDefinition.Statuses,
	) {
		resp.Diagnostics.AddError(
			"Email account suspension changed during update",
			"The remote email account suspension no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}

	updated, err := r.transition(
		ctx,
		*current,
		definition.Statuses,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update email account suspension",
			err.Error(),
		)
		return
	}

	if err := applyEmailAccountSuspensionsToResourceModel(
		&plan,
		*updated,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode email account suspension",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAccountSuspensionResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state EmailAccountSuspensionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	account, err := r.getAccount(ctx, state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read email account suspension",
			err.Error(),
		)
		return
	}
	if account == nil {
		return
	}

	expected := emailAccountSuspensionDefinitionFromResourceModel(state)
	current, err := account.Suspensions()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode email account suspension",
			err.Error(),
		)
		return
	}
	if !managedEmailAccountSuspensionsEqual(current, expected.Statuses) {
		resp.Diagnostics.AddError(
			"Unable to reset email account suspension",
			"The remote email account suspension no longer matches Terraform state, so the provider refuses to unsuspend it. Refresh and review the drift before retrying.",
		)
		return
	}

	if _, err := r.transition(
		ctx,
		*account,
		cpanelmail.AccountSuspensions{},
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to reset email account suspension",
			err.Error(),
		)
	}
}

func (r *emailAccountSuspensionResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if _, _, err := splitEmailAccountAddress(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid email account suspension import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("email"), req.ID)...,
	)
}

func (r *emailAccountSuspensionResource) Configure(
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

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf(
				"Expected *email.Client, got: %T.",
				providerData["email"],
			),
		)
		return
	}

	r.client = client
}

func (r *emailAccountSuspensionResource) transition(
	ctx context.Context,
	original cpanelmail.Account,
	target cpanelmail.AccountSuspensions,
) (*cpanelmail.Account, error) {
	current, err := original.Suspensions()
	if err != nil {
		return nil, err
	}
	rollbackCandidates, err := r.applyChanges(
		ctx,
		original.Email,
		current,
		target,
	)
	if err != nil {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			original.Email,
			current,
			rollbackCandidates...,
		)
		return nil, errors.New(emailMutationErrorDetail(err, rollbackErr))
	}

	updated, err := r.verify(ctx, original.Email, target)
	if err != nil {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			original.Email,
			current,
			target,
		)
		return nil, errors.New(emailMutationErrorDetail(err, rollbackErr))
	}

	return updated, nil
}

func (r *emailAccountSuspensionResource) applyChanges(
	ctx context.Context,
	address string,
	current cpanelmail.AccountSuspensions,
	target cpanelmail.AccountSuspensions,
) ([]cpanelmail.AccountSuspensions, error) {
	if current.Login != target.Login {
		before := current
		if err := r.client.SetLoginSuspended(
			ctx,
			address,
			target.Login,
		); err != nil {
			after := before
			after.Login = target.Login
			return []cpanelmail.AccountSuspensions{before, after},
				fmt.Errorf("set login suspension: %w", err)
		}
		current.Login = target.Login
	}
	if current.Incoming != target.Incoming {
		before := current
		if err := r.client.SetIncomingSuspended(
			ctx,
			address,
			target.Incoming,
		); err != nil {
			after := before
			after.Incoming = target.Incoming
			return []cpanelmail.AccountSuspensions{before, after},
				fmt.Errorf("set incoming-mail suspension: %w", err)
		}
		current.Incoming = target.Incoming
	}
	if current.Outgoing != target.Outgoing {
		before := current
		if err := r.client.SetOutgoingSuspended(
			ctx,
			address,
			target.Outgoing,
		); err != nil {
			after := before
			after.Outgoing = target.Outgoing
			return []cpanelmail.AccountSuspensions{before, after},
				fmt.Errorf("set outgoing-mail suspension: %w", err)
		}
		current.Outgoing = target.Outgoing
	}

	return nil, nil
}

func (r *emailAccountSuspensionResource) verify(
	ctx context.Context,
	address string,
	expected cpanelmail.AccountSuspensions,
) (*cpanelmail.Account, error) {
	account, err := r.getAccount(ctx, address)
	if err != nil {
		return nil, fmt.Errorf(
			"read email account suspension after mutation: %w",
			err,
		)
	}
	if account == nil {
		return nil, fmt.Errorf(
			"email account %q was not found after suspension mutation",
			address,
		)
	}
	actual, err := account.Suspensions()
	if err != nil {
		return nil, err
	}
	if actual.Login != expected.Login ||
		actual.Incoming != expected.Incoming ||
		actual.Outgoing != expected.Outgoing {
		return nil, fmt.Errorf(
			"email account %q suspension state is login=%t incoming=%t outgoing=%t; expected login=%t incoming=%t outgoing=%t",
			address,
			actual.Login,
			actual.Incoming,
			actual.Outgoing,
			expected.Login,
			expected.Incoming,
			expected.Outgoing,
		)
	}

	return account, nil
}

func (r *emailAccountSuspensionResource) restoreIfCurrentMatches(
	ctx context.Context,
	address string,
	original cpanelmail.AccountSuspensions,
	expectedCurrent ...cpanelmail.AccountSuspensions,
) error {
	account, err := r.getAccount(ctx, address)
	if err != nil {
		return fmt.Errorf(
			"read email account suspension before restore: %w",
			err,
		)
	}
	if account == nil {
		return fmt.Errorf(
			"email account %q no longer exists during suspension restore",
			address,
		)
	}
	current, err := account.Suspensions()
	if err != nil {
		return err
	}
	if managedEmailAccountSuspensionsEqual(current, original) {
		return nil
	}
	currentMatchesTransition := false
	for _, expected := range expectedCurrent {
		if managedEmailAccountSuspensionsEqual(current, expected) {
			currentMatchesTransition = true
			break
		}
	}
	if !currentMatchesTransition {
		return fmt.Errorf(
			"refuse to restore previous email account suspension because the current restrictions no longer match the Terraform transition",
		)
	}
	if _, err := r.applyChanges(ctx, address, current, original); err != nil {
		return fmt.Errorf(
			"restore previous email account suspension: %w",
			err,
		)
	}
	if _, err := r.verify(ctx, address, original); err != nil {
		return fmt.Errorf(
			"verify restored email account suspension: %w",
			err,
		)
	}

	return nil
}

func managedEmailAccountSuspensionsEqual(
	left cpanelmail.AccountSuspensions,
	right cpanelmail.AccountSuspensions,
) bool {
	return left.Login == right.Login &&
		left.Incoming == right.Incoming &&
		left.Outgoing == right.Outgoing
}

func (r *emailAccountSuspensionResource) getAccount(
	ctx context.Context,
	address string,
) (*cpanelmail.Account, error) {
	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		return nil, err
	}

	return r.client.GetAccount(ctx, user, domain)
}
