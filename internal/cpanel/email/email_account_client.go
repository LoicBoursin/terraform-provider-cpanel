package email

import (
	"context"
	"fmt"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateAccount(
	ctx context.Context,
	user string,
	domain string,
	password string,
	quotaMiB int64,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationAddAccount, map[string]string{
		"email":              user,
		"domain":             domain,
		"password":           password,
		"quota":              strconv.FormatInt(quotaMiB, 10),
		"send_welcome_email": "0",
	}, &response)
}

func (c *Client) DeleteAccount(
	ctx context.Context,
	user string,
	domain string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationDeleteAccount, map[string]string{
		"email":  user,
		"domain": domain,
	}, &response)
}

func (c *Client) SetPassword(
	ctx context.Context,
	user string,
	domain string,
	password string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationSetPassword, map[string]string{
		"email":    user,
		"domain":   domain,
		"password": password,
	}, &response)
}

func (c *Client) SetQuota(
	ctx context.Context,
	user string,
	domain string,
	quotaMiB int64,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationEditAccountQuota, map[string]string{
		"email":  user,
		"domain": domain,
		"quota":  strconv.FormatInt(quotaMiB, 10),
	}, &response)
}

func (c *Client) ListAccounts(ctx context.Context) (*AccountListResponse, error) {
	response := AccountListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListAccounts,
		map[string]string{
			"get_restrictions": "1",
			"no_disk":          "0",
		},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) GetAccount(
	ctx context.Context,
	user string,
	domain string,
) (*Account, error) {
	response := AccountListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListAccounts,
		map[string]string{
			"email":            user,
			"domain":           domain,
			"get_restrictions": "1",
			"no_disk":          "0",
		},
		&response,
	); err != nil {
		return nil, err
	}

	address := user + "@" + domain
	for _, account := range response.Data {
		if account.Email == address {
			accountCopy := account
			return &accountCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) ListMailDomains(ctx context.Context) ([]string, error) {
	return c.listMailDomainsStrict(ctx)
}

func (c *Client) SetLoginSuspended(
	ctx context.Context,
	address string,
	suspended bool,
) error {
	return c.setAccountSuspension(
		ctx,
		address,
		suspended,
		operationSuspendLogin,
		operationUnsuspendLogin,
	)
}

func (c *Client) SetIncomingSuspended(
	ctx context.Context,
	address string,
	suspended bool,
) error {
	return c.setAccountSuspension(
		ctx,
		address,
		suspended,
		operationSuspendIncoming,
		operationUnsuspendIncoming,
	)
}

func (c *Client) SetOutgoingSuspended(
	ctx context.Context,
	address string,
	suspended bool,
) error {
	return c.setAccountSuspension(
		ctx,
		address,
		suspended,
		operationSuspendOutgoing,
		operationUnsuspendOutgoing,
	)
}

func (c *Client) setAccountSuspension(
	ctx context.Context,
	address string,
	suspended bool,
	suspendOperation string,
	unsuspendOperation string,
) error {
	if address == "" {
		return fmt.Errorf("email account address must not be empty")
	}

	operation := unsuspendOperation
	if suspended {
		operation = suspendOperation
	}
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(
		ctx,
		operation,
		map[string]string{"email": address},
		&response,
	)
}
