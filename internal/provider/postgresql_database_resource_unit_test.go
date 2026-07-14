package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

func TestPostgreSQLDatabaseStateUpgradeConvertsPublishedUsers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	fixtureData, err := os.ReadFile(filepath.Join(
		"testdata",
		"v0.1.0",
		"postgresql_database_state.json",
	))
	if err != nil {
		t.Fatalf("os.ReadFile() error: %v", err)
	}
	var fixture struct {
		SchemaVersion int64 `json:"schema_version"`
		Attributes    struct {
			Name        string   `json:"name"`
			Users       []string `json:"users"`
			LastUpdated string   `json:"last_updated"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if fixture.SchemaVersion != 0 {
		t.Fatalf("fixture schema version = %d; want 0", fixture.SchemaVersion)
	}
	publishedSchema := resourceschema.Schema{
		Attributes: map[string]resourceschema.Attribute{
			"name": resourceschema.StringAttribute{Required: true},
			"users": resourceschema.ListAttribute{
				ElementType: types.StringType,
				Required:    true,
			},
			"last_updated": resourceschema.StringAttribute{Computed: true},
		},
	}

	resourceUnderTest := &postgreSQLDatabaseResource{}
	schemaResponse := &frameworkresource.SchemaResponse{}
	resourceUnderTest.Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	upgrader, ok := resourceUnderTest.UpgradeState(ctx)[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("PostgreSQL database state upgrader for version 0 is missing")
	}
	if got, want := upgrader.PriorSchema.Type().TerraformType(ctx),
		publishedSchema.Type().TerraformType(ctx); !got.Equal(want) {
		t.Fatalf("prior schema type = %s; want published type %s", got, want)
	}

	priorUsers, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		fixture.Attributes.Users,
	)
	if diagnostics.HasError() {
		t.Fatalf("types.ListValueFrom() diagnostics: %v", diagnostics)
	}
	priorState := tfsdk.State{Schema: publishedSchema}
	diagnostics = priorState.Set(ctx, &struct {
		Name        types.String `tfsdk:"name"`
		Users       types.List   `tfsdk:"users"`
		LastUpdated types.String `tfsdk:"last_updated"`
	}{
		Name:        types.StringValue(fixture.Attributes.Name),
		Users:       priorUsers,
		LastUpdated: types.StringValue(fixture.Attributes.LastUpdated),
	})
	if diagnostics.HasError() {
		t.Fatalf("prior State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.UpgradeStateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	upgrader.StateUpgrader(
		ctx,
		frameworkresource.UpgradeStateRequest{State: &priorState},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("StateUpgrader() diagnostics: %v", response.Diagnostics)
	}

	var upgraded PostgreSQLDatabaseModel
	diagnostics = response.State.Get(ctx, &upgraded)
	if diagnostics.HasError() {
		t.Fatalf("upgraded State.Get() diagnostics: %v", diagnostics)
	}
	var users []string
	diagnostics = upgraded.Users.ElementsAs(ctx, &users, false)
	if diagnostics.HasError() {
		t.Fatalf("upgraded users diagnostics: %v", diagnostics)
	}
	slices.Sort(users)
	if !slices.Equal(users, []string{"account_first", "account_second"}) {
		t.Fatalf("upgraded users = %v", users)
	}
	if upgraded.DeleteOnDestroy.IsNull() ||
		upgraded.DeleteOnDestroy.IsUnknown() ||
		upgraded.DeleteOnDestroy.ValueBool() {
		t.Fatal("upgraded state must preserve the remote database by default")
	}
}

func TestPostgreSQLDatabaseUpdateReportsPartialPrivilegeMutation(t *testing.T) {
	t.Parallel()

	const (
		databaseName = "account_database"
		existingUser = "account_existing"
		firstUser    = "account_first"
		secondUser   = "account_second"
	)

	tests := map[string]struct {
		operation       string
		mutationPath    string
		diagnosticTitle string
		stateUsers      []string
		planUsers       []string
		wantUsers       []string
	}{
		"grant": {
			operation:       "grant",
			mutationPath:    "/execute/Postgresql/grant_all_privileges",
			diagnosticTitle: "Unable to grant PostgreSQL database privileges",
			stateUsers:      []string{existingUser},
			planUsers:       []string{existingUser, firstUser, secondUser},
			wantUsers:       []string{existingUser, firstUser},
		},
		"revoke": {
			operation:       "revoke",
			mutationPath:    "/execute/Postgresql/revoke_all_privileges",
			diagnosticTitle: "Unable to revoke PostgreSQL database privileges",
			stateUsers:      []string{existingUser, firstUser, secondUser},
			planUsers:       []string{existingUser},
			wantUsers:       []string{existingUser, secondUser},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			currentUsers := slices.Clone(test.stateUsers)
			allUsers := []string{existingUser, firstUser, secondUser}
			var mutationUsers []string

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Postgresql/list_users":
					writePostgreSQLDatabaseResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data":   allUsers,
						},
					)
				case "/execute/Postgresql/list_databases":
					users := slices.Clone(currentUsers)
					slices.Sort(users)
					writePostgreSQLDatabaseResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data": []map[string]any{{
								"database":   databaseName,
								"disk_usage": 0,
								"users":      users,
							}},
						},
					)
				case test.mutationPath:
					if request.Method != http.MethodPost {
						t.Errorf("method = %s, want POST", request.Method)
					}
					user := request.FormValue("user")
					mutationUsers = append(mutationUsers, user)
					if len(mutationUsers) == 2 {
						writePostgreSQLDatabaseResourceTestJSON(
							t,
							response,
							map[string]any{
								"status": 0,
								"errors": []string{"forced mutation failure"},
							},
						)
						return
					}
					switch test.operation {
					case "grant":
						currentUsers = append(currentUsers, user)
					case "revoke":
						currentUsers = slices.DeleteFunc(
							currentUsers,
							func(value string) bool { return value == user },
						)
					default:
						t.Errorf("unexpected operation %q", test.operation)
					}
					writePostgreSQLDatabaseResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data":   []string{},
						},
					)
				default:
					t.Errorf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
					http.Error(response, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			baseClient, err := cpanel.NewClient(
				server.URL,
				"account",
				"api-token",
			)
			if err != nil {
				t.Fatalf("cpanel.NewClient() error: %v", err)
			}
			resource := &postgreSQLDatabaseResource{
				client: postgresql.NewClient(baseClient),
			}
			response := runPostgreSQLDatabaseUpdate(
				t,
				resource,
				PostgreSQLDatabaseModel{
					Name:            types.StringValue(databaseName),
					Users:           postgreSQLDatabaseUsersForTest(t, test.stateUsers),
					DeleteOnDestroy: types.BoolValue(false),
				},
				PostgreSQLDatabaseModel{
					Name:            types.StringValue(databaseName),
					Users:           postgreSQLDatabaseUsersForTest(t, test.planUsers),
					DeleteOnDestroy: types.BoolValue(false),
				},
			)

			if !response.Diagnostics.HasError() {
				t.Fatal("Update() returned no partial mutation error")
			}
			detail := fmt.Sprint(response.Diagnostics)
			for _, expected := range []string{
				test.diagnosticTitle,
				"did not attempt an automatic rollback",
				databaseName,
			} {
				if !strings.Contains(detail, expected) {
					t.Fatalf(
						"Update() diagnostics = %s, want containing %q",
						detail,
						expected,
					)
				}
			}
			if !slices.Equal(mutationUsers, []string{firstUser, secondUser}) {
				t.Fatalf(
					"mutation users = %v, want %v",
					mutationUsers,
					[]string{firstUser, secondUser},
				)
			}
			slices.Sort(currentUsers)
			wantUsers := slices.Clone(test.wantUsers)
			slices.Sort(wantUsers)
			if !slices.Equal(currentUsers, wantUsers) {
				t.Fatalf(
					"database users after failure = %v, want %v",
					currentUsers,
					wantUsers,
				)
			}
		})
	}
}

func runPostgreSQLDatabaseUpdate(
	t *testing.T,
	resource *postgreSQLDatabaseResource,
	stateModel PostgreSQLDatabaseModel,
	planModel PostgreSQLDatabaseModel,
) *frameworkresource.UpdateResponse {
	t.Helper()

	ctx := t.Context()
	schemaResponse := &frameworkresource.SchemaResponse{}
	NewPostgreSQLDatabaseResource().Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}

	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(ctx, &stateModel)
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics = plan.Set(ctx, &planModel)
	if diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	resource.Update(
		ctx,
		frameworkresource.UpdateRequest{
			State: state,
			Plan:  plan,
		},
		response,
	)

	return response
}

func postgreSQLDatabaseUsersForTest(
	t *testing.T,
	users []string,
) types.Set {
	t.Helper()

	value, diagnostics := types.SetValueFrom(
		t.Context(),
		types.StringType,
		users,
	)
	if diagnostics.HasError() {
		t.Fatalf("types.SetValueFrom() diagnostics: %v", diagnostics)
	}

	return value
}

func writePostgreSQLDatabaseResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
