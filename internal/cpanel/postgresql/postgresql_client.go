package postgresql

import (
	"context"
	"net/http"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(c *cpanel.Client) *Client {
	return &Client{
		Client: c,
	}
}

func (c *Client) executeReadOperation(
	ctx context.Context,
	function string,
	parameters map[string]string,
	output any,
) error {
	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModulePostgresql,
		function,
		parameters,
		output,
	)
}

func (c *Client) executeMutation(
	ctx context.Context,
	function string,
	parameters map[string]string,
	output any,
) error {
	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModulePostgresql,
		function,
		parameters,
		output,
	)
}
