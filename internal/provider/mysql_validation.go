package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

type mySQLNameKind string

const (
	mySQLDatabaseName mySQLNameKind = "database"
	mySQLUserName     mySQLNameKind = "user"
)

var mySQLNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func mySQLNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 64),
		stringvalidator.RegexMatches(
			mySQLNamePattern,
			"MySQL names may contain only letters, numbers, and underscores.",
		),
	}
}

func validateMySQLAccountName(
	ctx context.Context,
	client *mysql.Client,
	name string,
	kind mySQLNameKind,
) error {
	restrictions, err := client.GetRestrictions(ctx)
	if err != nil {
		return fmt.Errorf("read MySQL name restrictions: %w", err)
	}

	maxLength := restrictions.MaxDatabaseNameLength
	if kind == mySQLUserName {
		maxLength = restrictions.MaxUsernameLength
	}

	return validateMySQLName(name, restrictions.Prefix, maxLength)
}

func validateMySQLName(name, prefix string, maxLength int) error {
	if !strings.HasPrefix(name, prefix) {
		return fmt.Errorf("name must start with the cPanel database prefix %q", prefix)
	}
	if name == prefix {
		return fmt.Errorf("name must include at least one character after the cPanel database prefix %q", prefix)
	}
	if len(name) > maxLength {
		return fmt.Errorf("name must not exceed %d ASCII characters", maxLength)
	}
	if !mySQLNamePattern.MatchString(name) {
		return fmt.Errorf("name may contain only letters, numbers, and underscores")
	}

	return nil
}
