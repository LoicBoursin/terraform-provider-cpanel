package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

func TestValidateDirectoryIndexDefinition(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition directoryindex.Definition
		wantError  bool
	}{
		"valid": {
			definition: directoryindex.Definition{
				Directory: "public_html/downloads",
				Type:      directoryindex.IndexTypeFancy,
			},
		},
		"empty": {
			definition: directoryindex.Definition{
				Type: directoryindex.IndexTypeFancy,
			},
			wantError: true,
		},
		"absolute": {
			definition: directoryindex.Definition{
				Directory: "/home/example/public_html",
				Type:      directoryindex.IndexTypeFancy,
			},
			wantError: true,
		},
		"parent segment": {
			definition: directoryindex.Definition{
				Directory: "public_html/../etc",
				Type:      directoryindex.IndexTypeFancy,
			},
			wantError: true,
		},
		"trailing slash": {
			definition: directoryindex.Definition{
				Directory: "public_html/downloads/",
				Type:      directoryindex.IndexTypeFancy,
			},
			wantError: true,
		},
		"unsupported type": {
			definition: directoryindex.Definition{
				Directory: "public_html/downloads",
				Type:      "unexpected",
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateDirectoryIndexDefinition(test.definition)
			if test.wantError && err == nil {
				t.Fatal("validateDirectoryIndexDefinition() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateDirectoryIndexDefinition() error: %v",
					err,
				)
			}
		})
	}
}
