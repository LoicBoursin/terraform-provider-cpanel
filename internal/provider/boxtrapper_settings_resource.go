package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelboxtrapper "terraform-provider-cpanel/internal/cpanel/boxtrapper"
)

var (
	_ resource.Resource                = &boxTrapperSettingsResource{}
	_ resource.ResourceWithConfigure   = &boxTrapperSettingsResource{}
	_ resource.ResourceWithImportState = &boxTrapperSettingsResource{}
)

func NewBoxTrapperSettingsResource() resource.Resource {
	return &boxTrapperSettingsResource{}
}

type boxTrapperSettingsClient interface {
	Get(
		context.Context,
		string,
	) (*cpanelboxtrapper.Settings, error)
	SaveConfiguration(
		context.Context,
		string,
		cpanelboxtrapper.Definition,
		*string,
	) (*cpanelboxtrapper.Settings, []string, error)
	SetStatus(
		context.Context,
		string,
		bool,
	) (*cpanelboxtrapper.Settings, []string, error)
	LockAccount(string) func()
}

type boxTrapperSettingsResource struct {
	client boxTrapperSettingsClient
}

func (r *boxTrapperSettingsResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_boxtrapper_settings"
}

func (r *boxTrapperSettingsResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages BoxTrapper status and configuration for an existing cPanel mail account.",
		MarkdownDescription: "Manages BoxTrapper status and configuration for an existing cPanel mail account. The account can be a complete mailbox address or the cPanel system username. The `enabled` status can always be managed. Changing any configuration attribute requires cPanel to report a non-null `from_name`, which Terraform preserves exactly; otherwise the provider refuses the change because cPanel would irreversibly replace null with an empty string. Removing the resource restores the managed settings observed when Terraform first took ownership and never deletes the mail account or queued messages.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Required:            true,
				Description:         "The existing mailbox address or cPanel system username.",
				MarkdownDescription: "The existing mailbox address or cPanel system username.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 254),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether BoxTrapper is enabled for the account.",
				MarkdownDescription: "Whether BoxTrapper is enabled for the account.",
			},
			"enable_auto_whitelist": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "Whether BoxTrapper automatically allowlists addresses that the account contacts.",
				MarkdownDescription: "Whether BoxTrapper automatically allowlists addresses that the account contacts. When omitted on first creation, Terraform preserves the current cPanel value.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"from_addresses": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "The comma-separated sender addresses BoxTrapper associates with the account.",
				MarkdownDescription: "The comma-separated sender addresses BoxTrapper associates with the account. When omitted on first creation, Terraform preserves the current cPanel value.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"from_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The optional sender name reported by cPanel.",
				MarkdownDescription: "The optional sender name reported by cPanel. Terraform observes but does not manage this value. When it is null, Terraform can change only `enabled`; configuration changes are refused because cPanel would replace null with an empty string.",
			},
			"queue_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Description:         "The number of days BoxTrapper retains logs and queued messages.",
				MarkdownDescription: "The number of days BoxTrapper retains logs and queued messages. When omitted on first creation, Terraform preserves the current cPanel value.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"spam_score": schema.Float64Attribute{
				Optional:            true,
				Computed:            true,
				Description:         "The SpamAssassin score at which BoxTrapper bypasses challenge verification.",
				MarkdownDescription: "The SpamAssassin score at which BoxTrapper bypasses challenge verification. cPanel stores one decimal place. When omitted on first creation, Terraform preserves the current cPanel value.",
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.UseStateForUnknown(),
				},
			},
			"whitelist_by_association": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "Whether BoxTrapper allowlists recipients associated with an allowlisted sender.",
				MarkdownDescription: "Whether BoxTrapper allowlists recipients associated with an allowlisted sender. When omitted on first creation, Terraform preserves the current cPanel value.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_enabled": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether BoxTrapper was enabled before Terraform management.",
				MarkdownDescription: "Whether BoxTrapper was enabled before Terraform management.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_enable_auto_whitelist": schema.BoolAttribute{
				Computed:            true,
				Description:         "The automatic allowlist setting Terraform restores when the resource is removed.",
				MarkdownDescription: "The automatic allowlist setting Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_from_addresses": schema.StringAttribute{
				Computed:            true,
				Description:         "The sender address list Terraform restores when the resource is removed.",
				MarkdownDescription: "The sender address list Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_queue_days": schema.Int64Attribute{
				Computed:            true,
				Description:         "The queue retention period Terraform restores when the resource is removed.",
				MarkdownDescription: "The queue retention period Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"restore_spam_score": schema.Float64Attribute{
				Computed:            true,
				Description:         "The SpamAssassin score Terraform restores when the resource is removed.",
				MarkdownDescription: "The SpamAssassin score Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.UseStateForUnknown(),
				},
			},
			"restore_whitelist_by_association": schema.BoolAttribute{
				Computed:            true,
				Description:         "The associated-recipient allowlist setting Terraform restores when the resource is removed.",
				MarkdownDescription: "The associated-recipient allowlist setting Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *boxTrapperSettingsResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state BoxTrapperSettingsResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	account := state.Account.ValueString()
	if err := validateBoxTrapperAccount(account); err != nil {
		response.Diagnostics.AddError(
			"Invalid BoxTrapper account in state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockAccount(account)
	defer unlock()

	current, err := r.client.Get(ctx, account)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	if current == nil {
		response.State.RemoveResource(ctx)
		return
	}
	if boxTrapperRestoreStateMissing(state) {
		applyBoxTrapperRestoreToResourceModel(&state, *current)
	}
	applyBoxTrapperSettingsToResourceModel(&state, *current)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *boxTrapperSettingsResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan BoxTrapperSettingsResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	account := plan.Account.ValueString()
	if err := validateBoxTrapperAccount(account); err != nil {
		response.Diagnostics.AddError("Invalid BoxTrapper account", err.Error())
		return
	}

	unlock := r.client.LockAccount(account)
	defer unlock()

	original, err := r.client.Get(ctx, account)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	if original == nil {
		response.Diagnostics.AddError(
			"cPanel BoxTrapper account not found",
			fmt.Sprintf(
				"Create mail account %q before managing its BoxTrapper settings.",
				account,
			),
		)
		return
	}

	desired := boxTrapperDefinitionFromResourceModel(plan, *original)
	if err := cpanelboxtrapper.ValidateDefinition(desired); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	updated, warnings, err := r.transition(ctx, *original, desired)
	addBoxTrapperWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to configure cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}

	applyBoxTrapperRestoreToResourceModel(&plan, *original)
	applyBoxTrapperSettingsToResourceModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() &&
		!cpanelboxtrapper.SettingsMatchDefinition(*original, desired) {
		rollbackWarnings, rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			account,
			desired,
			original.Definition(),
		)
		addBoxTrapperWarnings(&response.Diagnostics, rollbackWarnings)
		if rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel BoxTrapper settings",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *boxTrapperSettingsResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan BoxTrapperSettingsResourceModel
	var state BoxTrapperSettingsResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	account := plan.Account.ValueString()
	if err := validateBoxTrapperAccount(account); err != nil {
		response.Diagnostics.AddError("Invalid BoxTrapper account", err.Error())
		return
	}

	unlock := r.client.LockAccount(account)
	defer unlock()

	original, err := r.client.Get(ctx, account)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	if original == nil {
		response.Diagnostics.AddError(
			"cPanel BoxTrapper account no longer exists",
			fmt.Sprintf(
				"Create mail account %q before updating its BoxTrapper settings.",
				account,
			),
		)
		return
	}

	desired := boxTrapperDefinitionFromResourceModel(plan, *original)
	if err := cpanelboxtrapper.ValidateDefinition(desired); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	expected := boxTrapperDefinitionFromResourceModel(state, *original)
	if !cpanelboxtrapper.SettingsMatchDefinition(*original, expected) {
		response.Diagnostics.AddError(
			"cPanel BoxTrapper settings changed during update",
			"The current cPanel BoxTrapper settings no longer match Terraform state, so the provider refuses to overwrite them. Refresh and review the drift before retrying.",
		)
		return
	}

	updated, warnings, err := r.transition(ctx, *original, desired)
	addBoxTrapperWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}

	if boxTrapperRestoreStateMissing(state) {
		applyBoxTrapperRestoreToResourceModel(&plan, *original)
	} else {
		copyBoxTrapperRestoreState(&plan, state)
	}
	applyBoxTrapperSettingsToResourceModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() &&
		!cpanelboxtrapper.SettingsMatchDefinition(*original, desired) {
		rollbackWarnings, rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			account,
			desired,
			original.Definition(),
		)
		addBoxTrapperWarnings(&response.Diagnostics, rollbackWarnings)
		if rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel BoxTrapper settings",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *boxTrapperSettingsResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state BoxTrapperSettingsResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}
	if boxTrapperRestoreStateMissing(state) {
		response.Diagnostics.AddError(
			"Unable to restore cPanel BoxTrapper settings",
			"The resource state does not contain the complete BoxTrapper settings that preceded Terraform management.",
		)
		return
	}

	account := state.Account.ValueString()
	if err := validateBoxTrapperAccount(account); err != nil {
		response.Diagnostics.AddError(
			"Invalid BoxTrapper account in state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockAccount(account)
	defer unlock()

	warnings, err := r.restoreIfCurrentMatches(
		ctx,
		account,
		boxTrapperCurrentDefinitionFromResourceModel(state),
		boxTrapperRestoreDefinitionFromResourceModel(state),
	)
	addBoxTrapperWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to restore cPanel BoxTrapper settings",
			err.Error(),
		)
	}
}

func (r *boxTrapperSettingsResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if err := validateBoxTrapperAccount(request.ID); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel BoxTrapper settings import identifier",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockAccount(request.ID)
	defer unlock()

	current, err := r.client.Get(ctx, request.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import cPanel BoxTrapper settings",
			err.Error(),
		)
		return
	}
	if current == nil {
		response.Diagnostics.AddError(
			"cPanel BoxTrapper account not found",
			fmt.Sprintf(
				"Account %q is not available to BoxTrapper on this cPanel account.",
				request.ID,
			),
		)
		return
	}

	state := BoxTrapperSettingsResourceModel{}
	applyBoxTrapperRestoreToResourceModel(&state, *current)
	applyBoxTrapperSettingsToResourceModel(&state, *current)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *boxTrapperSettingsResource) Configure(
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

	client, ok := providerData["boxtrapper"].(*cpanelboxtrapper.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected BoxTrapper Client Type",
			fmt.Sprintf(
				"Expected *boxtrapper.Client, got: %T.",
				providerData["boxtrapper"],
			),
		)
		return
	}

	r.client = client
}

func (r *boxTrapperSettingsResource) transition(
	ctx context.Context,
	original cpanelboxtrapper.Settings,
	target cpanelboxtrapper.Definition,
) (*cpanelboxtrapper.Settings, []string, error) {
	if cpanelboxtrapper.SettingsMatchDefinition(original, target) {
		return &original, nil, nil
	}

	current := original
	var warnings []string

	configurationTarget := target
	configurationTarget.Enabled = current.Enabled
	if !cpanelboxtrapper.SettingsMatchDefinition(
		current,
		configurationTarget,
	) {
		expected := boxTrapperSettingsWithDefinition(
			current,
			configurationTarget,
		)
		updated, operationWarnings, err := r.client.SaveConfiguration(
			ctx,
			original.Account,
			configurationTarget,
			current.FromName,
		)
		warnings = append(warnings, operationWarnings...)
		if err != nil {
			rollbackWarnings, rollbackErr := r.rollbackFailedMutation(
				ctx,
				original,
				current,
				expected,
				err,
			)
			warnings = append(warnings, rollbackWarnings...)
			return nil, warnings, errors.New(
				boxTrapperMutationErrorDetail(err, rollbackErr),
			)
		}
		current = *updated
	}

	if current.Enabled != target.Enabled {
		expected := current
		expected.Enabled = target.Enabled
		updated, operationWarnings, err := r.client.SetStatus(
			ctx,
			original.Account,
			target.Enabled,
		)
		warnings = append(warnings, operationWarnings...)
		if err != nil {
			rollbackWarnings, rollbackErr := r.rollbackFailedMutation(
				ctx,
				original,
				current,
				expected,
				err,
			)
			warnings = append(warnings, rollbackWarnings...)
			return nil, warnings, errors.New(
				boxTrapperMutationErrorDetail(err, rollbackErr),
			)
		}
		current = *updated
	}

	if !cpanelboxtrapper.SettingsMatchDefinition(current, target) {
		rollbackWarnings, rollbackErr := r.restoreObservedTransition(
			ctx,
			current,
			current.Definition(),
			original.Definition(),
		)
		warnings = append(warnings, rollbackWarnings...)
		return nil, warnings, errors.New(boxTrapperMutationErrorDetail(
			fmt.Errorf(
				"cPanel BoxTrapper settings do not match the requested configuration after mutation",
			),
			rollbackErr,
		))
	}

	return &current, warnings, nil
}

func (r *boxTrapperSettingsResource) rollbackFailedMutation(
	ctx context.Context,
	original cpanelboxtrapper.Settings,
	before cpanelboxtrapper.Settings,
	expected cpanelboxtrapper.Settings,
	mutationErr error,
) ([]string, error) {
	current, err := r.client.Get(ctx, original.Account)
	if err != nil {
		return nil, fmt.Errorf(
			"read cPanel BoxTrapper settings after failed mutation: %w",
			err,
		)
	}
	if current == nil {
		return nil, fmt.Errorf(
			"refuse to restore previous BoxTrapper settings because account %q disappeared after the failed mutation",
			original.Account,
		)
	}
	if boxTrapperSettingsMatchSettings(*current, original) {
		return nil, nil
	}

	currentMatchesBefore := boxTrapperSettingsMatchSettings(*current, before)
	currentMatchesExpected := boxTrapperSettingsMatchSettings(
		*current,
		expected,
	)
	if boxTrapperMutationErrorIsDeterministic(mutationErr) {
		if currentMatchesBefore {
			if boxTrapperSettingsMatchSettings(before, original) {
				return nil, nil
			}

			return r.restoreObservedTransition(
				ctx,
				*current,
				before.Definition(),
				original.Definition(),
			)
		}

		return nil, fmt.Errorf(
			"refuse to restore previous BoxTrapper settings because cPanel rejected the mutation but the current settings changed independently",
		)
	}

	if currentMatchesBefore {
		if boxTrapperSettingsMatchSettings(before, original) {
			return nil, nil
		}

		return r.restoreObservedTransition(
			ctx,
			*current,
			before.Definition(),
			original.Definition(),
		)
	}
	if currentMatchesExpected {
		return r.restoreObservedTransition(
			ctx,
			*current,
			expected.Definition(),
			original.Definition(),
		)
	}

	return nil, fmt.Errorf(
		"refuse to restore previous BoxTrapper settings because the current settings match neither the requested mutation nor the preceding configuration",
	)
}

func boxTrapperMutationErrorIsDeterministic(err error) bool {
	var verificationError *cpanelboxtrapper.MutationVerificationError
	if errors.As(err, &verificationError) {
		return false
	}

	var apiError *cpanelapi.APIError

	return errors.As(err, &apiError)
}

func (r *boxTrapperSettingsResource) restoreIfCurrentMatches(
	ctx context.Context,
	account string,
	expectedCurrent cpanelboxtrapper.Definition,
	target cpanelboxtrapper.Definition,
) ([]string, error) {
	if err := cpanelboxtrapper.ValidateDefinition(expectedCurrent); err != nil {
		return nil, err
	}
	if err := cpanelboxtrapper.ValidateDefinition(target); err != nil {
		return nil, err
	}

	current, err := r.client.Get(ctx, account)
	if err != nil {
		return nil, fmt.Errorf(
			"read cPanel BoxTrapper settings before guarded restore: %w",
			err,
		)
	}
	if current == nil {
		return nil, nil
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		expectedCurrent,
		target,
	)
}

func (r *boxTrapperSettingsResource) restoreObservedTransition(
	ctx context.Context,
	current cpanelboxtrapper.Settings,
	expectedCurrent cpanelboxtrapper.Definition,
	target cpanelboxtrapper.Definition,
) ([]string, error) {
	if cpanelboxtrapper.SettingsMatchDefinition(current, target) {
		return nil, nil
	}
	if !cpanelboxtrapper.SettingsMatchDefinition(
		current,
		expectedCurrent,
	) {
		return nil, fmt.Errorf(
			"refuse to restore cPanel BoxTrapper settings because the current configuration no longer matches the Terraform transition",
		)
	}

	restored, warnings, err := r.applyDefinition(ctx, current, target)
	if err != nil {
		return warnings, fmt.Errorf(
			"restore previous cPanel BoxTrapper settings: %w",
			err,
		)
	}
	if !cpanelboxtrapper.SettingsMatchDefinition(*restored, target) {
		return warnings, fmt.Errorf(
			"restored cPanel BoxTrapper settings do not match the previous configuration",
		)
	}

	return warnings, nil
}

func (r *boxTrapperSettingsResource) restore(
	ctx context.Context,
	account string,
	target cpanelboxtrapper.Definition,
) ([]string, error) {
	if err := cpanelboxtrapper.ValidateDefinition(target); err != nil {
		return nil, err
	}

	current, err := r.client.Get(ctx, account)
	if err != nil {
		return nil, fmt.Errorf(
			"read cPanel BoxTrapper settings before restore: %w",
			err,
		)
	}
	if current == nil ||
		cpanelboxtrapper.SettingsMatchDefinition(*current, target) {
		return nil, nil
	}

	_, warnings, err := r.applyDefinition(ctx, *current, target)
	if err != nil {
		return warnings, fmt.Errorf(
			"restore previous cPanel BoxTrapper settings: %w",
			err,
		)
	}

	return warnings, nil
}

func (r *boxTrapperSettingsResource) applyDefinition(
	ctx context.Context,
	current cpanelboxtrapper.Settings,
	target cpanelboxtrapper.Definition,
) (*cpanelboxtrapper.Settings, []string, error) {
	var warnings []string

	configurationTarget := target
	configurationTarget.Enabled = current.Enabled
	if !cpanelboxtrapper.SettingsMatchDefinition(
		current,
		configurationTarget,
	) {
		updated, operationWarnings, err := r.client.SaveConfiguration(
			ctx,
			current.Account,
			configurationTarget,
			current.FromName,
		)
		warnings = append(warnings, operationWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		current = *updated
	}

	if current.Enabled != target.Enabled {
		updated, operationWarnings, err := r.client.SetStatus(
			ctx,
			current.Account,
			target.Enabled,
		)
		warnings = append(warnings, operationWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		current = *updated
	}

	return &current, warnings, nil
}

func boxTrapperSettingsWithDefinition(
	settings cpanelboxtrapper.Settings,
	definition cpanelboxtrapper.Definition,
) cpanelboxtrapper.Settings {
	settings.Enabled = definition.Enabled
	settings.EnableAutoWhitelist = definition.EnableAutoWhitelist
	settings.FromAddresses = definition.FromAddresses
	settings.QueueDays = definition.QueueDays
	settings.SpamScore = definition.SpamScore
	settings.WhitelistByAssociation =
		definition.WhitelistByAssociation

	return settings
}

func boxTrapperSettingsMatchSettings(
	first cpanelboxtrapper.Settings,
	second cpanelboxtrapper.Settings,
) bool {
	return first.Account == second.Account &&
		cpanelboxtrapper.SettingsMatchDefinition(
			first,
			second.Definition(),
		)
}

func copyBoxTrapperRestoreState(
	target *BoxTrapperSettingsResourceModel,
	source BoxTrapperSettingsResourceModel,
) {
	target.RestoreEnabled = source.RestoreEnabled
	target.RestoreEnableAutoWhitelist =
		source.RestoreEnableAutoWhitelist
	target.RestoreFromAddresses = source.RestoreFromAddresses
	target.RestoreQueueDays = source.RestoreQueueDays
	target.RestoreSpamScore = source.RestoreSpamScore
	target.RestoreWhitelistByAssociation =
		source.RestoreWhitelistByAssociation
}

func addBoxTrapperWarnings(
	diagnostics interface {
		AddWarning(string, string)
	},
	warnings []string,
) {
	for _, warning := range warnings {
		diagnostics.AddWarning("cPanel BoxTrapper warning", warning)
	}
}

func boxTrapperMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel BoxTrapper settings: %v",
		primaryError,
		rollbackError,
	)
}
