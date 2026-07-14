package versioncontrol

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
	"unicode"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

const repositoryTypeGit = "git"

var restrictedDirectoryNames = map[string]struct{}{
	".cpanel":      {},
	".cphorde":     {},
	".htpasswds":   {},
	".ssh":         {},
	".trash":       {},
	"access-logs":  {},
	"cgi-bin":      {},
	"etc":          {},
	"logs":         {},
	"mail":         {},
	"perl5":        {},
	"spamassassin": {},
	"ssl":          {},
	"tmp":          {},
	"var":          {},
}

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

func (c *Client) List(ctx context.Context) ([]Repository, error) {
	homeDirectory, err := c.directoryClient.HomeDirectory(ctx)
	if err != nil {
		return nil, err
	}

	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleVersionControl,
		operationRetrieve,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	repositories := make([]Repository, 0, len(response.Data))
	seenRoots := make(map[string]struct{}, len(response.Data))
	for _, apiRepository := range response.Data {
		repository, err := repositoryFromAPI(homeDirectory, apiRepository)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seenRoots[repository.RepositoryRoot]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate Git repository root %q",
				repository.RepositoryRoot,
			)
		}
		seenRoots[repository.RepositoryRoot] = struct{}{}
		repositories = append(repositories, repository)
	}

	slices.SortFunc(repositories, func(left, right Repository) int {
		return strings.Compare(left.RepositoryRoot, right.RepositoryRoot)
	})

	return repositories, nil
}

func (c *Client) Get(
	ctx context.Context,
	repositoryRoot string,
) (*Repository, error) {
	repositories, err := c.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, repository := range repositories {
		if repository.RepositoryRoot == repositoryRoot {
			repositoryCopy := repository

			return &repositoryCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) RootExists(
	ctx context.Context,
	repositoryRoot string,
) (bool, error) {
	_, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		repositoryRoot,
	)

	return exists, err
}

func (c *Client) Create(
	ctx context.Context,
	definition Definition,
) (*Repository, error) {
	homeDirectory, absoluteRoot, err := c.repositoryPaths(
		ctx,
		definition.RepositoryRoot,
	)
	if err != nil {
		return nil, err
	}

	parameters := map[string]string{
		"name":            definition.Name,
		"repository_root": absoluteRoot,
		"type":            repositoryTypeGit,
	}
	if definition.SourceRepositoryURL != "" {
		sourceRepository, err := json.Marshal(SourceRepository{
			RemoteName: "origin",
			URL:        definition.SourceRepositoryURL,
		})
		if err != nil {
			return nil, fmt.Errorf("encode Git source repository: %w", err)
		}
		parameters["source_repository"] = string(sourceRepository)
	}

	response := RepositoryResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleVersionControl,
		operationCreate,
		parameters,
		&response,
	); err != nil {
		return nil, err
	}

	repository, err := repositoryFromAPI(
		homeDirectory,
		response.Data,
	)
	if err != nil {
		return nil, err
	}
	if repository.RepositoryRoot != definition.RepositoryRoot ||
		repository.Name != definition.Name {
		return nil, fmt.Errorf(
			"git repository create returned root %q and name %q; expected %q and %q",
			repository.RepositoryRoot,
			repository.Name,
			definition.RepositoryRoot,
			definition.Name,
		)
	}

	return &repository, nil
}

func (c *Client) Update(
	ctx context.Context,
	repositoryRoot string,
	name string,
) (*Repository, error) {
	homeDirectory, absoluteRoot, err := c.repositoryPaths(
		ctx,
		repositoryRoot,
	)
	if err != nil {
		return nil, err
	}

	response := RepositoryResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleVersionControl,
		operationUpdate,
		map[string]string{
			"name":            name,
			"repository_root": absoluteRoot,
		},
		&response,
	); err != nil {
		return nil, err
	}

	repository, err := repositoryFromAPI(
		homeDirectory,
		response.Data,
	)
	if err != nil {
		return nil, err
	}
	if repository.RepositoryRoot != repositoryRoot ||
		repository.Name != name {
		return nil, fmt.Errorf(
			"git repository update returned root %q and name %q; expected %q and %q",
			repository.RepositoryRoot,
			repository.Name,
			repositoryRoot,
			name,
		)
	}

	return &repository, nil
}

func (c *Client) Delete(
	ctx context.Context,
	repositoryRoot string,
) error {
	_, absoluteRoot, err := c.repositoryPaths(ctx, repositoryRoot)
	if err != nil {
		return err
	}

	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleVersionControl,
		operationDelete,
		map[string]string{"repository_root": absoluteRoot},
		&response,
	)
}

func (c *Client) DeleteDirectory(
	ctx context.Context,
	repositoryRoot string,
) error {
	if err := ValidateRepositoryRoot(repositoryRoot); err != nil {
		return err
	}

	stagingRoot := repositoryDeletionStagingRoot(repositoryRoot)
	trashName := path.Base(stagingRoot)

	originalExists, err := c.directoryExists(ctx, repositoryRoot)
	if err != nil {
		return err
	}
	stagingExists, err := c.directoryExists(ctx, stagingRoot)
	if err != nil {
		return err
	}
	trashExists, err := c.trashEntryExists(ctx, trashName)
	if err != nil {
		return err
	}

	if originalExists {
		if stagingExists || trashExists {
			return fmt.Errorf(
				"cannot stage directory %q for deletion because deletion marker %q already exists",
				repositoryRoot,
				trashName,
			)
		}
		if err := c.executeFileOperation(
			ctx,
			"rename",
			repositoryRoot,
			path.Base(stagingRoot),
		); err != nil {
			return fmt.Errorf(
				"stage directory %q for deletion: %w",
				repositoryRoot,
				err,
			)
		}

		originalExists, err = c.directoryExists(ctx, repositoryRoot)
		if err != nil {
			return err
		}
		stagingExists, err = c.directoryExists(ctx, stagingRoot)
		if err != nil {
			return err
		}
		if originalExists || !stagingExists {
			return fmt.Errorf(
				"directory %q rename to deletion marker %q was not applied",
				repositoryRoot,
				stagingRoot,
			)
		}
	}

	if trashExists {
		if err := c.emptyTrashEntry(ctx, trashName); err != nil {
			return err
		}
		trashExists = false
	}

	if stagingExists {
		if err := c.executeFileOperation(
			ctx,
			"trash",
			stagingRoot,
			"",
		); err != nil {
			return fmt.Errorf(
				"move deletion marker %q to trash: %w",
				stagingRoot,
				err,
			)
		}

		stagingExists, err = c.directoryExists(ctx, stagingRoot)
		if err != nil {
			return err
		}
		trashExists, err = c.trashEntryExists(ctx, trashName)
		if err != nil {
			return err
		}
		if stagingExists || !trashExists {
			return fmt.Errorf(
				"deletion marker %q was not moved to cPanel trash",
				stagingRoot,
			)
		}
	}

	if trashExists {
		if err := c.emptyTrashEntry(ctx, trashName); err != nil {
			return err
		}
	}

	originalExists, err = c.directoryExists(ctx, repositoryRoot)
	if err != nil {
		return err
	}
	stagingExists, err = c.directoryExists(ctx, stagingRoot)
	if err != nil {
		return err
	}
	trashExists, err = c.trashEntryExists(ctx, trashName)
	if err != nil {
		return err
	}
	if originalExists || stagingExists || trashExists {
		return fmt.Errorf(
			"directory %q deletion did not remove every staged path",
			repositoryRoot,
		)
	}

	return nil
}

func (c *Client) directoryExists(
	ctx context.Context,
	directory string,
) (bool, error) {
	_, exists, err := c.directoryClient.ResolveDirectory(ctx, directory)

	return exists, err
}

func (c *Client) trashEntryExists(
	ctx context.Context,
	name string,
) (bool, error) {
	trashExists, err := c.directoryExists(ctx, ".trash")
	if err != nil {
		return false, err
	}
	if !trashExists {
		return false, nil
	}

	response := directoryindex.FileListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleFileman,
		operationListFiles,
		map[string]string{
			"dir":         ".trash",
			"limit":       "100000",
			"show_hidden": "1",
		},
		&response,
	); err != nil {
		return false, err
	}

	found := false
	for _, entry := range response.Data {
		if entry.File != name {
			continue
		}
		if found {
			return false, fmt.Errorf(
				"cPanel trash contains duplicate entry %q",
				name,
			)
		}
		found = true
	}

	return found, nil
}

func (c *Client) executeFileOperation(
	ctx context.Context,
	operation string,
	source string,
	destination string,
) error {
	parameters := map[string]string{
		"op":           operation,
		"sourcefiles":  source,
		"doubledecode": "0",
	}
	if destination != "" {
		parameters["destfiles"] = destination
	}

	response := API2FileOperationResponse{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		operationFilePath,
		parameters,
		&response,
	); err != nil {
		return err
	}
	if len(response.CpanelResult.Data) != 1 {
		return fmt.Errorf(
			"fileman %s returned %d item results; expected 1",
			operation,
			len(response.CpanelResult.Data),
		)
	}

	item := response.CpanelResult.Data[0]
	if item.Result != 1 {
		message := item.Error
		if message == "" {
			message = item.Reason
		}
		if message == "" {
			message = "unknown file operation error"
		}

		return fmt.Errorf("fileman %s failed: %s", operation, message)
	}

	return nil
}

func (c *Client) emptyTrashEntry(
	ctx context.Context,
	name string,
) error {
	response := MutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		operationEmptyTrash,
		map[string]string{"only_these_files": name},
		&response,
	); err != nil {
		return fmt.Errorf("permanently delete trash entry %q: %w", name, err)
	}

	exists, err := c.trashEntryExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf(
			"trash entry %q still exists after permanent deletion",
			name,
		)
	}

	return nil
}

func repositoryDeletionStagingRoot(repositoryRoot string) string {
	sum := sha256.Sum256([]byte(repositoryRoot))
	name := fmt.Sprintf(
		".terraform-cpanel-git-delete-%x",
		sum[:8],
	)
	parent := path.Dir(repositoryRoot)
	if parent == "." {
		return name
	}

	return path.Join(parent, name)
}

func (c *Client) repositoryPaths(
	ctx context.Context,
	repositoryRoot string,
) (string, string, error) {
	if err := ValidateRepositoryRoot(repositoryRoot); err != nil {
		return "", "", err
	}

	homeDirectory, err := c.directoryClient.HomeDirectory(ctx)
	if err != nil {
		return "", "", fmt.Errorf(
			"read cPanel account home directory: %w",
			err,
		)
	}

	return homeDirectory, path.Join(homeDirectory, repositoryRoot), nil
}

func repositoryFromAPI(
	homeDirectory string,
	apiRepository APIRepository,
) (Repository, error) {
	if apiRepository.Type != repositoryTypeGit {
		return Repository{}, fmt.Errorf(
			"repository %q returned unsupported type %q",
			apiRepository.RepositoryRoot,
			apiRepository.Type,
		)
	}
	if apiRepository.Name == "" {
		return Repository{}, fmt.Errorf(
			"repository %q returned an empty name",
			apiRepository.RepositoryRoot,
		)
	}
	if apiRepository.Deployable != 0 && apiRepository.Deployable != 1 {
		return Repository{}, fmt.Errorf(
			"repository %q returned invalid deployable value %d",
			apiRepository.RepositoryRoot,
			apiRepository.Deployable,
		)
	}

	homePrefix := strings.TrimRight(homeDirectory, "/") + "/"
	if !strings.HasPrefix(apiRepository.RepositoryRoot, homePrefix) {
		return Repository{}, fmt.Errorf(
			"repository root %q is outside account home %q",
			apiRepository.RepositoryRoot,
			homeDirectory,
		)
	}
	repositoryRoot := strings.TrimPrefix(
		apiRepository.RepositoryRoot,
		homePrefix,
	)
	if err := ValidateRepositoryRoot(repositoryRoot); err != nil {
		return Repository{}, fmt.Errorf(
			"repository root %q is invalid: %w",
			apiRepository.RepositoryRoot,
			err,
		)
	}

	availableBranches := append(
		[]string(nil),
		apiRepository.AvailableBranches...,
	)
	readOnlyCloneURLs := append([]string(nil), apiRepository.CloneURLs.ReadOnly...)
	readWriteCloneURLs := append([]string(nil), apiRepository.CloneURLs.ReadWrite...)
	slices.Sort(availableBranches)
	slices.Sort(readOnlyCloneURLs)
	slices.Sort(readWriteCloneURLs)

	repository := Repository{
		Name:               apiRepository.Name,
		RepositoryRoot:     repositoryRoot,
		AbsoluteRoot:       apiRepository.RepositoryRoot,
		Type:               apiRepository.Type,
		Branch:             apiRepository.Branch,
		AvailableBranches:  availableBranches,
		ReadOnlyCloneURLs:  readOnlyCloneURLs,
		ReadWriteCloneURLs: readWriteCloneURLs,
		Deployable:         apiRepository.Deployable == 1,
	}
	if apiRepository.SourceRepository != nil {
		repository.SourceRepositoryName = apiRepository.SourceRepository.RemoteName
		repository.SourceRepositoryURL = apiRepository.SourceRepository.URL
	}

	return repository, nil
}

func ValidateRepositoryRoot(repositoryRoot string) error {
	if repositoryRoot == "" {
		return fmt.Errorf("git repository root must not be empty")
	}
	if path.IsAbs(repositoryRoot) {
		return fmt.Errorf(
			"git repository root must be relative to the cPanel account home",
		)
	}
	if path.Clean(repositoryRoot) != repositoryRoot ||
		repositoryRoot == "." ||
		repositoryRoot == ".." ||
		strings.HasPrefix(repositoryRoot, "../") {
		return fmt.Errorf(
			"git repository root must be a normalized relative path without . or .. segments",
		)
	}
	if strings.ContainsAny(
		repositoryRoot,
		"\\*|\"'<> &@$[]{}();?:=%#`",
	) {
		return fmt.Errorf(
			"git repository root contains a character rejected by cPanel",
		)
	}
	for _, character := range repositoryRoot {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf(
				"git repository root must not contain whitespace or control characters",
			)
		}
	}
	for _, segment := range strings.Split(repositoryRoot, "/") {
		if _, restricted := restrictedDirectoryNames[segment]; restricted {
			return fmt.Errorf(
				"git repository root must not use cPanel-controlled directory %q",
				segment,
			)
		}
	}

	return nil
}
