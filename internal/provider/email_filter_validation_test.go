package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestValidateEmailFilter(t *testing.T) {
	t.Parallel()

	valid := testEmailFilterDefinition()
	if err := validateEmailFilter(valid); err != nil {
		t.Fatalf("validateEmailFilter() error: %v", err)
	}

	numeric := testEmailFilterDefinition()
	numeric.Rules[0] = cpanelmail.FilterRule{
		Part:     "$h_x-Spam-Score:",
		Match:    "is above",
		Value:    "5",
		Operator: "and",
	}
	if err := validateEmailFilter(numeric); err != nil {
		t.Fatalf("validateEmailFilter(numeric) error: %v", err)
	}

	literalNull := testEmailFilterDefinition()
	literalNull.Actions[1].Destination = "null"
	if err := validateEmailFilter(literalNull); err != nil {
		t.Fatalf("validateEmailFilter(literal null) error: %v", err)
	}

	for _, part := range emailFilterParts {
		part := part
		t.Run("part "+part, func(t *testing.T) {
			t.Parallel()

			filter := testEmailFilterDefinition()
			filter.Rules[0].Part = part
			if part == "$h_x-Spam-Score:" {
				filter.Rules[0].Match = "is above"
				filter.Rules[0].Value = "5"
			} else if cpanelmail.IsMatchlessFilterPart(part) {
				filter.Rules[0].Match = ""
				filter.Rules[0].Value = ""
			}
			if err := validateEmailFilter(filter); err != nil {
				t.Fatalf("validateEmailFilter() error: %v", err)
			}
		})
	}

	testCases := map[string]func(*cpanelmail.Filter){
		"account-level filter": func(filter *cpanelmail.Filter) {
			filter.Account = "cpanel-user"
		},
		"invalid name": func(filter *cpanelmail.Filter) {
			filter.Name = "terraform|filter"
		},
		"missing rules": func(filter *cpanelmail.Filter) {
			filter.Rules = nil
		},
		"unsupported part": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Part = "$spam_score"
		},
		"non-round-tripping cPanel part": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Part = "$header_subject"
		},
		"matchless part with comparison": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Part = "not delivered"
		},
		"non-integer numeric match": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Match = "is above"
		},
		"newline value": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Value = "unsafe\nvalue"
		},
		"missing intermediate operator": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Operator = ""
		},
		"last operator": func(filter *cpanelmail.Filter) {
			filter.Rules[1].Operator = "and"
		},
		"pipe action": func(filter *cpanelmail.Filter) {
			filter.Actions[0].Action = "pipe"
		},
		"save action": func(filter *cpanelmail.Filter) {
			filter.Actions[0].Action = "save"
		},
		"invalid delivery": func(filter *cpanelmail.Filter) {
			filter.Actions[0].Destination = "|command@example.test"
		},
		"missing failure text": func(filter *cpanelmail.Filter) {
			filter.Actions[1].Destination = ""
		},
		"finish destination": func(filter *cpanelmail.Filter) {
			filter.Actions[2].Destination = "unexpected"
		},
	}

	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filter := testEmailFilterDefinition()
			mutate(&filter)
			if err := validateEmailFilter(filter); err == nil {
				t.Fatal("validateEmailFilter() returned no error")
			}
		})
	}
}

func TestValidateAccountEmailFilter(t *testing.T) {
	t.Parallel()

	filter := testEmailFilterDefinition()
	filter.Account = "terraform"
	if err := validateAccountEmailFilter(filter); err != nil {
		t.Fatalf("validateAccountEmailFilter() error: %v", err)
	}

	for _, account := range []string{"", "user@example.test", "bad|account"} {
		filter.Account = account
		if err := validateAccountEmailFilter(filter); err == nil {
			t.Fatalf(
				"validateAccountEmailFilter() accepted account %q",
				account,
			)
		}
	}
}

func TestEmailFilterDestinationValidators(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value     types.String
		wantValid bool
	}{
		"omitted": {
			value:     types.StringNull(),
			wantValid: true,
		},
		"one character": {
			value:     types.StringValue("x"),
			wantValid: true,
		},
		"maximum length": {
			value:     types.StringValue(strings.Repeat("x", emailFilterMaximumFailLength)),
			wantValid: true,
		},
		"empty string": {
			value: types.StringValue(""),
		},
		"too long": {
			value: types.StringValue(strings.Repeat("x", emailFilterMaximumFailLength+1)),
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := validator.StringRequest{
				ConfigValue: testCase.value,
				Path:        path.Root("destination"),
			}
			response := &validator.StringResponse{}
			for _, destinationValidator := range emailFilterDestinationValidators() {
				destinationValidator.ValidateString(
					context.Background(),
					request,
					response,
				)
			}

			if got := !response.Diagnostics.HasError(); got != testCase.wantValid {
				t.Fatalf(
					"emailFilterDestinationValidators() validity = %t, want %t; diagnostics: %v",
					got,
					testCase.wantValid,
					response.Diagnostics,
				)
			}
		})
	}
}

func TestValidateReadableEmailFilterAcceptsExternalDefinitions(t *testing.T) {
	t.Parallel()

	filter := testEmailFilterDefinition()
	filter.Name = "External/filter|name"
	filter.Rules[0].Part = "$future_header:"
	filter.Rules[0].Match = "future comparison"
	filter.Actions = []cpanelmail.FilterAction{
		{Action: "save", Destination: ".Terraform"},
		{Action: "pipe", Destination: "/usr/local/bin/filter"},
	}

	if err := validateReadableEmailFilter(filter); err != nil {
		t.Fatalf("validateReadableEmailFilter() error: %v", err)
	}
	if err := validateEmailFilter(filter); err == nil {
		t.Fatal("validateEmailFilter() accepted an external-only definition")
	}
}

func TestValidateReadableEmailFilterRejectsMalformedDefinitions(t *testing.T) {
	t.Parallel()

	testCases := map[string]func(*cpanelmail.Filter){
		"empty part": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Part = ""
		},
		"empty match": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Match = ""
		},
		"empty value": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Value = ""
		},
		"invalid connector": func(filter *cpanelmail.Filter) {
			filter.Rules[0].Operator = "xor"
		},
		"empty action": func(filter *cpanelmail.Filter) {
			filter.Actions[0].Action = ""
		},
	}

	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filter := testEmailFilterDefinition()
			mutate(&filter)
			if err := validateReadableEmailFilter(filter); err == nil {
				t.Fatal("validateReadableEmailFilter() returned no error")
			}
		})
	}
}

func TestEmailFilterModelRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	expected := testEmailFilterDefinition()
	model := EmailFilterModel{}
	diagnostics := applyEmailFilterToModel(ctx, &model, expected)
	if diagnostics.HasError() {
		t.Fatalf("applyEmailFilterToModel() diagnostics: %v", diagnostics)
	}

	actual, diagnostics := emailFilterFromModel(ctx, model)
	if diagnostics.HasError() {
		t.Fatalf("emailFilterFromModel() diagnostics: %v", diagnostics)
	}
	if !emailFiltersEqual(actual, expected) {
		t.Fatalf("round trip = %#v, want %#v", actual, expected)
	}
}

func TestEmailFilterMatchlessRuleModelRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	expected := testEmailFilterDefinition()
	expected.Rules = []cpanelmail.FilterRule{{
		Part: "not delivered",
	}}

	model := EmailFilterModel{}
	diagnostics := applyEmailFilterToModel(ctx, &model, expected)
	if diagnostics.HasError() {
		t.Fatalf("applyEmailFilterToModel() diagnostics: %v", diagnostics)
	}

	var rules []EmailFilterRuleModel
	diagnostics = model.Rules.ElementsAs(ctx, &rules, false)
	if diagnostics.HasError() {
		t.Fatalf("Rules.ElementsAs() diagnostics: %v", diagnostics)
	}
	if len(rules) != 1 ||
		rules[0].Match.ValueString() != "none" ||
		rules[0].Value.ValueString() != "" {
		t.Fatalf("matchless rule model = %#v", rules)
	}

	actual, diagnostics := emailFilterFromModel(ctx, model)
	if diagnostics.HasError() {
		t.Fatalf("emailFilterFromModel() diagnostics: %v", diagnostics)
	}
	if !emailFiltersEqual(actual, expected) {
		t.Fatalf("round trip = %#v, want %#v", actual, expected)
	}
}

func TestEmailFilterFinishDestinationStaysNull(t *testing.T) {
	t.Parallel()

	model := EmailFilterModel{}
	diagnostics := applyEmailFilterToModel(
		context.Background(),
		&model,
		testEmailFilterDefinition(),
	)
	if diagnostics.HasError() {
		t.Fatalf("applyEmailFilterToModel() diagnostics: %v", diagnostics)
	}

	var actions []EmailFilterActionModel
	diagnostics = model.Actions.ElementsAs(context.Background(), &actions, false)
	if diagnostics.HasError() {
		t.Fatalf("Actions.ElementsAs() diagnostics: %v", diagnostics)
	}
	if len(actions) != 3 {
		t.Fatalf("actions length = %d, want 3", len(actions))
	}
	if !actions[2].Destination.IsNull() {
		t.Fatalf(
			"finish destination = %#v, want null",
			actions[2].Destination,
		)
	}
	if actions[0].Destination.Equal(types.StringNull()) {
		t.Fatal("deliver destination is null")
	}
}

func TestParseEmailFilterImportID(t *testing.T) {
	t.Parallel()

	account, name, err := parseEmailFilterImportID(
		"terraform@example.test|Terraform filter",
	)
	if err != nil {
		t.Fatalf("parseEmailFilterImportID() error: %v", err)
	}
	if account != "terraform@example.test" || name != "Terraform filter" {
		t.Fatalf("parseEmailFilterImportID() = %q, %q", account, name)
	}

	for _, identifier := range []string{
		"",
		"terraform@example.test",
		"|filter",
		"terraform@example.test|",
		"terraform@example.test|filter|extra",
		"invalid|filter",
		"terraform@example.test|bad/name",
	} {
		if _, _, err := parseEmailFilterImportID(identifier); err == nil {
			t.Fatalf(
				"parseEmailFilterImportID(%q) returned no error",
				identifier,
			)
		}
	}
}

func testEmailFilterDefinition() cpanelmail.Filter {
	return cpanelmail.Filter{
		Account: "terraform@example.test",
		Name:    "Terraform filter",
		Enabled: true,
		Rules: []cpanelmail.FilterRule{
			{
				Part:     "$header_subject:",
				Match:    "contains",
				Value:    "Terraform",
				Operator: "and",
			},
			{
				Part:  "$header_from:",
				Match: "ends",
				Value: "@example.test",
			},
		},
		Actions: []cpanelmail.FilterAction{
			{
				Action:      "deliver",
				Destination: "archive@example.test",
			},
			{
				Action:      "fail",
				Destination: "Rejected by Terraform",
			},
			{Action: "finish"},
		},
	}
}
