package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type cronFieldValidator struct {
	name      string
	minimum   int
	maximum   int
	allowList bool
	allowStep bool
}

func (v cronFieldValidator) Description(_ context.Context) string {
	return v.acceptedValuesDescription()
}

func (v cronFieldValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v cronFieldValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	if err := validateCronField(request.ConfigValue.ValueString(), v); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid cron "+v.name,
			err.Error(),
		)
	}
}

func validateCronField(value string, field cronFieldValidator) error {
	if value == "*" {
		return nil
	}

	if strings.HasPrefix(value, "*/") {
		if !field.allowStep {
			return fmt.Errorf("%s; step expressions are not supported", field.acceptedValuesDescription())
		}

		step, err := parseCronNumber(strings.TrimPrefix(value, "*/"))
		if err != nil || step < 1 || step > field.maximum {
			return fmt.Errorf(
				"%s; step must be between 1 and %d",
				field.acceptedValuesDescription(),
				field.maximum,
			)
		}

		return nil
	}

	if strings.Contains(value, ",") {
		if !field.allowList {
			return fmt.Errorf("%s; lists are not supported", field.acceptedValuesDescription())
		}

		parts := strings.Split(value, ",")
		if len(parts) < 2 {
			return errors.New(field.acceptedValuesDescription())
		}

		seen := make(map[int]struct{}, len(parts))
		for _, part := range parts {
			number, err := parseCronNumber(part)
			if err != nil || number < field.minimum || number > field.maximum {
				return errors.New(field.acceptedValuesDescription())
			}
			if _, exists := seen[number]; exists {
				return fmt.Errorf("%s; list values must be unique", field.acceptedValuesDescription())
			}
			seen[number] = struct{}{}
		}

		return nil
	}

	number, err := parseCronNumber(value)
	if err != nil || number < field.minimum || number > field.maximum {
		return errors.New(field.acceptedValuesDescription())
	}

	return nil
}

func parseCronNumber(value string) (int, error) {
	if value == "" {
		return 0, strconv.ErrSyntax
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, strconv.ErrSyntax
		}
	}

	return strconv.Atoi(value)
}

func (v cronFieldValidator) acceptedValuesDescription() string {
	forms := []string{
		`"*"`,
		fmt.Sprintf("an integer between %d and %d", v.minimum, v.maximum),
	}
	if v.allowStep {
		forms = append(forms, `"*/N"`)
	}
	if v.allowList {
		forms = append(forms, "a comma-separated list of integers")
	}

	return fmt.Sprintf(
		"The cron %s must be %s.",
		v.name,
		strings.Join(forms, ", "),
	)
}

func cronMinuteValidators() []validator.String {
	return []validator.String{cronFieldValidator{
		name:      "minute",
		minimum:   0,
		maximum:   59,
		allowList: true,
		allowStep: true,
	}}
}

func cronHourValidators() []validator.String {
	return []validator.String{cronFieldValidator{
		name:      "hour",
		minimum:   0,
		maximum:   23,
		allowList: true,
		allowStep: true,
	}}
}

func cronDayValidators() []validator.String {
	return []validator.String{cronFieldValidator{
		name:      "day",
		minimum:   1,
		maximum:   31,
		allowStep: true,
	}}
}

func cronWeekdayValidators() []validator.String {
	return []validator.String{cronFieldValidator{
		name:    "weekday",
		minimum: 0,
		maximum: 7,
	}}
}

func cronMonthValidators() []validator.String {
	return []validator.String{cronFieldValidator{
		name:      "month",
		minimum:   1,
		maximum:   12,
		allowStep: true,
	}}
}
