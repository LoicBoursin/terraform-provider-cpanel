package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

func TestValidateModSecurityDomainDefinition(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		definition modsecurity.Definition
		wantError  bool
	}{
		"valid": {
			definition: modsecurity.Definition{
				Domain:  "www.example.test",
				Enabled: true,
			},
		},
		"uppercase": {
			definition: modsecurity.Definition{
				Domain:  "WWW.example.test",
				Enabled: true,
			},
			wantError: true,
		},
		"missing dot": {
			definition: modsecurity.Definition{
				Domain: "localhost",
			},
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateModSecurityDomainDefinition(testCase.definition)
			if testCase.wantError && err == nil {
				t.Fatal("validateModSecurityDomainDefinition() returned no error")
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateModSecurityDomainDefinition() error: %v",
					err,
				)
			}
		})
	}
}
