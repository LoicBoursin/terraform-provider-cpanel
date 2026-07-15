package fileman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"terraform-provider-cpanel/internal/cpanel"
)

var mutationMu sync.Mutex

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func LockMutations() func() {
	mutationMu.Lock()

	var once sync.Once

	return func() {
		once.Do(mutationMu.Unlock)
	}
}

func (c *Client) LockMutations() func() {
	return LockMutations()
}

func (c *Client) GetEntry(
	ctx context.Context,
	managedPath string,
) (*Entry, error) {
	return c.resolveEntry(ctx, managedPath, false)
}

func (c *Client) GetDirectory(
	ctx context.Context,
	managedPath string,
) (*Directory, error) {
	return c.getDirectory(ctx, managedPath, false)
}

func (c *Client) ListDirectory(
	ctx context.Context,
	managedPath string,
) ([]Entry, error) {
	directory, err := c.getDirectory(ctx, managedPath, false)
	if err != nil {
		return nil, err
	}
	if directory == nil {
		return nil, nil
	}

	rawEntries, err := c.listFiles(ctx, managedPath)
	if err != nil {
		return nil, fmt.Errorf("list directory %q: %w", managedPath, err)
	}

	entries := make([]Entry, 0, len(rawEntries))
	seenNames := make(map[string]struct{}, len(rawEntries))
	for index, raw := range rawEntries {
		name, err := entryName(raw)
		if err != nil {
			return nil, fmt.Errorf(
				"decode directory %q entry at index %d: %w",
				managedPath,
				index,
				err,
			)
		}
		if _, duplicate := seenNames[name]; duplicate {
			return nil, fmt.Errorf(
				"directory %q returned duplicate entry %q",
				managedPath,
				name,
			)
		}
		seenNames[name] = struct{}{}

		childPath := path.Join(managedPath, name)
		entry, err := entryFromAPI(
			raw,
			childPath,
			path.Join(directory.Entry.AbsolutePath, name),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decode directory %q entry %q: %w",
				managedPath,
				name,
				err,
			)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *Client) CreateDirectory(
	ctx context.Context,
	managedPath string,
) (*Directory, error) {
	if err := ValidateManagedPath(managedPath); err != nil {
		return nil, err
	}

	existing, err := c.GetEntry(ctx, managedPath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("path %q already exists", managedPath)
	}

	parentPath := path.Dir(managedPath)
	parent, err := c.getDirectory(
		ctx,
		parentPath,
		parentPath == managedRoot,
	)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, fmt.Errorf("parent directory %q does not exist", parentPath)
	}

	response := api2Response{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		operationMakeDirectory,
		map[string]string{
			"name":        path.Base(managedPath),
			"path":        parent.Entry.AbsolutePath,
			"permissions": "0755",
		},
		&response,
	); err != nil {
		return nil, err
	}

	items, err := api2Items(response, operationMakeDirectory)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf(
			"fileman mkdir returned %d item results; expected 1",
			len(items),
		)
	}
	if message, failed, err := api2ItemFailure(items[0]); err != nil {
		return nil, fmt.Errorf("decode Fileman::mkdir item result: %w", err)
	} else if failed {
		return nil, fmt.Errorf("fileman mkdir failed: %s", message)
	}
	if err := validateMkdirItem(
		items[0],
		path.Base(managedPath),
		parent.Entry.AbsolutePath,
	); err != nil {
		return nil, err
	}

	created, err := c.GetDirectory(ctx, managedPath)
	if err != nil {
		return nil, fmt.Errorf("verify created directory %q: %w", managedPath, err)
	}
	if created == nil {
		return nil, fmt.Errorf(
			"directory %q was not found after Fileman::mkdir",
			managedPath,
		)
	}
	if created.Entry.Permissions != "0755" {
		return nil, fmt.Errorf(
			"directory %q returned permissions %q; expected %q",
			managedPath,
			created.Entry.Permissions,
			"0755",
		)
	}

	return created, nil
}

func (c *Client) CreateEmptyTextFile(
	ctx context.Context,
	managedPath string,
) (*TextFile, error) {
	if err := ValidateManagedPath(managedPath); err != nil {
		return nil, err
	}

	existing, err := c.GetEntry(ctx, managedPath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("path %q already exists", managedPath)
	}

	parentPath := path.Dir(managedPath)
	parent, err := c.getDirectory(
		ctx,
		parentPath,
		parentPath == managedRoot,
	)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, fmt.Errorf("parent directory %q does not exist", parentPath)
	}

	name := path.Base(managedPath)
	response := api2Response{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		operationMakeFile,
		map[string]string{
			"name":        name,
			"path":        parent.Entry.AbsolutePath,
			"permissions": "0644",
		},
		&response,
	); err != nil {
		return nil, err
	}

	items, err := api2Items(response, operationMakeFile)
	if err != nil {
		return nil, err
	}
	if err := api2TopLevelError(response, operationMakeFile); err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf(
			"fileman mkfile returned %d item results; expected 1",
			len(items),
		)
	}
	if message, failed, err := api2ItemFailure(items[0]); err != nil {
		return nil, fmt.Errorf("decode Fileman::mkfile item result: %w", err)
	} else if failed {
		return nil, fmt.Errorf("fileman mkfile failed: %s", message)
	}
	if err := validateMkfileItem(
		items[0],
		name,
		parent.Entry.AbsolutePath,
	); err != nil {
		return nil, err
	}

	created, err := c.GetTextFile(ctx, managedPath)
	if err != nil {
		return nil, fmt.Errorf("verify created text file %q: %w", managedPath, err)
	}
	if created == nil {
		return nil, fmt.Errorf(
			"text file %q was not found after Fileman::mkfile",
			managedPath,
		)
	}

	expectedAbsolutePath := path.Join(parent.Entry.AbsolutePath, name)
	if created.Entry.Path != managedPath ||
		created.Entry.AbsolutePath != expectedAbsolutePath ||
		created.Entry.Type != EntryTypeFile {
		return nil, fmt.Errorf(
			"text file %q returned unexpected identity after Fileman::mkfile",
			managedPath,
		)
	}
	if created.Entry.Permissions != "0644" {
		return nil, fmt.Errorf(
			"text file %q returned permissions %q; expected %q",
			managedPath,
			created.Entry.Permissions,
			"0644",
		)
	}
	if created.Content != "" {
		return nil, fmt.Errorf(
			"text file %q was not empty after Fileman::mkfile",
			managedPath,
		)
	}
	if created.Entry.SizeBytes != 0 {
		return nil, fmt.Errorf(
			"text file %q returned size %d; expected 0",
			managedPath,
			created.Entry.SizeBytes,
		)
	}

	return created, nil
}

func (c *Client) GetTextFile(
	ctx context.Context,
	managedPath string,
) (*TextFile, error) {
	entry, err := c.GetEntry(ctx, managedPath)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	if entry.Type != EntryTypeFile {
		return nil, fmt.Errorf(
			"path %q has type %q; expected %q",
			managedPath,
			entry.Type,
			EntryTypeFile,
		)
	}

	response := rawDataResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleFileman,
		operationGetFileContent,
		map[string]string{
			"dir":                           path.Dir(managedPath),
			"file":                          path.Base(managedPath),
			"from_charset":                  "UTF-8",
			"to_charset":                    "UTF-8",
			"update_html_document_encoding": "0",
		},
		&response,
	); err != nil {
		return nil, err
	}

	data := fileContentData{}
	if err := decodeRequiredObject(
		response.Data,
		"Fileman::get_file_content data",
		&data,
	); err != nil {
		return nil, err
	}
	content, err := validateFileContentData(data, *entry)
	if err != nil {
		return nil, err
	}

	return &TextFile{
		Entry:   *entry,
		Content: content,
	}, nil
}

func (c *Client) SaveTextFile(
	ctx context.Context,
	managedPath string,
	content string,
) (*TextFile, error) {
	if err := ValidateManagedPath(managedPath); err != nil {
		return nil, err
	}
	if err := validateTextContent(content); err != nil {
		return nil, err
	}

	existing, err := c.GetEntry(ctx, managedPath)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Type != EntryTypeFile {
		return nil, fmt.Errorf(
			"path %q has type %q; expected %q",
			managedPath,
			existing.Type,
			EntryTypeFile,
		)
	}

	parentPath := path.Dir(managedPath)
	parent, err := c.getDirectory(
		ctx,
		parentPath,
		parentPath == managedRoot,
	)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, fmt.Errorf("parent directory %q does not exist", parentPath)
	}

	response := rawDataResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		operationSaveFileContent,
		map[string]string{
			"content":      content,
			"dir":          parentPath,
			"fallback":     "0",
			"file":         path.Base(managedPath),
			"from_charset": "UTF-8",
			"to_charset":   "UTF-8",
		},
		&response,
	); err != nil {
		return nil, err
	}

	data := saveContentData{}
	if err := decodeRequiredObject(
		response.Data,
		"Fileman::save_file_content data",
		&data,
	); err != nil {
		return nil, err
	}
	if err := validateSaveContentData(
		data,
		path.Join(parent.Entry.AbsolutePath, path.Base(managedPath)),
	); err != nil {
		return nil, err
	}

	saved, err := c.GetTextFile(ctx, managedPath)
	if err != nil {
		return nil, fmt.Errorf("verify saved text file %q: %w", managedPath, err)
	}
	if saved == nil {
		return nil, fmt.Errorf(
			"text file %q was not found after save_file_content",
			managedPath,
		)
	}
	if saved.Content != content {
		return nil, fmt.Errorf(
			"text file %q content differs after save_file_content",
			managedPath,
		)
	}

	return saved, nil
}

func (c *Client) DeletePath(
	ctx context.Context,
	managedPath string,
) error {
	entry, err := c.GetEntry(ctx, managedPath)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}

	response := api2Response{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		operationFileOperation,
		map[string]string{
			"doubledecode": "0",
			"op":           "unlink",
			"sourcefiles":  managedPath,
		},
		&response,
	); err != nil {
		return err
	}

	items, err := api2Items(response, operationFileOperation)
	if err != nil {
		return err
	}
	if len(items) != 1 {
		return fmt.Errorf(
			"fileman fileop returned %d item results; expected 1",
			len(items),
		)
	}
	item := items[0]
	if message, failed, err := api2ItemFailure(item); err != nil {
		return fmt.Errorf("decode Fileman::fileop item result: %w", err)
	} else if failed {
		return fmt.Errorf("fileman fileop unlink failed: %s", message)
	}

	result, err := requiredJSONInteger(item["result"], "result")
	if err != nil {
		return fmt.Errorf("decode Fileman::fileop item result: %w", err)
	}
	if result != 1 {
		return fmt.Errorf(
			"fileman fileop unlink returned result %d; expected 1",
			result,
		)
	}
	source, err := requiredString(item["src"], "src", false)
	if err != nil {
		return fmt.Errorf("decode Fileman::fileop item source: %w", err)
	}
	if source != entry.AbsolutePath {
		return fmt.Errorf(
			"fileman fileop unlink returned source %q; expected %q",
			source,
			entry.AbsolutePath,
		)
	}
	destination, exists := item["dest"]
	if !exists || !isJSONNull(destination) {
		return fmt.Errorf(
			"fileman fileop unlink returned a non-null destination",
		)
	}

	return nil
}

func (c *Client) getDirectory(
	ctx context.Context,
	managedPath string,
	allowManagedRoot bool,
) (*Directory, error) {
	entry, err := c.resolveEntry(ctx, managedPath, allowManagedRoot)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	if entry.Type != EntryTypeDirectory {
		return nil, fmt.Errorf(
			"path %q has type %q; expected %q",
			managedPath,
			entry.Type,
			EntryTypeDirectory,
		)
	}

	return &Directory{Entry: *entry}, nil
}

func (c *Client) resolveEntry(
	ctx context.Context,
	managedPath string,
	allowManagedRoot bool,
) (*Entry, error) {
	if err := validateResolvablePath(managedPath, allowManagedRoot); err != nil {
		return nil, err
	}

	homeDirectory, err := c.homeDirectory(ctx)
	if err != nil {
		return nil, err
	}

	relativeDirectory := ""
	absoluteDirectory := homeDirectory
	segments := strings.Split(managedPath, "/")
	for index, segment := range segments {
		rawEntries, err := c.listFiles(ctx, relativeDirectory)
		if err != nil {
			return nil, fmt.Errorf(
				"list directory %q while resolving %q: %w",
				relativeDirectory,
				managedPath,
				err,
			)
		}

		var matchedRaw *rawEntry
		for rawIndex := range rawEntries {
			name, err := entryName(rawEntries[rawIndex])
			if err != nil {
				return nil, fmt.Errorf(
					"decode directory %q entry at index %d: %w",
					relativeDirectory,
					rawIndex,
					err,
				)
			}
			if name != segment {
				continue
			}
			if matchedRaw != nil {
				return nil, fmt.Errorf(
					"directory %q returned duplicate entry %q",
					relativeDirectory,
					segment,
				)
			}
			matchedRaw = &rawEntries[rawIndex]
		}
		if matchedRaw == nil {
			return nil, nil
		}

		currentPath := path.Join(relativeDirectory, segment)
		currentAbsolutePath := path.Join(absoluteDirectory, segment)
		entry, err := entryFromAPI(
			*matchedRaw,
			currentPath,
			currentAbsolutePath,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decode resolved entry %q: %w",
				currentPath,
				err,
			)
		}
		if index < len(segments)-1 && entry.Type != EntryTypeDirectory {
			return nil, fmt.Errorf(
				"path component %q has type %q; expected %q",
				currentPath,
				entry.Type,
				EntryTypeDirectory,
			)
		}
		if index == len(segments)-1 {
			return &entry, nil
		}

		relativeDirectory = currentPath
		absoluteDirectory = currentAbsolutePath
	}

	return nil, fmt.Errorf("managed path %q did not contain any segments", managedPath)
}

func (c *Client) homeDirectory(ctx context.Context) (string, error) {
	response := userInformationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleVariables,
		operationGetUserInformation,
		map[string]string{},
		&response,
	); err != nil {
		return "", fmt.Errorf("read cPanel account home directory: %w", err)
	}

	homeDirectory, err := requiredString(response.Data.Home, "home", false)
	if err != nil {
		return "", fmt.Errorf("decode cPanel account home directory: %w", err)
	}
	if !path.IsAbs(homeDirectory) ||
		path.Clean(homeDirectory) != homeDirectory ||
		homeDirectory == "/" ||
		strings.ContainsRune(homeDirectory, '\\') ||
		containsControlCharacter(homeDirectory) {
		return "", fmt.Errorf(
			"cPanel returned invalid account home directory %q",
			homeDirectory,
		)
	}

	return homeDirectory, nil
}

func (c *Client) listFiles(
	ctx context.Context,
	directory string,
) ([]rawEntry, error) {
	response := rawDataResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleFileman,
		operationListFiles,
		map[string]string{
			"dir":                 directory,
			"include_permissions": "1",
			"limit":               "100000",
			"show_hidden":         "1",
		},
		&response,
	); err != nil {
		return nil, err
	}

	var entries []rawEntry
	if err := decodeRequiredArray(
		response.Data,
		"Fileman::list_files data",
		&entries,
	); err != nil {
		return nil, err
	}

	return entries, nil
}

func entryName(raw rawEntry) (string, error) {
	name, err := requiredString(raw.File, "file", false)
	if err != nil {
		return "", err
	}
	if err := validateEntryName(name); err != nil {
		return "", err
	}

	return name, nil
}

func entryFromAPI(
	raw rawEntry,
	managedPath string,
	expectedAbsolutePath string,
) (Entry, error) {
	name, err := entryName(raw)
	if err != nil {
		return Entry{}, err
	}
	if name != path.Base(managedPath) {
		return Entry{}, fmt.Errorf(
			"entry name %q does not match path %q",
			name,
			managedPath,
		)
	}

	absolutePath, err := requiredString(raw.FullPath, "fullpath", false)
	if err != nil {
		return Entry{}, err
	}
	if !path.IsAbs(absolutePath) ||
		path.Clean(absolutePath) != absolutePath ||
		absolutePath != expectedAbsolutePath {
		return Entry{}, fmt.Errorf(
			"fullpath %q does not match expected path %q",
			absolutePath,
			expectedAbsolutePath,
		)
	}

	entryType, err := requiredString(raw.Type, "type", false)
	if err != nil {
		return Entry{}, err
	}
	if entryType != EntryTypeDirectory && entryType != EntryTypeFile {
		return Entry{}, fmt.Errorf("unsupported entry type %q", entryType)
	}

	permissions, err := requiredString(raw.Permissions, "nicemode", false)
	if err != nil {
		return Entry{}, err
	}
	if !validPermissions(permissions) {
		return Entry{}, fmt.Errorf(
			"nicemode %q must contain exactly four octal digits",
			permissions,
		)
	}

	sizeBytes, err := requiredNonNegativeInteger(raw.Size, "size", true)
	if err != nil {
		return Entry{}, err
	}
	createdAt, err := requiredNonNegativeInteger(
		raw.CreatedAt,
		"ctime",
		false,
	)
	if err != nil {
		return Entry{}, err
	}
	modifiedAt, err := requiredNonNegativeInteger(
		raw.ModifiedAt,
		"mtime",
		false,
	)
	if err != nil {
		return Entry{}, err
	}

	return Entry{
		Path:         managedPath,
		AbsolutePath: absolutePath,
		Type:         entryType,
		Permissions:  permissions,
		SizeBytes:    sizeBytes,
		CreatedAt:    createdAt,
		ModifiedAt:   modifiedAt,
	}, nil
}

func api2Items(
	response api2Response,
	expectedFunction string,
) ([]map[string]json.RawMessage, error) {
	result := response.CpanelResult
	if result.APIVersion != 2 ||
		result.Module != cpanel.ModuleFileman ||
		result.Function != expectedFunction {
		return nil, fmt.Errorf(
			"unexpected API 2 response identity: version=%d module=%q function=%q",
			result.APIVersion,
			result.Module,
			result.Function,
		)
	}

	var items []map[string]json.RawMessage
	if err := decodeRequiredArray(
		result.Data,
		"API 2 Fileman item data",
		&items,
	); err != nil {
		return nil, err
	}

	return items, nil
}

func api2TopLevelError(
	response api2Response,
	function string,
) error {
	message, err := optionalString(response.CpanelResult.Error, "error")
	if err != nil {
		return fmt.Errorf(
			"decode Fileman::%s top-level error: %w",
			function,
			err,
		)
	}
	if message == "" {
		return nil
	}

	return &cpanel.APIError{
		API:      "API 2",
		Module:   cpanel.ModuleFileman,
		Function: function,
		Messages: []string{message},
	}
}

func api2ItemFailure(
	item map[string]json.RawMessage,
) (string, bool, error) {
	failed := false
	for _, field := range []string{"result", "status"} {
		raw, exists := item[field]
		if !exists || isJSONNull(raw) {
			continue
		}
		value, err := requiredJSONInteger(raw, field)
		if err != nil {
			return "", false, err
		}
		if value != 1 {
			failed = true
		}
	}

	for _, field := range []string{"err", "error", "reason"} {
		message, err := optionalString(item[field], field)
		if err != nil {
			return "", false, err
		}
		if message != "" {
			return message, true, nil
		}
	}

	statusMessage, err := optionalString(item["statusmsg"], "statusmsg")
	if err != nil {
		return "", false, err
	}
	if statusMessage != "" &&
		!strings.EqualFold(statusMessage, "success") &&
		!strings.EqualFold(statusMessage, "ok") {
		return statusMessage, true, nil
	}
	if failed {
		return "unknown item-level API error", true, nil
	}

	return "", false, nil
}

func validateMkdirItem(
	item map[string]json.RawMessage,
	expectedName string,
	expectedPath string,
) error {
	name, err := requiredString(item["name"], "name", false)
	if err != nil {
		return fmt.Errorf("decode Fileman::mkdir item name: %w", err)
	}
	if name != expectedName {
		return fmt.Errorf(
			"fileman mkdir returned name %q; expected %q",
			name,
			expectedName,
		)
	}

	parentPath, err := requiredString(item["path"], "path", false)
	if err != nil {
		return fmt.Errorf("decode Fileman::mkdir item path: %w", err)
	}
	if parentPath != expectedPath {
		return fmt.Errorf(
			"fileman mkdir returned path %q; expected %q",
			parentPath,
			expectedPath,
		)
	}

	permissions, err := requiredString(
		item["permissions"],
		"permissions",
		false,
	)
	if err != nil {
		return fmt.Errorf("decode Fileman::mkdir item permissions: %w", err)
	}
	if permissions != "0755" {
		return fmt.Errorf(
			"fileman mkdir returned permissions %q; expected %q",
			permissions,
			"0755",
		)
	}

	return nil
}

func validateMkfileItem(
	item map[string]json.RawMessage,
	expectedName string,
	expectedPath string,
) error {
	if len(item) != 3 {
		return fmt.Errorf(
			"fileman mkfile returned %d item fields; expected exactly 3",
			len(item),
		)
	}
	for field := range item {
		switch field {
		case "name", "path", "permissions":
		default:
			return fmt.Errorf(
				"fileman mkfile returned unexpected item field %q",
				field,
			)
		}
	}

	name, err := requiredString(item["name"], "name", false)
	if err != nil {
		return fmt.Errorf("decode Fileman::mkfile item name: %w", err)
	}
	if name != expectedName {
		return fmt.Errorf(
			"fileman mkfile returned name %q; expected %q",
			name,
			expectedName,
		)
	}

	parentPath, err := requiredString(item["path"], "path", false)
	if err != nil {
		return fmt.Errorf("decode Fileman::mkfile item path: %w", err)
	}
	if parentPath != expectedPath {
		return fmt.Errorf(
			"fileman mkfile returned path %q; expected %q",
			parentPath,
			expectedPath,
		)
	}

	permissions, err := requiredString(
		item["permissions"],
		"permissions",
		false,
	)
	if err != nil {
		return fmt.Errorf(
			"decode Fileman::mkfile item permissions: %w",
			err,
		)
	}
	if permissions != "0644" {
		return fmt.Errorf(
			"fileman mkfile returned permissions %q; expected %q",
			permissions,
			"0644",
		)
	}

	return nil
}

func validateFileContentData(data fileContentData, entry Entry) (string, error) {
	absolutePath, err := requiredString(data.Path, "path", false)
	if err != nil {
		return "", fmt.Errorf("decode get_file_content path: %w", err)
	}
	if absolutePath != entry.AbsolutePath {
		return "", fmt.Errorf(
			"get_file_content returned path %q; expected %q",
			absolutePath,
			entry.AbsolutePath,
		)
	}

	directory, err := requiredString(data.Directory, "dir", false)
	if err != nil {
		return "", fmt.Errorf("decode get_file_content directory: %w", err)
	}
	if directory != path.Dir(entry.AbsolutePath) {
		return "", fmt.Errorf(
			"get_file_content returned directory %q; expected %q",
			directory,
			path.Dir(entry.AbsolutePath),
		)
	}

	filename, err := requiredString(data.Filename, "filename", false)
	if err != nil {
		return "", fmt.Errorf("decode get_file_content filename: %w", err)
	}
	if filename != path.Base(entry.Path) {
		return "", fmt.Errorf(
			"get_file_content returned filename %q; expected %q",
			filename,
			path.Base(entry.Path),
		)
	}
	if err := validateCharsetPair(
		data.FromCharset,
		data.ToCharset,
		"get_file_content",
	); err != nil {
		return "", err
	}

	content, err := requiredString(data.Content, "content", true)
	if err != nil {
		return "", fmt.Errorf("decode get_file_content content: %w", err)
	}
	if err := validateTextContent(content); err != nil {
		return "", fmt.Errorf("get_file_content returned invalid text: %w", err)
	}

	return content, nil
}

func validateSaveContentData(
	data saveContentData,
	expectedAbsolutePath string,
) error {
	absolutePath, err := requiredString(data.Path, "path", false)
	if err != nil {
		return fmt.Errorf("decode save_file_content path: %w", err)
	}
	if absolutePath != expectedAbsolutePath {
		return fmt.Errorf(
			"save_file_content returned path %q; expected %q",
			absolutePath,
			expectedAbsolutePath,
		)
	}

	return validateCharsetPair(
		data.FromCharset,
		data.ToCharset,
		"save_file_content",
	)
}

func validateCharsetPair(
	rawFrom json.RawMessage,
	rawTo json.RawMessage,
	operation string,
) error {
	fromCharset, err := requiredString(rawFrom, "from_charset", false)
	if err != nil {
		return fmt.Errorf("decode %s from_charset: %w", operation, err)
	}
	toCharset, err := requiredString(rawTo, "to_charset", false)
	if err != nil {
		return fmt.Errorf("decode %s to_charset: %w", operation, err)
	}
	if !strings.EqualFold(fromCharset, "utf-8") ||
		!strings.EqualFold(toCharset, "utf-8") {
		return fmt.Errorf(
			"%s returned charset conversion %q to %q; expected UTF-8 to UTF-8",
			operation,
			fromCharset,
			toCharset,
		)
	}

	return nil
}

func validateTextContent(content string) error {
	if !utf8.ValidString(content) {
		return fmt.Errorf("text content must be valid UTF-8")
	}
	if strings.ContainsRune(content, '\x00') {
		return fmt.Errorf("text content must not contain a null byte")
	}

	return nil
}

func validPermissions(value string) bool {
	if len(value) != 4 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '7' {
			return false
		}
	}

	return true
}

func requiredString(
	raw json.RawMessage,
	field string,
	allowEmpty bool,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", fmt.Errorf("required string field %q is missing or null", field)
	}
	if value[0] != '"' {
		return "", fmt.Errorf("field %q must be a string", field)
	}

	var parsed string
	if err := json.Unmarshal(value, &parsed); err != nil {
		return "", fmt.Errorf("decode string field %q: %w", field, err)
	}
	if parsed == "" && !allowEmpty {
		return "", fmt.Errorf("required string field %q is empty", field)
	}

	return parsed, nil
}

func optionalString(raw json.RawMessage, field string) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || isJSONNull(raw) {
		return "", nil
	}

	return requiredString(raw, field, true)
}

func requiredNonNegativeInteger(
	raw json.RawMessage,
	field string,
	allowEmptyZero bool,
) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("required integer field %q is missing or null", field)
	}

	var text string
	if value[0] == '"' {
		if err := json.Unmarshal(value, &text); err != nil {
			return 0, fmt.Errorf("decode integer field %q: %w", field, err)
		}
		if text == "" && allowEmptyZero {
			return 0, nil
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var number json.Number
		if err := decoder.Decode(&number); err != nil {
			return 0, fmt.Errorf("field %q must be an integer", field)
		}
		text = number.String()
	}

	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf(
				"integer field %q must contain decimal digits only",
				field,
			)
		}
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse integer field %q value %q: %w", field, text, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("integer field %q must not be negative", field)
	}

	return parsed, nil
}

func requiredJSONInteger(raw json.RawMessage, field string) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("required integer field %q is missing or null", field)
	}

	var parsed int64
	if err := json.Unmarshal(value, &parsed); err != nil {
		return 0, fmt.Errorf("field %q must be a JSON integer", field)
	}

	return parsed, nil
}

func decodeRequiredArray(
	raw json.RawMessage,
	label string,
	output any,
) error {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return fmt.Errorf("%s is missing or null", label)
	}
	if value[0] != '[' {
		return fmt.Errorf("%s must be an array", label)
	}
	if err := json.Unmarshal(value, output); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}

	return nil
}

func decodeRequiredObject(
	raw json.RawMessage,
	label string,
	output any,
) error {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return fmt.Errorf("%s is missing or null", label)
	}
	if value[0] != '{' {
		return fmt.Errorf("%s must be an object", label)
	}
	if err := json.Unmarshal(value, output); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}

	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
