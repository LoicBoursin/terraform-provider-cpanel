package cron

import (
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

func (c *Client) executeOperation(function string, queryParams map[string]string, inputModel interface{}) error {
	return c.Client.ExecuteAPI2Operation(cpanel.ModuleCron, function, queryParams, inputModel)
}
