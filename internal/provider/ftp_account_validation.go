package provider

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/ftp"
)

var (
	ftpAccountPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)
	ftpUserPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

func ftpAccountValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(3, 254),
		stringvalidator.RegexMatches(
			ftpAccountPattern,
			"FTP account names must contain one user and one domain separated by @.",
		),
	}
}

func validateFTPAccountUsername(
	ctx context.Context,
	client *ftp.Client,
	username string,
) (string, string, error) {
	user, domain, err := splitFTPAccountUsername(username)
	if err != nil {
		return "", "", err
	}

	domains, err := client.ListDomains(ctx)
	if err != nil {
		return "", "", fmt.Errorf("read cPanel domains: %w", err)
	}
	if !slices.Contains(domains, domain) {
		return "", "", fmt.Errorf(
			"domain %q is not available for FTP accounts on this cPanel account",
			domain,
		)
	}

	return user, domain, nil
}

func splitFTPAccountUsername(username string) (string, string, error) {
	if strings.Count(username, "@") != 1 {
		return "", "", fmt.Errorf("FTP account username must contain exactly one @")
	}

	user, domain, _ := strings.Cut(username, "@")
	if user == "" || domain == "" {
		return "", "", fmt.Errorf("FTP account username must include a user and domain")
	}
	if len(user) > 64 {
		return "", "", fmt.Errorf("FTP account user must not exceed 64 ASCII characters")
	}
	if !ftpUserPattern.MatchString(user) {
		return "", "", fmt.Errorf(
			"FTP account user may contain only letters, numbers, dots, underscores, and hyphens",
		)
	}
	if strings.HasPrefix(user, ".") || strings.HasSuffix(user, ".") || strings.Contains(user, "..") {
		return "", "", fmt.Errorf("FTP account user contains an invalid dot placement")
	}
	if domain != strings.ToLower(domain) {
		return "", "", fmt.Errorf("FTP account domain must be lowercase")
	}
	if len(domain) > 253 {
		return "", "", fmt.Errorf("FTP account domain must not exceed 253 ASCII characters")
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", "", fmt.Errorf("FTP account domain must contain at least one dot")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return "", "", fmt.Errorf("FTP account domain contains an invalid label")
		}
		if !domainLabelPattern.MatchString(label) ||
			strings.HasPrefix(label, "-") ||
			strings.HasSuffix(label, "-") {
			return "", "", fmt.Errorf("FTP account domain contains an invalid label %q", label)
		}
	}

	return user, domain, nil
}

func validateFTPHomeDirectory(homeDirectory string) error {
	if homeDirectory == "" {
		return fmt.Errorf("FTP home directory must not be empty")
	}
	if strings.HasPrefix(homeDirectory, "/") {
		return fmt.Errorf("FTP home directory must be relative to the cPanel account home")
	}
	if strings.ContainsRune(homeDirectory, '\x00') {
		return fmt.Errorf("FTP home directory must not contain a null byte")
	}
	if path.Clean(homeDirectory) != homeDirectory ||
		homeDirectory == "." ||
		homeDirectory == ".." ||
		strings.HasPrefix(homeDirectory, "../") {
		return fmt.Errorf(
			"FTP home directory must be a normalized relative path without . or .. segments",
		)
	}

	return nil
}
