package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateAPITokenExpiration(t *testing.T) {
	t.Parallel()

	const now = int64(1784044689)
	testCases := map[string]struct {
		expiresAt int64
		wantError bool
	}{
		"no expiration": {
			expiresAt: 0,
		},
		"future": {
			expiresAt: now + 1,
		},
		"current time": {
			expiresAt: now,
			wantError: true,
		},
		"past": {
			expiresAt: now - 1,
			wantError: true,
		},
		"negative": {
			expiresAt: -1,
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateAPITokenExpiration(testCase.expiresAt, now)
			if testCase.wantError && err == nil {
				t.Fatalf(
					"validateAPITokenExpiration(%d) returned no error",
					testCase.expiresAt,
				)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateAPITokenExpiration(%d) error: %v",
					testCase.expiresAt,
					err,
				)
			}
		})
	}
}

func TestAPITokenNamePattern(t *testing.T) {
	t.Parallel()

	testCases := map[string]bool{
		"terraform-token_1": true,
		"TerraformToken":    true,
		"with spaces":       false,
		"with.dot":          false,
		"":                  false,
	}

	for value, expected := range testCases {
		if got := apiTokenNamePattern.MatchString(value); got != expected {
			t.Errorf(
				"apiTokenNamePattern.MatchString(%q) = %t, want %t",
				value,
				got,
				expected,
			)
		}
	}
}

func TestAPITokenNameValidators(t *testing.T) {
	t.Parallel()

	testCases := map[string]bool{
		"valid":                 true,
		strings.Repeat("a", 50): true,
		strings.Repeat("a", 51): false,
		"with spaces":           false,
		"":                      false,
	}

	for value, expectedValid := range testCases {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			request := validator.StringRequest{
				ConfigValue: types.StringValue(value),
				Path:        path.Root("name"),
			}
			response := &validator.StringResponse{}
			for _, nameValidator := range apiTokenNameValidators() {
				nameValidator.ValidateString(
					context.Background(),
					request,
					response,
				)
			}

			if got := !response.Diagnostics.HasError(); got != expectedValid {
				t.Fatalf(
					"apiTokenNameValidators() validity for %q = %t, want %t; diagnostics: %v",
					value,
					got,
					expectedValid,
					response.Diagnostics,
				)
			}
		})
	}
}
