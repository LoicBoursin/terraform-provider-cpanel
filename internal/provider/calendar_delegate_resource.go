package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

var (
	_ resource.Resource                = &calendarDelegateResource{}
	_ resource.ResourceWithConfigure   = &calendarDelegateResource{}
	_ resource.ResourceWithImportState = &calendarDelegateResource{}
)

func NewCalendarDelegateResource() resource.Resource {
	return &calendarDelegateResource{}
}

type calendarDelegateClient interface {
	AddDelegate(context.Context, cpanelcalendar.Definition) error
	CalendarExists(context.Context, string, string) (bool, error)
	GetDelegate(
		context.Context,
		string,
		string,
		string,
	) (*cpanelcalendar.Delegate, error)
	LockDelegate(string, string, string) func()
	RemoveDelegate(context.Context, string, string, string) error
	UpdateDelegate(context.Context, cpanelcalendar.Definition) error
}

type calendarDelegateResource struct {
	client calendarDelegateClient
}

func (r *calendarDelegateResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_calendar_delegate"
}

func (r *calendarDelegateResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages access from one existing cPanel mail account to another account's default calendar.",
		MarkdownDescription: "Manages access from one existing cPanel mail account to another account's default CalDAV calendar. Both mail accounts remain separate resources and are preserved when the delegation is removed.",
		Attributes: map[string]schema.Attribute{
			"delegator": schema.StringAttribute{
				Required:            true,
				Description:         "The existing mail account that owns the calendar.",
				MarkdownDescription: "The existing mail account that owns the calendar.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"delegatee": schema.StringAttribute{
				Required:            true,
				Description:         "The existing mail account that receives access.",
				MarkdownDescription: "The existing mail account that receives access.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"calendar": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(cpanelcalendar.DefaultCalendar),
				Description:         "The cPanel calendar collection identifier.",
				MarkdownDescription: "The cPanel calendar collection identifier. Only the default `calendar` collection is supported so configurations remain compatible with the current `CPDAVD` API and the legacy `CCS` API.",
				Validators:          calendarNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"readonly": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Whether the delegate may only read the calendar.",
				MarkdownDescription: "Whether the delegate may only read the calendar. Defaults to full read/write access.",
			},
			"calendar_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The CPDAVD display name for the delegated calendar. Legacy CCS returns an empty value.",
				MarkdownDescription: "The `CPDAVD` display name for the delegated calendar. The value is empty when the legacy `CCS` API is used.",
			},
		},
	}
}

func (r *calendarDelegateResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state CalendarDelegateModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	definition := calendarDelegateDefinitionFromModel(state)
	if err := validateCalendarDelegateDefinition(definition); err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegate state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockDelegate(
		definition.Delegator,
		definition.Calendar,
		definition.Delegatee,
	)
	defer unlock()

	delegate, err := r.client.GetDelegate(
		ctx,
		definition.Delegator,
		definition.Calendar,
		definition.Delegatee,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read calendar delegate",
			err.Error(),
		)
		return
	}
	if delegate == nil {
		response.State.RemoveResource(ctx)
		return
	}

	applyCalendarDelegateToModel(&state, *delegate)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *calendarDelegateResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan CalendarDelegateModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired := calendarDelegateDefinitionFromModel(plan)
	if err := validateCalendarDelegateDefinition(desired); err != nil {
		response.Diagnostics.AddError("Invalid calendar delegate", err.Error())
		return
	}

	unlock := r.client.LockDelegate(
		desired.Delegator,
		desired.Calendar,
		desired.Delegatee,
	)
	defer unlock()

	if err := r.validateCalendars(ctx, desired); err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegation accounts",
			err.Error(),
		)
		return
	}

	existing, err := r.getDelegate(ctx, desired)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read calendar delegate",
			err.Error(),
		)
		return
	}
	if existing != nil {
		response.Diagnostics.AddError(
			"Calendar delegate already exists",
			"This exact calendar delegation already exists. Import it instead of taking ownership implicitly.",
		)
		return
	}

	created, rollbackAllowed, err := r.createAndVerify(ctx, desired)
	if err != nil {
		rollbackErr := r.rollbackCreatedIfAllowed(
			ctx,
			desired,
			rollbackAllowed,
		)
		response.Diagnostics.AddError(
			"Unable to create calendar delegate",
			calendarDelegateMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyCalendarDelegateToModel(&plan, *created)
	stateDiagnostics := response.State.Set(ctx, &plan)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreated(ctx, desired)
		response.Diagnostics.Append(stateDiagnostics...)
		response.Diagnostics.AddError(
			"Unable to store calendar delegate state",
			calendarDelegateMutationErrorDetail(
				errors.New("write Terraform state"),
				rollbackErr,
			),
		)
		return
	}
	response.Diagnostics.Append(stateDiagnostics...)
}

func (r *calendarDelegateResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan CalendarDelegateModel
	var state CalendarDelegateModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired := calendarDelegateDefinitionFromModel(plan)
	previous := calendarDelegateDefinitionFromModel(state)
	if err := validateCalendarDelegateDefinition(desired); err != nil {
		response.Diagnostics.AddError("Invalid calendar delegate", err.Error())
		return
	}
	if err := validateCalendarDelegateDefinition(previous); err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegate state",
			err.Error(),
		)
		return
	}
	if desired.Delegator != previous.Delegator ||
		desired.Delegatee != previous.Delegatee ||
		desired.Calendar != previous.Calendar {
		response.Diagnostics.AddError(
			"Unsupported calendar delegate identity update",
			"Changing the delegator, delegatee, or calendar requires replacement.",
		)
		return
	}

	unlock := r.client.LockDelegate(
		desired.Delegator,
		desired.Calendar,
		desired.Delegatee,
	)
	defer unlock()

	if err := r.validateCalendars(ctx, desired); err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegation accounts",
			err.Error(),
		)
		return
	}

	current, err := r.getDelegate(ctx, previous)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read calendar delegate",
			err.Error(),
		)
		return
	}
	if current == nil {
		response.Diagnostics.AddError(
			"Calendar delegate no longer exists",
			"Refresh the Terraform state before updating the calendar delegation.",
		)
		return
	}
	if !calendarDelegatesEqual(*current, previous) {
		response.Diagnostics.AddError(
			"Calendar delegate changed during update",
			"The remote calendar delegation no longer matches the refreshed Terraform state. Run Terraform again before applying this update.",
		)
		return
	}

	updated, err := r.updateAndVerify(ctx, desired)
	if err != nil {
		rollbackErr := r.rollbackUpdated(ctx, previous, desired)
		response.Diagnostics.AddError(
			"Unable to update calendar delegate",
			calendarDelegateMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyCalendarDelegateToModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackUpdated(ctx, previous, desired)
		response.Diagnostics.Append(stateDiagnostics...)
		response.Diagnostics.AddError(
			"Unable to store calendar delegate state",
			calendarDelegateMutationErrorDetail(
				errors.New("write Terraform state"),
				rollbackErr,
			),
		)
		return
	}
	response.Diagnostics.Append(stateDiagnostics...)
}

func (r *calendarDelegateResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state CalendarDelegateModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	expected := calendarDelegateDefinitionFromModel(state)
	if err := validateCalendarDelegateDefinition(expected); err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegate state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockDelegate(
		expected.Delegator,
		expected.Calendar,
		expected.Delegatee,
	)
	defer unlock()

	current, err := r.getDelegate(ctx, expected)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read calendar delegate",
			err.Error(),
		)
		return
	}
	if current == nil {
		return
	}
	if !calendarDelegatesEqual(*current, expected) {
		response.Diagnostics.AddError(
			"Calendar delegate changed before deletion",
			"The remote calendar delegation no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.client.RemoveDelegate(
		ctx,
		expected.Delegator,
		expected.Calendar,
		expected.Delegatee,
	); err != nil {
		actual, readErr := r.getDelegate(ctx, expected)
		if readErr == nil && actual == nil {
			return
		}
		response.Diagnostics.AddError(
			"Unable to delete calendar delegate",
			errors.Join(
				err,
				wrapOptionalError(
					"verify calendar delegate after failed deletion",
					readErr,
				),
			).Error(),
		)
		return
	}

	if err := r.verifyAbsent(ctx, expected); err != nil {
		response.Diagnostics.AddError(
			"Unable to verify calendar delegate deletion",
			err.Error(),
		)
	}
}

func (r *calendarDelegateResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	definition, err := parseCalendarDelegateImportID(request.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegate import identifier",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		response.State.SetAttribute(
			ctx,
			path.Root("delegator"),
			definition.Delegator,
		)...,
	)
	response.Diagnostics.Append(
		response.State.SetAttribute(
			ctx,
			path.Root("calendar"),
			definition.Calendar,
		)...,
	)
	response.Diagnostics.Append(
		response.State.SetAttribute(
			ctx,
			path.Root("delegatee"),
			definition.Delegatee,
		)...,
	)
}

func (r *calendarDelegateResource) Configure(
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

	client, ok := providerData["calendar"].(*cpanelcalendar.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Calendar Client Type",
			fmt.Sprintf(
				"Expected *calendar.Client, got: %T.",
				providerData["calendar"],
			),
		)
		return
	}

	r.client = client
}

func (r *calendarDelegateResource) validateCalendars(
	ctx context.Context,
	definition cpanelcalendar.Definition,
) error {
	for _, address := range []string{
		definition.Delegator,
		definition.Delegatee,
	} {
		exists, err := r.client.CalendarExists(
			ctx,
			address,
			definition.Calendar,
		)
		if err != nil {
			return fmt.Errorf(
				"read calendar %q for %q: %w",
				definition.Calendar,
				address,
				err,
			)
		}
		if !exists {
			return fmt.Errorf(
				"mail account %q does not expose the %q CalDAV calendar; create the mailbox before its calendar delegations",
				address,
				definition.Calendar,
			)
		}
	}

	return nil
}

func (r *calendarDelegateResource) getDelegate(
	ctx context.Context,
	definition cpanelcalendar.Definition,
) (*cpanelcalendar.Delegate, error) {
	return r.client.GetDelegate(
		ctx,
		definition.Delegator,
		definition.Calendar,
		definition.Delegatee,
	)
}

func (r *calendarDelegateResource) createAndVerify(
	ctx context.Context,
	desired cpanelcalendar.Definition,
) (*cpanelcalendar.Delegate, bool, error) {
	mutationErr := r.client.AddDelegate(ctx, desired)
	rollbackAllowed := mutationErr == nil
	actual, readErr := r.getDelegate(ctx, desired)
	if readErr != nil {
		return nil, rollbackAllowed, errors.Join(
			mutationErr,
			fmt.Errorf("read calendar delegate after creation: %w", readErr),
		)
	}
	if actual == nil {
		if mutationErr != nil {
			return nil, rollbackAllowed, mutationErr
		}
		return nil, true, fmt.Errorf(
			"calendar delegate was not found after creation",
		)
	}
	if !calendarDelegatesEqual(*actual, desired) {
		return nil, mutationErr == nil, errors.Join(
			mutationErr,
			fmt.Errorf("calendar delegate returned unexpected state after creation"),
		)
	}
	if mutationErr != nil {
		return nil, false, errors.Join(
			mutationErr,
			fmt.Errorf(
				"cPanel may have created the calendar delegate despite the failed response; Terraform will not adopt or delete it automatically, so inspect it and import it if it is intended to remain",
			),
		)
	}

	return actual, true, nil
}

func (r *calendarDelegateResource) rollbackCreatedIfAllowed(
	ctx context.Context,
	desired cpanelcalendar.Definition,
	allowed bool,
) error {
	if !allowed {
		return nil
	}

	return r.rollbackCreated(ctx, desired)
}

func (r *calendarDelegateResource) updateAndVerify(
	ctx context.Context,
	desired cpanelcalendar.Definition,
) (*cpanelcalendar.Delegate, error) {
	mutationErr := r.client.UpdateDelegate(ctx, desired)
	actual, readErr := r.getDelegate(ctx, desired)
	if readErr != nil {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf("read calendar delegate after update: %w", readErr),
		)
	}
	if actual == nil {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf("calendar delegate was not found after update"),
		)
	}
	if !calendarDelegatesEqual(*actual, desired) {
		return nil, errors.Join(
			mutationErr,
			fmt.Errorf("calendar delegate returned unexpected state after update"),
		)
	}

	return actual, nil
}

func (r *calendarDelegateResource) verifyAbsent(
	ctx context.Context,
	definition cpanelcalendar.Definition,
) error {
	actual, err := r.getDelegate(ctx, definition)
	if err != nil {
		return fmt.Errorf("read calendar delegate after deletion: %w", err)
	}
	if actual != nil {
		return fmt.Errorf("calendar delegate still exists after deletion")
	}

	return nil
}

func (r *calendarDelegateResource) rollbackCreated(
	ctx context.Context,
	desired cpanelcalendar.Definition,
) error {
	actual, err := r.getDelegate(ctx, desired)
	if err != nil {
		return fmt.Errorf("read calendar delegate before create rollback: %w", err)
	}
	if actual == nil {
		return nil
	}
	if !calendarDelegatesEqual(*actual, desired) {
		return fmt.Errorf(
			"refuse to roll back calendar delegate because its remote state no longer matches the attempted creation",
		)
	}
	if err := r.client.RemoveDelegate(
		ctx,
		desired.Delegator,
		desired.Calendar,
		desired.Delegatee,
	); err != nil {
		if verifyErr := r.verifyAbsent(ctx, desired); verifyErr == nil {
			return nil
		}
		return fmt.Errorf("remove calendar delegate during rollback: %w", err)
	}
	if err := r.verifyAbsent(ctx, desired); err != nil {
		return fmt.Errorf("verify calendar delegate create rollback: %w", err)
	}

	return nil
}

func (r *calendarDelegateResource) rollbackUpdated(
	ctx context.Context,
	previous cpanelcalendar.Definition,
	attempted cpanelcalendar.Definition,
) error {
	actual, err := r.getDelegate(ctx, attempted)
	if err != nil {
		return fmt.Errorf("read calendar delegate before update rollback: %w", err)
	}
	if actual == nil {
		return fmt.Errorf(
			"calendar delegate no longer exists during update rollback",
		)
	}
	if calendarDelegatesEqual(*actual, previous) {
		return nil
	}
	if !calendarDelegatesEqual(*actual, attempted) {
		return fmt.Errorf(
			"refuse to roll back calendar delegate because its remote state no longer matches the attempted update",
		)
	}
	if err := r.client.UpdateDelegate(ctx, previous); err != nil {
		restored, readErr := r.getDelegate(ctx, previous)
		if readErr == nil &&
			restored != nil &&
			calendarDelegatesEqual(*restored, previous) {
			return nil
		}
		return errors.Join(
			fmt.Errorf("restore previous calendar delegate access: %w", err),
			wrapOptionalError(
				"verify calendar delegate after failed rollback",
				readErr,
			),
		)
	}

	restored, err := r.getDelegate(ctx, previous)
	if err != nil {
		return fmt.Errorf("verify calendar delegate update rollback: %w", err)
	}
	if restored == nil || !calendarDelegatesEqual(*restored, previous) {
		return fmt.Errorf(
			"calendar delegate did not return to its previous state during rollback",
		)
	}

	return nil
}

func calendarDelegateMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous calendar delegation state: %v",
		primaryError,
		rollbackError,
	)
}
