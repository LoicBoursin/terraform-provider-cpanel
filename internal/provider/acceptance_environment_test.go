package provider

import (
	"strings"
	"testing"
)

func TestValidateAcceptanceEnvironment(t *testing.T) {
	validEnvironment := map[string]string{
		"CPANEL_HOST":                   "https://cpanel.test:2083/",
		"CPANEL_USERNAME":               "account",
		"CPANEL_API_TOKEN":              "provider-secret",
		"CPANEL_API_TOKEN_NAME":         "provider-token",
		"CPANEL_EXPECTED_TEST_HOST":     "https://cpanel.test:2083",
		"CPANEL_EXPECTED_TEST_USERNAME": "account",
		"CPANEL_ACCEPT_DESTRUCTIVE":     "1",
	}

	tests := map[string]struct {
		overrides map[string]string
		wantError string
	}{
		"valid": {},
		"missing required value": {
			overrides: map[string]string{
				"CPANEL_API_TOKEN_NAME": "",
			},
			wantError: "CPANEL_API_TOKEN_NAME must be set",
		},
		"destructive opt-in required": {
			overrides: map[string]string{
				"CPANEL_ACCEPT_DESTRUCTIVE": "0",
			},
			wantError: "CPANEL_ACCEPT_DESTRUCTIVE must be set to 1",
		},
		"host must match": {
			overrides: map[string]string{
				"CPANEL_EXPECTED_TEST_HOST": "https://other.test:2083",
			},
			wantError: "CPANEL_HOST must match CPANEL_EXPECTED_TEST_HOST",
		},
		"username must match": {
			overrides: map[string]string{
				"CPANEL_EXPECTED_TEST_USERNAME": "other",
			},
			wantError: "CPANEL_USERNAME must match CPANEL_EXPECTED_TEST_USERNAME",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for variable, value := range validEnvironment {
				t.Setenv(variable, value)
			}
			for variable, value := range test.overrides {
				t.Setenv(variable, value)
			}

			err := validateAcceptanceEnvironment()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf(
						"validateAcceptanceEnvironment() error = %v, want nil",
						err,
					)
				}

				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf(
					"validateAcceptanceEnvironment() error = %v, want containing %q",
					err,
					test.wantError,
				)
			}
		})
	}
}
