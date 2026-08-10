package apachehandler

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

func (c *Client) ListUser(ctx context.Context) ([]Handler, error) {
	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleMime,
		operationList,
		map[string]string{"type": "user"},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) Get(
	ctx context.Context,
	extension string,
) (*Handler, error) {
	handlers, err := c.ListUser(ctx)
	if err != nil {
		return nil, err
	}

	var match *Handler
	for _, candidate := range handlers {
		if candidate.Extension != extension {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf(
				"multiple user Apache handlers match extension %q",
				extension,
			)
		}
		candidateCopy := candidate
		match = &candidateCopy
	}

	return match, nil
}

func (c *Client) Add(
	ctx context.Context,
	definition Definition,
) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleMime,
		operationAdd,
		map[string]string{
			"extension": definition.Extension,
			"handler":   definition.Handler,
		},
		&response,
	)
}

func (c *Client) Delete(
	ctx context.Context,
	extension string,
) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleMime,
		operationDelete,
		map[string]string{"extension": extension},
		&response,
	)
}
