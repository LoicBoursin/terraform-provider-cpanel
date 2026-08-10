package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var localeCodeValidationPattern = regexp.MustCompile(
	`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`,
)

func localeCodeValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 64),
		stringvalidator.RegexMatches(
			localeCodeValidationPattern,
			"must contain lowercase letters, digits, and underscore-separated segments",
		),
	}
}
