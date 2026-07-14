package provider

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func directoryPrivacyAuthNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 255),
	}
}

func validateDirectoryPrivacyDefinition(
	definition directoryprivacy.Definition,
) error {
	if err := validateDirectoryIndexDefinition(
		directoryindex.Definition{
			Directory: definition.Directory,
			Type:      directoryindex.IndexTypeInherit,
		},
	); err != nil {
		return err
	}
	if definition.AuthName == "" {
		return fmt.Errorf("directory privacy auth name must not be empty")
	}
	if len(definition.AuthName) > 255 {
		return fmt.Errorf(
			"directory privacy auth name must not exceed 255 bytes",
		)
	}
	if strings.TrimSpace(definition.AuthName) != definition.AuthName {
		return fmt.Errorf(
			"directory privacy auth name must not have leading or trailing whitespace",
		)
	}
	for _, character := range definition.AuthName {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"directory privacy auth name must not contain control characters",
			)
		}
	}

	return nil
}
