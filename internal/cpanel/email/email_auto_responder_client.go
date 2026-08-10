package email

import (
	"context"
	"strconv"
)

func (c *Client) SetAutoResponder(
	ctx context.Context,
	user string,
	domain string,
	autoResponder AutoResponder,
) error {
	isHTML := "0"
	if autoResponder.IsHTML == 1 {
		isHTML = "1"
	}

	response := AutoResponderResponse{}

	return c.executeMutation(ctx, operationAddAutoResponder, map[string]string{
		"email":    user,
		"domain":   domain,
		"from":     autoResponder.From,
		"subject":  autoResponder.Subject,
		"body":     autoResponder.Body,
		"charset":  autoResponder.Charset,
		"interval": strconv.FormatInt(autoResponder.Interval, 10),
		"is_html":  isHTML,
		"start":    strconv.FormatInt(autoResponder.StartUnix(), 10),
		"stop":     strconv.FormatInt(autoResponder.StopUnix(), 10),
	}, &response)
}

func (c *Client) DeleteAutoResponder(
	ctx context.Context,
	address string,
) error {
	response := AutoResponderResponse{}

	return c.executeMutation(ctx, operationDeleteAutoResp, map[string]string{
		"email": address,
	}, &response)
}

func (c *Client) ListAutoResponders(
	ctx context.Context,
	domain string,
) ([]AutoResponderSummary, error) {
	response := AutoResponderListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListAutoResp,
		map[string]string{"domain": domain},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) GetAutoResponder(
	ctx context.Context,
	address string,
	domain string,
) (*AutoResponder, error) {
	summaries, err := c.ListAutoResponders(ctx, domain)
	if err != nil {
		return nil, err
	}

	found := false
	for _, summary := range summaries {
		if summary.Email == address {
			found = true
			break
		}
	}
	if !found {
		return nil, nil
	}

	response := AutoResponderResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationGetAutoResponder,
		map[string]string{"email": address},
		&response,
	); err != nil {
		return nil, err
	}

	response.Data.Email = address

	return &response.Data, nil
}
