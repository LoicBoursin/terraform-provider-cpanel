package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelpassenger "terraform-provider-cpanel/internal/cpanel/passenger"
)

func TestPassengerApplicationCreateReconcilesAmbiguousAppliedResponse(
	t *testing.T,
) {
	definition := testPassengerApplicationDefinition()
	state := newPassengerApplicationResourceTestState()
	state.registerOutcome = "ambiguous_applied"
	resource, server := newPassengerApplicationResourceTestServer(t, state)
	defer server.Close()

	response := runPassengerApplicationCreate(t, resource, definition)

	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics: %v", response.Diagnostics)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.unregisterCalls != 0 {
		t.Fatalf(
			"unregister calls = %d, want 0",
			state.unregisterCalls,
		)
	}
	application := state.applications[definition.Name]
	if !application.Matches(definition) {
		t.Fatalf(
			"ambiguous creation was not preserved: %#v",
			application,
		)
	}

	var model PassengerApplicationResourceModel
	response.Diagnostics.Append(
		response.State.Get(t.Context(), &model)...,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", response.Diagnostics)
	}
	if model.Name.ValueString() != definition.Name ||
		model.AbsolutePath.ValueString() !=
			"/home/example/"+definition.Path {
		t.Fatalf("created state = %#v", model)
	}
}

func TestPassengerApplicationDeleteRequiresCompleteDefinitionMatch(
	t *testing.T,
) {
	expected := testPassengerApplicationDefinition()
	testCases := []struct {
		name   string
		mutate func(*cpanelpassenger.Definition)
	}{
		{
			name: "path changed",
			mutate: func(definition *cpanelpassenger.Definition) {
				definition.Path = "passenger-concurrent"
			},
		},
		{
			name: "domain changed",
			mutate: func(definition *cpanelpassenger.Definition) {
				definition.Domain = "concurrent.example.test"
			},
		},
		{
			name: "base URI changed",
			mutate: func(definition *cpanelpassenger.Definition) {
				definition.BaseURI = "/concurrent"
			},
		},
		{
			name: "deployment mode changed",
			mutate: func(definition *cpanelpassenger.Definition) {
				definition.DeploymentMode =
					cpanelpassenger.DeploymentModeDevelopment
			},
		},
		{
			name: "enabled changed",
			mutate: func(definition *cpanelpassenger.Definition) {
				definition.Enabled = !definition.Enabled
			},
		},
		{
			name: "environment changed",
			mutate: func(definition *cpanelpassenger.Definition) {
				definition.EnvironmentVariables = map[string]string{
					"APP_ENV": "concurrent",
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			current := expected
			current.EnvironmentVariables = clonePassengerEnvironment(
				expected.EnvironmentVariables,
			)
			testCase.mutate(&current)
			state := newPassengerApplicationResourceTestState()
			state.setApplication(current)
			resource, server := newPassengerApplicationResourceTestServer(
				t,
				state,
			)
			defer server.Close()

			response := runPassengerApplicationDelete(
				t,
				resource,
				expected,
			)

			if !response.Diagnostics.HasError() {
				t.Fatal("Delete() returned no error for remote drift")
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.unregisterCalls != 0 {
				t.Fatalf(
					"unregister calls = %d, want 0",
					state.unregisterCalls,
				)
			}
			application := state.applications[expected.Name]
			if !application.Matches(current) {
				t.Fatalf(
					"Delete() changed concurrent application: %#v",
					application,
				)
			}
		})
	}
}

func TestPassengerApplicationDeleteExactDefinition(t *testing.T) {
	expected := testPassengerApplicationDefinition()
	state := newPassengerApplicationResourceTestState()
	state.setApplication(expected)
	resource, server := newPassengerApplicationResourceTestServer(t, state)
	defer server.Close()

	response := runPassengerApplicationDelete(t, resource, expected)

	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.unregisterCalls != 1 {
		t.Fatalf("unregister calls = %d, want 1", state.unregisterCalls)
	}
	if _, exists := state.applications[expected.Name]; exists {
		t.Fatal("Delete() left the exact application registered")
	}
}

func TestPassengerApplicationDeleteReconcilesAmbiguousResponse(
	t *testing.T,
) {
	expected := testPassengerApplicationDefinition()
	replacement := expected
	replacement.Path = "passenger-concurrent"

	testCases := []struct {
		name            string
		outcome         string
		replacement     *cpanelpassenger.Definition
		wantError       bool
		wantApplication *cpanelpassenger.Definition
		wantWarning     bool
	}{
		{
			name:    "deletion applied",
			outcome: "ambiguous_deleted",
		},
		{
			name:            "deletion not applied",
			outcome:         "ambiguous_unchanged",
			wantError:       true,
			wantApplication: &expected,
		},
		{
			name:            "concurrent replacement",
			outcome:         "ambiguous_replaced",
			replacement:     &replacement,
			wantApplication: &replacement,
			wantWarning:     true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := newPassengerApplicationResourceTestState()
			state.unregisterOutcome = testCase.outcome
			state.unregisterReplacement = testCase.replacement
			state.setApplication(expected)
			resource, server := newPassengerApplicationResourceTestServer(
				t,
				state,
			)
			defer server.Close()

			response := runPassengerApplicationDelete(
				t,
				resource,
				expected,
			)

			if response.Diagnostics.HasError() != testCase.wantError {
				t.Fatalf(
					"Delete() error = %t, want %t: %v",
					response.Diagnostics.HasError(),
					testCase.wantError,
					response.Diagnostics,
				)
			}
			if (len(response.Diagnostics.Warnings()) > 0) !=
				testCase.wantWarning {
				t.Fatalf(
					"Delete() warnings = %v, wantWarning %t",
					response.Diagnostics.Warnings(),
					testCase.wantWarning,
				)
			}

			state.mu.Lock()
			defer state.mu.Unlock()
			if state.unregisterCalls != 1 {
				t.Fatalf(
					"unregister calls = %d, want 1",
					state.unregisterCalls,
				)
			}
			application, exists := state.applications[expected.Name]
			if testCase.wantApplication == nil {
				if exists {
					t.Fatalf(
						"Delete() left application %#v",
						application,
					)
				}
				return
			}
			if !exists || !application.Matches(*testCase.wantApplication) {
				t.Fatalf(
					"current application = %#v, want %#v",
					application,
					*testCase.wantApplication,
				)
			}
		})
	}
}

func TestPassengerApplicationRestoreIsConditional(t *testing.T) {
	originalDefinition := testPassengerApplicationDefinition()
	original := testPassengerApplication(originalDefinition)
	attempted := originalDefinition
	attempted.DeploymentMode = cpanelpassenger.DeploymentModeDevelopment
	attempted.Enabled = true
	attempted.EnvironmentVariables = map[string]string{
		"APP_ENV": "attempted",
	}
	concurrent := attempted
	concurrent.Path = "passenger-concurrent"

	testCases := []struct {
		name        string
		current     []cpanelpassenger.Definition
		editOutcome string
		wantError   bool
		wantEdits   int
	}{
		{
			name:      "original already restored",
			current:   []cpanelpassenger.Definition{originalDefinition},
			wantEdits: 0,
		},
		{
			name:      "attempted definition is restored",
			current:   []cpanelpassenger.Definition{attempted},
			wantEdits: 1,
		},
		{
			name:      "concurrent definition is preserved",
			current:   []cpanelpassenger.Definition{concurrent},
			wantError: true,
		},
		{
			name:        "ambiguous restore already applied",
			current:     []cpanelpassenger.Definition{attempted},
			editOutcome: "ambiguous_applied",
			wantEdits:   1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := newPassengerApplicationResourceTestState()
			state.editOutcome = testCase.editOutcome
			for _, definition := range testCase.current {
				state.setApplication(definition)
			}
			resource, server := newPassengerApplicationResourceTestServer(
				t,
				state,
			)
			defer server.Close()

			err := resource.restore(t.Context(), original, attempted)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"restore() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.editCalls != testCase.wantEdits {
				t.Fatalf(
					"edit calls = %d, want %d",
					state.editCalls,
					testCase.wantEdits,
				)
			}
			if testCase.wantError {
				application := state.applications[concurrent.Name]
				if !application.Matches(concurrent) {
					t.Fatalf(
						"restore() changed concurrent application: %#v",
						application,
					)
				}
				return
			}
			application := state.applications[originalDefinition.Name]
			if !application.Matches(originalDefinition) {
				t.Fatalf(
					"restored application = %#v, want %#v",
					application,
					originalDefinition,
				)
			}
		})
	}
}

func TestPassengerApplicationRestoreRefusesDuplicateRename(t *testing.T) {
	originalDefinition := testPassengerApplicationDefinition()
	original := testPassengerApplication(originalDefinition)
	attempted := originalDefinition
	attempted.Name = "Terraform Passenger attempted"
	attempted.DeploymentMode = cpanelpassenger.DeploymentModeDevelopment

	state := newPassengerApplicationResourceTestState()
	state.setApplication(originalDefinition)
	state.setApplication(attempted)
	resource, server := newPassengerApplicationResourceTestServer(t, state)
	defer server.Close()

	err := resource.restore(t.Context(), original, attempted)

	if err == nil {
		t.Fatal("restore() returned no error for duplicate names")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.editCalls != 0 {
		t.Fatalf("edit calls = %d, want 0", state.editCalls)
	}
	if !state.applications[originalDefinition.Name].Matches(
		originalDefinition,
	) || !state.applications[attempted.Name].Matches(attempted) {
		t.Fatal("restore() changed one of the concurrently registered names")
	}
}

func TestPassengerApplicationRestoreRenamedAttempt(t *testing.T) {
	originalDefinition := testPassengerApplicationDefinition()
	original := testPassengerApplication(originalDefinition)
	attempted := originalDefinition
	attempted.Name = "Terraform Passenger attempted"
	attempted.DeploymentMode = cpanelpassenger.DeploymentModeDevelopment

	testCases := []struct {
		name        string
		current     cpanelpassenger.Definition
		editOutcome string
		wantEdits   int
	}{
		{
			name:      "original already restored",
			current:   originalDefinition,
			wantEdits: 0,
		},
		{
			name:      "attempted rename is restored",
			current:   attempted,
			wantEdits: 1,
		},
		{
			name:        "ambiguous rename restore already applied",
			current:     attempted,
			editOutcome: "ambiguous_applied",
			wantEdits:   1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := newPassengerApplicationResourceTestState()
			state.editOutcome = testCase.editOutcome
			state.setApplication(testCase.current)
			resource, server := newPassengerApplicationResourceTestServer(
				t,
				state,
			)
			defer server.Close()

			if err := resource.restore(
				t.Context(),
				original,
				attempted,
			); err != nil {
				t.Fatalf("restore() error: %v", err)
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.editCalls != testCase.wantEdits {
				t.Fatalf(
					"edit calls = %d, want %d",
					state.editCalls,
					testCase.wantEdits,
				)
			}
			application := state.applications[originalDefinition.Name]
			if !application.Matches(originalDefinition) {
				t.Fatalf(
					"restored application = %#v, want %#v",
					application,
					originalDefinition,
				)
			}
			if _, exists := state.applications[attempted.Name]; exists {
				t.Fatalf(
					"attempted name %q remains registered",
					attempted.Name,
				)
			}
		})
	}
}

type passengerApplicationResourceTestState struct {
	mu                    sync.Mutex
	applications          map[string]cpanelpassenger.Application
	registerOutcome       string
	editOutcome           string
	unregisterOutcome     string
	unregisterReplacement *cpanelpassenger.Definition
	registerCalls         int
	editCalls             int
	unregisterCalls       int
}

func newPassengerApplicationResourceTestState() *passengerApplicationResourceTestState {
	return &passengerApplicationResourceTestState{
		applications: map[string]cpanelpassenger.Application{},
	}
}

func (s *passengerApplicationResourceTestState) setApplication(
	definition cpanelpassenger.Definition,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applications[definition.Name] = testPassengerApplication(definition)
}

func newPassengerApplicationResourceTestServer(
	t *testing.T,
	state *passengerApplicationResourceTestState,
) (*passengerApplicationResource, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writePassengerApplicationResourceTestJSON(
				t,
				response,
				map[string]any{
					"status": 1,
					"data": map[string]any{
						"home": "/home/example",
					},
				},
			)
		case "/execute/Fileman/list_files":
			directory := request.URL.Query().Get("dir")
			data := []map[string]any{}
			if directory == "" {
				data = []map[string]any{
					{
						"file":     "passenger-original",
						"fullpath": "/home/example/passenger-original",
						"type":     "dir",
					},
					{
						"file":     "passenger-concurrent",
						"fullpath": "/home/example/passenger-concurrent",
						"type":     "dir",
					},
				}
			}
			writePassengerApplicationResourceTestJSON(
				t,
				response,
				map[string]any{"status": 1, "data": data},
			)
		case "/execute/PassengerApps/list_applications":
			state.mu.Lock()
			data := make(map[string]any, len(state.applications))
			for name, application := range state.applications {
				data[name] = passengerApplicationResourceTestAPI(
					application,
				)
			}
			state.mu.Unlock()
			writePassengerApplicationResourceTestJSON(
				t,
				response,
				map[string]any{"status": 1, "data": data},
			)
		case "/execute/PassengerApps/register_application":
			definition := passengerApplicationDefinitionFromTestRequest(
				t,
				request,
				"",
			)
			state.mu.Lock()
			state.registerCalls++
			state.applications[definition.Name] =
				testPassengerApplication(definition)
			outcome := state.registerOutcome
			state.mu.Unlock()
			if outcome == "ambiguous_applied" {
				_, _ = response.Write([]byte("{"))
				return
			}
			writePassengerApplicationMutationResponse(
				t,
				response,
				testPassengerApplication(definition),
			)
		case "/execute/PassengerApps/edit_application":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			currentName := request.Form.Get("name")
			state.mu.Lock()
			current := state.applications[currentName]
			state.mu.Unlock()
			definition := passengerApplicationDefinitionFromParsedTestRequest(
				request,
				current.BaseURI,
			)
			state.mu.Lock()
			state.editCalls++
			delete(state.applications, currentName)
			state.applications[definition.Name] =
				testPassengerApplication(definition)
			outcome := state.editOutcome
			state.mu.Unlock()
			if outcome == "ambiguous_applied" {
				_, _ = response.Write([]byte("{"))
				return
			}
			writePassengerApplicationMutationResponse(
				t,
				response,
				testPassengerApplication(definition),
			)
		case "/execute/PassengerApps/unregister_application":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			state.mu.Lock()
			state.unregisterCalls++
			name := request.Form.Get("name")
			outcome := state.unregisterOutcome
			switch outcome {
			case "ambiguous_unchanged":
			case "ambiguous_replaced":
				delete(state.applications, name)
				if state.unregisterReplacement != nil {
					state.applications[name] = testPassengerApplication(
						*state.unregisterReplacement,
					)
				}
			default:
				delete(state.applications, name)
			}
			state.mu.Unlock()
			if strings.HasPrefix(outcome, "ambiguous_") {
				_, _ = response.Write([]byte("{"))
				return
			}
			writePassengerApplicationResourceTestJSON(
				t,
				response,
				map[string]any{"status": 1, "data": nil},
			)
		default:
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		server.Close()
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return &passengerApplicationResource{
		client: cpanelpassenger.NewClient(baseClient),
	}, server
}

func passengerApplicationDefinitionFromTestRequest(
	t *testing.T,
	request *http.Request,
	fallbackBaseURI string,
) cpanelpassenger.Definition {
	t.Helper()
	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}

	return passengerApplicationDefinitionFromParsedTestRequest(
		request,
		fallbackBaseURI,
	)
}

func passengerApplicationDefinitionFromParsedTestRequest(
	request *http.Request,
	fallbackBaseURI string,
) cpanelpassenger.Definition {
	name := request.Form.Get("new_name")
	if name == "" {
		name = request.Form.Get("name")
	}
	baseURI := request.Form.Get("base_uri")
	if baseURI == "" {
		baseURI = fallbackBaseURI
	}
	environment := map[string]string{}
	names := request.Form["envvar_name"]
	values := request.Form["envvar_value"]
	for index := range names {
		environment[names[index]] = values[index]
	}

	return cpanelpassenger.Definition{
		Name:                 name,
		Path:                 strings.TrimPrefix(request.Form.Get("path"), "/home/example/"),
		Domain:               request.Form.Get("domain"),
		BaseURI:              baseURI,
		DeploymentMode:       request.Form.Get("deployment_mode"),
		Enabled:              request.Form.Get("enabled") == "1",
		EnvironmentVariables: environment,
	}
}

func passengerApplicationResourceTestAPI(
	application cpanelpassenger.Application,
) map[string]any {
	enabled := "0"
	if application.Enabled {
		enabled = "1"
	}

	return map[string]any{
		"name":            application.Name,
		"path":            application.AbsolutePath,
		"domain":          application.Domain,
		"base_uri":        application.BaseURI,
		"deployment_mode": application.DeploymentMode,
		"enabled":         enabled,
		"envvars":         application.EnvironmentVariables,
		"deps": map[string]any{
			"gem": 0,
			"npm": 0,
			"pip": 0,
		},
		"nodejs": 0,
		"python": 0,
		"ruby":   0,
	}
}

func writePassengerApplicationMutationResponse(
	t *testing.T,
	response http.ResponseWriter,
	application cpanelpassenger.Application,
) {
	t.Helper()
	writePassengerApplicationResourceTestJSON(
		t,
		response,
		map[string]any{
			"status": 1,
			"data":   passengerApplicationResourceTestAPI(application),
		},
	)
}

func writePassengerApplicationResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func runPassengerApplicationCreate(
	t *testing.T,
	resource *passengerApplicationResource,
	definition cpanelpassenger.Definition,
) *frameworkresource.CreateResponse {
	t.Helper()

	schema := passengerApplicationResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schema}
	model := passengerApplicationResourceTestModel(
		t,
		definition,
		true,
	)
	if diagnostics := plan.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schema},
	}
	resource.Create(
		t.Context(),
		frameworkresource.CreateRequest{Plan: plan},
		response,
	)

	return response
}

func runPassengerApplicationDelete(
	t *testing.T,
	resource *passengerApplicationResource,
	definition cpanelpassenger.Definition,
) *frameworkresource.DeleteResponse {
	t.Helper()

	schema := passengerApplicationResourceTestSchema(t)
	state := tfsdk.State{Schema: schema}
	model := passengerApplicationResourceTestModel(
		t,
		definition,
		false,
	)
	if diagnostics := state.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.DeleteResponse{}
	resource.Delete(
		t.Context(),
		frameworkresource.DeleteRequest{State: state},
		response,
	)

	return response
}

func passengerApplicationResourceTestSchema(
	t *testing.T,
) resourceschema.Schema {
	t.Helper()

	response := &frameworkresource.SchemaResponse{}
	NewPassengerApplicationResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	return response.Schema
}

func passengerApplicationResourceTestModel(
	t *testing.T,
	definition cpanelpassenger.Definition,
	computedUnknown bool,
) PassengerApplicationResourceModel {
	t.Helper()

	environment, diagnostics := types.MapValueFrom(
		t.Context(),
		types.StringType,
		definition.EnvironmentVariables,
	)
	if diagnostics.HasError() {
		t.Fatalf("MapValueFrom() diagnostics: %v", diagnostics)
	}
	model := PassengerApplicationResourceModel{
		Name:                 types.StringValue(definition.Name),
		Path:                 types.StringValue(definition.Path),
		Domain:               types.StringValue(definition.Domain),
		BaseURI:              types.StringValue(definition.BaseURI),
		DeploymentMode:       types.StringValue(definition.DeploymentMode),
		Enabled:              types.BoolValue(definition.Enabled),
		EnvironmentVariables: environment,
	}
	if computedUnknown {
		model.AbsolutePath = types.StringUnknown()
		model.DependencyCommands = types.MapUnknown(types.StringType)
		model.NodeJS = types.StringUnknown()
		model.Python = types.StringUnknown()
		model.Ruby = types.StringUnknown()
	} else {
		model.AbsolutePath = types.StringValue(
			"/home/example/" + definition.Path,
		)
		model.DependencyCommands = types.MapValueMust(
			types.StringType,
			map[string]attr.Value{},
		)
		model.NodeJS = types.StringNull()
		model.Python = types.StringNull()
		model.Ruby = types.StringNull()
	}

	return model
}

func testPassengerApplicationDefinition() cpanelpassenger.Definition {
	return cpanelpassenger.Definition{
		Name:           "Terraform Passenger original",
		Path:           "passenger-original",
		Domain:         "example.test",
		BaseURI:        "/passenger",
		DeploymentMode: cpanelpassenger.DeploymentModeProduction,
		Enabled:        false,
		EnvironmentVariables: map[string]string{
			"APP_ENV": "original",
		},
	}
}

func testPassengerApplication(
	definition cpanelpassenger.Definition,
) cpanelpassenger.Application {
	return cpanelpassenger.Application{
		Name:           definition.Name,
		Path:           definition.Path,
		AbsolutePath:   "/home/example/" + definition.Path,
		Domain:         definition.Domain,
		BaseURI:        definition.BaseURI,
		DeploymentMode: definition.DeploymentMode,
		Enabled:        definition.Enabled,
		EnvironmentVariables: clonePassengerEnvironment(
			definition.EnvironmentVariables,
		),
		DependencyCommands: map[string]string{
			"gem": "",
			"npm": "",
			"pip": "",
		},
	}
}

func clonePassengerEnvironment(
	environment map[string]string,
) map[string]string {
	cloned := make(map[string]string, len(environment))
	for name, value := range environment {
		cloned[name] = value
	}

	return cloned
}
