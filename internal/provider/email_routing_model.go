package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

type EmailRoutingResourceModel struct {
	Domain           types.String `tfsdk:"domain"`
	Mode             types.String `tfsdk:"mode"`
	DetectedMode     types.String `tfsdk:"detected_mode"`
	PrimaryExchanger types.String `tfsdk:"primary_exchanger"`
	RestoreMode      types.String `tfsdk:"restore_mode"`
}

type EmailRoutingDataSourceModel struct {
	Domain           types.String `tfsdk:"domain"`
	Mode             types.String `tfsdk:"mode"`
	DetectedMode     types.String `tfsdk:"detected_mode"`
	PrimaryExchanger types.String `tfsdk:"primary_exchanger"`
}

func emailRoutingDefinitionFromResourceModel(
	model EmailRoutingResourceModel,
) cpanelmail.RoutingDefinition {
	return cpanelmail.RoutingDefinition{
		Domain: model.Domain.ValueString(),
		Mode:   cpanelmail.RoutingMode(model.Mode.ValueString()),
	}
}

func applyEmailRoutingToResourceModel(
	model *EmailRoutingResourceModel,
	routing cpanelmail.Routing,
) {
	model.Domain = types.StringValue(routing.Domain)
	model.Mode = types.StringValue(string(routing.Mode))
	model.DetectedMode = types.StringValue(string(routing.DetectedMode))
	if routing.PrimaryExchanger == nil {
		model.PrimaryExchanger = types.StringNull()
	} else {
		model.PrimaryExchanger = types.StringValue(*routing.PrimaryExchanger)
	}
}

func emailRoutingToDataSourceModel(
	routing cpanelmail.Routing,
) *EmailRoutingDataSourceModel {
	model := &EmailRoutingDataSourceModel{
		Domain:       types.StringValue(routing.Domain),
		Mode:         types.StringValue(string(routing.Mode)),
		DetectedMode: types.StringValue(string(routing.DetectedMode)),
	}
	if routing.PrimaryExchanger == nil {
		model.PrimaryExchanger = types.StringNull()
	} else {
		model.PrimaryExchanger = types.StringValue(*routing.PrimaryExchanger)
	}

	return model
}

func emailRoutingRestoreModeMissing(
	model EmailRoutingResourceModel,
) bool {
	return model.RestoreMode.IsNull() ||
		model.RestoreMode.IsUnknown() ||
		model.RestoreMode.ValueString() == ""
}
