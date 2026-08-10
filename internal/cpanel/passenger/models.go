package passenger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"

	"terraform-provider-cpanel/internal/cpanel"
)

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data map[string]APIApplication `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data APIApplication `json:"data"`
}

type APIApplication struct {
	BaseURI        string            `json:"base_uri"`
	DeploymentMode string            `json:"deployment_mode"`
	Dependencies   APIDependencies   `json:"deps"`
	Domain         string            `json:"domain"`
	EnabledRaw     json.RawMessage   `json:"enabled"`
	Environment    map[string]string `json:"envvars"`
	Name           string            `json:"name"`
	NodeJS         APIString         `json:"nodejs"`
	Path           string            `json:"path"`
	Python         APIString         `json:"python"`
	Ruby           APIString         `json:"ruby"`
}

type APIDependencies struct {
	Gem APIString `json:"gem"`
	NPM APIString `json:"npm"`
	Pip APIString `json:"pip"`
}

type APIString string

func (value *APIString) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		*value = ""

		return nil
	}

	var stringValue string
	if err := json.Unmarshal(trimmed, &stringValue); err == nil {
		*value = APIString(stringValue)

		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var numberValue json.Number
	if err := decoder.Decode(&numberValue); err == nil {
		*value = APIString(numberValue.String())

		return nil
	}

	return fmt.Errorf(
		"expected a string, number, or null, got %s",
		trimmed,
	)
}

type Application struct {
	Name                 string
	Path                 string
	AbsolutePath         string
	Domain               string
	BaseURI              string
	DeploymentMode       string
	Enabled              bool
	EnvironmentVariables map[string]string
	DependencyCommands   map[string]string
	NodeJS               string
	Python               string
	Ruby                 string
}

type Definition struct {
	Name                 string
	Path                 string
	Domain               string
	BaseURI              string
	DeploymentMode       string
	Enabled              bool
	EnvironmentVariables map[string]string
}

func (a Application) Definition() Definition {
	return Definition{
		Name:                 a.Name,
		Path:                 a.Path,
		Domain:               a.Domain,
		BaseURI:              a.BaseURI,
		DeploymentMode:       a.DeploymentMode,
		Enabled:              a.Enabled,
		EnvironmentVariables: cloneStringMap(a.EnvironmentVariables),
	}
}

func (a Application) Matches(definition Definition) bool {
	return a.Name == definition.Name &&
		a.Path == definition.Path &&
		a.Domain == definition.Domain &&
		a.BaseURI == definition.BaseURI &&
		a.DeploymentMode == definition.DeploymentMode &&
		a.Enabled == definition.Enabled &&
		maps.Equal(
			a.EnvironmentVariables,
			definition.EnvironmentVariables,
		)
}
