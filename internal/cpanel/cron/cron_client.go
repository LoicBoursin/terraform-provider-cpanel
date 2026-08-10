package cron

import (
	"context"
	"net/http"
	"sync"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client

	mutationMu sync.Mutex
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
	return c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleCron,
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
	return c.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleCron,
		function,
		parameters,
		output,
	)
}
