package redirect

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	TypePermanent  = "permanent"
	TypeTemporary  = "temporary"
	WWWModeBoth    = "both"
	WWWModeWithout = "without"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) List(ctx context.Context) ([]Redirect, error) {
	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleMime,
		operationList,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) Get(
	ctx context.Context,
	domain string,
	source string,
) (*Redirect, error) {
	redirects, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	var match *Redirect
	for _, candidate := range redirects {
		if candidate.Domain != domain || candidate.Source != source {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf(
				"multiple redirects match domain %q and source %q",
				domain,
				source,
			)
		}
		candidateCopy := candidate
		match = &candidateCopy
	}

	return match, nil
}

func (c *Client) Add(ctx context.Context, definition Definition) error {
	response := MutationResponse{}

	redirectType := "permanent"
	if definition.Type == TypeTemporary {
		redirectType = "temp"
	}
	redirectWWW := 0
	if definition.WWWMode == WWWModeWithout {
		redirectWWW = 1
	}
	redirectWildcard := 0
	if definition.Wildcard {
		redirectWildcard = 1
	}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleMime,
		operationAdd,
		map[string]string{
			"domain":            definition.Domain,
			"src":               definition.Source,
			"redirect":          definition.Destination,
			"type":              redirectType,
			"redirect_www":      strconv.Itoa(redirectWWW),
			"redirect_wildcard": strconv.Itoa(redirectWildcard),
		},
		&response,
	)
}

func (c *Client) Delete(
	ctx context.Context,
	domain string,
	source string,
) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleMime,
		operationDelete,
		map[string]string{
			"domain": domain,
			"src":    source,
		},
		&response,
	)
}
