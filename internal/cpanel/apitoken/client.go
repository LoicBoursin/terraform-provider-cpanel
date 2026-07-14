package apitoken

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) List(ctx context.Context) ([]Token, error) {
	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleTokens,
		operationList,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) Get(ctx context.Context, name string) (*Token, error) {
	tokens, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, token := range tokens {
		if token.Name == name {
			tokenCopy := token

			return &tokenCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) Create(
	ctx context.Context,
	name string,
	expiresAt int64,
) (*CreatedToken, error) {
	parameters := map[string]string{"name": name}
	if expiresAt != 0 {
		parameters["expires_at"] = strconv.FormatInt(expiresAt, 10)
	}

	response := CreateResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleTokens,
		operationCreate,
		parameters,
		&response,
	); err != nil {
		return nil, err
	}
	if response.Data.Token == "" {
		return &response.Data, fmt.Errorf("cPanel returned an empty API token")
	}

	return &response.Data, nil
}

func (c *Client) Rename(ctx context.Context, name, newName string) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleTokens,
		operationRename,
		map[string]string{
			"name":     name,
			"new_name": newName,
		},
		&response,
	)
}

func (c *Client) Revoke(ctx context.Context, name string) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleTokens,
		operationRevoke,
		map[string]string{"name": name},
		&response,
	)
}
