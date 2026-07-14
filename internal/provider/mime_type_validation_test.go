package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

func TestValidateMIMETypeDefinition(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		definition mimetype.Definition
		valid      bool
	}{
		"valid": {
			definition: mimetype.Definition{
				Type:       "application/x-example",
				Extensions: []string{".foo", ".tar.gz"},
			},
			valid: true,
		},
		"uppercase type": {
			definition: mimetype.Definition{
				Type:       "Application/X-Example",
				Extensions: []string{".foo"},
			},
		},
		"missing subtype": {
			definition: mimetype.Definition{
				Type:       "application",
				Extensions: []string{".foo"},
			},
		},
		"empty extensions": {
			definition: mimetype.Definition{
				Type: "application/x-example",
			},
		},
		"missing dot": {
			definition: mimetype.Definition{
				Type:       "application/x-example",
				Extensions: []string{"foo"},
			},
		},
		"extension whitespace": {
			definition: mimetype.Definition{
				Type:       "application/x-example",
				Extensions: []string{".foo bar"},
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateMIMETypeDefinition(testCase.definition)
			if got := err == nil; got != testCase.valid {
				t.Fatalf(
					"validateMIMETypeDefinition(%#v) validity = %t, want %t; error: %v",
					testCase.definition,
					got,
					testCase.valid,
					err,
				)
			}
		})
	}
}
