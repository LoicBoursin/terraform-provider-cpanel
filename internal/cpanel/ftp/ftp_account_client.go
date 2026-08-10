package ftp

import (
	"context"
	"net/http"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateAccount(
	ctx context.Context,
	user string,
	domain string,
	password string,
	homeDirectory string,
	quotaMiB int64,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationAddAccount, map[string]string{
		"user":    user,
		"domain":  domain,
		"pass":    password,
		"homedir": homeDirectory,
		"quota":   strconv.FormatInt(quotaMiB, 10),
	}, &response)
}

func (c *Client) DeleteAccount(
	ctx context.Context,
	user string,
	domain string,
	deleteHomeDirectory bool,
) error {
	response := cpanel.UAPIDataSourceModel{}

	destroy := "0"
	if deleteHomeDirectory {
		destroy = "1"
	}

	return c.executeMutation(ctx, operationDeleteAccount, map[string]string{
		"user":    user,
		"domain":  domain,
		"destroy": destroy,
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
		"user":   user,
		"domain": domain,
		"pass":   password,
	}, &response)
}

func (c *Client) SetQuota(
	ctx context.Context,
	user string,
	domain string,
	quotaMiB int64,
) error {
	response := cpanel.UAPIDataSourceModel{}

	kill := "0"
	if quotaMiB == 0 {
		kill = "1"
	}

	return c.executeMutation(ctx, operationSetQuota, map[string]string{
		"user":   user,
		"domain": domain,
		"quota":  strconv.FormatInt(quotaMiB, 10),
		"kill":   kill,
	}, &response)
}

func (c *Client) SetHomeDirectory(
	ctx context.Context,
	user string,
	domain string,
	homeDirectory string,
) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationSetHomeDirectory, map[string]string{
		"user":    user,
		"domain":  domain,
		"homedir": homeDirectory,
	}, &response)
}

func (c *Client) ListAccounts(ctx context.Context) (*AccountListResponse, error) {
	response := AccountListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListAccounts,
		map[string]string{"include_acct_types": "sub"},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) GetAccount(ctx context.Context, login string) (*Account, error) {
	response, err := c.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}

	for _, account := range response.Data {
		if account.Login == login && account.AccountType == "sub" {
			accountCopy := account
			return &accountCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) ListDomains(ctx context.Context) ([]string, error) {
	response := DomainListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		"DomainInfo",
		"list_domains",
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	domains := []string{response.Data.MainDomain}
	domains = append(domains, response.Data.AddonDomains...)
	domains = append(domains, response.Data.SubDomains...)
	domains = append(domains, response.Data.ParkedDomains...)

	return domains, nil
}
