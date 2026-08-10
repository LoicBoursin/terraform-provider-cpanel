package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/mysql"
)

func TestMySQLDatabaseUpdateReportsPartialPrivilegeMutation(t *testing.T) {
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
			mutationPath:    "/execute/Mysql/set_privileges_on_database",
			diagnosticTitle: "Unable to grant MySQL database privileges",
			stateUsers:      []string{existingUser},
			planUsers:       []string{existingUser, firstUser, secondUser},
			wantUsers:       []string{existingUser, firstUser},
		},
		"revoke": {
			operation:       "revoke",
			mutationPath:    "/execute/Mysql/revoke_access_to_database",
			diagnosticTitle: "Unable to revoke MySQL database privileges",
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
				case "/execute/Mysql/get_restrictions":
					writeMySQLDatabaseResourceTestJSON(t, response, map[string]any{
						"status": 1,
						"data": map[string]any{
							"max_database_name_length": 64,
							"max_username_length":      64,
							"prefix":                   "account_",
						},
					})
				case "/execute/Mysql/list_users":
					users := make([]map[string]any, 0, len(allUsers))
					for _, user := range allUsers {
						users = append(users, map[string]any{
							"user":      user,
							"shortuser": strings.TrimPrefix(user, "account_"),
							"databases": []string{},
						})
					}
					writeMySQLDatabaseResourceTestJSON(t, response, map[string]any{
						"status": 1,
						"data":   users,
					})
				case "/execute/Mysql/list_databases":
					users := slices.Clone(currentUsers)
					slices.Sort(users)
					writeMySQLDatabaseResourceTestJSON(t, response, map[string]any{
						"status": 1,
						"data": []map[string]any{{
							"database":   databaseName,
							"disk_usage": 0,
							"users":      users,
						}},
					})
				case "/execute/Mysql/get_privileges_on_database":
					writeMySQLDatabaseResourceTestJSON(t, response, map[string]any{
						"status": 1,
						"data":   []string{"ALL PRIVILEGES"},
					})
				case test.mutationPath:
					if request.Method != http.MethodPost {
						t.Errorf("method = %s, want POST", request.Method)
					}
					user := request.FormValue("user")
					mutationUsers = append(mutationUsers, user)
					if len(mutationUsers) == 2 {
						writeMySQLDatabaseResourceTestJSON(t, response, map[string]any{
							"status": 0,
							"errors": []string{"forced mutation failure"},
						})
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
					writeMySQLDatabaseResourceTestJSON(t, response, map[string]any{
						"status": 1,
						"data":   nil,
					})
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
			resource := &mySQLDatabaseResource{
				client: mysql.NewClient(baseClient),
			}
			response := runMySQLDatabaseUpdate(
				t,
				resource,
				MySQLDatabaseModel{
					Name:            types.StringValue(databaseName),
					Users:           mySQLDatabaseUsersForTest(t, test.stateUsers),
					DeleteOnDestroy: types.BoolValue(false),
				},
				MySQLDatabaseModel{
					Name:            types.StringValue(databaseName),
					Users:           mySQLDatabaseUsersForTest(t, test.planUsers),
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

func runMySQLDatabaseUpdate(
	t *testing.T,
	resource *mySQLDatabaseResource,
	stateModel MySQLDatabaseModel,
	planModel MySQLDatabaseModel,
) *frameworkresource.UpdateResponse {
	t.Helper()

	ctx := t.Context()
	schemaResponse := &frameworkresource.SchemaResponse{}
	NewMySQLDatabaseResource().Schema(
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

func mySQLDatabaseUsersForTest(t *testing.T, users []string) types.Set {
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

func writeMySQLDatabaseResourceTestJSON(
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
