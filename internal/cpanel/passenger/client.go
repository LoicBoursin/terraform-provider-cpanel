package passenger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

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

func (c *Client) List(ctx context.Context) ([]Application, error) {
	homeDirectory, err := c.directoryClient.HomeDirectory(ctx)
	if err != nil {
		return nil, err
	}

	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModulePassengerApps,
		operationList,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	applications := make([]Application, 0, len(response.Data))
	for key, apiApplication := range response.Data {
		application, err := applicationFromAPI(
			homeDirectory,
			key,
			apiApplication,
		)
		if err != nil {
			return nil, err
		}
		applications = append(applications, application)
	}
	slices.SortFunc(applications, func(left, right Application) int {
		return strings.Compare(left.Name, right.Name)
	})

	return applications, nil
}

func (c *Client) Get(
	ctx context.Context,
	name string,
) (*Application, error) {
	applications, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, application := range applications {
		if application.Name == name {
			applicationCopy := application

			return &applicationCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) PathExists(
	ctx context.Context,
	applicationPath string,
) (bool, error) {
	if err := ValidatePath(applicationPath); err != nil {
		return false, err
	}
	_, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		applicationPath,
	)

	return exists, err
}

func (c *Client) Create(
	ctx context.Context,
	definition Definition,
) error {
	if err := ValidateDefinition(definition); err != nil {
		return err
	}

	absolutePath, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		definition.Path,
	)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf(
			"passenger application path %q does not exist",
			definition.Path,
		)
	}

	parameters := definitionParameters(definition, absolutePath)
	parameters.Set("base_uri", definition.BaseURI)

	response := MutationResponse{}
	if err := c.ExecuteUAPIOperationValues(
		ctx,
		http.MethodPost,
		cpanel.ModulePassengerApps,
		operationRegister,
		parameters,
		&response,
	); err != nil {
		return err
	}

	return validateMutationResponse(
		response.Data,
		definition,
		absolutePath,
	)
}

func (c *Client) Update(
	ctx context.Context,
	currentName string,
	definition Definition,
) error {
	if currentName == "" {
		return fmt.Errorf(
			"current Passenger application name must not be empty",
		)
	}
	if err := ValidateDefinition(definition); err != nil {
		return err
	}

	absolutePath, exists, err := c.directoryClient.ResolveDirectory(
		ctx,
		definition.Path,
	)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf(
			"passenger application path %q does not exist",
			definition.Path,
		)
	}

	parameters := definitionParameters(definition, absolutePath)
	parameters.Set("name", currentName)
	if currentName != definition.Name {
		parameters.Set("new_name", definition.Name)
	}

	response := MutationResponse{}
	if err := c.ExecuteUAPIOperationValues(
		ctx,
		http.MethodPost,
		cpanel.ModulePassengerApps,
		operationEdit,
		parameters,
		&response,
	); err != nil {
		return err
	}

	return validateMutationResponse(
		response.Data,
		definition,
		absolutePath,
	)
}

func (c *Client) Delete(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("passenger application name must not be empty")
	}

	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModulePassengerApps,
		operationUnregister,
		map[string]string{"name": name},
		&response,
	)
}

func definitionParameters(
	definition Definition,
	absolutePath string,
) url.Values {
	parameters := url.Values{
		"deployment_mode": {definition.DeploymentMode},
		"domain":          {definition.Domain},
		"enabled":         {booleanParameter(definition.Enabled)},
		"name":            {definition.Name},
		"path":            {absolutePath},
	}

	names := make([]string, 0, len(definition.EnvironmentVariables))
	for name := range definition.EnvironmentVariables {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		parameters.Set("clear_envvars", "1")

		return parameters
	}

	parameters.Set("clear_envvars", "0")
	for _, name := range names {
		parameters.Add("envvar_name", name)
		parameters.Add(
			"envvar_value",
			definition.EnvironmentVariables[name],
		)
	}

	return parameters
}

func validateMutationResponse(
	apiApplication APIApplication,
	definition Definition,
	absolutePath string,
) error {
	if apiApplication.Name == "" {
		return fmt.Errorf(
			"cPanel Passenger mutation did not return application %q data",
			definition.Name,
		)
	}
	enabled, err := parseBooleanFlagJSON(apiApplication.EnabledRaw)
	if err != nil {
		return fmt.Errorf(
			"decode Passenger application %q enabled status: %w",
			definition.Name,
			err,
		)
	}
	if apiApplication.Name != definition.Name ||
		path.Clean(apiApplication.Path) != absolutePath ||
		apiApplication.Domain != definition.Domain ||
		apiApplication.BaseURI != definition.BaseURI ||
		apiApplication.DeploymentMode != definition.DeploymentMode ||
		enabled != definition.Enabled ||
		!maps.Equal(
			apiApplication.Environment,
			definition.EnvironmentVariables,
		) {
		return fmt.Errorf(
			"cPanel Passenger mutation returned an unexpected configuration for application %q",
			definition.Name,
		)
	}

	return nil
}

func applicationFromAPI(
	homeDirectory string,
	key string,
	apiApplication APIApplication,
) (Application, error) {
	name := apiApplication.Name
	if name == "" {
		name = key
	}
	if key == "" || name != key {
		return Application{}, fmt.Errorf(
			"cPanel returned Passenger application key %q with name %q",
			key,
			apiApplication.Name,
		)
	}

	absolutePath := path.Clean(apiApplication.Path)
	cleanHome := path.Clean(homeDirectory)
	homePrefix := cleanHome + "/"
	if !path.IsAbs(absolutePath) ||
		!strings.HasPrefix(absolutePath, homePrefix) {
		return Application{}, fmt.Errorf(
			"passenger application %q returned path %q outside account home %q",
			name,
			apiApplication.Path,
			cleanHome,
		)
	}
	relativePath := strings.TrimPrefix(absolutePath, homePrefix)
	if err := ValidatePath(relativePath); err != nil {
		return Application{}, fmt.Errorf(
			"passenger application %q returned invalid path: %w",
			name,
			err,
		)
	}

	enabled, err := parseBooleanFlagJSON(apiApplication.EnabledRaw)
	if err != nil {
		return Application{}, fmt.Errorf(
			"decode Passenger application %q enabled status: %w",
			name,
			err,
		)
	}
	application := Application{
		Name:                 name,
		Path:                 relativePath,
		AbsolutePath:         absolutePath,
		Domain:               apiApplication.Domain,
		BaseURI:              apiApplication.BaseURI,
		DeploymentMode:       apiApplication.DeploymentMode,
		Enabled:              enabled,
		EnvironmentVariables: cloneStringMap(apiApplication.Environment),
		DependencyCommands: map[string]string{
			"gem": string(apiApplication.Dependencies.Gem),
			"npm": string(apiApplication.Dependencies.NPM),
			"pip": string(apiApplication.Dependencies.Pip),
		},
		NodeJS: normalizeRuntimePath(apiApplication.NodeJS),
		Python: normalizeRuntimePath(apiApplication.Python),
		Ruby:   normalizeRuntimePath(apiApplication.Ruby),
	}
	if err := ValidateDefinition(application.Definition()); err != nil {
		return Application{}, fmt.Errorf(
			"passenger application %q returned invalid configuration: %w",
			name,
			err,
		)
	}

	return application, nil
}

func parseBooleanFlagJSON(raw json.RawMessage) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return false, fmt.Errorf("value is missing")
	}

	switch string(value) {
	case "0", `"0"`, "false":
		return false, nil
	case "1", `"1"`, "true":
		return true, nil
	default:
		return false, fmt.Errorf("expected 0 or 1, got %q", value)
	}
}

func booleanParameter(value bool) string {
	if value {
		return "1"
	}

	return "0"
}

func normalizeRuntimePath(value APIString) string {
	if value == "0" {
		return ""
	}

	return string(value)
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return map[string]string{}
	}

	return maps.Clone(value)
}
