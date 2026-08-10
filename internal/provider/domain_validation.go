package provider

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

func domainNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(3, 253),
	}
}

func validateDomainName(name string) error {
	if name != strings.ToLower(name) {
		return fmt.Errorf("domain name must be lowercase")
	}
	if len(name) > 253 {
		return fmt.Errorf("domain name must not exceed 253 ASCII characters")
	}

	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain name must contain at least one dot")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("domain name contains an invalid label")
		}
		if !domainLabelPattern.MatchString(label) ||
			strings.HasPrefix(label, "-") ||
			strings.HasSuffix(label, "-") {
			return fmt.Errorf("domain name contains an invalid label %q", label)
		}
	}

	return nil
}

func resolveSubdomainParts(
	ctx context.Context,
	client *cpaneldomain.Client,
	domain string,
) (string, string, error) {
	if err := validateDomainName(domain); err != nil {
		return "", "", err
	}

	baseDomains, err := client.ListBaseDomains(ctx)
	if err != nil {
		return "", "", fmt.Errorf("read cPanel base domains: %w", err)
	}

	var rootDomain string
	for _, candidate := range baseDomains {
		if domain == candidate || !strings.HasSuffix(domain, "."+candidate) {
			continue
		}
		if len(candidate) > len(rootDomain) {
			rootDomain = candidate
		}
	}
	if rootDomain == "" {
		return "", "", fmt.Errorf(
			"domain %q is not a subdomain of a main or addon domain on this cPanel account",
			domain,
		)
	}

	subdomain := strings.TrimSuffix(domain, "."+rootDomain)
	if subdomain == "" {
		return "", "", fmt.Errorf("subdomain name must not be empty")
	}

	return subdomain, rootDomain, nil
}

func validateDomainDocumentRoot(documentRoot string) error {
	if documentRoot == "" {
		return fmt.Errorf("document root must not be empty")
	}
	if strings.HasPrefix(documentRoot, "/") {
		return fmt.Errorf("document root must be relative to the cPanel account home")
	}
	if strings.ContainsRune(documentRoot, '\x00') {
		return fmt.Errorf("document root must not contain a null byte")
	}
	if path.Clean(documentRoot) != documentRoot ||
		documentRoot == "." ||
		documentRoot == ".." ||
		strings.HasPrefix(documentRoot, "../") {
		return fmt.Errorf(
			"document root must be a normalized relative path without . or .. segments",
		)
	}

	firstSegment, _, _ := strings.Cut(documentRoot, "/")
	reserved := []string{
		".cpanel",
		".cphorde",
		".htpasswds",
		".spamassassin",
		".ssh",
		".trash",
		"cgi-bin",
		"etc",
		"logs",
		"mail",
		"perl5",
		"ssl",
		"tmp",
		"var",
	}
	if slices.Contains(reserved, firstSegment) {
		return fmt.Errorf("document root may not use reserved directory %q", firstSegment)
	}

	return nil
}
