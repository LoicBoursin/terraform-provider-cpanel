package mysql

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) CreateUser(ctx context.Context, name, password string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationCreateUser, map[string]string{
		"name":     name,
		"password": password,
	}, &response)
}

func (c *Client) DeleteUser(ctx context.Context, name string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(
		ctx,
		operationDeleteUser,
		map[string]string{"name": name},
		&response,
	)
}

func (c *Client) ListUsers(ctx context.Context) (*UserListResponse, error) {
	response := UserListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListUsers,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *Client) RenameUser(ctx context.Context, oldName, newName string) error {
	response := cpanel.UAPIDataSourceModel{}

	return c.executeMutation(ctx, operationRenameUser, map[string]string{
		"oldname": oldName,
		"newname": newName,
	}, &response)
}

func (c *Client) SetPassword(ctx context.Context, user, password string) error {
	response := SetPasswordResponse{}

	if err := c.executeMutation(ctx, operationSetPassword, map[string]string{
		"user":     user,
		"password": password,
	}, &response); err != nil {
		return err
	}

	if len(response.Data.Failures) == 0 {
		return nil
	}

	messages := make([]string, 0, len(response.Data.Failures))
	for _, failure := range response.Data.Failures {
		message := failure.Error
		if failure.Host != "" {
			message = fmt.Sprintf("%s: %s", failure.Host, message)
		}
		messages = append(messages, message)
	}

	return fmt.Errorf("set MySQL password: %s", strings.Join(messages, "; "))
}

func (c *Client) UserExists(ctx context.Context, name string) (bool, error) {
	users, err := c.ListUsers(ctx)
	if err != nil {
		return false, err
	}

	for _, user := range users.Data {
		if user.User == name {
			return true, nil
		}
	}

	return false, nil
}
