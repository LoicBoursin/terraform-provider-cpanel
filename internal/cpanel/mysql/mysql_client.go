package mysql

import (
	"context"
	"net/http"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
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
		cpanel.ModuleMysql,
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
		cpanel.ModuleMysql,
		function,
		parameters,
		output,
	)
}
