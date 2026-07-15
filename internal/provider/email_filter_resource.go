package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailFilterResource{}
	_ resource.ResourceWithConfigure   = &emailFilterResource{}
	_ resource.ResourceWithImportState = &emailFilterResource{}
)

func NewEmailFilterResource() resource.Resource {
	return &emailFilterResource{}
}

type emailFilterClient interface {
	DeleteFilter(context.Context, string, string) error
	GetAccount(context.Context, string, string) (*cpanelmail.Account, error)
	GetFilter(context.Context, string, string) (*cpanelmail.Filter, error)
	LockFilterAccount(string) func()
	SetFilterEnabled(context.Context, string, string, bool) error
	StoreFilter(
		context.Context,
		string,
		string,
		cpanelmail.Filter,
	) error
}

type emailFilterResource struct {
	client emailFilterClient
}

func (r *emailFilterResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_filter"
}

func (r *emailFilterResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages one ordered user-level cPanel email filter for an existing mailbox.",
		MarkdownDescription: "Manages one ordered user-level cPanel email filter for an existing mailbox. Account-level filters are deliberately unsupported. For safety, actions are limited to `deliver`, `fail`, and `finish`; arbitrary file writes and command execution through `save` or `pipe` are rejected.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Required:            true,
				Description:         "The existing mailbox that owns the user-level filter.",
				MarkdownDescription: "The existing mailbox that owns the user-level filter.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel filter name.",
				MarkdownDescription: "The cPanel filter name. Changing the name renames the filter in place.",
				Validators:          emailFilterNameValidators(),
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				Description:         "Whether cPanel should evaluate the filter.",
				MarkdownDescription: "Whether cPanel should evaluate the filter. Defaults to `true`.",
			},
			"rules": schema.ListNestedAttribute{
				Required:            true,
				Description:         "The ordered conditions evaluated by cPanel.",
				MarkdownDescription: "The ordered conditions evaluated by cPanel. String and numeric comparisons are supported. Every rule except the last must use `and` or `or`; the last rule must use `none`.",
				Validators: []validator.List{
					listvalidator.NoNullValues(),
					listvalidator.SizeBetween(1, emailFilterMaximumItems),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"part": schema.StringAttribute{
							Required:    true,
							Description: "The email section inspected by the rule.",
							MarkdownDescription: "The email section inspected by the rule. Supported canonical cPanel values are `" +
								strings.Join(emailFilterParts, "`, `") + "`.",
							Validators: emailFilterPartValidators(),
						},
						"match": schema.StringAttribute{
							Required:    true,
							Description: "The string comparison applied by the rule.",
							MarkdownDescription: "The comparison applied by the rule. Supported values are `" +
								strings.Join(emailFilterMatches, "`, `") +
								"`, plus `none` for `not delivered` and `error_message`. Numeric comparisons require a base-10 integer value.",
							Validators: emailFilterMatchValidators(),
						},
						"value": schema.StringAttribute{
							Required:            true,
							Description:         "The value matched by the rule.",
							MarkdownDescription: "The value matched by the rule. Use an empty string with `match = \"none\"` for `not delivered` and `error_message`. NUL, carriage return, and newline characters are rejected from other values.",
							Validators: []validator.String{
								stringvalidator.LengthBetween(
									0,
									emailFilterMaximumValueLength,
								),
							},
						},
						"operator": schema.StringAttribute{
							Required:            true,
							Description:         "The connection to the next rule.",
							MarkdownDescription: "The connection to the next rule: `and`, `or`, or `none` for the final rule.",
							Validators:          emailFilterOperatorValidators(),
						},
					},
				},
			},
			"actions": schema.ListNestedAttribute{
				Required:            true,
				Description:         "The ordered safe actions performed when the rules match.",
				MarkdownDescription: "The ordered safe actions performed when the rules match. Only `deliver`, `fail`, and `finish` are supported.",
				Validators: []validator.List{
					listvalidator.NoNullValues(),
					listvalidator.SizeBetween(1, emailFilterMaximumItems),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							Required:            true,
							Description:         "The action type.",
							MarkdownDescription: "The action type: `deliver`, `fail`, or `finish`.",
							Validators:          emailFilterActionValidators(),
						},
						"destination": schema.StringAttribute{
							Optional:            true,
							Description:         "The destination email address for deliver or rejection text for fail.",
							MarkdownDescription: "The destination email address for `deliver` or rejection text for `fail`. Omit it for `finish`.",
							Validators:          emailFilterDestinationValidators(),
						},
					},
				},
			},
		},
	}
}

func (r *emailFilterResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state EmailFilterModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	unlock := r.client.LockFilterAccount(state.Account.ValueString())
	defer unlock()

	filter, err := r.client.GetFilter(
		ctx,
		state.Account.ValueString(),
		state.Name.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError("Unable to read email filter", err.Error())
		return
	}
	if filter == nil {
		response.State.RemoveResource(ctx)
		return
	}
	if err := validateEmailFilter(*filter); err != nil {
		response.Diagnostics.AddError(
			"Email filter is not safely manageable",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(applyEmailFilterToModel(ctx, &state, *filter)...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *emailFilterResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan EmailFilterModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired, diagnostics := emailFilterFromModel(ctx, plan)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := validateEmailFilter(desired); err != nil {
		response.Diagnostics.AddError("Invalid email filter", err.Error())
		return
	}

	unlock := r.client.LockFilterAccount(desired.Account)
	defer unlock()

	if err := r.validateExistingMailbox(ctx, desired.Account); err != nil {
		response.Diagnostics.AddError("Invalid email filter account", err.Error())
		return
	}

	existing, err := r.client.GetFilter(ctx, desired.Account, desired.Name)
	if err != nil {
		response.Diagnostics.AddError("Unable to read email filter", err.Error())
		return
	}
	if existing != nil {
		response.Diagnostics.AddError(
			"Email filter already exists",
			"An email filter with this account and name already exists. Import it instead of taking ownership implicitly.",
		)
		return
	}

	created, err := r.applyRemoteFilter(ctx, "", desired)
	if err != nil {
		rollbackErr := r.rollbackCreatedFilter(ctx, desired)
		response.Diagnostics.AddError(
			"Unable to create email filter",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	response.Diagnostics.Append(applyEmailFilterToModel(ctx, &plan, *created)...)
	if response.Diagnostics.HasError() {
		rollbackErr := r.rollbackCreatedFilter(ctx, desired)
		response.Diagnostics.AddError(
			"Unable to store email filter state",
			emailMutationErrorDetail(
				errors.New("convert email filter state"),
				rollbackErr,
			),
		)
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreatedFilter(ctx, desired)
		response.Diagnostics.Append(stateDiagnostics...)
		response.Diagnostics.AddError(
			"Unable to store email filter state",
			emailMutationErrorDetail(
				errors.New("write Terraform state"),
				rollbackErr,
			),
		)
		return
	}
	response.Diagnostics.Append(stateDiagnostics...)
}

func (r *emailFilterResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan EmailFilterModel
	var state EmailFilterModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired, desiredDiagnostics := emailFilterFromModel(ctx, plan)
	previous, previousDiagnostics := emailFilterFromModel(ctx, state)
	response.Diagnostics.Append(desiredDiagnostics...)
	response.Diagnostics.Append(previousDiagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := validateEmailFilter(desired); err != nil {
		response.Diagnostics.AddError("Invalid email filter", err.Error())
		return
	}
	if err := validateEmailFilter(previous); err != nil {
		response.Diagnostics.AddError(
			"Invalid email filter state",
			err.Error(),
		)
		return
	}
	if desired.Account != previous.Account {
		response.Diagnostics.AddError(
			"Unsupported email filter account update",
			"Changing the filter account requires replacement.",
		)
		return
	}

	unlock := r.client.LockFilterAccount(desired.Account)
	defer unlock()

	if err := r.validateExistingMailbox(ctx, desired.Account); err != nil {
		response.Diagnostics.AddError("Invalid email filter account", err.Error())
		return
	}

	current, err := r.client.GetFilter(ctx, previous.Account, previous.Name)
	if err != nil {
		response.Diagnostics.AddError("Unable to read email filter", err.Error())
		return
	}
	if current == nil {
		response.Diagnostics.AddError(
			"Email filter no longer exists",
			"Refresh the Terraform state before updating the email filter.",
		)
		return
	}
	if !emailFiltersEqual(*current, previous) {
		response.Diagnostics.AddError(
			"Email filter changed during update",
			"The remote email filter no longer matches the refreshed Terraform state. Run Terraform again before applying this update.",
		)
		return
	}

	if desired.Name != previous.Name {
		conflict, conflictErr := r.client.GetFilter(
			ctx,
			desired.Account,
			desired.Name,
		)
		if conflictErr != nil {
			response.Diagnostics.AddError(
				"Unable to check renamed email filter",
				conflictErr.Error(),
			)
			return
		}
		if conflict != nil {
			response.Diagnostics.AddError(
				"Renamed email filter already exists",
				"Another filter already uses the requested name. Import that filter or choose a different name.",
			)
			return
		}
	}

	updated, err := r.applyRemoteFilter(ctx, previous.Name, desired)
	if err != nil {
		rollbackErr := r.rollbackUpdatedFilter(ctx, previous, desired)
		response.Diagnostics.AddError(
			"Unable to update email filter",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	response.Diagnostics.Append(applyEmailFilterToModel(ctx, &plan, *updated)...)
	if response.Diagnostics.HasError() {
		rollbackErr := r.rollbackUpdatedFilter(ctx, previous, desired)
		response.Diagnostics.AddError(
			"Unable to store email filter state",
			emailMutationErrorDetail(
				errors.New("convert email filter state"),
				rollbackErr,
			),
		)
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackUpdatedFilter(ctx, previous, desired)
		response.Diagnostics.Append(stateDiagnostics...)
		response.Diagnostics.AddError(
			"Unable to store email filter state",
			emailMutationErrorDetail(
				errors.New("write Terraform state"),
				rollbackErr,
			),
		)
		return
	}
	response.Diagnostics.Append(stateDiagnostics...)
}

func (r *emailFilterResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state EmailFilterModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	expected, diagnostics := emailFilterFromModel(ctx, state)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := validateEmailFilter(expected); err != nil {
		response.Diagnostics.AddError(
			"Invalid email filter state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockFilterAccount(expected.Account)
	defer unlock()

	current, err := r.client.GetFilter(ctx, expected.Account, expected.Name)
	if err != nil {
		response.Diagnostics.AddError("Unable to read email filter", err.Error())
		return
	}
	if current == nil {
		return
	}
	if !emailFiltersEqual(*current, expected) {
		response.Diagnostics.AddError(
			"Email filter changed before deletion",
			"The remote filter no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.client.DeleteFilter(ctx, expected.Account, expected.Name); err != nil {
		actual, readErr := r.client.GetFilter(
			ctx,
			expected.Account,
			expected.Name,
		)
		if readErr == nil && actual == nil {
			return
		}
		response.Diagnostics.AddError(
			"Unable to delete email filter",
			errors.Join(
				err,
				wrapOptionalError("verify email filter after failed deletion", readErr),
			).Error(),
		)
		return
	}

	if err := r.verifyFilterAbsent(ctx, expected.Account, expected.Name); err != nil {
		response.Diagnostics.AddError(
			"Unable to verify email filter deletion",
			err.Error(),
		)
	}
}

func (r *emailFilterResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	account, name, err := parseEmailFilterImportID(request.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Invalid email filter import identifier",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		response.State.SetAttribute(ctx, path.Root("account"), account)...,
	)
	response.Diagnostics.Append(
		response.State.SetAttribute(ctx, path.Root("name"), name)...,
	)
}

func (r *emailFilterResource) Configure(
	_ context.Context,
	request resource.ConfigureRequest,
	response *resource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		response.Diagnostics.AddError(
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

func (r *emailFilterResource) validateExistingMailbox(
	ctx context.Context,
	address string,
) error {
	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		return err
	}

	account, err := r.client.GetAccount(ctx, user, domain)
	if err != nil {
		return fmt.Errorf("read email account %q: %w", address, err)
	}
	if account == nil {
		return fmt.Errorf(
			"email account %q does not exist; create the mailbox before its filters",
			address,
		)
	}

	return nil
}

func (r *emailFilterResource) applyRemoteFilter(
	ctx context.Context,
	previousName string,
	desired cpanelmail.Filter,
) (*cpanelmail.Filter, error) {
	storeErr := r.client.StoreFilter(ctx, desired.Account, previousName, desired)
	if storeErr != nil {
		actual, readErr := r.client.GetFilter(
			ctx,
			desired.Account,
			desired.Name,
		)
		if readErr != nil {
			return nil, errors.Join(
				storeErr,
				fmt.Errorf("inspect email filter after ambiguous store: %w", readErr),
			)
		}
		if actual == nil || !emailFilterDefinitionsEqual(*actual, desired) {
			return nil, storeErr
		}
	}

	enableErr := r.client.SetFilterEnabled(
		ctx,
		desired.Account,
		desired.Name,
		desired.Enabled,
	)
	if enableErr != nil {
		actual, readErr := r.client.GetFilter(
			ctx,
			desired.Account,
			desired.Name,
		)
		if readErr != nil {
			return nil, errors.Join(
				enableErr,
				fmt.Errorf("inspect email filter after ambiguous status update: %w", readErr),
			)
		}
		if actual == nil || !emailFiltersEqual(*actual, desired) {
			return nil, enableErr
		}
	}

	actual, err := r.verifyFilter(ctx, desired)
	if err != nil {
		return nil, err
	}
	if previousName != "" && previousName != desired.Name {
		previous, readErr := r.client.GetFilter(
			ctx,
			desired.Account,
			previousName,
		)
		if readErr != nil {
			return nil, fmt.Errorf(
				"verify previous email filter name after rename: %w",
				readErr,
			)
		}
		if previous != nil {
			return nil, fmt.Errorf(
				"email filter %q still exists after rename to %q",
				previousName,
				desired.Name,
			)
		}
	}

	return actual, nil
}

func (r *emailFilterResource) verifyFilter(
	ctx context.Context,
	expected cpanelmail.Filter,
) (*cpanelmail.Filter, error) {
	actual, err := r.client.GetFilter(ctx, expected.Account, expected.Name)
	if err != nil {
		return nil, fmt.Errorf("read email filter after mutation: %w", err)
	}
	if actual == nil {
		return nil, fmt.Errorf(
			"email filter %q for %q was not found after mutation",
			expected.Name,
			expected.Account,
		)
	}
	if err := validateEmailFilter(*actual); err != nil {
		return nil, fmt.Errorf(
			"email filter returned unsupported state after mutation: %w",
			err,
		)
	}
	if !emailFiltersEqual(*actual, expected) {
		return nil, fmt.Errorf(
			"email filter %q for %q does not match the requested state",
			expected.Name,
			expected.Account,
		)
	}

	return actual, nil
}

func (r *emailFilterResource) verifyFilterAbsent(
	ctx context.Context,
	account string,
	name string,
) error {
	actual, err := r.client.GetFilter(ctx, account, name)
	if err != nil {
		return fmt.Errorf("read email filter after deletion: %w", err)
	}
	if actual != nil {
		return fmt.Errorf(
			"email filter %q for %q still exists after deletion",
			name,
			account,
		)
	}

	return nil
}

func (r *emailFilterResource) rollbackCreatedFilter(
	ctx context.Context,
	desired cpanelmail.Filter,
) error {
	actual, err := r.client.GetFilter(ctx, desired.Account, desired.Name)
	if err != nil {
		return fmt.Errorf("inspect created email filter before rollback: %w", err)
	}
	if actual == nil {
		return nil
	}
	if !emailFiltersEqual(*actual, desired) {
		return errors.New(
			"refuse to roll back email filter creation because the remote state no longer matches the attempted state",
		)
	}
	if err := r.client.DeleteFilter(ctx, desired.Account, desired.Name); err != nil {
		return fmt.Errorf("delete created email filter during rollback: %w", err)
	}

	return r.verifyFilterAbsent(ctx, desired.Account, desired.Name)
}

func (r *emailFilterResource) rollbackUpdatedFilter(
	ctx context.Context,
	previous cpanelmail.Filter,
	desired cpanelmail.Filter,
) error {
	previousIdentity, err := r.client.GetFilter(
		ctx,
		previous.Account,
		previous.Name,
	)
	if err != nil {
		return fmt.Errorf("inspect previous email filter identity for rollback: %w", err)
	}

	var desiredIdentity *cpanelmail.Filter
	if desired.Name == previous.Name {
		desiredIdentity = previousIdentity
	} else {
		desiredIdentity, err = r.client.GetFilter(
			ctx,
			desired.Account,
			desired.Name,
		)
		if err != nil {
			return fmt.Errorf("inspect renamed email filter for rollback: %w", err)
		}
	}

	if previousIdentity != nil &&
		emailFiltersEqual(*previousIdentity, previous) &&
		(desired.Name == previous.Name || desiredIdentity == nil) {
		return nil
	}

	candidate := desiredIdentity
	if desired.Name != previous.Name &&
		candidate == nil &&
		previousIdentity != nil &&
		emailFiltersEqual(*previousIdentity, desired) {
		candidate = previousIdentity
	}
	if candidate == nil || !emailFiltersEqual(*candidate, desired) {
		return errors.New(
			"refuse to roll back email filter update because the remote state cannot be attributed safely to the attempted update",
		)
	}
	if desired.Name != previous.Name &&
		previousIdentity != nil &&
		previousIdentity != candidate {
		return errors.New(
			"refuse to roll back email filter update because both the old and new identities exist",
		)
	}

	return r.restorePreviousFilter(ctx, candidate.Name, previous, desired.Name)
}

func (r *emailFilterResource) restorePreviousFilter(
	ctx context.Context,
	currentName string,
	previous cpanelmail.Filter,
	desiredName string,
) error {
	storeErr := r.client.StoreFilter(
		ctx,
		previous.Account,
		currentName,
		previous,
	)
	if storeErr != nil {
		actual, readErr := r.client.GetFilter(
			ctx,
			previous.Account,
			previous.Name,
		)
		if readErr != nil ||
			actual == nil ||
			!emailFilterDefinitionsEqual(*actual, previous) {
			return errors.Join(
				fmt.Errorf("restore previous email filter definition: %w", storeErr),
				wrapOptionalError(
					"inspect email filter after ambiguous rollback",
					readErr,
				),
			)
		}
	}

	statusErr := r.client.SetFilterEnabled(
		ctx,
		previous.Account,
		previous.Name,
		previous.Enabled,
	)
	if statusErr != nil {
		actual, readErr := r.client.GetFilter(
			ctx,
			previous.Account,
			previous.Name,
		)
		if readErr != nil || actual == nil || !emailFiltersEqual(*actual, previous) {
			return errors.Join(
				fmt.Errorf("restore previous email filter status: %w", statusErr),
				wrapOptionalError(
					"inspect email filter after ambiguous status rollback",
					readErr,
				),
			)
		}
	}

	if _, err := r.verifyFilter(ctx, previous); err != nil {
		return fmt.Errorf("verify restored email filter: %w", err)
	}
	if desiredName != previous.Name {
		if err := r.verifyFilterAbsent(
			ctx,
			previous.Account,
			desiredName,
		); err != nil {
			return fmt.Errorf("verify renamed identity removal during rollback: %w", err)
		}
	}

	return nil
}

func parseEmailFilterImportID(identifier string) (string, string, error) {
	account, name, found := strings.Cut(identifier, "|")
	if !found || account == "" || name == "" || strings.Contains(name, "|") {
		return "", "", fmt.Errorf(
			"expected an identifier in account|name form, got %q",
			identifier,
		)
	}
	if _, _, err := splitEmailAccountAddress(account); err != nil {
		return "", "", fmt.Errorf("invalid filter account: %w", err)
	}
	if err := validateEmailFilterName(name); err != nil {
		return "", "", err
	}

	return account, name, nil
}

func wrapOptionalError(label string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", label, err)
}
