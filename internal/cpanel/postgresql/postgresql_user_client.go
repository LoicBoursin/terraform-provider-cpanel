package postgresql

func (c *Client) CreateUser(input UserCreateModel) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationCreateUser, map[string]string{
		"name":     input.Name,
		"password": input.Password,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) DeleteUser(input UserDeleteModel) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationDeleteUser, map[string]string{"name": input.Name}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) GrantAllPrivileges(input UserGrantAllPrivilegesModel) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationGrantAllPrivileges, map[string]string{
		"database": input.Database,
		"user":     input.User,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) GetUsers() (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationListUsers, map[string]string{}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) RenameUser(input UserRenameModel) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationRenameUser, map[string]string{
		"newname":  input.NewName,
		"oldname":  input.OldName,
		"password": input.Password,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) RevokeAllPrivileges(input UserRevokeAllPrivilegesModel) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationRevokeAllPrivileges, map[string]string{
		"database": input.Database,
		"user":     input.User,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) SetPassword(input UserSetPasswordModel) (*UserDataSourceModel, error) {
	postgreSQLUser := UserDataSourceModel{}
	err := c.executeOperation(OperationSetPassword, map[string]string{
		"user":     input.User,
		"password": input.Password,
	}, &postgreSQLUser)

	if err != nil {
		return nil, err
	}

	return &postgreSQLUser, nil
}

func (c *Client) UserExists(name string) (bool, error) {
	user, err := c.GetUsers()
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
