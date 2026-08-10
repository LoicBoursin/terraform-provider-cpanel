package email

import (
	"context"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateDomainForwarder(
	ctx context.Context,
	domain string,
	destination string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationAddDomainForward, map[string]string{
		"domain":     domain,
		"destdomain": destination,
	}, &response)
}

func (c *Client) DeleteDomainForwarder(
	ctx context.Context,
	domain string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationDeleteDomainFwd, map[string]string{
		"domain": domain,
	}, &response)
}

func (c *Client) ListDomainForwarders(
	ctx context.Context,
) ([]DomainForwarder, error) {
	return c.listDomainForwardersStrict(ctx)
}

func (c *Client) GetDomainForwarder(
	ctx context.Context,
	domain string,
) (*DomainForwarder, error) {
	forwarders, err := c.ListDomainForwarders(ctx)
	if err != nil {
		return nil, err
	}

	for _, forwarder := range forwarders {
		if forwarder.Domain == domain {
			forwarderCopy := forwarder

			return &forwarderCopy, nil
		}
	}

	return nil, nil
}
