package provider

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

func gitRepositoryNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 255),
	}
}

func gitRepositoryRootValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
	}
}

func gitSourceRepositoryURLValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
	}
}

func validateGitRepositoryDefinition(
	definition versioncontrol.Definition,
) error {
	if err := versioncontrol.ValidateRepositoryRoot(
		definition.RepositoryRoot,
	); err != nil {
		return err
	}
	if definition.Name == "" {
		return fmt.Errorf("git repository name must not be empty")
	}
	if len(definition.Name) > 255 {
		return fmt.Errorf("git repository name must not exceed 255 bytes")
	}
	if strings.TrimSpace(definition.Name) != definition.Name {
		return fmt.Errorf(
			"git repository name must not have leading or trailing whitespace",
		)
	}
	for _, character := range definition.Name {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"git repository name must not contain control characters",
			)
		}
	}

	if definition.SourceRepositoryURL == "" {
		return nil
	}
	if len(definition.SourceRepositoryURL) > 4096 {
		return fmt.Errorf(
			"git source repository URL must not exceed 4096 bytes",
		)
	}
	parsedURL, err := url.Parse(definition.SourceRepositoryURL)
	if err != nil {
		return fmt.Errorf("git source repository URL is invalid")
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "ssh" {
		return fmt.Errorf(
			"git source repository URL must use HTTPS or SSH",
		)
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("git source repository URL must include a host")
	}
	if parsedURL.User != nil {
		_, hasPassword := parsedURL.User.Password()
		if parsedURL.Scheme == "https" || hasPassword {
			return fmt.Errorf(
				"git source repository URL must not embed credentials",
			)
		}
	}
	if parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return fmt.Errorf(
			"git source repository URL must not contain a query or fragment",
		)
	}

	return nil
}
