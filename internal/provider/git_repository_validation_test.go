package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

func TestValidateGitRepositoryDefinition(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition versioncontrol.Definition
		wantError  bool
	}{
		"empty repository": {
			definition: versioncontrol.Definition{
				Name:           "Website",
				RepositoryRoot: "repositories/site",
			},
		},
		"HTTPS source": {
			definition: versioncontrol.Definition{
				Name:                "Website",
				RepositoryRoot:      "repositories/site",
				SourceRepositoryURL: "https://example.com/site.git",
			},
		},
		"SSH source": {
			definition: versioncontrol.Definition{
				Name:                "Website",
				RepositoryRoot:      "repositories/site",
				SourceRepositoryURL: "ssh://git@example.com/site.git",
			},
		},
		"invalid root": {
			definition: versioncontrol.Definition{
				Name:           "Website",
				RepositoryRoot: "../site",
			},
			wantError: true,
		},
		"empty name": {
			definition: versioncontrol.Definition{
				RepositoryRoot: "repositories/site",
			},
			wantError: true,
		},
		"surrounding name whitespace": {
			definition: versioncontrol.Definition{
				Name:           " Website ",
				RepositoryRoot: "repositories/site",
			},
			wantError: true,
		},
		"insecure source": {
			definition: versioncontrol.Definition{
				Name:                "Website",
				RepositoryRoot:      "repositories/site",
				SourceRepositoryURL: "http://example.com/site.git",
			},
			wantError: true,
		},
		"embedded password": {
			definition: versioncontrol.Definition{
				Name:                "Website",
				RepositoryRoot:      "repositories/site",
				SourceRepositoryURL: "https://user:token@example.com/site.git",
			},
			wantError: true,
		},
		"embedded username or token": {
			definition: versioncontrol.Definition{
				Name:                "Website",
				RepositoryRoot:      "repositories/site",
				SourceRepositoryURL: "https://token@example.com/site.git",
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateGitRepositoryDefinition(test.definition)
			if test.wantError && err == nil {
				t.Fatal("validateGitRepositoryDefinition() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateGitRepositoryDefinition() error: %v",
					err,
				)
			}
		})
	}
}
