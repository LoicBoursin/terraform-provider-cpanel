package domain

import (
	"context"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateDomainAlias(ctx context.Context, domain string) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModulePark,
		operationAddDomainAlias,
		map[string]string{
			"domain":      domain,
			"disallowdot": "0",
		},
		&response,
	)
}

func (c *Client) DeleteDomainAlias(ctx context.Context, domain string) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModulePark,
		operationDeleteDomainAlias,
		map[string]string{"domain": domain},
		&response,
	)
}

func (c *Client) ListDomainAliases(ctx context.Context) (*DomainAliasListResponse, error) {
	response := DomainAliasListResponse{}
	if err := c.executeAPI2ReadOperation(
		ctx,
		cpanel.ModulePark,
		operationListDomainAliases,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) GetDomainAlias(ctx context.Context, name string) (*DomainAlias, error) {
	response, err := c.ListDomainAliases(ctx)
	if err != nil {
		return nil, err
	}

	for _, domainAlias := range response.CpanelResult.Data {
		if domainAlias.Domain == name {
			domainAliasCopy := domainAlias
			return &domainAliasCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) GetMainDomain(ctx context.Context) (string, error) {
	domainList, err := c.getDomainList(ctx)
	if err != nil {
		return "", err
	}

	return domainList.MainDomain, nil
}
