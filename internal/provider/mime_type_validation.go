package provider

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

var (
	mimeTypePattern = regexp.MustCompile(
		`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`,
	)
	mimeExtensionPattern = regexp.MustCompile(
		`^\.[A-Za-z0-9][A-Za-z0-9._+-]*$`,
	)
)

func mimeTypeValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(3, 255),
		stringvalidator.RegexMatches(
			mimeTypePattern,
			"must be a lowercase media type such as application/example",
		),
	}
}

func mimeExtensionValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(2, 128),
		stringvalidator.RegexMatches(
			mimeExtensionPattern,
			"must begin with a dot and contain only letters, digits, dots, underscores, plus signs, or hyphens",
		),
	}
}

func validateMIMETypeDefinition(definition mimetype.Definition) error {
	if !mimeTypePattern.MatchString(definition.Type) {
		return fmt.Errorf(
			"MIME type must be lowercase and contain exactly one type/subtype separator",
		)
	}
	if len(definition.Extensions) == 0 {
		return fmt.Errorf("MIME type must contain at least one extension")
	}
	for _, extension := range definition.Extensions {
		if !mimeExtensionPattern.MatchString(extension) {
			return fmt.Errorf("invalid MIME extension %q", extension)
		}
	}

	return nil
}
