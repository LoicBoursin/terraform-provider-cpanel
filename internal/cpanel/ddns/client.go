package ddns

import (
	"context"
	"fmt"
	"net/http"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) List(ctx context.Context) ([]Domain, error) {
	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDynamicDNS,
		operationList,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) Get(ctx context.Context, domain string) (*Domain, error) {
	domains, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, dynamicDomain := range domains {
		if dynamicDomain.Domain == domain {
			domainCopy := dynamicDomain

			return &domainCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) Create(
	ctx context.Context,
	domain string,
	description string,
) (*CreatedDomain, error) {
	response := CreateResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDynamicDNS,
		operationCreate,
		map[string]string{
			"domain":      domain,
			"description": description,
		},
		&response,
	); err != nil {
		return nil, err
	}
	if response.Data.ID == "" {
		return nil, fmt.Errorf("cPanel returned an empty Dynamic DNS ID")
	}

	return &response.Data, nil
}

func (c *Client) SetDescription(
	ctx context.Context,
	id string,
	description string,
) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDynamicDNS,
		operationSetDescription,
		map[string]string{
			"id":          id,
			"description": description,
		},
		&response,
	)
}

func (c *Client) Recreate(ctx context.Context, id string) (string, error) {
	response := RecreateResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDynamicDNS,
		operationRecreate,
		map[string]string{"id": id},
		&response,
	); err != nil {
		return "", err
	}
	if response.Data.ID == "" {
		return "", fmt.Errorf("cPanel returned an empty recreated Dynamic DNS ID")
	}

	return response.Data.ID, nil
}

func (c *Client) Delete(ctx context.Context, id string) (bool, error) {
	response := DeleteResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDynamicDNS,
		operationDelete,
		map[string]string{"id": id},
		&response,
	); err != nil {
		return false, err
	}

	return response.Data.Deleted == 1, nil
}
