package directoryindex

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) Get(ctx context.Context, directory string) (*Index, error) {
	absoluteDirectory, exists, err := c.ResolveDirectory(ctx, directory)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	response := IndexResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDirectoryIndexes,
		operationGetIndexing,
		map[string]string{"dir": absoluteDirectory},
		&response,
	); err != nil {
		return nil, err
	}
	if !isSupportedIndexType(response.Data) {
		return nil, fmt.Errorf(
			"directory %q returned unsupported indexing type %q",
			directory,
			response.Data,
		)
	}

	return &Index{
		Directory:         directory,
		AbsoluteDirectory: absoluteDirectory,
		Type:              response.Data,
	}, nil
}

func (c *Client) Set(
	ctx context.Context,
	directory string,
	indexType string,
) (*Index, error) {
	if !isSupportedIndexType(indexType) {
		return nil, fmt.Errorf("unsupported directory indexing type %q", indexType)
	}

	absoluteDirectory, exists, err := c.ResolveDirectory(ctx, directory)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("directory %q does not exist", directory)
	}

	response := IndexResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDirectoryIndexes,
		operationSetIndexing,
		map[string]string{
			"dir":  absoluteDirectory,
			"type": indexType,
		},
		&response,
	); err != nil {
		return nil, err
	}
	if response.Data != indexType {
		return nil, fmt.Errorf(
			"directory %q indexing mutation returned %q; expected %q",
			directory,
			response.Data,
			indexType,
		)
	}

	return &Index{
		Directory:         directory,
		AbsoluteDirectory: absoluteDirectory,
		Type:              response.Data,
	}, nil
}

func (c *Client) HomeDirectory(ctx context.Context) (string, error) {
	response := UserInformationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleVariables,
		operationGetUserInformation,
		map[string]string{},
		&response,
	); err != nil {
		return "", err
	}
	if response.Data.Home == "" || !path.IsAbs(response.Data.Home) {
		return "", fmt.Errorf(
			"cPanel returned invalid account home directory %q",
			response.Data.Home,
		)
	}

	return path.Clean(response.Data.Home), nil
}

func (c *Client) ResolveDirectory(
	ctx context.Context,
	directory string,
) (string, bool, error) {
	if err := validateRelativeDirectory(directory); err != nil {
		return "", false, err
	}

	homeDirectory, err := c.HomeDirectory(ctx)
	if err != nil {
		return "", false, fmt.Errorf("read cPanel account home directory: %w", err)
	}

	currentRelativeDirectory := ""
	currentAbsoluteDirectory := homeDirectory
	for _, segment := range strings.Split(directory, "/") {
		entries, err := c.listFiles(ctx, currentRelativeDirectory)
		if err != nil {
			return "", false, fmt.Errorf(
				"list directory %q: %w",
				currentRelativeDirectory,
				err,
			)
		}

		var match *FileEntry
		for _, entry := range entries {
			if entry.File != segment {
				continue
			}
			if match != nil {
				return "", false, fmt.Errorf(
					"multiple file entries match %q in directory %q",
					segment,
					currentRelativeDirectory,
				)
			}
			entryCopy := entry
			match = &entryCopy
		}
		if match == nil || match.Type != "dir" {
			return "", false, nil
		}

		currentRelativeDirectory = path.Join(
			currentRelativeDirectory,
			segment,
		)
		currentAbsoluteDirectory = path.Join(
			currentAbsoluteDirectory,
			segment,
		)
		if path.Clean(match.FullPath) != currentAbsoluteDirectory {
			return "", false, fmt.Errorf(
				"directory %q resolved to unexpected path %q",
				currentRelativeDirectory,
				match.FullPath,
			)
		}
	}

	return currentAbsoluteDirectory, true, nil
}

func (c *Client) listFiles(
	ctx context.Context,
	directory string,
) ([]FileEntry, error) {
	response := FileListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleFileman,
		operationListFiles,
		map[string]string{
			"dir":         directory,
			"limit":       "100000",
			"show_hidden": "1",
		},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func validateRelativeDirectory(directory string) error {
	if directory == "" {
		return fmt.Errorf("directory must not be empty")
	}
	if path.IsAbs(directory) {
		return fmt.Errorf("directory must be relative to the cPanel account home")
	}
	if strings.ContainsRune(directory, '\x00') {
		return fmt.Errorf("directory must not contain a null byte")
	}
	if path.Clean(directory) != directory ||
		directory == "." ||
		directory == ".." ||
		strings.HasPrefix(directory, "../") {
		return fmt.Errorf(
			"directory must be a normalized relative path without . or .. segments",
		)
	}

	return nil
}

func isSupportedIndexType(indexType string) bool {
	switch indexType {
	case IndexTypeDisabled, IndexTypeFancy, IndexTypeInherit, IndexTypeStandard:
		return true
	default:
		return false
	}
}
