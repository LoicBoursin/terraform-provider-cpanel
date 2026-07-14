package provider

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

var apacheHandlerPattern = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9._+-]*$`,
)

func apacheHandlerValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 128),
		stringvalidator.RegexMatches(
			apacheHandlerPattern,
			"must contain only letters, digits, dots, underscores, plus signs, or hyphens",
		),
	}
}

func validateApacheHandlerDefinition(
	definition apachehandler.Definition,
) error {
	if !mimeExtensionPattern.MatchString(definition.Extension) {
		return fmt.Errorf("invalid Apache handler extension %q", definition.Extension)
	}
	if !apacheHandlerPattern.MatchString(definition.Handler) {
		return fmt.Errorf("invalid Apache handler name %q", definition.Handler)
	}

	return nil
}
