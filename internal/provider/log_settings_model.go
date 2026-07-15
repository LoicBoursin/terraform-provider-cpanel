package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
)

type LogSettingsResourceModel struct {
	Account                types.String `tfsdk:"account"`
	ArchiveLogs            types.Bool   `tfsdk:"archive_logs"`
	PruneArchives          types.Bool   `tfsdk:"prune_archives"`
	RetentionDays          types.Int64  `tfsdk:"retention_days"`
	EffectiveRetentionDays types.Int64  `tfsdk:"effective_retention_days"`
	UsingDefaultRetention  types.Bool   `tfsdk:"using_default_retention"`
	RestoreArchiveLogs     types.Bool   `tfsdk:"restore_archive_logs"`
	RestorePruneArchives   types.Bool   `tfsdk:"restore_prune_archives"`
	RestoreRetentionDays   types.Int64  `tfsdk:"restore_retention_days"`
}

type LogSettingsDataSourceModel struct {
	Account                types.String `tfsdk:"account"`
	ArchiveLogs            types.Bool   `tfsdk:"archive_logs"`
	PruneArchives          types.Bool   `tfsdk:"prune_archives"`
	RetentionDays          types.Int64  `tfsdk:"retention_days"`
	EffectiveRetentionDays types.Int64  `tfsdk:"effective_retention_days"`
	UsingDefaultRetention  types.Bool   `tfsdk:"using_default_retention"`
}

func logSettingsDefinitionFromResourceModel(
	model LogSettingsResourceModel,
) cpanellogmanager.Definition {
	return cpanellogmanager.Definition{
		ArchiveLogs:   model.ArchiveLogs.ValueBool(),
		PruneArchive:  model.PruneArchives.ValueBool(),
		RetentionDays: model.RetentionDays.ValueInt64(),
	}
}

func logSettingsRestoreDefinitionFromResourceModel(
	model LogSettingsResourceModel,
) cpanellogmanager.Definition {
	return cpanellogmanager.Definition{
		ArchiveLogs:   model.RestoreArchiveLogs.ValueBool(),
		PruneArchive:  model.RestorePruneArchives.ValueBool(),
		RetentionDays: model.RestoreRetentionDays.ValueInt64(),
	}
}

func applyLogSettingsToResourceModel(
	model *LogSettingsResourceModel,
	settings cpanellogmanager.Settings,
) {
	definition := settings.Definition()
	model.Account = types.StringValue(cpanellogmanager.AccountIdentity)
	model.ArchiveLogs = types.BoolValue(settings.ArchiveLogs)
	model.PruneArchives = types.BoolValue(settings.PruneArchive)
	model.RetentionDays = types.Int64Value(definition.RetentionDays)
	model.EffectiveRetentionDays = types.Int64Value(settings.RetentionDays)
	model.UsingDefaultRetention = types.BoolValue(settings.UsingDefault)
}

func applyLogSettingsRestoreToResourceModel(
	model *LogSettingsResourceModel,
	settings cpanellogmanager.Settings,
) {
	definition := settings.Definition()
	model.RestoreArchiveLogs = types.BoolValue(definition.ArchiveLogs)
	model.RestorePruneArchives = types.BoolValue(definition.PruneArchive)
	model.RestoreRetentionDays = types.Int64Value(definition.RetentionDays)
}

func logSettingsToDataSourceModel(
	settings cpanellogmanager.Settings,
) *LogSettingsDataSourceModel {
	definition := settings.Definition()

	return &LogSettingsDataSourceModel{
		Account:                types.StringValue(cpanellogmanager.AccountIdentity),
		ArchiveLogs:            types.BoolValue(settings.ArchiveLogs),
		PruneArchives:          types.BoolValue(settings.PruneArchive),
		RetentionDays:          types.Int64Value(definition.RetentionDays),
		EffectiveRetentionDays: types.Int64Value(settings.RetentionDays),
		UsingDefaultRetention:  types.BoolValue(settings.UsingDefault),
	}
}
