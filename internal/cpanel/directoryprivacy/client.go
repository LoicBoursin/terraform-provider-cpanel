package directoryprivacy

import (
	"context"
	"fmt"
	"net/http"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

type Client struct {
	*cpanel.Client

	directoryClient *directoryindex.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{
		Client:          client,
		directoryClient: directoryindex.NewClient(client),
	}
}

func (c *Client) Get(
	ctx context.Context,
	directory string,
) (*Privacy, error) {
	absoluteDirectory, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		directory,
	)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	response := Response{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDirectoryPrivacy,
		operationGet,
		map[string]string{"dir": absoluteDirectory},
		&response,
	); err != nil {
		return nil, err
	}

	return privacyFromResponse(directory, absoluteDirectory, response.Data)
}

func (c *Client) Configure(
	ctx context.Context,
	directory string,
	authName string,
	enabled bool,
) (*Privacy, error) {
	absoluteDirectory, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		directory,
	)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("directory %q does not exist", directory)
	}

	parameters := map[string]string{
		"dir": absoluteDirectory,
	}
	if enabled {
		parameters["enabled"] = "1"
		parameters["authname"] = authName
	} else {
		parameters["enabled"] = "0"
	}

	response := Response{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDirectoryPrivacy,
		operationConfigure,
		parameters,
		&response,
	); err != nil {
		return nil, err
	}

	privacy, err := privacyFromResponse(
		directory,
		absoluteDirectory,
		response.Data,
	)
	if err != nil {
		return nil, err
	}
	if privacy.Protected != enabled {
		return nil, fmt.Errorf(
			"directory %q protection mutation returned protected=%t; expected %t",
			directory,
			privacy.Protected,
			enabled,
		)
	}
	if enabled && privacy.AuthName != authName {
		return nil, fmt.Errorf(
			"directory %q protection mutation returned auth name %q; expected %q",
			directory,
			privacy.AuthName,
			authName,
		)
	}

	return privacy, nil
}

func (c *Client) ListUsers(
	ctx context.Context,
	directory string,
) ([]User, error) {
	absoluteDirectory, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		directory,
	)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	response := UserListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDirectoryPrivacy,
		operationListUsers,
		map[string]string{"dir": absoluteDirectory},
		&response,
	); err != nil {
		return nil, err
	}

	users := make([]User, 0, len(response.Data))
	for _, username := range response.Data {
		if username == "" {
			return nil, fmt.Errorf(
				"directory %q returned an empty authorized username",
				directory,
			)
		}
		users = append(users, User{
			Directory:         directory,
			AbsoluteDirectory: absoluteDirectory,
			Username:          username,
		})
	}

	return users, nil
}

func (c *Client) GetUser(
	ctx context.Context,
	directory string,
	username string,
) (*User, error) {
	users, err := c.ListUsers(ctx, directory)
	if err != nil {
		return nil, err
	}

	var match *User
	for _, user := range users {
		if user.Username != username {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf(
				"directory %q returned duplicate authorized user %q",
				directory,
				username,
			)
		}
		userCopy := user
		match = &userCopy
	}

	return match, nil
}

func (c *Client) AddUser(
	ctx context.Context,
	definition UserDefinition,
) error {
	absoluteDirectory, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		definition.Directory,
	)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf(
			"directory %q does not exist",
			definition.Directory,
		)
	}

	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDirectoryPrivacy,
		operationAddUser,
		map[string]string{
			"dir":      absoluteDirectory,
			"user":     definition.Username,
			"password": definition.Password,
		},
		&response,
	)
}

func (c *Client) DeleteUser(
	ctx context.Context,
	directory string,
	username string,
) error {
	absoluteDirectory, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		directory,
	)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDirectoryPrivacy,
		operationDeleteUser,
		map[string]string{
			"dir":  absoluteDirectory,
			"user": username,
		},
		&response,
	)
}

func privacyFromResponse(
	directory string,
	absoluteDirectory string,
	data ResponseData,
) (*Privacy, error) {
	if data.Protected != 0 && data.Protected != 1 {
		return nil, fmt.Errorf(
			"directory %q returned invalid protected value %d",
			directory,
			data.Protected,
		)
	}

	protected := data.Protected == 1
	if protected && data.AuthType != "Basic" {
		return nil, fmt.Errorf(
			"directory %q returned authentication type %q; expected Basic",
			directory,
			data.AuthType,
		)
	}

	if !protected {
		data.AuthName = ""
		data.AuthType = "None"
		data.PasswordFile = ""
	}

	return &Privacy{
		Directory:         directory,
		AbsoluteDirectory: absoluteDirectory,
		AuthName:          data.AuthName,
		AuthType:          data.AuthType,
		PasswordFile:      data.PasswordFile,
		Protected:         protected,
	}, nil
}
