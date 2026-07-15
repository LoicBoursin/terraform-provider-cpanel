package provider

import (
	"testing"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

func TestValidateCalendarDelegateDefinition(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		definition cpanelcalendar.Definition
		wantError  bool
	}{
		"valid readonly": {
			definition: cpanelcalendar.Definition{
				Delegator: "owner@example.test",
				Delegatee: "delegate@example.test",
				Calendar:  cpanelcalendar.DefaultCalendar,
				ReadOnly:  true,
			},
		},
		"valid read write": {
			definition: cpanelcalendar.Definition{
				Delegator: "owner@example.test",
				Delegatee: "delegate@example.test",
				Calendar:  cpanelcalendar.DefaultCalendar,
			},
		},
		"invalid delegator": {
			definition: cpanelcalendar.Definition{
				Delegator: "owner",
				Delegatee: "delegate@example.test",
				Calendar:  cpanelcalendar.DefaultCalendar,
			},
			wantError: true,
		},
		"invalid delegatee": {
			definition: cpanelcalendar.Definition{
				Delegator: "owner@example.test",
				Delegatee: "delegate",
				Calendar:  cpanelcalendar.DefaultCalendar,
			},
			wantError: true,
		},
		"same account": {
			definition: cpanelcalendar.Definition{
				Delegator: "owner@example.test",
				Delegatee: "owner@example.test",
				Calendar:  cpanelcalendar.DefaultCalendar,
			},
			wantError: true,
		},
		"unsupported calendar": {
			definition: cpanelcalendar.Definition{
				Delegator: "owner@example.test",
				Delegatee: "delegate@example.test",
				Calendar:  "tasks",
			},
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateCalendarDelegateDefinition(testCase.definition)
			if testCase.wantError && err == nil {
				t.Fatal("validateCalendarDelegateDefinition() returned no error")
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateCalendarDelegateDefinition() error: %v",
					err,
				)
			}
		})
	}
}

func TestParseCalendarDelegateImportID(t *testing.T) {
	t.Parallel()

	definition, err := parseCalendarDelegateImportID(
		"owner@example.test|calendar|delegate@example.test",
	)
	if err != nil {
		t.Fatalf("parseCalendarDelegateImportID() error: %v", err)
	}
	if definition.Delegator != "owner@example.test" ||
		definition.Delegatee != "delegate@example.test" ||
		definition.Calendar != cpanelcalendar.DefaultCalendar {
		t.Fatalf("parseCalendarDelegateImportID() = %#v", definition)
	}

	for _, invalid := range []string{
		"",
		"owner@example.test|delegate@example.test",
		"owner@example.test|tasks|delegate@example.test",
		"owner@example.test|calendar|owner@example.test",
	} {
		if _, err := parseCalendarDelegateImportID(invalid); err == nil {
			t.Fatalf("parseCalendarDelegateImportID(%q) returned no error", invalid)
		}
	}
}
