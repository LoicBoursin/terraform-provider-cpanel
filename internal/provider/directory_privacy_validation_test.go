package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestValidateDirectoryPrivacyDefinition(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition directoryprivacy.Definition
		wantError  bool
	}{
		"valid": {
			definition: directoryprivacy.Definition{
				Directory: "public_html/private",
				AuthName:  "Private files",
			},
		},
		"invalid directory": {
			definition: directoryprivacy.Definition{
				Directory: "../private",
				AuthName:  "Private files",
			},
			wantError: true,
		},
		"empty auth name": {
			definition: directoryprivacy.Definition{
				Directory: "public_html/private",
			},
			wantError: true,
		},
		"surrounding whitespace": {
			definition: directoryprivacy.Definition{
				Directory: "public_html/private",
				AuthName:  " Private files ",
			},
			wantError: true,
		},
		"newline": {
			definition: directoryprivacy.Definition{
				Directory: "public_html/private",
				AuthName:  "Private\nfiles",
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateDirectoryPrivacyDefinition(test.definition)
			if test.wantError && err == nil {
				t.Fatal("validateDirectoryPrivacyDefinition() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateDirectoryPrivacyDefinition() error: %v",
					err,
				)
			}
		})
	}
}
