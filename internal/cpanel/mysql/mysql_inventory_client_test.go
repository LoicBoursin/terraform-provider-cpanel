package mysql

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestClientListsMySQLDatabaseNames(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertMySQLUAPIInventoryRequest(
			t,
			request,
			operationListDatabases,
		)
		_, _ = response.Write([]byte(`{
			"status": 1,
			"data": [
				{
					"database": "account_zeta",
					"disk_usage": 42,
					"users": ["account_writer"],
					"grants": ["ALL PRIVILEGES"],
					"server": "db.internal.example"
				},
				{
					"database": "account_alpha",
					"disk_usage": null,
					"users": null
				}
			]
		}`))
	}))
	defer server.Close()

	names, err := newMySQLTestClient(t, server.URL).
		ListDatabaseNames(t.Context())
	if err != nil {
		t.Fatalf("ListDatabaseNames() error: %v", err)
	}
	want := []string{"account_alpha", "account_zeta"}
	if !slices.Equal(names, want) {
		t.Fatalf("ListDatabaseNames() = %v, want %v", names, want)
	}
}

func TestClientListsMySQLUserNames(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertMySQLUAPIInventoryRequest(
			t,
			request,
			operationListUsers,
		)
		_, _ = response.Write([]byte(`{
			"status": 1,
			"data": [
				{
					"user": "account_writer",
					"shortuser": "writer",
					"databases": ["account_primary"],
					"password": "must-not-be-exposed"
				},
				{
					"user": "account_reader",
					"shortuser": null,
					"databases": null
				}
			]
		}`))
	}))
	defer server.Close()

	names, err := newMySQLTestClient(t, server.URL).
		ListUserNames(t.Context())
	if err != nil {
		t.Fatalf("ListUserNames() error: %v", err)
	}
	want := []string{"account_reader", "account_writer"}
	if !slices.Equal(names, want) {
		t.Fatalf("ListUserNames() = %v, want %v", names, want)
	}
}

func TestClientListsRemoteMySQLHostNames(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertMySQLAPI2InventoryRequest(t, request)
		_, _ = response.Write([]byte(`{
			"cpanelresult": {
				"apiversion": 2,
				"module": "MysqlFE",
				"func": "listhosts",
				"event": {"result": 1},
				"data": [
					{
						"host": "REMOTE.EXAMPLE.TEST",
						"uri_host": "REMOTE.EXAMPLE.TEST",
						"note": "application server"
					},
					{
						"host": "198.51.100.79/28",
						"uri_host": "198.51.100.79%2F28"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	hosts, err := newMySQLTestClient(t, server.URL).
		ListRemoteHostNames(t.Context())
	if err != nil {
		t.Fatalf("ListRemoteHostNames() error: %v", err)
	}
	want := []string{"198.51.100.64/28", "remote.example.test"}
	if !slices.Equal(hosts, want) {
		t.Fatalf("ListRemoteHostNames() = %v, want %v", hosts, want)
	}
}

func TestClientGetsStrictMySQLRestrictions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		prefixJSON string
		wantPrefix string
	}{
		"enabled prefix": {
			prefixJSON: `"account_"`,
			wantPrefix: "account_",
		},
		"disabled prefix as null": {
			prefixJSON: `null`,
		},
		"disabled prefix as empty string": {
			prefixJSON: `""`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertMySQLUAPIInventoryRequest(
					t,
					request,
					operationGetRestrictions,
				)
				_, _ = response.Write([]byte(`{
					"status": 1,
					"data": {
						"prefix": ` + test.prefixJSON + `,
						"max_database_name_length": 64,
						"max_username_length": 47,
						"server_version": "not exposed",
						"server_host": "not exposed"
					}
				}`))
			}))
			defer server.Close()

			restrictions, err := newMySQLTestClient(t, server.URL).
				GetRestrictions(t.Context())
			if err != nil {
				t.Fatalf("GetRestrictions() error: %v", err)
			}
			want := &Restrictions{
				MaxUsernameLength:     47,
				MaxDatabaseNameLength: 64,
				Prefix:                test.wantPrefix,
			}
			if !reflect.DeepEqual(restrictions, want) {
				t.Fatalf(
					"GetRestrictions() = %#v, want %#v",
					restrictions,
					want,
				)
			}
		})
	}
}

func TestClientAcceptsEmptyMySQLNameInventories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response string
		call     func(*Client) ([]string, error)
	}{
		"databases": {
			response: `{"status":1,"data":[]}`,
			call: func(client *Client) ([]string, error) {
				return client.ListDatabaseNames(t.Context())
			},
		},
		"users": {
			response: `{"status":1,"data":[]}`,
			call: func(client *Client) ([]string, error) {
				return client.ListUserNames(t.Context())
			},
		},
		"remote hosts": {
			response: `{"cpanelresult":{"event":{"result":1},"data":[]}}`,
			call: func(client *Client) ([]string, error) {
				return client.ListRemoteHostNames(t.Context())
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newMySQLInventoryResponseClient(t, test.response)
			values, err := test.call(client)
			if err != nil {
				t.Fatalf("inventory call error: %v", err)
			}
			if values == nil || len(values) != 0 {
				t.Fatalf(
					"inventory = %#v, want non-nil empty slice",
					values,
				)
			}
		})
	}
}

func TestClientRejectsMalformedMySQLDatabaseNameInventories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response string
		wantErr  string
	}{
		"missing data": {
			response: `{"status":1}`,
			wantErr:  "non-null array",
		},
		"null data": {
			response: `{"status":1,"data":null}`,
			wantErr:  "non-null array",
		},
		"unexpected data object": {
			response: `{"status":1,"data":{}}`,
			wantErr:  "as an array",
		},
		"null item": {
			response: `{"status":1,"data":[null]}`,
			wantErr:  "non-null object",
		},
		"missing database": {
			response: `{"status":1,"data":[{"disk_usage":1}]}`,
			wantErr:  `missing required field "database"`,
		},
		"null database": {
			response: `{"status":1,"data":[{"database":null}]}`,
			wantErr:  `field "database" must be a non-null string`,
		},
		"unexpected database type": {
			response: `{"status":1,"data":[{"database":42}]}`,
			wantErr:  `field "database" must be a string`,
		},
		"duplicate database": {
			response: `{"status":1,"data":[{"database":"account_db"},{"database":"account_db"}]}`,
			wantErr:  "duplicate MySQL database name",
		},
		"invalid database": {
			response: `{"status":1,"data":[{"database":"account-db"}]}`,
			wantErr:  "ASCII letters, numbers, and underscores",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newMySQLInventoryResponseClient(t, test.response)
			_, err := client.ListDatabaseNames(t.Context())
			assertMySQLInventoryError(t, err, test.wantErr)
		})
	}
}

func TestClientRejectsMalformedMySQLUserNameInventories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response string
		wantErr  string
	}{
		"missing data": {
			response: `{"status":1}`,
			wantErr:  "non-null array",
		},
		"null data": {
			response: `{"status":1,"data":null}`,
			wantErr:  "non-null array",
		},
		"unexpected data string": {
			response: `{"status":1,"data":"account_user"}`,
			wantErr:  "as an array",
		},
		"unexpected item array": {
			response: `{"status":1,"data":[[]]}`,
			wantErr:  "as an object",
		},
		"missing user": {
			response: `{"status":1,"data":[{"shortuser":"user"}]}`,
			wantErr:  `missing required field "user"`,
		},
		"null user": {
			response: `{"status":1,"data":[{"user":null}]}`,
			wantErr:  `field "user" must be a non-null string`,
		},
		"duplicate user": {
			response: `{"status":1,"data":[{"user":"account_user"},{"user":"account_user"}]}`,
			wantErr:  "duplicate MySQL user name",
		},
		"whitespace user": {
			response: `{"status":1,"data":[{"user":" account_user"}]}`,
			wantErr:  "non-empty trimmed string",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newMySQLInventoryResponseClient(t, test.response)
			_, err := client.ListUserNames(t.Context())
			assertMySQLInventoryError(t, err, test.wantErr)
		})
	}
}

func TestClientRejectsMalformedRemoteMySQLHostNameInventories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response string
		wantErr  string
	}{
		"missing data": {
			response: `{"cpanelresult":{"event":{"result":1}}}`,
			wantErr:  "non-null array",
		},
		"null data": {
			response: `{"cpanelresult":{"event":{"result":1},"data":null}}`,
			wantErr:  "non-null array",
		},
		"unexpected data object": {
			response: `{"cpanelresult":{"event":{"result":1},"data":{}}}`,
			wantErr:  "decode API 2 response envelope",
		},
		"null item": {
			response: `{"cpanelresult":{"event":{"result":1},"data":[null]}}`,
			wantErr:  "non-null object",
		},
		"missing host": {
			response: `{"cpanelresult":{"event":{"result":1},"data":[{"uri_host":"remote.example.test"}]}}`,
			wantErr:  `missing required field "host"`,
		},
		"null host": {
			response: `{"cpanelresult":{"event":{"result":1},"data":[{"host":null}]}}`,
			wantErr:  `field "host" must be a non-null string`,
		},
		"invalid host": {
			response: `{"cpanelresult":{"event":{"result":1},"data":[{"host":"2001:db8::1"}]}}`,
			wantErr:  "must use IPv4",
		},
		"duplicate normalized host": {
			response: `{"cpanelresult":{"event":{"result":1},"data":[{"host":"REMOTE.EXAMPLE.TEST"},{"host":"remote.example.test"}]}}`,
			wantErr:  "duplicate remote MySQL host",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newMySQLInventoryResponseClient(t, test.response)
			_, err := client.ListRemoteHostNames(t.Context())
			assertMySQLInventoryError(t, err, test.wantErr)
		})
	}
}

func TestClientRejectsMalformedMySQLRestrictions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response string
		wantErr  string
	}{
		"missing data": {
			response: `{"status":1}`,
			wantErr:  "non-null object",
		},
		"null data": {
			response: `{"status":1,"data":null}`,
			wantErr:  "non-null object",
		},
		"unexpected data array": {
			response: `{"status":1,"data":[]}`,
			wantErr:  "as an object",
		},
		"missing prefix": {
			response: `{"status":1,"data":{"max_database_name_length":64,"max_username_length":47}}`,
			wantErr:  `missing required field "prefix"`,
		},
		"unexpected prefix type": {
			response: `{"status":1,"data":{"prefix":42,"max_database_name_length":64,"max_username_length":47}}`,
			wantErr:  `field "prefix" must be a string`,
		},
		"invalid prefix characters": {
			response: `{"status":1,"data":{"prefix":"account-","max_database_name_length":64,"max_username_length":47}}`,
			wantErr:  "ASCII letters, numbers, and underscores",
		},
		"prefix without underscore": {
			response: `{"status":1,"data":{"prefix":"account","max_database_name_length":64,"max_username_length":47}}`,
			wantErr:  "must end with an underscore",
		},
		"missing database limit": {
			response: `{"status":1,"data":{"prefix":"account_","max_username_length":47}}`,
			wantErr:  `missing required field "max_database_name_length"`,
		},
		"null database limit": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":null,"max_username_length":47}}`,
			wantErr:  "must be a positive integer",
		},
		"string database limit": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":"64","max_username_length":47}}`,
			wantErr:  "must be a positive integer",
		},
		"fractional username limit": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":64,"max_username_length":47.5}}`,
			wantErr:  "must be a positive integer",
		},
		"zero username limit": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":64,"max_username_length":0}}`,
			wantErr:  "must be greater than zero",
		},
		"negative database limit": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":-1,"max_username_length":47}}`,
			wantErr:  "must be greater than zero",
		},
		"database limit shorter than prefix": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":8,"max_username_length":47}}`,
			wantErr:  "must allow at least one character",
		},
		"username limit shorter than prefix": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":64,"max_username_length":8}}`,
			wantErr:  "must allow at least one character",
		},
		"unexpected top-level data value": {
			response: `{"status":1,"data":{"prefix":"account_","max_database_name_length":64,"max_username_length":{}}}`,
			wantErr:  "must be a positive integer",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newMySQLInventoryResponseClient(t, test.response)
			_, err := client.GetRestrictions(t.Context())
			assertMySQLInventoryError(t, err, test.wantErr)
		})
	}
}

func TestClientRejectsMySQLInventoryWarnings(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response string
		call     func(*Client) error
	}{
		"databases": {
			response: `{"status":1,"warnings":["database inventory is incomplete"],"data":[]}`,
			call: func(client *Client) error {
				_, err := client.ListDatabaseNames(t.Context())

				return err
			},
		},
		"users": {
			response: `{"status":1,"warnings":["user inventory is incomplete"],"data":[]}`,
			call: func(client *Client) error {
				_, err := client.ListUserNames(t.Context())

				return err
			},
		},
		"restrictions": {
			response: `{"status":1,"warnings":["restriction data may be stale"],"data":{"prefix":"account_","max_database_name_length":64,"max_username_length":47}}`,
			call: func(client *Client) error {
				_, err := client.GetRestrictions(t.Context())

				return err
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newMySQLInventoryResponseClient(t, test.response)
			assertMySQLInventoryError(
				t,
				test.call(client),
				"returned warnings and cannot be treated as complete",
			)
		})
	}
}

func TestNormalizeMySQLInventoryName(t *testing.T) {
	t.Parallel()

	valid := []string{
		"account_database",
		"Account_123",
		strings.Repeat("a", maxMySQLInventoryNameLength),
	}
	for _, value := range valid {
		normalized, err := normalizeMySQLInventoryName(value, "database")
		if err != nil {
			t.Fatalf(
				"normalizeMySQLInventoryName(%q) error: %v",
				value,
				err,
			)
		}
		if normalized != value {
			t.Fatalf(
				"normalizeMySQLInventoryName(%q) = %q",
				value,
				normalized,
			)
		}
	}

	invalid := []string{
		"",
		" account_database",
		"account database",
		"account-database",
		"account_database\n",
		"café",
		strings.Repeat("a", maxMySQLInventoryNameLength+1),
	}
	for _, value := range invalid {
		if _, err := normalizeMySQLInventoryName(
			value,
			"database",
		); err == nil {
			t.Fatalf(
				"normalizeMySQLInventoryName(%q) returned no error",
				value,
			)
		}
	}
}

func assertMySQLUAPIInventoryRequest(
	t *testing.T,
	request *http.Request,
	operation string,
) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	wantPath := "/execute/Mysql/" + operation
	if request.URL.Path != wantPath {
		t.Errorf("path = %q, want %q", request.URL.Path, wantPath)
	}
	if request.URL.RawQuery != "" {
		t.Errorf("query = %q, want empty", request.URL.RawQuery)
	}
}

func assertMySQLAPI2InventoryRequest(
	t *testing.T,
	request *http.Request,
) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	if request.URL.Path != "/json-api/cpanel" {
		t.Errorf("path = %q, want /json-api/cpanel", request.URL.Path)
	}
	query := request.URL.Query()
	want := map[string]string{
		"cpanel_jsonapi_apiversion": "2",
		"cpanel_jsonapi_user":       "username",
		"cpanel_jsonapi_module":     "MysqlFE",
		"cpanel_jsonapi_func":       "listhosts",
	}
	if len(query) != len(want) {
		t.Errorf("query = %v, want exactly %v", query, want)
	}
	for key, value := range want {
		if query.Get(key) != value {
			t.Errorf("%s = %q, want %q", key, query.Get(key), value)
		}
	}
}

func newMySQLInventoryResponseClient(
	t *testing.T,
	responseBody string,
) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)

	return newMySQLTestClient(t, server.URL)
}

func assertMySQLInventoryError(
	t *testing.T,
	err error,
	want string,
) {
	t.Helper()

	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}
