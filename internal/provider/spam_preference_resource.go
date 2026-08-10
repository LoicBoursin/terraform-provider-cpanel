package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelspam "terraform-provider-cpanel/internal/cpanel/spamassassin"
)

var (
	_ resource.Resource                = &spamPreferenceResource{}
	_ resource.ResourceWithConfigure   = &spamPreferenceResource{}
	_ resource.ResourceWithImportState = &spamPreferenceResource{}
)

func NewSpamPreferenceResource() resource.Resource {
	return &spamPreferenceResource{}
}

type spamPreferenceClient interface {
	LockPreference(string) func()
	GetPreference(context.Context, string) (*cpanelspam.Preference, error)
	SetPreference(
		context.Context,
		string,
		[]string,
	) (*cpanelspam.Preference, error)
	RemovePreference(
		context.Context,
		string,
	) (*cpanelspam.Preference, error)
}

type spamPreferenceResource struct {
	client spamPreferenceClient
}

func (r *spamPreferenceResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_spam_preference"
}

func (r *spamPreferenceResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages one documented cPanel SpamAssassin user preference.",
		MarkdownDescription: "Manages one documented cPanel SpamAssassin user preference. Removing the resource restores the exact presence and values observed when Terraform first took ownership. Arbitrary custom SpamAssassin configuration is deliberately unsupported.",
		Attributes: map[string]schema.Attribute{
			"preference": schema.StringAttribute{
				Required:            true,
				Description:         "The supported SpamAssassin preference name.",
				MarkdownDescription: "The supported SpamAssassin preference name: `required_score`, `score`, `whitelist_from`, or `blacklist_from`.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						cpanelspam.SupportedPreferenceNames()...,
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"values": schema.SetAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The complete value set for the preference.",
				MarkdownDescription: "The complete value set for the preference. `required_score` requires one number greater than zero and less than 1000. `score` accepts documented `RULE_NAME SCORE` entries. Allowlist and blocklist values must be complete email addresses without wildcards.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.NoNullValues(),
					setvalidator.ValueStringsAre(
						stringvalidator.LengthBetween(1, 512),
					),
				},
			},
			"configured": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the preference is explicitly configured.",
				MarkdownDescription: "Whether the preference is explicitly configured. A managed resource always reports `true`.",
			},
			"restore_present": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the preference existed before Terraform management.",
				MarkdownDescription: "Whether the preference existed before Terraform management.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_values": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The values that Terraform restores when the resource is removed.",
				MarkdownDescription: "The values that Terraform restores when the resource is removed. The set is empty when the preference was originally absent.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *spamPreferenceResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state SpamPreferenceResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetPreference(
		ctx,
		state.Preference.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}
	if !current.Present {
		response.State.RemoveResource(ctx)
		return
	}
	if spamPreferenceRestoreStateMissing(state) {
		response.Diagnostics.Append(
			applySpamPreferenceRestoreToResourceModel(
				ctx,
				&state,
				*current,
			)...,
		)
	}
	response.Diagnostics.Append(
		applySpamPreferenceToResourceModel(ctx, &state, *current)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *spamPreferenceResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan SpamPreferenceResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	target, diagnostics := spamPreferenceDefinitionFromResourceModel(ctx, plan)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := cpanelspam.ValidateDefinition(target); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockPreference(target.Name)
	defer unlock()

	original, err := r.client.GetPreference(ctx, target.Name)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}
	updated, err := r.transition(ctx, *original, target)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to configure cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		applySpamPreferenceRestoreToResourceModel(
			ctx,
			&plan,
			*original,
		)...,
	)
	response.Diagnostics.Append(
		applySpamPreferenceToResourceModel(ctx, &plan, *updated)...,
	)
	if response.Diagnostics.HasError() {
		r.appendRollbackDiagnostic(
			ctx,
			&response.Diagnostics,
			target,
			original.Definition(),
		)
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		r.appendRollbackDiagnostic(
			ctx,
			&response.Diagnostics,
			target,
			original.Definition(),
		)
	}
}

func (r *spamPreferenceResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan SpamPreferenceResourceModel
	var state SpamPreferenceResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	target, diagnostics := spamPreferenceDefinitionFromResourceModel(ctx, plan)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := cpanelspam.ValidateDefinition(target); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockPreference(target.Name)
	defer unlock()

	original, err := r.client.GetPreference(ctx, target.Name)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}
	expected, expectedDiagnostics :=
		spamPreferenceDefinitionFromResourceModel(ctx, state)
	response.Diagnostics.Append(expectedDiagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if !cpanelspam.PreferenceMatchesDefinition(*original, expected) {
		response.Diagnostics.AddError(
			"cPanel SpamAssassin preference changed during update",
			"The current cPanel SpamAssassin preference no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}
	updated, err := r.transition(ctx, *original, target)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}

	if spamPreferenceRestoreStateMissing(state) {
		response.Diagnostics.Append(
			applySpamPreferenceRestoreToResourceModel(
				ctx,
				&plan,
				*original,
			)...,
		)
	} else {
		plan.RestorePresent = state.RestorePresent
		plan.RestoreValues = state.RestoreValues
	}
	response.Diagnostics.Append(
		applySpamPreferenceToResourceModel(ctx, &plan, *updated)...,
	)
	if response.Diagnostics.HasError() {
		r.appendRollbackDiagnostic(
			ctx,
			&response.Diagnostics,
			target,
			original.Definition(),
		)
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		r.appendRollbackDiagnostic(
			ctx,
			&response.Diagnostics,
			target,
			original.Definition(),
		)
	}
}

func (r *spamPreferenceResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state SpamPreferenceResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}
	if spamPreferenceRestoreStateMissing(state) {
		response.Diagnostics.AddError(
			"Unable to restore cPanel SpamAssassin preference",
			"The resource state does not contain the preference presence and values that preceded Terraform management.",
		)
		return
	}

	managed, diagnostics := spamPreferenceDefinitionFromResourceModel(ctx, state)
	response.Diagnostics.Append(diagnostics...)
	restore, restoreDiagnostics :=
		spamPreferenceRestoreDefinitionFromResourceModel(ctx, state)
	response.Diagnostics.Append(restoreDiagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := cpanelspam.ValidateDefinition(managed); err != nil {
		response.Diagnostics.AddError(
			"Invalid managed cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}
	if err := cpanelspam.ValidateDefinition(restore); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel SpamAssassin preference restore state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockPreference(managed.Name)
	defer unlock()

	if err := r.restoreIfCurrentMatches(ctx, managed, restore); err != nil {
		response.Diagnostics.AddError(
			"Unable to restore cPanel SpamAssassin preference",
			err.Error(),
		)
	}
}

func (r *spamPreferenceResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if err := cpanelspam.ValidateName(request.ID); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel SpamAssassin preference import identifier",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockPreference(request.ID)
	defer unlock()

	current, err := r.client.GetPreference(ctx, request.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import cPanel SpamAssassin preference",
			err.Error(),
		)
		return
	}
	if !current.Present {
		response.Diagnostics.AddError(
			"Unable to import cPanel SpamAssassin preference",
			fmt.Sprintf(
				"SpamAssassin preference %q is not configured.",
				request.ID,
			),
		)
		return
	}

	state := SpamPreferenceResourceModel{}
	response.Diagnostics.Append(
		applySpamPreferenceRestoreToResourceModel(
			ctx,
			&state,
			*current,
		)...,
	)
	response.Diagnostics.Append(
		applySpamPreferenceToResourceModel(ctx, &state, *current)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *spamPreferenceResource) Configure(
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

	client, ok := providerData["spamassassin"].(*cpanelspam.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SpamAssassin Client Type",
			fmt.Sprintf(
				"Expected *spamassassin.Client, got: %T.",
				providerData["spamassassin"],
			),
		)
		return
	}

	r.client = client
}

func (r *spamPreferenceResource) transition(
	ctx context.Context,
	original cpanelspam.Preference,
	target cpanelspam.Definition,
) (*cpanelspam.Preference, error) {
	if cpanelspam.PreferenceMatchesDefinition(original, target) {
		return &original, nil
	}

	updated, err := r.applyDefinition(ctx, target)
	if err != nil {
		rollbackErr := r.rollbackFailedTransition(
			ctx,
			original,
			target,
			err,
		)

		return nil, errors.New(
			spamPreferenceMutationErrorDetail(err, rollbackErr),
		)
	}

	return updated, nil
}

func (r *spamPreferenceResource) rollbackFailedTransition(
	ctx context.Context,
	original cpanelspam.Preference,
	target cpanelspam.Definition,
	mutationErr error,
) error {
	current, err := r.client.GetPreference(ctx, target.Name)
	if err != nil {
		return fmt.Errorf(
			"read cPanel SpamAssassin preference after failed mutation: %w",
			err,
		)
	}
	if cpanelspam.PreferenceMatchesDefinition(
		*current,
		original.Definition(),
	) {
		return nil
	}
	if spamPreferenceMutationErrorIsDeterministic(mutationErr) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel SpamAssassin preference because cPanel rejected the mutation but the current value changed independently",
		)
	}
	if !cpanelspam.PreferenceMatchesDefinition(*current, target) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel SpamAssassin preference because the current value matches neither the requested transition nor the previous configuration",
		)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		target,
		original.Definition(),
	)
}

func spamPreferenceMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError

	return errors.As(err, &apiError)
}

func (r *spamPreferenceResource) restoreIfCurrentMatches(
	ctx context.Context,
	expectedCurrent cpanelspam.Definition,
	target cpanelspam.Definition,
) error {
	if err := cpanelspam.ValidateDefinition(expectedCurrent); err != nil {
		return err
	}
	if err := cpanelspam.ValidateDefinition(target); err != nil {
		return err
	}

	current, err := r.client.GetPreference(ctx, expectedCurrent.Name)
	if err != nil {
		return fmt.Errorf(
			"read cPanel SpamAssassin preference before guarded restore: %w",
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

func (r *spamPreferenceResource) restoreObservedTransition(
	ctx context.Context,
	current cpanelspam.Preference,
	expectedCurrent cpanelspam.Definition,
	target cpanelspam.Definition,
) error {
	if cpanelspam.PreferenceMatchesDefinition(current, target) {
		return nil
	}
	if !cpanelspam.PreferenceMatchesDefinition(current, expectedCurrent) {
		return fmt.Errorf(
			"refuse to restore cPanel SpamAssassin preference %q because its current value no longer matches the Terraform transition",
			expectedCurrent.Name,
		)
	}
	if _, err := r.applyDefinition(ctx, target); err != nil {
		return fmt.Errorf(
			"restore previous cPanel SpamAssassin preference: %w",
			err,
		)
	}

	return nil
}

func (r *spamPreferenceResource) applyDefinition(
	ctx context.Context,
	definition cpanelspam.Definition,
) (*cpanelspam.Preference, error) {
	if err := cpanelspam.ValidateDefinition(definition); err != nil {
		return nil, err
	}
	if definition.Present {
		return r.client.SetPreference(
			ctx,
			definition.Name,
			definition.Values,
		)
	}

	return r.client.RemovePreference(ctx, definition.Name)
}

func (r *spamPreferenceResource) appendRollbackDiagnostic(
	ctx context.Context,
	diagnostics *diag.Diagnostics,
	expectedCurrent cpanelspam.Definition,
	target cpanelspam.Definition,
) {
	if rollbackErr := r.restoreIfCurrentMatches(
		ctx,
		expectedCurrent,
		target,
	); rollbackErr != nil {
		diagnostics.AddError(
			"Unable to restore cPanel SpamAssassin preference",
			rollbackErr.Error(),
		)
	}
}

func spamPreferenceMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel SpamAssassin preference: %v",
		primaryError,
		rollbackError,
	)
}
