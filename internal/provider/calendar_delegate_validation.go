package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

const calendarDelegateImportSeparator = "|"

func calendarNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(cpanelcalendar.DefaultCalendar),
	}
}

func validateCalendarDelegateDefinition(
	definition cpanelcalendar.Definition,
) error {
	if _, _, err := splitEmailAccountAddress(definition.Delegator); err != nil {
		return fmt.Errorf("invalid delegator: %w", err)
	}
	if _, _, err := splitEmailAccountAddress(definition.Delegatee); err != nil {
		return fmt.Errorf("invalid delegatee: %w", err)
	}
	if definition.Delegator == definition.Delegatee {
		return fmt.Errorf("delegator and delegatee must be different email accounts")
	}
	if definition.Calendar != cpanelcalendar.DefaultCalendar {
		return fmt.Errorf(
			"calendar must be %q",
			cpanelcalendar.DefaultCalendar,
		)
	}

	return nil
}

func parseCalendarDelegateImportID(
	id string,
) (cpanelcalendar.Definition, error) {
	parts := strings.Split(id, calendarDelegateImportSeparator)
	if len(parts) != 3 {
		return cpanelcalendar.Definition{}, fmt.Errorf(
			"calendar delegate import identifiers must use delegator%s%s%sdelegatee",
			calendarDelegateImportSeparator,
			cpanelcalendar.DefaultCalendar,
			calendarDelegateImportSeparator,
		)
	}

	definition := cpanelcalendar.Definition{
		Delegator: parts[0],
		Calendar:  parts[1],
		Delegatee: parts[2],
	}
	if err := validateCalendarDelegateDefinition(definition); err != nil {
		return cpanelcalendar.Definition{}, err
	}

	return definition, nil
}

func calendarDelegatesEqual(
	left cpanelcalendar.Delegate,
	right cpanelcalendar.Definition,
) bool {
	return left.Delegator == right.Delegator &&
		left.Delegatee == right.Delegatee &&
		left.Calendar == right.Calendar &&
		left.ReadOnly == right.ReadOnly
}
