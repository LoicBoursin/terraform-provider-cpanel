package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelcontact "terraform-provider-cpanel/internal/cpanel/contactinformation"
)

type NotificationPreferencesResourceModel struct {
	Account            types.String `tfsdk:"account"`
	Preferences        types.Map    `tfsdk:"preferences"`
	Descriptions       types.Map    `tfsdk:"descriptions"`
	RestorePreferences types.Map    `tfsdk:"restore_preferences"`
}

type NotificationPreferencesDataSourceModel struct {
	Account      types.String `tfsdk:"account"`
	Preferences  types.Map    `tfsdk:"preferences"`
	Descriptions types.Map    `tfsdk:"descriptions"`
}

func notificationPreferencesDefinitionFromMap(
	ctx context.Context,
	value types.Map,
) (map[string]bool, diag.Diagnostics) {
	definition := map[string]bool{}
	diagnostics := value.ElementsAs(ctx, &definition, false)

	return definition, diagnostics
}

func applyNotificationPreferencesToResourceModel(
	ctx context.Context,
	model *NotificationPreferencesResourceModel,
	value cpanelcontact.NotificationPreferences,
) diag.Diagnostics {
	preferences, diagnostics := types.MapValueFrom(
		ctx,
		types.BoolType,
		value.Preferences,
	)
	descriptions, descriptionDiagnostics := types.MapValueFrom(
		ctx,
		types.StringType,
		value.Descriptions,
	)
	diagnostics.Append(descriptionDiagnostics...)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Account = types.StringValue(cpanelcontact.AccountIdentity)
	model.Preferences = preferences
	model.Descriptions = descriptions

	return diagnostics
}

func applyNotificationPreferencesRestoreToResourceModel(
	ctx context.Context,
	model *NotificationPreferencesResourceModel,
	value cpanelcontact.NotificationPreferences,
) diag.Diagnostics {
	restorePreferences, diagnostics := types.MapValueFrom(
		ctx,
		types.BoolType,
		value.Preferences,
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.RestorePreferences = restorePreferences

	return diagnostics
}

func notificationPreferencesToDataSourceModel(
	ctx context.Context,
	value cpanelcontact.NotificationPreferences,
) (*NotificationPreferencesDataSourceModel, diag.Diagnostics) {
	preferences, diagnostics := types.MapValueFrom(
		ctx,
		types.BoolType,
		value.Preferences,
	)
	descriptions, descriptionDiagnostics := types.MapValueFrom(
		ctx,
		types.StringType,
		value.Descriptions,
	)
	diagnostics.Append(descriptionDiagnostics...)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &NotificationPreferencesDataSourceModel{
		Account:      types.StringValue(cpanelcontact.AccountIdentity),
		Preferences:  preferences,
		Descriptions: descriptions,
	}, diagnostics
}

func notificationPreferencesRestoreStateMissing(
	state NotificationPreferencesResourceModel,
) bool {
	return state.RestorePreferences.IsNull() ||
		state.RestorePreferences.IsUnknown() ||
		len(state.RestorePreferences.Elements()) == 0
}
