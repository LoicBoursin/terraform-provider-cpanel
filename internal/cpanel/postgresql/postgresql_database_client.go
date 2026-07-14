package postgresql

import "context"

func (c *Client) CreateDatabase(
	ctx context.Context,
	input DatabaseCreateModel,
) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeMutation(
		ctx,
		OperationCreateDatabase,
		map[string]string{"name": input.Name},
		&postgreSQLDatabase,
	)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}

func (c *Client) DeleteDatabase(
	ctx context.Context,
	input DatabaseDeleteModel,
) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeMutation(
		ctx,
		OperationDeleteDatabase,
		map[string]string{"name": input.Name},
		&postgreSQLDatabase,
	)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}

func (c *Client) GetDatabases(ctx context.Context) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeReadOperation(
		ctx,
		OperationListDatabases,
		map[string]string{},
		&postgreSQLDatabase,
	)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}

func (c *Client) UpdateDatabase(
	ctx context.Context,
	input DatabaseUpdateModel,
) (*DatabaseDataSourceModel, error) {
	postgreSQLDatabase := DatabaseDataSourceModel{}
	err := c.executeMutation(ctx, OperationRenameDatabase, map[string]string{
		"oldname": input.OldName,
		"newname": input.NewName,
	}, &postgreSQLDatabase)

	if err != nil {
		return nil, err
	}

	return &postgreSQLDatabase, nil
}
