package provider

import (
	"context"
	"fmt"
	"slices"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func validateEmailDomain(
	ctx context.Context,
	client *cpanelmail.Client,
	domain string,
) error {
	if err := validateDomainName(domain); err != nil {
		return err
	}

	domains, err := client.ListMailDomains(ctx)
	if err != nil {
		return fmt.Errorf("read cPanel mail domains: %w", err)
	}
	if !slices.Contains(domains, domain) {
		return fmt.Errorf(
			"domain %q is not available for email on this cPanel account",
			domain,
		)
	}

	return nil
}
