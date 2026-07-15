package passenger

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsPassengerApplications(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writePassengerJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"home": "/home/example",
				},
			})
		case "/execute/PassengerApps/list_applications":
			writePassengerJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"Zulu": map[string]any{
						"name":            "Zulu",
						"path":            "/home/example/apps/zulu",
						"domain":          "example.test",
						"base_uri":        "/zulu",
						"deployment_mode": "production",
						"enabled":         "1",
						"envvars": map[string]string{
							"APP_ENV": "production",
						},
						"deps": map[string]string{
							"gem": "0",
							"npm": "npm install",
							"pip": "0",
						},
						"nodejs": "/usr/bin/node",
					},
					"Alpha": map[string]any{
						"name":            "Alpha",
						"path":            "/home/example/apps/alpha",
						"domain":          "example.test",
						"base_uri":        "/alpha",
						"deployment_mode": "development",
						"enabled":         0,
						"envvars":         nil,
						"deps": map[string]any{
							"gem": 0,
							"npm": "0",
							"pip": 0,
						},
						"nodejs": 0,
						"python": nullJSONValue{},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	applications, err := newPassengerTestClient(t, server).List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(applications) != 2 ||
		applications[0].Name != "Alpha" ||
		applications[1].Name != "Zulu" {
		t.Fatalf("applications = %#v", applications)
	}
	if applications[1].Path != "apps/zulu" ||
		!applications[1].Enabled ||
		applications[1].NodeJS != "/usr/bin/node" ||
		applications[1].DependencyCommands["npm"] != "npm install" {
		t.Fatalf("Zulu application = %#v", applications[1])
	}
}

func TestClientCreatesPassengerApplicationWithRepeatedEnvironmentVariables(
	t *testing.T,
) {
	t.Parallel()

	definition := Definition{
		Name:           "Terraform Passenger",
		Path:           "tfcpanel-git-passenger",
		Domain:         "example.test",
		BaseURI:        "/terraform",
		DeploymentMode: DeploymentModeProduction,
		Enabled:        false,
		EnvironmentVariables: map[string]string{
			"BETA":  "two",
			"ALPHA": "one",
		},
	}

	server := passengerMutationTestServer(
		t,
		definition,
		operationRegister,
		func(request *http.Request) {
			if got := request.Form["envvar_name"]; !slices.Equal(
				got,
				[]string{"ALPHA", "BETA"},
			) {
				t.Errorf("envvar_name = %#v", got)
			}
			if got := request.Form["envvar_value"]; !slices.Equal(
				got,
				[]string{"one", "two"},
			) {
				t.Errorf("envvar_value = %#v", got)
			}
			if request.Form.Get("clear_envvars") != "0" ||
				request.Form.Get("base_uri") != definition.BaseURI {
				t.Errorf("form = %v", request.Form)
			}
		},
	)
	defer server.Close()

	if err := newPassengerTestClient(t, server).Create(
		t.Context(),
		definition,
	); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
}

func TestClientUpdatesPassengerApplicationAndClearsEnvironment(t *testing.T) {
	t.Parallel()

	definition := Definition{
		Name:                 "Terraform Passenger renamed",
		Path:                 "tfcpanel-git-passenger",
		Domain:               "example.test",
		BaseURI:              "/terraform",
		DeploymentMode:       DeploymentModeDevelopment,
		Enabled:              true,
		EnvironmentVariables: map[string]string{},
	}

	server := passengerMutationTestServer(
		t,
		definition,
		operationEdit,
		func(request *http.Request) {
			if request.Form.Get("name") != "Terraform Passenger" ||
				request.Form.Get("new_name") != definition.Name ||
				request.Form.Get("clear_envvars") != "1" ||
				len(request.Form["envvar_name"]) != 0 {
				t.Errorf("form = %v", request.Form)
			}
		},
	)
	defer server.Close()

	if err := newPassengerTestClient(t, server).Update(
		t.Context(),
		"Terraform Passenger",
		definition,
	); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
}

func TestClientUpdatesPassengerApplicationWithoutRedundantRename(
	t *testing.T,
) {
	t.Parallel()

	definition := Definition{
		Name:                 "Terraform Passenger",
		Path:                 "tfcpanel-git-passenger",
		Domain:               "example.test",
		BaseURI:              "/terraform",
		DeploymentMode:       DeploymentModeProduction,
		Enabled:              false,
		EnvironmentVariables: map[string]string{"APP_ENV": "test"},
	}

	server := passengerMutationTestServer(
		t,
		definition,
		operationEdit,
		func(request *http.Request) {
			if request.Form.Get("name") != definition.Name {
				t.Errorf("name = %q", request.Form.Get("name"))
			}
			if _, exists := request.Form["new_name"]; exists {
				t.Errorf("new_name must be omitted: %v", request.Form)
			}
		},
	)
	defer server.Close()

	if err := newPassengerTestClient(t, server).Update(
		t.Context(),
		definition.Name,
		definition,
	); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
}

func passengerMutationTestServer(
	t *testing.T,
	definition Definition,
	operation string,
	checkForm func(*http.Request),
) *httptest.Server {
	t.Helper()

	absolutePath := "/home/example/" + definition.Path

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writePassengerJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"home": "/home/example",
				},
			})
		case "/execute/Fileman/list_files":
			writePassengerJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"file":     definition.Path,
					"fullpath": absolutePath,
					"type":     "dir",
				}},
			})
		case "/execute/PassengerApps/" + operation:
			if request.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			checkForm(request)
			if request.Form.Get("path") != absolutePath ||
				request.Form.Get("domain") != definition.Domain ||
				request.Form.Get("deployment_mode") !=
					definition.DeploymentMode ||
				request.Form.Get("enabled") !=
					booleanParameter(definition.Enabled) {
				t.Errorf("form = %v", request.Form)
			}
			writePassengerJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"name":            definition.Name,
					"path":            absolutePath,
					"domain":          definition.Domain,
					"base_uri":        definition.BaseURI,
					"deployment_mode": definition.DeploymentMode,
					"enabled":         booleanParameter(definition.Enabled),
					"envvars":         definition.EnvironmentVariables,
					"deps": map[string]any{
						"gem": 0,
						"npm": "0",
						"pip": 0,
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
}

func TestAPIStringRejectsNonScalarJSON(t *testing.T) {
	t.Parallel()

	for _, value := range []string{`true`, `[]`, `{}`} {
		var decoded APIString
		if err := json.Unmarshal([]byte(value), &decoded); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded", value)
		}
	}
}

type nullJSONValue struct{}

func (nullJSONValue) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

func TestApplicationDefinitionReturnsIndependentEnvironmentMap(t *testing.T) {
	t.Parallel()

	application := Application{
		EnvironmentVariables: map[string]string{"APP_ENV": "production"},
	}
	definition := application.Definition()
	definition.EnvironmentVariables["APP_ENV"] = "development"
	if !maps.Equal(
		application.EnvironmentVariables,
		map[string]string{"APP_ENV": "production"},
	) {
		t.Fatalf(
			"application environment mutated: %#v",
			application.EnvironmentVariables,
		)
	}
}

func newPassengerTestClient(
	t *testing.T,
	server *httptest.Server,
) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func writePassengerJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
