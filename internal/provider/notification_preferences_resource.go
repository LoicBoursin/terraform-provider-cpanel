package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelcontact "terraform-provider-cpanel/internal/cpanel/contactinformation"
)

var (
	_ resource.Resource                = &notificationPreferencesResource{}
	_ resource.ResourceWithConfigure   = &notificationPreferencesResource{}
	_ resource.ResourceWithImportState = &notificationPreferencesResource{}
)

func NewNotificationPreferencesResource() resource.Resource {
	return &notificationPreferencesResource{}
}

type notificationPreferencesClient interface {
	GetNotificationPreferences(
		context.Context,
	) (*cpanelcontact.NotificationPreferences, error)
	SetNotificationPreferences(
		context.Context,
		map[string]bool,
	) (*cpanelcontact.NotificationPreferences, error)
}

type notificationPreferencesResource struct {
	client notificationPreferencesClient
}

func (r *notificationPreferencesResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_notification_preferences"
}

func (r *notificationPreferencesResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages the cPanel account's notification preferences.",
		MarkdownDescription: "Manages the complete cPanel account notification preference map. Removing the resource restores the complete settings observed when Terraform first took ownership.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"preferences": schema.MapAttribute{
				ElementType:         types.BoolType,
				Required:            true,
				Description:         "The complete notification preference map.",
				MarkdownDescription: "The complete notification preference map returned by cPanel. The configured key set must exactly match the account's available preference names.",
				Validators: []validator.Map{
					mapvalidator.SizeAtLeast(1),
					mapvalidator.KeysAre(
						stringvalidator.RegexMatches(
							regexp.MustCompile(`^notify_[a-z0-9_]+$`),
							"must start with notify_ and contain only lowercase letters, digits, and underscores",
						),
					),
				},
			},
			"descriptions": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "Localized descriptions keyed by notification preference name.",
				MarkdownDescription: "Localized descriptions keyed by notification preference name.",
			},
			"restore_preferences": schema.MapAttribute{
				ElementType:         types.BoolType,
				Computed:            true,
				Description:         "The complete preference map that Terraform restores when the resource is removed.",
				MarkdownDescription: "The complete preference map that Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *notificationPreferencesResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state NotificationPreferencesResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel notification preferences",
			err.Error(),
		)
		return
	}
	if notificationPreferencesRestoreStateMissing(state) {
		response.Diagnostics.Append(
			applyNotificationPreferencesRestoreToResourceModel(
				ctx,
				&state,
				*current,
			)...,
		)
	}
	response.Diagnostics.Append(
		applyNotificationPreferencesToResourceModel(ctx, &state, *current)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *notificationPreferencesResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan NotificationPreferencesResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired, diagnostics := notificationPreferencesDefinitionFromMap(
		ctx,
		plan.Preferences,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := cpanelcontact.ValidateDefinition(desired); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel notification preferences",
			err.Error(),
		)
		return
	}

	original, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel notification preferences",
			err.Error(),
		)
		return
	}
	updated, err := r.transition(ctx, *original, desired)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to configure cPanel notification preferences",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		applyNotificationPreferencesRestoreToResourceModel(
			ctx,
			&plan,
			*original,
		)...,
	)
	response.Diagnostics.Append(
		applyNotificationPreferencesToResourceModel(ctx, &plan, *updated)...,
	)
	if response.Diagnostics.HasError() {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			desired,
			original.Definition(),
		)
		if rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel notification preferences",
				rollbackErr.Error(),
			)
		}
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() &&
		!cpanelcontact.PreferencesMatchDefinition(*original, desired) {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			desired,
			original.Definition(),
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel notification preferences",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *notificationPreferencesResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan NotificationPreferencesResourceModel
	var state NotificationPreferencesResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired, diagnostics := notificationPreferencesDefinitionFromMap(
		ctx,
		plan.Preferences,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := cpanelcontact.ValidateDefinition(desired); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel notification preferences",
			err.Error(),
		)
		return
	}

	original, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel notification preferences",
			err.Error(),
		)
		return
	}
	expected, expectedDiagnostics := notificationPreferencesDefinitionFromMap(
		ctx,
		state.Preferences,
	)
	response.Diagnostics.Append(expectedDiagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if !cpanelcontact.PreferencesMatchDefinition(*original, expected) {
		response.Diagnostics.AddError(
			"cPanel notification preferences changed during update",
			"The current cPanel notification preferences no longer match Terraform state, so the provider refuses to overwrite them. Refresh and review the drift before retrying.",
		)
		return
	}
	updated, err := r.transition(ctx, *original, desired)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update cPanel notification preferences",
			err.Error(),
		)
		return
	}

	if notificationPreferencesRestoreStateMissing(state) {
		response.Diagnostics.Append(
			applyNotificationPreferencesRestoreToResourceModel(
				ctx,
				&plan,
				*original,
			)...,
		)
	} else {
		plan.RestorePreferences = state.RestorePreferences
	}
	response.Diagnostics.Append(
		applyNotificationPreferencesToResourceModel(ctx, &plan, *updated)...,
	)
	if response.Diagnostics.HasError() {
		rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			desired,
			original.Definition(),
		)
		if rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel notification preferences",
				rollbackErr.Error(),
			)
		}
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() &&
		!cpanelcontact.PreferencesMatchDefinition(*original, desired) {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			desired,
			original.Definition(),
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel notification preferences",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *notificationPreferencesResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state NotificationPreferencesResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}
	if notificationPreferencesRestoreStateMissing(state) {
		response.Diagnostics.AddError(
			"Unable to restore cPanel notification preferences",
			"The resource state does not contain the complete notification preferences that preceded Terraform management.",
		)
		return
	}

	expectedCurrent, diagnostics := notificationPreferencesDefinitionFromMap(
		ctx,
		state.Preferences,
	)
	response.Diagnostics.Append(diagnostics...)
	target, diagnostics := notificationPreferencesDefinitionFromMap(
		ctx,
		state.RestorePreferences,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := r.restoreIfCurrentMatches(
		ctx,
		expectedCurrent,
		target,
	); err != nil {
		response.Diagnostics.AddError(
			"Unable to restore cPanel notification preferences",
			err.Error(),
		)
	}
}

func (r *notificationPreferencesResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if request.ID != cpanelcontact.AccountIdentity {
		response.Diagnostics.AddError(
			"Invalid cPanel notification preferences import identifier",
			fmt.Sprintf(
				"Use %q to import the account notification preferences.",
				cpanelcontact.AccountIdentity,
			),
		)
		return
	}

	current, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import cPanel notification preferences",
			err.Error(),
		)
		return
	}

	state := NotificationPreferencesResourceModel{}
	response.Diagnostics.Append(
		applyNotificationPreferencesRestoreToResourceModel(
			ctx,
			&state,
			*current,
		)...,
	)
	response.Diagnostics.Append(
		applyNotificationPreferencesToResourceModel(ctx, &state, *current)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *notificationPreferencesResource) Configure(
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

	client, ok := providerData["contactinformation"].(*cpanelcontact.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected ContactInformation Client Type",
			fmt.Sprintf(
				"Expected *contactinformation.Client, got: %T.",
				providerData["contactinformation"],
			),
		)
		return
	}

	r.client = client
}

func (r *notificationPreferencesResource) transition(
	ctx context.Context,
	original cpanelcontact.NotificationPreferences,
	target map[string]bool,
) (*cpanelcontact.NotificationPreferences, error) {
	if cpanelcontact.PreferencesMatchDefinition(original, target) {
		return &original, nil
	}

	updated, err := r.client.SetNotificationPreferences(ctx, target)
	if err != nil {
		rollbackErr := r.rollbackFailedTransition(
			ctx,
			original,
			target,
			err,
		)
		return nil, errors.New(
			notificationPreferencesMutationErrorDetail(err, rollbackErr),
		)
	}

	return updated, nil
}

func (r *notificationPreferencesResource) rollbackFailedTransition(
	ctx context.Context,
	original cpanelcontact.NotificationPreferences,
	target map[string]bool,
	mutationErr error,
) error {
	current, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		return fmt.Errorf(
			"read cPanel notification preferences after failed mutation: %w",
			err,
		)
	}
	if cpanelcontact.PreferencesMatchDefinition(
		*current,
		original.Definition(),
	) {
		return nil
	}
	if notificationPreferencesMutationErrorIsDeterministic(mutationErr) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel notification preferences because cPanel rejected the mutation but the current settings changed independently",
		)
	}
	if !cpanelcontact.PreferencesMatchDefinition(*current, target) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel notification preferences because the current settings match neither the requested transition nor the previous configuration",
		)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		target,
		original.Definition(),
	)
}

func notificationPreferencesMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError

	return errors.As(err, &apiError)
}

func (r *notificationPreferencesResource) restoreIfCurrentMatches(
	ctx context.Context,
	expectedCurrent map[string]bool,
	target map[string]bool,
) error {
	if err := cpanelcontact.ValidateDefinition(expectedCurrent); err != nil {
		return err
	}
	if err := cpanelcontact.ValidateDefinition(target); err != nil {
		return err
	}

	current, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		return fmt.Errorf(
			"read cPanel notification preferences before guarded restore: %w",
			err,
		)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		expectedCurrent,
		target,
	)
}

func (r *notificationPreferencesResource) restoreObservedTransition(
	ctx context.Context,
	current cpanelcontact.NotificationPreferences,
	expectedCurrent map[string]bool,
	target map[string]bool,
) error {
	if cpanelcontact.PreferencesMatchDefinition(current, target) {
		return nil
	}
	if !cpanelcontact.PreferencesMatchDefinition(current, expectedCurrent) {
		return fmt.Errorf(
			"refuse to restore cPanel notification preferences because the current configuration no longer matches the Terraform transition",
		)
	}
	if _, err := r.client.SetNotificationPreferences(ctx, target); err != nil {
		return fmt.Errorf(
			"restore previous cPanel notification preferences: %w",
			err,
		)
	}

	return nil
}

func (r *notificationPreferencesResource) restore(
	ctx context.Context,
	target map[string]bool,
) error {
	if err := cpanelcontact.ValidateDefinition(target); err != nil {
		return err
	}

	current, err := r.client.GetNotificationPreferences(ctx)
	if err != nil {
		return fmt.Errorf(
			"read cPanel notification preferences before restore: %w",
			err,
		)
	}
	if cpanelcontact.PreferencesMatchDefinition(*current, target) {
		return nil
	}
	if _, err := r.client.SetNotificationPreferences(ctx, target); err != nil {
		return fmt.Errorf(
			"restore previous cPanel notification preferences: %w",
			err,
		)
	}

	return nil
}

func notificationPreferencesMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel notification preferences: %v",
		primaryError,
		rollbackError,
	)
}
