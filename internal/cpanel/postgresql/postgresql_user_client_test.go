package postgresql

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientPostgreSQLUserMutationsUsePOST(t *testing.T) {
	t.Parallel()

	const (
		createPassword = "create-password-&=?"
		renamePassword = "rename-password-&=?"
		updatePassword = "update-password-&=?"
	)

	tests := map[string]struct {
		operation  string
		parameters map[string]string
		secrets    []string
		call       func(context.Context, *Client) (*UserDataSourceModel, error)
	}{
		"create user": {
			operation: OperationCreateUser,
			parameters: map[string]string{
				"name":     "account_writer",
				"password": createPassword,
			},
			secrets: []string{createPassword},
			call: func(ctx context.Context, client *Client) (*UserDataSourceModel, error) {
				return client.CreateUser(ctx, UserCreateModel{
					Name:     "account_writer",
					Password: createPassword,
				})
			},
		},
		"delete user": {
			operation: OperationDeleteUser,
			parameters: map[string]string{
				"name": "account_writer",
			},
			call: func(ctx context.Context, client *Client) (*UserDataSourceModel, error) {
				return client.DeleteUser(ctx, UserDeleteModel{Name: "account_writer"})
			},
		},
		"grant all privileges": {
			operation: OperationGrantAllPrivileges,
			parameters: map[string]string{
				"database": "account_primary",
				"user":     "account_writer",
			},
			call: func(ctx context.Context, client *Client) (*UserDataSourceModel, error) {
				return client.GrantAllPrivileges(ctx, UserGrantAllPrivilegesModel{
					Database: "account_primary",
					User:     "account_writer",
				})
			},
		},
		"rename user": {
			operation: OperationRenameUser,
			parameters: map[string]string{
				"newname":  "account_editor",
				"oldname":  "account_writer",
				"password": renamePassword,
			},
			secrets: []string{renamePassword},
			call: func(ctx context.Context, client *Client) (*UserDataSourceModel, error) {
				return client.RenameUser(ctx, UserRenameModel{
					NewName:  "account_editor",
					OldName:  "account_writer",
					Password: renamePassword,
				})
			},
		},
		"revoke all privileges": {
			operation: OperationRevokeAllPrivileges,
			parameters: map[string]string{
				"database": "account_primary",
				"user":     "account_writer",
			},
			call: func(ctx context.Context, client *Client) (*UserDataSourceModel, error) {
				return client.RevokeAllPrivileges(ctx, UserRevokeAllPrivilegesModel{
					Database: "account_primary",
					User:     "account_writer",
				})
			},
		},
		"set password": {
			operation: OperationSetPassword,
			parameters: map[string]string{
				"password": updatePassword,
				"user":     "account_writer",
			},
			secrets: []string{updatePassword},
			call: func(ctx context.Context, client *Client) (*UserDataSourceModel, error) {
				return client.SetPassword(ctx, UserSetPasswordModel{
					Password: updatePassword,
					User:     "account_writer",
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
				secrets:    test.secrets,
			}, `{"status":1,"messages":["completed"],"data":["mutation-result"]}`)

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
			if !slices.Equal(response.Messages, []string{"completed"}) {
				t.Errorf("messages = %v, want [completed]", response.Messages)
			}
			if !slices.Equal(response.Data, []string{"mutation-result"}) {
				t.Errorf("data = %v, want [mutation-result]", response.Data)
			}
		})
	}
}

func TestClientRenamePostgreSQLUserReturnsAmbiguousMutationWithoutRecovery(
	t *testing.T,
) {
	t.Parallel()

	const password = "rename-password-&=?"
	var postRequests, getRequests int
	client := newPostgreSQLTestClient(t, "https://cpanel.test")
	client.HTTPClient.Transport = postgreSQLRoundTripFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		switch request.Method {
		case http.MethodPost:
			postRequests++
			assertPostgreSQLRequest(t, request, postgreSQLRequestExpectation{
				method: http.MethodPost,
				path:   "/execute/Postgresql/" + OperationRenameUser,
				parameters: postgreSQLValues(map[string]string{
					"newname":  "account_editor",
					"oldname":  "account_writer",
					"password": password,
				}),
				secrets: []string{password},
			})

			return nil, io.EOF
		case http.MethodGet:
			getRequests++
			t.Fatal("RenameUser() performed implicit recovery inventory")
		default:
			t.Fatalf("unexpected request method: %s", request.Method)
		}

		return nil, nil
	})

	response, err := client.RenameUser(context.Background(), UserRenameModel{
		NewName:  "account_editor",
		OldName:  "account_writer",
		Password: password,
	})
	if response != nil {
		t.Fatalf("RenameUser() response = %#v, want nil", response)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("RenameUser() error = %v, want wrapping io.EOF", err)
	}
	if postRequests != 1 {
		t.Fatalf("POST request count = %d, want 1", postRequests)
	}
	if getRequests != 0 {
		t.Fatalf("GET request count = %d, want 0", getRequests)
	}
}

func TestClientGetPostgreSQLUsersUsesGETAndDecodesInventory(t *testing.T) {
	t.Parallel()

	server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
		method:     http.MethodGet,
		path:       "/execute/Postgresql/" + OperationListUsers,
		parameters: postgreSQLValues(map[string]string{}),
	}, `{
		"status": 1,
		"metadata": {"transformed": 2},
		"warnings": ["inventory warning"],
		"data": ["account_writer", "account_reader"]
	}`)

	users, err := newPostgreSQLTestClient(t, server.URL).GetUsers(context.Background())
	if err != nil {
		t.Fatalf("GetUsers() error: %v", err)
	}
	if users == nil {
		t.Fatal("GetUsers() response is nil")
	}
	if users.Status != 1 {
		t.Errorf("status = %d, want 1", users.Status)
	}
	if users.Metadata.Transformed != 2 {
		t.Errorf("metadata.transformed = %d, want 2", users.Metadata.Transformed)
	}
	if !slices.Equal(users.Warnings, []string{"inventory warning"}) {
		t.Errorf("warnings = %v, want [inventory warning]", users.Warnings)
	}
	if !slices.Equal(users.Data, []string{"account_writer", "account_reader"}) {
		t.Errorf("users = %v", users.Data)
	}
}

func TestClientPostgreSQLUserExists(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		responseBody string
		want         bool
	}{
		"true for exact match": {
			responseBody: `{"status":1,"data":["account_writer","account_reader"]}`,
			want:         true,
		},
		"false for absent user": {
			responseBody: `{"status":1,"data":["account_writer_archive","account_reader"]}`,
			want:         false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
				method:     http.MethodGet,
				path:       "/execute/Postgresql/" + OperationListUsers,
				parameters: postgreSQLValues(map[string]string{}),
			}, test.responseBody)

			exists, err := newPostgreSQLTestClient(t, server.URL).UserExists(
				context.Background(),
				"account_writer",
			)
			if err != nil {
				t.Fatalf("UserExists() error: %v", err)
			}
			if exists != test.want {
				t.Errorf("UserExists() = %t, want %t", exists, test.want)
			}
		})
	}
}

func TestClientPostgreSQLUserOperationsPropagateAPIErrors(t *testing.T) {
	t.Parallel()

	const password = "api-error-password-&=?"

	tests := map[string]struct {
		operation  string
		method     string
		parameters map[string]string
		secrets    []string
		call       func(context.Context, *Client) (bool, error)
	}{
		"create user": {
			operation: OperationCreateUser,
			method:    http.MethodPost,
			parameters: map[string]string{
				"name":     "account_writer",
				"password": password,
			},
			secrets: []string{password},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.CreateUser(ctx, UserCreateModel{
					Name:     "account_writer",
					Password: password,
				})

				return response == nil, err
			},
		},
		"delete user": {
			operation: OperationDeleteUser,
			method:    http.MethodPost,
			parameters: map[string]string{
				"name": "account_writer",
			},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.DeleteUser(ctx, UserDeleteModel{Name: "account_writer"})

				return response == nil, err
			},
		},
		"get users": {
			operation:  OperationListUsers,
			method:     http.MethodGet,
			parameters: map[string]string{},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.GetUsers(ctx)

				return response == nil, err
			},
		},
		"grant all privileges": {
			operation: OperationGrantAllPrivileges,
			method:    http.MethodPost,
			parameters: map[string]string{
				"database": "account_primary",
				"user":     "account_writer",
			},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.GrantAllPrivileges(ctx, UserGrantAllPrivilegesModel{
					Database: "account_primary",
					User:     "account_writer",
				})

				return response == nil, err
			},
		},
		"rename user": {
			operation: OperationRenameUser,
			method:    http.MethodPost,
			parameters: map[string]string{
				"newname":  "account_editor",
				"oldname":  "account_writer",
				"password": password,
			},
			secrets: []string{password},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.RenameUser(ctx, UserRenameModel{
					NewName:  "account_editor",
					OldName:  "account_writer",
					Password: password,
				})

				return response == nil, err
			},
		},
		"revoke all privileges": {
			operation: OperationRevokeAllPrivileges,
			method:    http.MethodPost,
			parameters: map[string]string{
				"database": "account_primary",
				"user":     "account_writer",
			},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.RevokeAllPrivileges(ctx, UserRevokeAllPrivilegesModel{
					Database: "account_primary",
					User:     "account_writer",
				})

				return response == nil, err
			},
		},
		"set password": {
			operation: OperationSetPassword,
			method:    http.MethodPost,
			parameters: map[string]string{
				"password": password,
				"user":     "account_writer",
			},
			secrets: []string{password},
			call: func(ctx context.Context, client *Client) (bool, error) {
				response, err := client.SetPassword(ctx, UserSetPasswordModel{
					Password: password,
					User:     "account_writer",
				})

				return response == nil, err
			},
		},
		"user exists": {
			operation:  OperationListUsers,
			method:     http.MethodGet,
			parameters: map[string]string{},
			call: func(ctx context.Context, client *Client) (bool, error) {
				exists, err := client.UserExists(ctx, "account_writer")

				return !exists, err
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
				method:     test.method,
				path:       "/execute/Postgresql/" + test.operation,
				parameters: postgreSQLValues(test.parameters),
				secrets:    test.secrets,
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

func TestClientGetPostgreSQLUsersRejectsMalformedResponses(t *testing.T) {
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
			responseBody: `{"status":1,"data":{"user":"account_writer"}}`,
			wantError:    "decode UAPI response",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := newPostgreSQLTestServer(t, postgreSQLRequestExpectation{
				method:     http.MethodGet,
				path:       "/execute/Postgresql/" + OperationListUsers,
				parameters: postgreSQLValues(map[string]string{}),
			}, test.responseBody)

			users, err := newPostgreSQLTestClient(t, server.URL).GetUsers(context.Background())
			if users != nil {
				t.Errorf("GetUsers() response = %#v, want nil", users)
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("GetUsers() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}
