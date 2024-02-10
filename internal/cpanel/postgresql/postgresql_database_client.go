package postgresql

func (c *Client) CreateDatabase(input DatabaseCreateModel) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeOperation(OperationCreateDatabase, map[string]string{"name": input.Name}, &postgreSQLDatabase)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}

func (c *Client) DeleteDatabase(input DatabaseDeleteModel) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeOperation(OperationDeleteDatabase, map[string]string{"name": input.Name}, &postgreSQLDatabase)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}

func (c *Client) GetDatabases() (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeOperation(OperationListDatabases, map[string]string{}, &postgreSQLDatabase)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}

func (c *Client) UpdateDatabase(input DatabaseUpdateModel) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeOperation(OperationRenameDatabase, map[string]string{
		"oldname": input.OldName,
		"newname": input.NewName,
	}, &postgreSQLDatabase)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}
