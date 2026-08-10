package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

type CalendarDelegateModel struct {
	Delegator    types.String `tfsdk:"delegator"`
	Delegatee    types.String `tfsdk:"delegatee"`
	Calendar     types.String `tfsdk:"calendar"`
	ReadOnly     types.Bool   `tfsdk:"readonly"`
	CalendarName types.String `tfsdk:"calendar_name"`
}

func calendarDelegateDefinitionFromModel(
	model CalendarDelegateModel,
) cpanelcalendar.Definition {
	return cpanelcalendar.Definition{
		Delegator: model.Delegator.ValueString(),
		Delegatee: model.Delegatee.ValueString(),
		Calendar:  model.Calendar.ValueString(),
		ReadOnly:  model.ReadOnly.ValueBool(),
	}
}

func applyCalendarDelegateToModel(
	model *CalendarDelegateModel,
	delegate cpanelcalendar.Delegate,
) {
	model.Delegator = types.StringValue(delegate.Delegator)
	model.Delegatee = types.StringValue(delegate.Delegatee)
	model.Calendar = types.StringValue(delegate.Calendar)
	model.ReadOnly = types.BoolValue(delegate.ReadOnly)
	model.CalendarName = types.StringValue(delegate.CalendarName)
}
