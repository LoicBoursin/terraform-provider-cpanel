package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
)

var (
	_ resource.Resource                = &logSettingsResource{}
	_ resource.ResourceWithConfigure   = &logSettingsResource{}
	_ resource.ResourceWithImportState = &logSettingsResource{}
)

func NewLogSettingsResource() resource.Resource {
	return &logSettingsResource{}
}

type logSettingsClient interface {
	Get(context.Context) (*cpanellogmanager.Settings, error)
	Set(
		context.Context,
		cpanellogmanager.Definition,
	) (*cpanellogmanager.Settings, error)
}

type logSettingsResource struct {
	client logSettingsClient
}

func (r *logSettingsResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_log_settings"
}

func (r *logSettingsResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages the cPanel account's raw access log archival settings.",
		MarkdownDescription: "Manages the cPanel account's raw access log archival settings. Removing the resource restores the complete settings observed when Terraform first took ownership.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"archive_logs": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether cPanel archives processed access logs in the account home directory.",
				MarkdownDescription: "Whether cPanel archives processed access logs in the account home directory.",
			},
			"prune_archives": schema.BoolAttribute{
				Required:            true,
				Description:         "Whether cPanel removes the previous month's archived logs.",
				MarkdownDescription: "Whether cPanel removes the previous month's archived logs.",
			},
			"retention_days": schema.Int64Attribute{
				Required:            true,
				Description:         "The configured retention period. -1 uses the server default, 0 retains logs indefinitely, and positive values are days.",
				MarkdownDescription: "The configured retention period. `-1` uses the server default, `0` retains logs indefinitely, and positive values are days.",
				Validators: []validator.Int64{
					int64validator.AtLeast(-1),
				},
			},
			"effective_retention_days": schema.Int64Attribute{
				Computed:            true,
				Description:         "The effective number of retention days reported by cPanel.",
				MarkdownDescription: "The effective number of retention days reported by cPanel. `0` means indefinitely.",
			},
			"using_default_retention": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the account uses the server-wide retention period.",
				MarkdownDescription: "Whether the account uses the server-wide retention period.",
			},
			"restore_archive_logs": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether log archival was enabled before Terraform management.",
				MarkdownDescription: "Whether log archival was enabled before Terraform management.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_prune_archives": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether archive pruning was enabled before Terraform management.",
				MarkdownDescription: "Whether archive pruning was enabled before Terraform management.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"restore_retention_days": schema.Int64Attribute{
				Computed:            true,
				Description:         "The retention configuration that Terraform restores when the resource is removed.",
				MarkdownDescription: "The retention configuration that Terraform restores when the resource is removed. `-1` means the server default.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *logSettingsResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state LogSettingsResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	current, err := r.client.Get(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel log settings",
			err.Error(),
		)
		return
	}
	if logSettingsRestoreStateMissing(state) {
		applyLogSettingsRestoreToResourceModel(&state, *current)
	}
	applyLogSettingsToResourceModel(&state, *current)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *logSettingsResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan LogSettingsResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired := logSettingsDefinitionFromResourceModel(plan)
	if err := cpanellogmanager.ValidateDefinition(desired); err != nil {
		response.Diagnostics.AddError("Invalid cPanel log settings", err.Error())
		return
	}

	original, err := r.client.Get(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel log settings",
			err.Error(),
		)
		return
	}
	updated, err := r.transition(ctx, *original, desired)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to configure cPanel log settings",
			err.Error(),
		)
		return
	}

	applyLogSettingsRestoreToResourceModel(&plan, *original)
	applyLogSettingsToResourceModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() &&
		!cpanellogmanager.SettingsMatchDefinition(*original, desired) {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			desired,
			original.Definition(),
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel log settings",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *logSettingsResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan LogSettingsResourceModel
	var state LogSettingsResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	desired := logSettingsDefinitionFromResourceModel(plan)
	if err := cpanellogmanager.ValidateDefinition(desired); err != nil {
		response.Diagnostics.AddError("Invalid cPanel log settings", err.Error())
		return
	}

	original, err := r.client.Get(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel log settings",
			err.Error(),
		)
		return
	}
	if !cpanellogmanager.SettingsMatchDefinition(
		*original,
		logSettingsDefinitionFromResourceModel(state),
	) {
		response.Diagnostics.AddError(
			"cPanel log settings changed during update",
			"The current cPanel log settings no longer match Terraform state, so the provider refuses to overwrite them. Refresh and review the drift before retrying.",
		)
		return
	}
	updated, err := r.transition(ctx, *original, desired)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update cPanel log settings",
			err.Error(),
		)
		return
	}

	if logSettingsRestoreStateMissing(state) {
		applyLogSettingsRestoreToResourceModel(&plan, *original)
	} else {
		plan.RestoreArchiveLogs = state.RestoreArchiveLogs
		plan.RestorePruneArchives = state.RestorePruneArchives
		plan.RestoreRetentionDays = state.RestoreRetentionDays
	}
	applyLogSettingsToResourceModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() &&
		!cpanellogmanager.SettingsMatchDefinition(*original, desired) {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			desired,
			original.Definition(),
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel log settings",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *logSettingsResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state LogSettingsResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}
	if logSettingsRestoreStateMissing(state) {
		response.Diagnostics.AddError(
			"Unable to restore cPanel log settings",
			"The resource state does not contain the complete log settings that preceded Terraform management.",
		)
		return
	}

	if err := r.restoreIfCurrentMatches(
		ctx,
		logSettingsDefinitionFromResourceModel(state),
		logSettingsRestoreDefinitionFromResourceModel(state),
	); err != nil {
		response.Diagnostics.AddError(
			"Unable to restore cPanel log settings",
			err.Error(),
		)
	}
}

func (r *logSettingsResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if request.ID != cpanellogmanager.AccountIdentity {
		response.Diagnostics.AddError(
			"Invalid cPanel log settings import identifier",
			fmt.Sprintf(
				"Use %q to import the account log settings.",
				cpanellogmanager.AccountIdentity,
			),
		)
		return
	}

	current, err := r.client.Get(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import cPanel log settings",
			err.Error(),
		)
		return
	}

	state := LogSettingsResourceModel{}
	applyLogSettingsRestoreToResourceModel(&state, *current)
	applyLogSettingsToResourceModel(&state, *current)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *logSettingsResource) Configure(
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

	client, ok := providerData["logmanager"].(*cpanellogmanager.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected LogManager Client Type",
			fmt.Sprintf(
				"Expected *logmanager.Client, got: %T.",
				providerData["logmanager"],
			),
		)
		return
	}

	r.client = client
}

func (r *logSettingsResource) transition(
	ctx context.Context,
	original cpanellogmanager.Settings,
	target cpanellogmanager.Definition,
) (*cpanellogmanager.Settings, error) {
	if cpanellogmanager.SettingsMatchDefinition(original, target) {
		return &original, nil
	}

	updated, err := r.client.Set(ctx, target)
	if err != nil {
		rollbackErr := r.rollbackFailedTransition(
			ctx,
			original,
			target,
			err,
		)
		return nil, errors.New(logSettingsMutationErrorDetail(err, rollbackErr))
	}

	return updated, nil
}

func (r *logSettingsResource) rollbackFailedTransition(
	ctx context.Context,
	original cpanellogmanager.Settings,
	target cpanellogmanager.Definition,
	mutationErr error,
) error {
	current, err := r.client.Get(ctx)
	if err != nil {
		return fmt.Errorf(
			"read cPanel log settings after failed mutation: %w",
			err,
		)
	}
	if cpanellogmanager.SettingsMatchDefinition(
		*current,
		original.Definition(),
	) {
		return nil
	}
	if logSettingsMutationErrorIsDeterministic(mutationErr) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel log settings because cPanel rejected the mutation but the current settings changed independently",
		)
	}
	if !cpanellogmanager.SettingsMatchDefinition(*current, target) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel log settings because the current settings match neither the requested transition nor the previous configuration",
		)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		target,
		original.Definition(),
	)
}

func logSettingsMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError

	return errors.As(err, &apiError)
}

func (r *logSettingsResource) restoreIfCurrentMatches(
	ctx context.Context,
	expectedCurrent cpanellogmanager.Definition,
	target cpanellogmanager.Definition,
) error {
	if err := cpanellogmanager.ValidateDefinition(expectedCurrent); err != nil {
		return err
	}
	if err := cpanellogmanager.ValidateDefinition(target); err != nil {
		return err
	}

	current, err := r.client.Get(ctx)
	if err != nil {
		return fmt.Errorf("read cPanel log settings before guarded restore: %w", err)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		expectedCurrent,
		target,
	)
}

func (r *logSettingsResource) restoreObservedTransition(
	ctx context.Context,
	current cpanellogmanager.Settings,
	expectedCurrent cpanellogmanager.Definition,
	target cpanellogmanager.Definition,
) error {
	if cpanellogmanager.SettingsMatchDefinition(current, target) {
		return nil
	}
	if !cpanellogmanager.SettingsMatchDefinition(
		current,
		expectedCurrent,
	) {
		return fmt.Errorf(
			"refuse to restore cPanel log settings because the current configuration no longer matches the Terraform transition",
		)
	}
	if _, err := r.client.Set(ctx, target); err != nil {
		return fmt.Errorf("restore previous cPanel log settings: %w", err)
	}

	return nil
}

func (r *logSettingsResource) restore(
	ctx context.Context,
	target cpanellogmanager.Definition,
) error {
	if err := cpanellogmanager.ValidateDefinition(target); err != nil {
		return err
	}

	current, err := r.client.Get(ctx)
	if err != nil {
		return fmt.Errorf("read cPanel log settings before restore: %w", err)
	}
	if cpanellogmanager.SettingsMatchDefinition(*current, target) {
		return nil
	}
	if _, err := r.client.Set(ctx, target); err != nil {
		return fmt.Errorf("restore previous cPanel log settings: %w", err)
	}

	return nil
}

func logSettingsRestoreStateMissing(
	state LogSettingsResourceModel,
) bool {
	return state.RestoreArchiveLogs.IsNull() ||
		state.RestoreArchiveLogs.IsUnknown() ||
		state.RestorePruneArchives.IsNull() ||
		state.RestorePruneArchives.IsUnknown() ||
		state.RestoreRetentionDays.IsNull() ||
		state.RestoreRetentionDays.IsUnknown()
}

func logSettingsMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel log settings: %v",
		primaryError,
		rollbackError,
	)
}
