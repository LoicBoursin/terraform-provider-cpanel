package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func validateEmailForwarderSource(
	ctx context.Context,
	client *cpanelmail.Client,
	address string,
) (string, error) {
	_, domain, err := validateEmailAccountAddress(ctx, client, address)
	if err != nil {
		return "", err
	}

	return domain, nil
}

func validateEmailForwarderDestination(address string) error {
	if len(address) > 254 {
		return errors.New("forwarder destination must not exceed 254 ASCII characters")
	}
	if strings.ContainsAny(address, "|, \t\r\n") {
		return errors.New(
			"forwarder destination must be one email address without separators or whitespace",
		)
	}
	if strings.Count(address, "@") != 1 {
		return errors.New("forwarder destination must contain exactly one @")
	}

	localPart, domain, _ := strings.Cut(address, "@")
	if localPart == "" || domain == "" {
		return errors.New("forwarder destination must include a local part and domain")
	}
	if len(localPart) > 64 {
		return errors.New("forwarder destination local part must not exceed 64 characters")
	}
	if domain != strings.ToLower(domain) {
		return errors.New("forwarder destination domain must be lowercase")
	}
	if err := validateDomainName(domain); err != nil {
		return fmt.Errorf("invalid forwarder destination domain: %w", err)
	}

	return nil
}
