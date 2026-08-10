package postgresql

import "context"

func (c *Client) CreateUser(
	ctx context.Context,
	input UserCreateModel,
) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeMutation(ctx, OperationCreateUser, map[string]string{
		"name":     input.Name,
		"password": input.Password,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) DeleteUser(
	ctx context.Context,
	input UserDeleteModel,
) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeMutation(
		ctx,
		OperationDeleteUser,
		map[string]string{"name": input.Name},
		&postgreSQLUser,
	)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) GrantAllPrivileges(
	ctx context.Context,
	input UserGrantAllPrivilegesModel,
) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeMutation(ctx, OperationGrantAllPrivileges, map[string]string{
		"database": input.Database,
		"user":     input.User,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) GetUsers(ctx context.Context) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeReadOperation(ctx, OperationListUsers, map[string]string{}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) RenameUser(
	ctx context.Context,
	input UserRenameModel,
) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeMutation(ctx, OperationRenameUser, map[string]string{
		"newname":  input.NewName,
		"oldname":  input.OldName,
		"password": input.Password,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) RevokeAllPrivileges(
	ctx context.Context,
	input UserRevokeAllPrivilegesModel,
) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeMutation(ctx, OperationRevokeAllPrivileges, map[string]string{
		"database": input.Database,
		"user":     input.User,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) SetPassword(
	ctx context.Context,
	input UserSetPasswordModel,
) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeMutation(ctx, OperationSetPassword, map[string]string{
		"user":     input.User,
		"password": input.Password,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) UserExists(ctx context.Context, name string) (bool, error) {
	user, err := c.GetUsers(ctx)
	if err != nil {
		return false, err
	}

	for _, u := range user.Data {
		if u == name {
			return true, nil
		}
	}

	return false, nil
}
