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

func directoryPrivacyUsernameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 255),
	}
}

func directoryPrivacyUserPasswordValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtLeast(1),
	}
}

func validateDirectoryPrivacyUserIdentity(
	directory string,
	username string,
) error {
	if err := validateDirectoryIndexDefinition(
		directoryindex.Definition{
			Directory: directory,
			Type:      directoryindex.IndexTypeInherit,
		},
	); err != nil {
		return err
	}
	if strings.Contains(directory, "|") {
		return fmt.Errorf(
			"directory privacy user directory must not contain |",
		)
	}
	if username == "" {
		return fmt.Errorf("directory privacy username must not be empty")
	}
	if len(username) > 255 {
		return fmt.Errorf(
			"directory privacy username must not exceed 255 bytes",
		)
	}
	if strings.TrimSpace(username) != username {
		return fmt.Errorf(
			"directory privacy username must not have leading or trailing whitespace",
		)
	}
	if strings.ContainsAny(username, ":|") {
		return fmt.Errorf(
			"directory privacy username must not contain : or |",
		)
	}
	for _, character := range username {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"directory privacy username must not contain control characters",
			)
		}
	}

	return nil
}

func validateDirectoryPrivacyUserDefinition(
	definition directoryprivacy.UserDefinition,
) error {
	if err := validateDirectoryPrivacyUserIdentity(
		definition.Directory,
		definition.Username,
	); err != nil {
		return err
	}
	if definition.Password == "" {
		return fmt.Errorf("directory privacy user password must not be empty")
	}

	return nil
}

func parseDirectoryPrivacyUserImportID(
	importID string,
) (string, string, error) {
	if strings.Count(importID, "|") != 1 {
		return "", "", fmt.Errorf(
			"directory privacy user import identifier must use directory|username",
		)
	}

	directory, username, _ := strings.Cut(importID, "|")
	if err := validateDirectoryPrivacyUserIdentity(
		directory,
		username,
	); err != nil {
		return "", "", err
	}

	return directory, username, nil
}
