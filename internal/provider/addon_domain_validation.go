package provider

import (
	"fmt"
	"strings"
)

func validateAddonDomainInternalSubdomain(name string) error {
	if name == "" {
		return fmt.Errorf("internal subdomain must not be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("internal subdomain must not exceed 63 ASCII characters")
	}
	if !domainLabelPattern.MatchString(name) ||
		strings.HasPrefix(name, "-") ||
		strings.HasSuffix(name, "-") {
		return fmt.Errorf(
			"internal subdomain may contain only letters, numbers, and non-edge hyphens",
		)
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("internal subdomain must be lowercase")
	}

	return nil
}
