package mimetype

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

func (c *Client) ListUser(ctx context.Context) ([]MIMEType, error) {
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
	mimeType string,
) (*MIMEType, error) {
	mimeTypes, err := c.ListUser(ctx)
	if err != nil {
		return nil, err
	}

	var match *MIMEType
	for _, candidate := range mimeTypes {
		if candidate.Type != mimeType {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf(
				"multiple user MIME type entries match %q",
				mimeType,
			)
		}
		candidateCopy := candidate
		match = &candidateCopy
	}

	return match, nil
}

func (c *Client) AddExtension(
	ctx context.Context,
	mimeType string,
	extension string,
) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleMime,
		operationAdd,
		map[string]string{
			"type":      mimeType,
			"extension": extension,
		},
		&response,
	)
}

func (c *Client) Delete(
	ctx context.Context,
	mimeType string,
) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleMime,
		operationDelete,
		map[string]string{"type": mimeType},
		&response,
	)
}
