package provider

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var postgreSQLNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func postgreSQLNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 63),
		stringvalidator.RegexMatches(
			postgreSQLNamePattern,
			"PostgreSQL names may contain only letters, numbers, and underscores.",
		),
	}
}

func validatePostgreSQLAccountName(username, name string) error {
	prefix := username + "_"
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("name must start with the cPanel account prefix %q", prefix)
	}
	if name == prefix {
		return fmt.Errorf("name must include at least one character after the cPanel account prefix %q", prefix)
	}
	if len(name) > 63 {
		return fmt.Errorf("name must not exceed 63 ASCII characters")
	}
	if !postgreSQLNamePattern.MatchString(name) {
		return fmt.Errorf("name may contain only letters, numbers, and underscores")
	}

	return nil
}
