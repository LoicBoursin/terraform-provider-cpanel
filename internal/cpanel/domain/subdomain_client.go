package domain

import (
	"context"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateSubdomain(
	ctx context.Context,
	subdomain string,
	rootDomain string,
	documentRoot string,
) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModuleSubDomain,
		operationAddSubdomain,
		map[string]string{
			"domain":      subdomain,
			"rootdomain":  rootDomain,
			"dir":         documentRoot,
			"disallowdot": "0",
		},
		&response,
	)
}

func (c *Client) DeleteSubdomain(ctx context.Context, domain string) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModuleSubDomain,
		operationDeleteSubdomain,
		map[string]string{"domain": domain},
		&response,
	)
}

func (c *Client) DeleteFilePath(ctx context.Context, filePath string) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModuleFileman,
		operationDeleteFilePath,
		map[string]string{
			"op":           "unlink",
			"sourcefiles":  filePath,
			"doubledecode": "0",
		},
		&response,
	)
}

func (c *Client) SetSubdomainDocumentRoot(
	ctx context.Context,
	subdomain string,
	rootDomain string,
	documentRoot string,
) error {
	response := API2MutationResponse{}

	return c.executeAPI2Mutation(
		ctx,
		cpanel.ModuleSubDomain,
		operationChangeSubdomainDocumentRoot,
		map[string]string{
			"subdomain":  subdomain,
			"rootdomain": rootDomain,
			"dir":        documentRoot,
		},
		&response,
	)
}

func (c *Client) ListSubdomains(ctx context.Context) (*SubdomainListResponse, error) {
	response := SubdomainListResponse{}
	if err := c.executeAPI2ReadOperation(
		ctx,
		cpanel.ModuleSubDomain,
		operationListSubdomains,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) GetSubdomain(ctx context.Context, name string) (*Subdomain, error) {
	response, err := c.ListSubdomains(ctx)
	if err != nil {
		return nil, err
	}

	for _, subdomain := range response.CpanelResult.Data {
		if subdomain.Domain == name {
			subdomainCopy := subdomain
			return &subdomainCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) ListBaseDomains(ctx context.Context) ([]string, error) {
	domainList, err := c.getDomainList(ctx)
	if err != nil {
		return nil, err
	}

	domains := []string{domainList.MainDomain}
	domains = append(domains, domainList.AddonDomains...)

	return domains, nil
}
