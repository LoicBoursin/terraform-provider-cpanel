package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelspam "terraform-provider-cpanel/internal/cpanel/spamassassin"
)

type SpamPreferenceResourceModel struct {
	Preference     types.String `tfsdk:"preference"`
	Values         types.Set    `tfsdk:"values"`
	Configured     types.Bool   `tfsdk:"configured"`
	RestorePresent types.Bool   `tfsdk:"restore_present"`
	RestoreValues  types.Set    `tfsdk:"restore_values"`
}

type SpamPreferenceDataSourceModel struct {
	Preference types.String `tfsdk:"preference"`
	Values     types.Set    `tfsdk:"values"`
	Configured types.Bool   `tfsdk:"configured"`
}

func spamPreferenceDefinitionFromResourceModel(
	ctx context.Context,
	model SpamPreferenceResourceModel,
) (cpanelspam.Definition, diag.Diagnostics) {
	var values []string
	diagnostics := model.Values.ElementsAs(ctx, &values, false)

	return cpanelspam.Definition{
		Name:    model.Preference.ValueString(),
		Values:  values,
		Present: true,
	}.Sorted(), diagnostics
}

func spamPreferenceRestoreDefinitionFromResourceModel(
	ctx context.Context,
	model SpamPreferenceResourceModel,
) (cpanelspam.Definition, diag.Diagnostics) {
	var values []string
	diagnostics := model.RestoreValues.ElementsAs(ctx, &values, false)

	return cpanelspam.Definition{
		Name:    model.Preference.ValueString(),
		Values:  values,
		Present: model.RestorePresent.ValueBool(),
	}.Sorted(), diagnostics
}

func applySpamPreferenceToResourceModel(
	ctx context.Context,
	model *SpamPreferenceResourceModel,
	preference cpanelspam.Preference,
) diag.Diagnostics {
	values, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		preference.Values,
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Preference = types.StringValue(preference.Name)
	model.Values = values
	model.Configured = types.BoolValue(preference.Present)

	return diagnostics
}

func applySpamPreferenceRestoreToResourceModel(
	ctx context.Context,
	model *SpamPreferenceResourceModel,
	preference cpanelspam.Preference,
) diag.Diagnostics {
	values, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		preference.Values,
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.RestorePresent = types.BoolValue(preference.Present)
	model.RestoreValues = values

	return diagnostics
}

func spamPreferenceToDataSourceModel(
	ctx context.Context,
	preference cpanelspam.Preference,
) (*SpamPreferenceDataSourceModel, diag.Diagnostics) {
	values, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		preference.Values,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &SpamPreferenceDataSourceModel{
		Preference: types.StringValue(preference.Name),
		Values:     values,
		Configured: types.BoolValue(preference.Present),
	}, diagnostics
}

func spamPreferenceRestoreStateMissing(
	state SpamPreferenceResourceModel,
) bool {
	return state.RestorePresent.IsNull() ||
		state.RestorePresent.IsUnknown() ||
		state.RestoreValues.IsNull() ||
		state.RestoreValues.IsUnknown()
}
