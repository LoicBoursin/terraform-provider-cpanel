package postgresql

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientPostgreSQLDatabaseMutationsUsePOST(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		operation  string
		parameters map[string]string
		call       func(context.Context, *Client) (*DatabaseDataSourceModel, error)
	}{
		"create database": {
			operation: OperationCreateDatabase,
			parameters: map[string]string{
				"name": "account_primary",
			},
			call: func(ctx context.Context, client *Client) (*DatabaseDataSourceModel, error) {
				return client.CreateDatabase(ctx, DatabaseCreateModel{Name: "account_primary"})
			},
		},
		"delete database": {
			operation: OperationDeleteDatabase,
			parameters: map[string]string{
				"name": "account_primary",
			},
			call: func(ctx context.Context, client *Client) (*DatabaseDataSourceModel, error) {
				return client.DeleteDatabase(ctx, DatabaseDeleteModel{Name: "account_primary"})
			},
		},
		"rename database": {
			operation: OperationRenameDatabase,
			parameters: map[string]string{
				"newname": "account_archive",
				"oldname": "account_primary",
			},
			call: func(ctx context.Context, client *Client) (*DatabaseDataSourceModel, error) {
				return client.UpdateDatabase(ctx, DatabaseUpdateModel{
					NewName: "account_archive",
					OldName: "account_primary",
				})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
				method:     http.MethodPost,
				path:       "/execute/Postgresql/" + test.operation,
				parameters: postgreSQLValues(test.parameters),
			}, `{"status":1,"messages":["completed"],"data":[]}`)

			response, err := test.call(
				context.Background(),
				newPostgreSQLTestClient(t, server.URL),
			)
			if err != nil {
				t.Fatalf("operation error: %v", err)
			}
			if response == nil {
				t.Fatal("operation response is nil")
			}
			if response.Status != 1 {
				t.Errorf("status = %d, want 1", response.Status)
			}
			if !reflect.DeepEqual(response.Messages, []string{"completed"}) {
				t.Errorf("messages = %v, want [completed]", response.Messages)
			}
			if len(response.Data) != 0 {
				t.Errorf("data = %#v, want empty", response.Data)
			}
		})
	}
}

func TestClientGetPostgreSQLDatabasesUsesGETAndDecodesInventory(t *testing.T) {
	t.Parallel()

	server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
		method:     http.MethodGet,
		path:       "/execute/Postgresql/" + OperationListDatabases,
		parameters: postgreSQLValues(map[string]string{}),
	}, `{
		"status": 1,
		"metadata": {"transformed": 2},
		"data": [
			{
				"database": "account_primary",
				"disk_usage": 42,
				"users": ["account_writer", "account_reader"]
			},
			{
				"database": "account_empty",
				"disk_usage": 0,
				"users": []
			}
		]
	}`)

	databases, err := newPostgreSQLTestClient(t, server.URL).GetDatabases(context.Background())
	if err != nil {
		t.Fatalf("GetDatabases() error: %v", err)
	}
	if databases == nil {
		t.Fatal("GetDatabases() response is nil")
	}
	if databases.Status != 1 {
		t.Errorf("status = %d, want 1", databases.Status)
	}
	if databases.Metadata.Transformed != 2 {
		t.Errorf("metadata.transformed = %d, want 2", databases.Metadata.Transformed)
	}

	want := []DatabaseDataSourceDataModel{
		{
			Database:  "account_primary",
			DiskUsage: 42,
			Users:     []string{"account_writer", "account_reader"},
		},
		{
			Database:  "account_empty",
			DiskUsage: 0,
			Users:     []string{},
		},
	}
	if !reflect.DeepEqual(databases.Data, want) {
		t.Errorf("databases = %#v, want %#v", databases.Data, want)
	}
}

func TestClientPostgreSQLDatabaseOperationsPropagateAPIErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		operation  string
		method     string
		parameters map[string]string
		call       func(context.Context, *Client) (bool, error)
	}{
		"create database": {
			operation: OperationCreateDatabase,
			method:    http.MethodPost,
			parameters: map[string]string{
				"name": "account_primary",
			},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.CreateDatabase(
					ctx,
					DatabaseCreateModel{Name: "account_primary"},
				)

				return response == nil, err
			},
		},
		"delete database": {
			operation: OperationDeleteDatabase,
			method:    http.MethodPost,
			parameters: map[string]string{
				"name": "account_primary",
			},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.DeleteDatabase(
					ctx,
					DatabaseDeleteModel{Name: "account_primary"},
				)

				return response == nil, err
			},
		},
		"get databases": {
			operation:  OperationListDatabases,
			method:     http.MethodGet,
			parameters: map[string]string{},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.GetDatabases(ctx)

				return response == nil, err
			},
		},
		"rename database": {
			operation: OperationRenameDatabase,
			method:    http.MethodPost,
			parameters: map[string]string{
				"newname": "account_archive",
				"oldname": "account_primary",
			},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.UpdateDatabase(ctx, DatabaseUpdateModel{
					NewName: "account_archive",
					OldName: "account_primary",
				})

				return response == nil, err
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
				method:     test.method,
				path:       "/execute/Postgresql/" + test.operation,
				parameters: postgreSQLValues(test.parameters),
			}, `{
				"status": 0,
				"errors": ["permission denied"],
				"messages": ["operation rejected"]
			}`)

			zeroResult, err := test.call(
				context.Background(),
				newPostgreSQLTestClient(t, server.URL),
			)
			if !zeroResult {
				t.Error("operation returned a non-zero result after an API error")
			}

			var apiError *cpanel.APIError
			if !errors.As(err, &apiError) {
				t.Fatalf("error = %T %v, want *cpanel.APIError", err, err)
			}
			if apiError.API != "UAPI" {
				t.Errorf("API = %q, want UAPI", apiError.API)
			}
			if apiError.Module != cpanel.ModulePostgresql {
				t.Errorf("module = %q, want %q", apiError.Module, cpanel.ModulePostgresql)
			}
			if apiError.Function != test.operation {
				t.Errorf("function = %q, want %q", apiError.Function, test.operation)
			}
			if !strings.Contains(err.Error(), "permission denied; operation rejected") {
				t.Errorf("error = %q", err)
			}
		})
	}
}

func TestClientGetPostgreSQLDatabasesRejectsMalformedResponses(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		responseBody string
		wantError    string
	}{
		"invalid envelope JSON": {
			responseBody: `{"status":`,
			wantError:    "decode UAPI response envelope",
		},
		"invalid inventory shape": {
			responseBody: `{"status":1,"data":{"database":"account_primary"}}`,
			wantError:    "decode UAPI response",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
				method:     http.MethodGet,
				path:       "/execute/Postgresql/" + OperationListDatabases,
				parameters: postgreSQLValues(map[string]string{}),
			}, test.responseBody)

			databases, err := newPostgreSQLTestClient(t, server.URL).GetDatabases(
				context.Background(),
			)
			if databases != nil {
				t.Errorf("GetDatabases() response = %#v, want nil", databases)
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("GetDatabases() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}
