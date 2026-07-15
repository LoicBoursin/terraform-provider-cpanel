package provider

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

const (
	emailFilterMaximumItems       = 4096
	emailFilterMaximumNameLength  = 128
	emailFilterMaximumValueLength = 4096
	emailFilterMaximumFailLength  = 998
)

var (
	emailFilterNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]*$`)
	emailFilterIntegerPattern = regexp.MustCompile(`^-?[0-9]+$`)

	emailFilterParts = []string{
		"$h_x-Spam-Bar:",
		"$h_x-Spam-Score:",
		"$h_X-Spam-Status:",
		"$h_List-id:",
		"$header_from:",
		"$header_subject:",
		"$header_to:",
		"$reply_address:",
		"$message_body",
		"$message_headers",
		"foranyaddress $h_to:,$h_cc:,$h_bcc:",
		"not delivered",
		"error_message",
	}

	emailFilterStringMatches = []string{
		"is",
		"matches",
		"contains",
		"does not contain",
		"begins",
		"does not begin",
		"ends",
		"does not end",
		"does not match",
	}
	emailFilterNumericMatches = []string{
		"is above",
		"is not above",
		"is below",
		"is not below",
	}
	emailFilterMatches = append(
		append([]string{}, emailFilterStringMatches...),
		emailFilterNumericMatches...,
	)

	emailFilterOperators = []string{"and", "or", "none"}
	emailFilterActions   = []string{"deliver", "fail", "finish"}
)

func emailFilterNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, emailFilterMaximumNameLength),
		stringvalidator.RegexMatches(
			emailFilterNamePattern,
			"Filter names must start with a letter or number and may contain only letters, numbers, spaces, dots, underscores, and hyphens.",
		),
	}
}

func emailFilterLookupNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtLeast(1),
	}
}

func emailFilterPartValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(emailFilterParts...),
	}
}

func emailFilterMatchValidators() []validator.String {
	configuredMatches := append([]string{}, emailFilterMatches...)
	configuredMatches = append(configuredMatches, "none")

	return []validator.String{
		stringvalidator.OneOf(configuredMatches...),
	}
}

func emailFilterOperatorValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(emailFilterOperators...),
	}
}

func emailFilterActionValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(emailFilterActions...),
	}
}

func emailFilterDestinationValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, emailFilterMaximumFailLength),
	}
}

func validateEmailFilter(filter cpanelmail.Filter) error {
	if err := validateReadableEmailFilter(filter); err != nil {
		return err
	}
	if err := validateEmailFilterName(filter.Name); err != nil {
		return err
	}

	for index, rule := range filter.Rules {
		if !slices.Contains(emailFilterParts, rule.Part) {
			return fmt.Errorf(
				"email filter rule %d has unsupported part %q",
				index+1,
				rule.Part,
			)
		}
		if cpanelmail.IsMatchlessFilterPart(rule.Part) {
			if rule.Match != "" || rule.Value != "" {
				return fmt.Errorf(
					"email filter rule %d must use match \"none\" and an empty value for part %q",
					index+1,
					rule.Part,
				)
			}

			continue
		}
		if !slices.Contains(emailFilterMatches, rule.Match) {
			return fmt.Errorf(
				"email filter rule %d has unsupported match %q",
				index+1,
				rule.Match,
			)
		}
		if err := validateEmailFilterText(
			rule.Value,
			emailFilterMaximumValueLength,
			fmt.Sprintf("email filter rule %d value", index+1),
		); err != nil {
			return err
		}
		if slices.Contains(emailFilterNumericMatches, rule.Match) &&
			!emailFilterIntegerPattern.MatchString(rule.Value) {
			return fmt.Errorf(
				"email filter rule %d value must be a base-10 integer for match %q",
				index+1,
				rule.Match,
			)
		}
	}

	for index, action := range filter.Actions {
		switch action.Action {
		case "deliver":
			if err := validateEmailForwarderDestination(action.Destination); err != nil {
				return fmt.Errorf(
					"email filter action %d has invalid deliver destination: %w",
					index+1,
					err,
				)
			}
		case "fail":
			if err := validateEmailFilterText(
				action.Destination,
				emailFilterMaximumFailLength,
				fmt.Sprintf("email filter action %d failure message", index+1),
			); err != nil {
				return err
			}
		case "finish":
			if action.Destination != "" {
				return fmt.Errorf(
					"email filter action %d must not set a destination for finish",
					index+1,
				)
			}
		default:
			return fmt.Errorf(
				"email filter action %d has unsupported action %q; only deliver, fail, and finish are allowed",
				index+1,
				action.Action,
			)
		}
	}

	return nil
}

func validateReadableEmailFilter(filter cpanelmail.Filter) error {
	if _, _, err := splitEmailAccountAddress(filter.Account); err != nil {
		return fmt.Errorf("invalid filter account: %w", err)
	}
	if filter.Name == "" {
		return errors.New("email filter name must not be empty")
	}
	if len(filter.Rules) == 0 {
		return errors.New("email filter must contain at least one rule")
	}
	if len(filter.Rules) > emailFilterMaximumItems {
		return fmt.Errorf(
			"email filter must not contain more than %d rules",
			emailFilterMaximumItems,
		)
	}
	if len(filter.Actions) == 0 {
		return errors.New("email filter must contain at least one action")
	}
	if len(filter.Actions) > emailFilterMaximumItems {
		return fmt.Errorf(
			"email filter must not contain more than %d actions",
			emailFilterMaximumItems,
		)
	}

	for index, rule := range filter.Rules {
		if rule.Part == "" {
			return fmt.Errorf("email filter rule %d has an empty part", index+1)
		}
		if rule.Match == "" && rule.Value != "" {
			return fmt.Errorf(
				"email filter rule %d has an empty match with a non-empty value",
				index+1,
			)
		}
		if rule.Match != "" && rule.Value == "" {
			return fmt.Errorf(
				"email filter rule %d has a non-empty match with an empty value",
				index+1,
			)
		}

		last := index == len(filter.Rules)-1
		if last && rule.Operator != "" {
			return fmt.Errorf(
				"email filter rule %d must use operator \"none\" because it is the last rule",
				index+1,
			)
		}
		if !last && rule.Operator != "and" && rule.Operator != "or" {
			return fmt.Errorf(
				"email filter rule %d must use operator \"and\" or \"or\"",
				index+1,
			)
		}
	}

	for index, action := range filter.Actions {
		if action.Action == "" {
			return fmt.Errorf("email filter action %d has an empty action", index+1)
		}
	}

	return nil
}

func validateEmailFilterName(name string) error {
	if len(name) == 0 {
		return errors.New("email filter name must not be empty")
	}
	if len(name) > emailFilterMaximumNameLength {
		return fmt.Errorf(
			"email filter name must not exceed %d ASCII characters",
			emailFilterMaximumNameLength,
		)
	}
	if strings.Contains(name, "|") {
		return errors.New("email filter name must not contain |")
	}
	if !emailFilterNamePattern.MatchString(name) {
		return errors.New(
			"email filter name must start with a letter or number and contain only letters, numbers, spaces, dots, underscores, and hyphens",
		)
	}

	return nil
}

func validateEmailFilterText(value string, maximum int, label string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s must not exceed %d bytes", label, maximum)
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must not contain NUL, carriage return, or newline", label)
	}

	return nil
}
