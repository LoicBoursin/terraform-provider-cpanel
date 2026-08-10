package domain

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

func (c *Client) executeAPI2ReadOperation(
	ctx context.Context,
	module string,
	function string,
	parameters map[string]string,
	output any,
) error {
	return c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		module,
		function,
		parameters,
		output,
	)
}

func (c *Client) executeAPI2Mutation(
	ctx context.Context,
	module string,
	function string,
	parameters map[string]string,
	output *API2MutationResponse,
) error {
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		module,
		function,
		parameters,
		output,
	); err != nil {
		return err
	}

	for _, result := range output.CpanelResult.Data {
		if result.Result == 1 {
			continue
		}

		messages := []string{}
		if result.Reason != "" {
			messages = append(messages, result.Reason)
		}

		return &cpanel.APIError{
			API:      "API 2",
			Module:   module,
			Function: function,
			Messages: messages,
		}
	}

	return nil
}
