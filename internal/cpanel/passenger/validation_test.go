package passenger

import "testing"

func TestValidateDefinition(t *testing.T) {
	t.Parallel()

	valid := Definition{
		Name:           "Terraform Passenger",
		Path:           "applications/site",
		Domain:         "example.test",
		BaseURI:        "/application",
		DeploymentMode: DeploymentModeProduction,
		Enabled:        true,
		EnvironmentVariables: map[string]string{
			"APP_ENV": "production",
		},
	}
	if err := ValidateDefinition(valid); err != nil {
		t.Fatalf("ValidateDefinition() error: %v", err)
	}

	tests := map[string]Definition{
		"empty name": {
			Path:           valid.Path,
			Domain:         valid.Domain,
			BaseURI:        valid.BaseURI,
			DeploymentMode: valid.DeploymentMode,
		},
		"absolute path": {
			Name:           valid.Name,
			Path:           "/home/example/app",
			Domain:         valid.Domain,
			BaseURI:        valid.BaseURI,
			DeploymentMode: valid.DeploymentMode,
		},
		"reserved path": {
			Name:           valid.Name,
			Path:           ".ssh/app",
			Domain:         valid.Domain,
			BaseURI:        valid.BaseURI,
			DeploymentMode: valid.DeploymentMode,
		},
		"relative base uri": {
			Name:           valid.Name,
			Path:           valid.Path,
			Domain:         valid.Domain,
			BaseURI:        "application",
			DeploymentMode: valid.DeploymentMode,
		},
		"unsupported mode": {
			Name:           valid.Name,
			Path:           valid.Path,
			Domain:         valid.Domain,
			BaseURI:        valid.BaseURI,
			DeploymentMode: "staging",
		},
		"invalid environment name": {
			Name:           valid.Name,
			Path:           valid.Path,
			Domain:         valid.Domain,
			BaseURI:        valid.BaseURI,
			DeploymentMode: valid.DeploymentMode,
			EnvironmentVariables: map[string]string{
				"1APP": "value",
			},
		},
		"non printable environment value": {
			Name:           valid.Name,
			Path:           valid.Path,
			Domain:         valid.Domain,
			BaseURI:        valid.BaseURI,
			DeploymentMode: valid.DeploymentMode,
			EnvironmentVariables: map[string]string{
				"APP": "line\nbreak",
			},
		},
	}
	for name, definition := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := ValidateDefinition(definition); err == nil {
				t.Fatal("ValidateDefinition() returned no error")
			}
		})
	}
}
