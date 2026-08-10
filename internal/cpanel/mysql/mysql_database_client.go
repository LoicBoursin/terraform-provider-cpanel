package mysql

import (
	"context"
	"fmt"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateDatabase(ctx context.Context, name string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(
		ctx,
		operationCreateDatabase,
		map[string]string{"name": name},
		&response,
	)
}

func (c *Client) DeleteDatabase(ctx context.Context, name string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(
		ctx,
		operationDeleteDatabase,
		map[string]string{"name": name},
		&response,
	)
}

func (c *Client) ListDatabases(ctx context.Context) (*DatabaseListResponse, error) {
	response := DatabaseListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListDatabases,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) RenameDatabase(ctx context.Context, oldName, newName string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationRenameDatabase, map[string]string{
		"oldname": oldName,
		"newname": newName,
	}, &response)
}

func (c *Client) GrantAllPrivileges(ctx context.Context, user, database string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationSetPrivilegesOnDatabase, map[string]string{
		"user":       user,
		"database":   database,
		"privileges": allPrivileges,
	}, &response)
}

func (c *Client) RevokeAccess(ctx context.Context, user, database string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationRevokeAccessToDatabase, map[string]string{
		"user":     user,
		"database": database,
	}, &response)
}

func (c *Client) GetPrivileges(
	ctx context.Context,
	user string,
	database string,
) ([]string, error) {
	response := PrivilegesResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationGetPrivilegesOnDatabase,
		map[string]string{
			"user":     user,
			"database": database,
		},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func (c *Client) GetRestrictions(ctx context.Context) (*Restrictions, error) {
	response := restrictionsResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationGetRestrictions,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectMySQLInventoryWarnings(
		"MySQL restrictions",
		response.Warnings,
	); err != nil {
		return nil, err
	}

	if response.Data == nil {
		return nil, fmt.Errorf(
			"cPanel MySQL restrictions data must be a non-null object",
		)
	}

	return response.Data, nil
}
