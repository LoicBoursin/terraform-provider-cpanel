package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

func TestValidateApacheHandlerDefinition(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		definition apachehandler.Definition
		valid      bool
	}{
		"valid": {
			definition: apachehandler.Definition{
				Extension: ".foo",
				Handler:   "example-handler",
			},
			valid: true,
		},
		"missing extension dot": {
			definition: apachehandler.Definition{
				Extension: "foo",
				Handler:   "example-handler",
			},
		},
		"handler whitespace": {
			definition: apachehandler.Definition{
				Extension: ".foo",
				Handler:   "example handler",
			},
		},
		"empty handler": {
			definition: apachehandler.Definition{
				Extension: ".foo",
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateApacheHandlerDefinition(testCase.definition)
			if got := err == nil; got != testCase.valid {
				t.Fatalf(
					"validateApacheHandlerDefinition(%#v) validity = %t, want %t; error: %v",
					testCase.definition,
					got,
					testCase.valid,
					err,
				)
			}
		})
	}
}
