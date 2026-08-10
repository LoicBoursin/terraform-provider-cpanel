package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelboxtrapper "terraform-provider-cpanel/internal/cpanel/boxtrapper"
)

type BoxTrapperSettingsResourceModel struct {
	Account                types.String  `tfsdk:"account"`
	Enabled                types.Bool    `tfsdk:"enabled"`
	EnableAutoWhitelist    types.Bool    `tfsdk:"enable_auto_whitelist"`
	FromAddresses          types.String  `tfsdk:"from_addresses"`
	FromName               types.String  `tfsdk:"from_name"`
	QueueDays              types.Int64   `tfsdk:"queue_days"`
	SpamScore              types.Float64 `tfsdk:"spam_score"`
	WhitelistByAssociation types.Bool    `tfsdk:"whitelist_by_association"`

	RestoreEnabled                types.Bool    `tfsdk:"restore_enabled"`
	RestoreEnableAutoWhitelist    types.Bool    `tfsdk:"restore_enable_auto_whitelist"`
	RestoreFromAddresses          types.String  `tfsdk:"restore_from_addresses"`
	RestoreQueueDays              types.Int64   `tfsdk:"restore_queue_days"`
	RestoreSpamScore              types.Float64 `tfsdk:"restore_spam_score"`
	RestoreWhitelistByAssociation types.Bool    `tfsdk:"restore_whitelist_by_association"`
}

type BoxTrapperSettingsDataSourceModel struct {
	Account                types.String  `tfsdk:"account"`
	Enabled                types.Bool    `tfsdk:"enabled"`
	EnableAutoWhitelist    types.Bool    `tfsdk:"enable_auto_whitelist"`
	FromAddresses          types.String  `tfsdk:"from_addresses"`
	FromName               types.String  `tfsdk:"from_name"`
	QueueDays              types.Int64   `tfsdk:"queue_days"`
	SpamScore              types.Float64 `tfsdk:"spam_score"`
	WhitelistByAssociation types.Bool    `tfsdk:"whitelist_by_association"`
}

func boxTrapperDefinitionFromResourceModel(
	model BoxTrapperSettingsResourceModel,
	fallback cpanelboxtrapper.Settings,
) cpanelboxtrapper.Definition {
	definition := fallback.Definition()
	definition.Enabled = model.Enabled.ValueBool()
	if !model.EnableAutoWhitelist.IsNull() &&
		!model.EnableAutoWhitelist.IsUnknown() {
		definition.EnableAutoWhitelist =
			model.EnableAutoWhitelist.ValueBool()
	}
	if !model.FromAddresses.IsNull() &&
		!model.FromAddresses.IsUnknown() {
		definition.FromAddresses = model.FromAddresses.ValueString()
	}
	if !model.QueueDays.IsNull() && !model.QueueDays.IsUnknown() {
		definition.QueueDays = model.QueueDays.ValueInt64()
	}
	if !model.SpamScore.IsNull() && !model.SpamScore.IsUnknown() {
		definition.SpamScore = model.SpamScore.ValueFloat64()
	}
	if !model.WhitelistByAssociation.IsNull() &&
		!model.WhitelistByAssociation.IsUnknown() {
		definition.WhitelistByAssociation =
			model.WhitelistByAssociation.ValueBool()
	}

	return definition
}

func boxTrapperRestoreDefinitionFromResourceModel(
	model BoxTrapperSettingsResourceModel,
) cpanelboxtrapper.Definition {
	return cpanelboxtrapper.Definition{
		Enabled: model.RestoreEnabled.ValueBool(),
		EnableAutoWhitelist: model.RestoreEnableAutoWhitelist.
			ValueBool(),
		FromAddresses: model.RestoreFromAddresses.ValueString(),
		QueueDays:     model.RestoreQueueDays.ValueInt64(),
		SpamScore:     model.RestoreSpamScore.ValueFloat64(),
		WhitelistByAssociation: model.RestoreWhitelistByAssociation.
			ValueBool(),
	}
}

func boxTrapperCurrentDefinitionFromResourceModel(
	model BoxTrapperSettingsResourceModel,
) cpanelboxtrapper.Definition {
	return cpanelboxtrapper.Definition{
		Enabled:                model.Enabled.ValueBool(),
		EnableAutoWhitelist:    model.EnableAutoWhitelist.ValueBool(),
		FromAddresses:          model.FromAddresses.ValueString(),
		QueueDays:              model.QueueDays.ValueInt64(),
		SpamScore:              model.SpamScore.ValueFloat64(),
		WhitelistByAssociation: model.WhitelistByAssociation.ValueBool(),
	}
}

func applyBoxTrapperSettingsToResourceModel(
	model *BoxTrapperSettingsResourceModel,
	settings cpanelboxtrapper.Settings,
) {
	definition := settings.Definition()
	model.Account = types.StringValue(settings.Account)
	model.Enabled = types.BoolValue(definition.Enabled)
	model.EnableAutoWhitelist = types.BoolValue(
		definition.EnableAutoWhitelist,
	)
	model.FromAddresses = types.StringValue(definition.FromAddresses)
	if settings.FromName == nil {
		model.FromName = types.StringNull()
	} else {
		model.FromName = types.StringValue(*settings.FromName)
	}
	model.QueueDays = types.Int64Value(definition.QueueDays)
	model.SpamScore = types.Float64Value(definition.SpamScore)
	model.WhitelistByAssociation = types.BoolValue(
		definition.WhitelistByAssociation,
	)
}

func applyBoxTrapperRestoreToResourceModel(
	model *BoxTrapperSettingsResourceModel,
	settings cpanelboxtrapper.Settings,
) {
	definition := settings.Definition()
	model.RestoreEnabled = types.BoolValue(definition.Enabled)
	model.RestoreEnableAutoWhitelist = types.BoolValue(
		definition.EnableAutoWhitelist,
	)
	model.RestoreFromAddresses = types.StringValue(
		definition.FromAddresses,
	)
	model.RestoreQueueDays = types.Int64Value(definition.QueueDays)
	model.RestoreSpamScore = types.Float64Value(definition.SpamScore)
	model.RestoreWhitelistByAssociation = types.BoolValue(
		definition.WhitelistByAssociation,
	)
}

func boxTrapperSettingsToDataSourceModel(
	settings cpanelboxtrapper.Settings,
) *BoxTrapperSettingsDataSourceModel {
	definition := settings.Definition()
	model := &BoxTrapperSettingsDataSourceModel{
		Account:             types.StringValue(settings.Account),
		Enabled:             types.BoolValue(definition.Enabled),
		EnableAutoWhitelist: types.BoolValue(definition.EnableAutoWhitelist),
		FromAddresses:       types.StringValue(definition.FromAddresses),
		QueueDays:           types.Int64Value(definition.QueueDays),
		SpamScore:           types.Float64Value(definition.SpamScore),
		WhitelistByAssociation: types.BoolValue(
			definition.WhitelistByAssociation,
		),
	}
	if settings.FromName == nil {
		model.FromName = types.StringNull()
	} else {
		model.FromName = types.StringValue(*settings.FromName)
	}

	return model
}

func boxTrapperRestoreStateMissing(
	model BoxTrapperSettingsResourceModel,
) bool {
	return model.RestoreEnabled.IsNull() ||
		model.RestoreEnabled.IsUnknown() ||
		model.RestoreEnableAutoWhitelist.IsNull() ||
		model.RestoreEnableAutoWhitelist.IsUnknown() ||
		model.RestoreFromAddresses.IsNull() ||
		model.RestoreFromAddresses.IsUnknown() ||
		model.RestoreQueueDays.IsNull() ||
		model.RestoreQueueDays.IsUnknown() ||
		model.RestoreSpamScore.IsNull() ||
		model.RestoreSpamScore.IsUnknown() ||
		model.RestoreWhitelistByAssociation.IsNull() ||
		model.RestoreWhitelistByAssociation.IsUnknown()
}
