package mysql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientReadsMySQLInventory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}

		switch request.URL.Path {
		case "/execute/Mysql/get_restrictions":
			_, _ = response.Write([]byte(
				`{"status":1,"data":{"max_username_length":47,"max_database_name_length":64,"prefix":"account_"}}`,
			))
		case "/execute/Mysql/list_databases":
			_, _ = response.Write([]byte(
				`{"status":1,"data":[{"database":"account_database","users":["account_user"],"disk_usage":42}]}`,
			))
		case "/execute/Mysql/list_users":
			_, _ = response.Write([]byte(
				`{"status":1,"data":[{"user":"account_user","shortuser":"user","databases":["account_database"]}]}`,
			))
		default:
			t.Errorf("unexpected path: %s", request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newMySQLTestClient(t, server.URL)
	restrictions, err := client.GetRestrictions(context.Background())
	if err != nil {
		t.Fatalf("GetRestrictions() error: %v", err)
	}
	if restrictions.Prefix != "account_" || restrictions.MaxUsernameLength != 47 {
		t.Fatalf("restrictions = %#v", restrictions)
	}

	databases, err := client.ListDatabases(context.Background())
	if err != nil {
		t.Fatalf("ListDatabases() error: %v", err)
	}
	if len(databases.Data) != 1 || databases.Data[0].Database != "account_database" {
		t.Fatalf("databases = %#v", databases.Data)
	}

	users, err := client.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers() error: %v", err)
	}
	if len(users.Data) != 1 || users.Data[0].User != "account_user" {
		t.Fatalf("users = %#v", users.Data)
	}
}

func TestClientMySQLMutationsUsePOST(t *testing.T) {
	t.Parallel()

	const password = "mysql-password-&=?"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Mysql/create_user" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if strings.Contains(request.URL.String(), password) || request.URL.RawQuery != "" {
			t.Errorf("request URL contains password: %s", request.URL.Redacted())
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("name"); got != "account_user" {
			t.Errorf("name = %q", got)
		}
		if got := request.Form.Get("password"); got != password {
			t.Errorf("password = %q", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newMySQLTestClient(t, server.URL)
	if err := client.CreateUser(context.Background(), "account_user", password); err != nil {
		t.Fatalf("CreateUser() error: %v", err)
	}
}

func TestClientReadsMySQLPrivileges(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Mysql/get_privileges_on_database" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("user"); got != "account_user" {
			t.Errorf("user = %q", got)
		}
		if got := request.URL.Query().Get("database"); got != "account_database" {
			t.Errorf("database = %q", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":["ALL PRIVILEGES"]}`))
	}))
	defer server.Close()

	client := newMySQLTestClient(t, server.URL)
	privileges, err := client.GetPrivileges(
		context.Background(),
		"account_user",
		"account_database",
	)
	if err != nil {
		t.Fatalf("GetPrivileges() error: %v", err)
	}
	if len(privileges) != 1 || privileges[0] != "ALL PRIVILEGES" {
		t.Fatalf("privileges = %v", privileges)
	}
}

func TestClientReturnsMySQLPasswordFailures(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":{"failures":[{"host":"db.example.test","error":"password rejected"}]}}`,
		))
	}))
	defer server.Close()

	client := newMySQLTestClient(t, server.URL)
	err := client.SetPassword(context.Background(), "account_user", "new-password")
	if err == nil || !strings.Contains(err.Error(), "db.example.test: password rejected") {
		t.Fatalf("SetPassword() error = %v", err)
	}
}

func newMySQLTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
