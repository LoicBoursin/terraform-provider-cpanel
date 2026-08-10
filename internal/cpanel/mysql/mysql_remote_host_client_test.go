package mysql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientReadsRemoteMySQLHostsAndNotes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/json-api/cpanel":
			query := request.URL.Query()
			if query.Get("cpanel_jsonapi_module") != "MysqlFE" ||
				query.Get("cpanel_jsonapi_func") != "listhosts" {
				t.Errorf("query = %v", query)
			}
			_, _ = response.Write([]byte(
				`{"cpanelresult":{"event":{"result":1},"data":[{"host":"REMOTE.EXAMPLE.TEST","uri_host":"REMOTE.EXAMPLE.TEST"},{"host":"198.51.100.77","uri_host":"198.51.100.77"}]}}`,
			))
		case "/execute/Mysql/get_host_notes":
			_, _ = response.Write([]byte(
				`{"status":1,"data":{"REMOTE.EXAMPLE.TEST":"application server"}}`,
			))
		default:
			t.Errorf("unexpected path: %s", request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	remoteHosts, err := NewClient(baseClient).ListRemoteHosts(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("ListRemoteHosts() error: %v", err)
	}
	want := []RemoteHost{
		{Host: "198.51.100.77"},
		{Host: "remote.example.test", Note: "application server"},
	}
	if !reflect.DeepEqual(remoteHosts, want) {
		t.Fatalf("ListRemoteHosts() = %#v, want %#v", remoteHosts, want)
	}
}

func TestClientMutatesRemoteMySQLHostsWithPOST(t *testing.T) {
	t.Parallel()

	var functions []string
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("host"); got != "198.51.100.77" {
			t.Errorf("host = %q", got)
		}
		if request.URL.Path == "/execute/Mysql/add_host_note" &&
			request.Form.Get("note") != "application server" {
			t.Errorf("note = %q", request.Form.Get("note"))
		}
		functions = append(
			functions,
			request.URL.Path,
		)

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	client := NewClient(baseClient)

	if err := client.AddRemoteHost(
		context.Background(),
		"198.51.100.77",
	); err != nil {
		t.Fatalf("AddRemoteHost() error: %v", err)
	}
	if err := client.SetRemoteHostNote(
		context.Background(),
		"198.51.100.77",
		"application server",
	); err != nil {
		t.Fatalf("SetRemoteHostNote() error: %v", err)
	}
	if err := client.DeleteRemoteHost(
		context.Background(),
		"198.51.100.77",
	); err != nil {
		t.Fatalf("DeleteRemoteHost() error: %v", err)
	}

	want := []string{
		"/execute/Mysql/add_host",
		"/execute/Mysql/add_host_note",
		"/execute/Mysql/delete_host",
	}
	if !reflect.DeepEqual(functions, want) {
		t.Fatalf("functions = %v, want %v", functions, want)
	}
}

func TestNormalizeRemoteMySQLHost(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		host      string
		want      string
		wantError bool
	}{
		"IPv4": {
			host: "198.51.100.77",
			want: "198.51.100.77",
		},
		"CIDR": {
			host: "198.51.100.79/28",
			want: "198.51.100.64/28",
		},
		"wildcard": {
			host: "198.051.%.%",
			want: "198.51.%.%",
		},
		"hostname": {
			host: "REMOTE.Example.Test",
			want: "remote.example.test",
		},
		"IPv6": {
			host:      "2001:db8::1",
			wantError: true,
		},
		"invalid IPv4": {
			host:      "198.51.100.999",
			wantError: true,
		},
		"invalid wildcard": {
			host:      "198.51.%",
			wantError: true,
		},
		"invalid hostname": {
			host:      "-remote.example.test",
			wantError: true,
		},
		"whitespace": {
			host:      " remote.example.test",
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeRemoteHost(test.host)
			if test.wantError && err == nil {
				t.Fatal("NormalizeRemoteHost() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("NormalizeRemoteHost() error: %v", err)
			}
			if got != test.want {
				t.Fatalf("NormalizeRemoteHost() = %q, want %q", got, test.want)
			}
		})
	}
}
