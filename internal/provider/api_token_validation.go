package provider

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var apiTokenNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func apiTokenNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 50),
		stringvalidator.RegexMatches(
			apiTokenNamePattern,
			"API token name may contain only letters, numbers, dashes, and underscores",
		),
	}
}

func apiTokenExpirationValidators() []validator.Int64 {
	return []validator.Int64{
		int64validator.AtLeast(0),
	}
}

func validateAPITokenExpiration(expiresAt, now int64) error {
	if expiresAt < 0 {
		return fmt.Errorf("API token expiration must not be negative")
	}
	if expiresAt != 0 && expiresAt <= now {
		return fmt.Errorf(
			"API token expiration must be 0 or later than the current Unix timestamp",
		)
	}

	return nil
}
