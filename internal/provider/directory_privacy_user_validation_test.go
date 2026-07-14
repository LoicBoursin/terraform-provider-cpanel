package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestValidateDirectoryPrivacyUserDefinition(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition directoryprivacy.UserDefinition
		wantError  bool
	}{
		"valid": {
			definition: directoryprivacy.UserDefinition{
				Directory: "public_html/private",
				Username:  "alice",
				Password:  "Secret-2026!",
			},
		},
		"invalid directory": {
			definition: directoryprivacy.UserDefinition{
				Directory: "../private",
				Username:  "alice",
				Password:  "Secret-2026!",
			},
			wantError: true,
		},
		"ambiguous directory": {
			definition: directoryprivacy.UserDefinition{
				Directory: "public_html/private|archive",
				Username:  "alice",
				Password:  "Secret-2026!",
			},
			wantError: true,
		},
		"empty username": {
			definition: directoryprivacy.UserDefinition{
				Directory: "public_html/private",
				Password:  "Secret-2026!",
			},
			wantError: true,
		},
		"colon username": {
			definition: directoryprivacy.UserDefinition{
				Directory: "public_html/private",
				Username:  "alice:admin",
				Password:  "Secret-2026!",
			},
			wantError: true,
		},
		"surrounding whitespace": {
			definition: directoryprivacy.UserDefinition{
				Directory: "public_html/private",
				Username:  " alice ",
				Password:  "Secret-2026!",
			},
			wantError: true,
		},
		"empty password": {
			definition: directoryprivacy.UserDefinition{
				Directory: "public_html/private",
				Username:  "alice",
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateDirectoryPrivacyUserDefinition(test.definition)
			if test.wantError && err == nil {
				t.Fatal("validateDirectoryPrivacyUserDefinition() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateDirectoryPrivacyUserDefinition() error: %v",
					err,
				)
			}
		})
	}
}

func TestParseDirectoryPrivacyUserImportID(t *testing.T) {
	t.Parallel()

	directory, username, err := parseDirectoryPrivacyUserImportID(
		"public_html/private|alice",
	)
	if err != nil {
		t.Fatalf("parseDirectoryPrivacyUserImportID() error: %v", err)
	}
	if directory != "public_html/private" || username != "alice" {
		t.Fatalf("identity = %q, %q", directory, username)
	}

	for _, importID := range []string{
		"",
		"public_html/private",
		"public_html/private|",
		"|alice",
		"public_html/private|alice|extra",
	} {
		if _, _, err := parseDirectoryPrivacyUserImportID(importID); err == nil {
			t.Fatalf(
				"parseDirectoryPrivacyUserImportID(%q) returned no error",
				importID,
			)
		}
	}
}
