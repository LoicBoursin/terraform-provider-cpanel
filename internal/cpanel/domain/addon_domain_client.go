package domain

import (
	"context"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateAddonDomain(
	ctx context.Context,
	domain string,
	internalSubdomain string,
	documentRoot string,
) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModuleAddonDomain,
		operationAddAddonDomain,
		map[string]string{
			"newdomain":       domain,
			"subdomain":       internalSubdomain,
			"dir":             documentRoot,
			"ftp_is_optional": "1",
		},
		&response,
	)
}

func (c *Client) DeleteAddonDomain(
	ctx context.Context,
	domain string,
	domainKey string,
) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModuleAddonDomain,
		operationDeleteAddonDomain,
		map[string]string{
			"domain":    domain,
			"subdomain": domainKey,
		},
		&response,
	)
}

func (c *Client) ListAddonDomains(ctx context.Context) (*AddonDomainListResponse, error) {
	response := AddonDomainListResponse{}
	if err := c.executeAPI2ReadOperation(
		ctx,
		cpanel.ModuleAddonDomain,
		operationListAddonDomains,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) GetAddonDomain(ctx context.Context, name string) (*AddonDomain, error) {
	response, err := c.ListAddonDomains(ctx)
	if err != nil {
		return nil, err
	}

	for _, addonDomain := range response.CpanelResult.Data {
		if addonDomain.Domain == name {
			addonDomainCopy := addonDomain
			return &addonDomainCopy, nil
		}
	}

	return nil, nil
}
