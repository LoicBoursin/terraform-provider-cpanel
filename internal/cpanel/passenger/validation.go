package passenger

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const (
	DeploymentModeDevelopment = "development"
	DeploymentModeProduction  = "production"
)

var environmentVariableNamePattern = regexp.MustCompile(
	`^[A-Za-z_-][A-Za-z0-9_-]{0,255}$`,
)

var reservedApplicationDirectories = []string{
	".cpanel",
	".cphorde",
	".htpasswds",
	".spamassassin",
	".ssh",
	".trash",
	"cgi-bin",
	"etc",
	"logs",
	"mail",
	"perl5",
	"ssl",
	"tmp",
	"var",
}

func ValidateDefinition(definition Definition) error {
	if err := ValidateName(definition.Name); err != nil {
		return err
	}
	if err := ValidatePath(definition.Path); err != nil {
		return err
	}
	if definition.Domain == "" {
		return fmt.Errorf("passenger application domain must not be empty")
	}
	if err := ValidateBaseURI(definition.BaseURI); err != nil {
		return err
	}
	switch definition.DeploymentMode {
	case DeploymentModeDevelopment, DeploymentModeProduction:
	default:
		return fmt.Errorf(
			"unsupported Passenger deployment mode %q",
			definition.DeploymentMode,
		)
	}
	if err := ValidateEnvironmentVariables(
		definition.EnvironmentVariables,
	); err != nil {
		return err
	}

	return nil
}

func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("passenger application name must not be empty")
	}
	if len(name) > 50 {
		return fmt.Errorf(
			"passenger application name must not exceed 50 bytes",
		)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf(
			"passenger application name must not have leading or trailing whitespace",
		)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"passenger application name must not contain control characters",
			)
		}
	}

	return nil
}

func ValidatePath(applicationPath string) error {
	if applicationPath == "" {
		return fmt.Errorf("passenger application path must not be empty")
	}
	if len(applicationPath) > 4096 {
		return fmt.Errorf(
			"passenger application path must not exceed 4096 bytes",
		)
	}
	if path.IsAbs(applicationPath) {
		return fmt.Errorf(
			"passenger application path must be relative to the cPanel account home",
		)
	}
	if path.Clean(applicationPath) != applicationPath ||
		applicationPath == "." ||
		applicationPath == ".." ||
		strings.HasPrefix(applicationPath, "../") {
		return fmt.Errorf(
			"passenger application path must be a normalized relative path without . or .. segments",
		)
	}
	for _, character := range applicationPath {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf(
				"passenger application path must not contain whitespace or control characters",
			)
		}
	}
	firstSegment, _, _ := strings.Cut(applicationPath, "/")
	if slices.Contains(reservedApplicationDirectories, firstSegment) {
		return fmt.Errorf(
			"passenger application path may not use reserved directory %q",
			firstSegment,
		)
	}

	return nil
}

func ValidateBaseURI(baseURI string) error {
	if baseURI == "" {
		return fmt.Errorf("passenger application base URI must not be empty")
	}
	if len(baseURI) > 2048 {
		return fmt.Errorf(
			"passenger application base URI must not exceed 2048 bytes",
		)
	}
	if !strings.HasPrefix(baseURI, "/") {
		return fmt.Errorf("passenger application base URI must begin with /")
	}
	if path.Clean(baseURI) != baseURI {
		return fmt.Errorf(
			"passenger application base URI must be a normalized absolute URL path",
		)
	}
	if strings.ContainsAny(baseURI, "?#\\") {
		return fmt.Errorf(
			"passenger application base URI must not contain a query, fragment, or backslash",
		)
	}
	for _, character := range baseURI {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf(
				"passenger application base URI must not contain whitespace or control characters",
			)
		}
	}

	return nil
}

func ValidateEnvironmentVariables(environment map[string]string) error {
	for name, value := range environment {
		if !environmentVariableNamePattern.MatchString(name) {
			return fmt.Errorf(
				"passenger environment variable name %q must contain only letters, numbers, underscores, or dashes and must not begin with a number",
				name,
			)
		}
		if value == "" {
			return fmt.Errorf(
				"passenger environment variable %q must not be empty",
				name,
			)
		}
		if len(value) > 1024 {
			return fmt.Errorf(
				"passenger environment variable %q must not exceed 1024 bytes",
				name,
			)
		}
		for _, character := range value {
			if character < 0x20 || character > 0x7e {
				return fmt.Errorf(
					"passenger environment variable %q must contain only printable ASCII characters",
					name,
				)
			}
		}
	}

	return nil
}
