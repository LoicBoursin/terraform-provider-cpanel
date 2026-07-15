package provider

import (
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func emailFilterFromModel(
	ctx context.Context,
	model EmailFilterModel,
) (cpanelmail.Filter, diag.Diagnostics) {
	var diagnostics diag.Diagnostics
	var ruleModels []EmailFilterRuleModel
	var actionModels []EmailFilterActionModel

	diagnostics.Append(model.Rules.ElementsAs(ctx, &ruleModels, false)...)
	diagnostics.Append(model.Actions.ElementsAs(ctx, &actionModels, false)...)
	if diagnostics.HasError() {
		return cpanelmail.Filter{}, diagnostics
	}

	filter := cpanelmail.Filter{
		Account: model.Account.ValueString(),
		Name:    model.Name.ValueString(),
		Enabled: model.Enabled.ValueBool(),
		Rules:   make([]cpanelmail.FilterRule, 0, len(ruleModels)),
		Actions: make([]cpanelmail.FilterAction, 0, len(actionModels)),
	}
	for _, rule := range ruleModels {
		match := rule.Match.ValueString()
		if match == "none" {
			match = ""
		}
		operator := rule.Operator.ValueString()
		if operator == "none" {
			operator = ""
		}
		filter.Rules = append(filter.Rules, cpanelmail.FilterRule{
			Part:     rule.Part.ValueString(),
			Match:    match,
			Value:    rule.Value.ValueString(),
			Operator: operator,
		})
	}
	for _, action := range actionModels {
		destination := ""
		if !action.Destination.IsNull() && !action.Destination.IsUnknown() {
			destination = action.Destination.ValueString()
		}
		filter.Actions = append(filter.Actions, cpanelmail.FilterAction{
			Action:      action.Action.ValueString(),
			Destination: destination,
		})
	}

	return filter, diagnostics
}

func applyEmailFilterToModel(
	ctx context.Context,
	model *EmailFilterModel,
	filter cpanelmail.Filter,
) diag.Diagnostics {
	ruleModels := make([]EmailFilterRuleModel, 0, len(filter.Rules))
	for _, rule := range filter.Rules {
		match := rule.Match
		if match == "" {
			match = "none"
		}
		operator := rule.Operator
		if operator == "" {
			operator = "none"
		}
		ruleModels = append(ruleModels, EmailFilterRuleModel{
			Part:     types.StringValue(rule.Part),
			Match:    types.StringValue(match),
			Value:    types.StringValue(rule.Value),
			Operator: types.StringValue(operator),
		})
	}

	actionModels := make([]EmailFilterActionModel, 0, len(filter.Actions))
	for _, action := range filter.Actions {
		destination := types.StringNull()
		if action.Destination != "" {
			destination = types.StringValue(action.Destination)
		}
		actionModels = append(actionModels, EmailFilterActionModel{
			Action:      types.StringValue(action.Action),
			Destination: destination,
		})
	}

	rules, diagnostics := types.ListValueFrom(
		ctx,
		types.ObjectType{AttrTypes: emailFilterRuleAttributeTypes},
		ruleModels,
	)
	if diagnostics.HasError() {
		return diagnostics
	}
	actions, actionDiagnostics := types.ListValueFrom(
		ctx,
		types.ObjectType{AttrTypes: emailFilterActionAttributeTypes},
		actionModels,
	)
	diagnostics.Append(actionDiagnostics...)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Account = types.StringValue(filter.Account)
	model.Name = types.StringValue(filter.Name)
	model.Enabled = types.BoolValue(filter.Enabled)
	model.Rules = rules
	model.Actions = actions

	return diagnostics
}

func emailFiltersEqual(left, right cpanelmail.Filter) bool {
	return left.Account == right.Account &&
		left.Name == right.Name &&
		left.Enabled == right.Enabled &&
		emailFilterDefinitionsEqual(left, right)
}

func emailFilterDefinitionsEqual(left, right cpanelmail.Filter) bool {
	return slices.Equal(left.Rules, right.Rules) &&
		slices.Equal(left.Actions, right.Actions)
}
