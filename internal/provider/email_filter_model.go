package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type EmailFilterModel struct {
	Account types.String `tfsdk:"account"`
	Name    types.String `tfsdk:"name"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Rules   types.List   `tfsdk:"rules"`
	Actions types.List   `tfsdk:"actions"`
}

type EmailFilterRuleModel struct {
	Part     types.String `tfsdk:"part"`
	Match    types.String `tfsdk:"match"`
	Value    types.String `tfsdk:"value"`
	Operator types.String `tfsdk:"operator"`
}

type EmailFilterActionModel struct {
	Action      types.String `tfsdk:"action"`
	Destination types.String `tfsdk:"destination"`
}

var emailFilterRuleAttributeTypes = map[string]attr.Type{
	"part":     types.StringType,
	"match":    types.StringType,
	"value":    types.StringType,
	"operator": types.StringType,
}

var emailFilterActionAttributeTypes = map[string]attr.Type{
	"action":      types.StringType,
	"destination": types.StringType,
}
