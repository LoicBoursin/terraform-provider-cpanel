package email

import (
	"context"
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
	response := MailDomainListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListMailDomains,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	domains := make([]string, 0, len(response.Data))
	for _, domain := range response.Data {
		domains = append(domains, domain.Domain)
	}

	return domains, nil
}
