package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var (
	emailAddressPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)
	emailUserPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	domainLabelPattern  = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
)

func emailAddressValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(3, 254),
		stringvalidator.RegexMatches(
			emailAddressPattern,
			"Email addresses must contain one local part and one domain separated by @.",
		),
	}
}

func validateEmailAccountAddress(
	ctx context.Context,
	client emailMailDomainClient,
	address string,
) (string, string, error) {
	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		return "", "", err
	}

	domains, err := client.ListMailDomains(ctx)
	if err != nil {
		return "", "", fmt.Errorf("read cPanel mail domains: %w", err)
	}
	if !slices.Contains(domains, domain) {
		return "", "", fmt.Errorf(
			"domain %q is not available for email accounts on this cPanel account",
			domain,
		)
	}

	return user, domain, nil
}

func splitEmailAccountAddress(address string) (string, string, error) {
	if strings.Count(address, "@") != 1 {
		return "", "", fmt.Errorf("email address must contain exactly one @")
	}

	user, domain, _ := strings.Cut(address, "@")
	if user == "" || domain == "" {
		return "", "", fmt.Errorf("email address must include a user and domain")
	}
	if len(user) > 64 {
		return "", "", fmt.Errorf("email account user must not exceed 64 ASCII characters")
	}
	if !emailUserPattern.MatchString(user) {
		return "", "", fmt.Errorf(
			"email account user may contain only letters, numbers, dots, underscores, and hyphens",
		)
	}
	if strings.HasPrefix(user, ".") || strings.HasSuffix(user, ".") || strings.Contains(user, "..") {
		return "", "", fmt.Errorf("email account user contains an invalid dot placement")
	}
	if domain != strings.ToLower(domain) {
		return "", "", fmt.Errorf("email account domain must be lowercase")
	}
	if len(domain) > 253 {
		return "", "", fmt.Errorf("email account domain must not exceed 253 ASCII characters")
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", "", fmt.Errorf("email account domain must contain at least one dot")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return "", "", fmt.Errorf("email account domain contains an invalid label")
		}
		if !domainLabelPattern.MatchString(label) ||
			strings.HasPrefix(label, "-") ||
			strings.HasSuffix(label, "-") {
			return "", "", fmt.Errorf("email account domain contains an invalid label %q", label)
		}
	}

	return user, domain, nil
}
