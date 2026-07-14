package email

import (
	"context"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateForwarder(
	ctx context.Context,
	address string,
	domain string,
	destination string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationAddForwarder, map[string]string{
		"domain":   domain,
		"email":    address,
		"fwdopt":   "fwd",
		"fwdemail": destination,
	}, &response)
}

func (c *Client) DeleteForwarder(
	ctx context.Context,
	address string,
	destination string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationDeleteForwarder, map[string]string{
		"address":   address,
		"forwarder": destination,
	}, &response)
}

func (c *Client) ListForwarders(
	ctx context.Context,
	domain string,
) ([]Forwarder, error) {
	response := ForwarderListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListForwarders,
		map[string]string{"domain": domain},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) GetForwarder(
	ctx context.Context,
	domain string,
	address string,
	destination string,
) (*Forwarder, error) {
	forwarders, err := c.ListForwarders(ctx, domain)
	if err != nil {
		return nil, err
	}

	for _, forwarder := range forwarders {
		if forwarder.Address == address && forwarder.Destination == destination {
			forwarderCopy := forwarder

			return &forwarderCopy, nil
		}
	}

	return nil, nil
}
