package ddns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndFindsDynamicDNSDomains(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/DynamicDNS/list" {
			t.Errorf("path = %s", request.URL.Path)
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"created_time":     1784046318,
					"description":      "Home network",
					"domain":           "home.example.com",
					"id":               "dynamic-dns-id",
					"ipv4":             []string{"192.0.2.10"},
					"ipv6":             []string{"2001:db8::10"},
					"last_run_times":   []int64{1784046500},
					"last_update_time": 1784046400,
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	domain, err := NewClient(baseClient).Get(t.Context(), "home.example.com")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if domain == nil {
		t.Fatal("Get() returned nil")
	}
	if domain.ID != "dynamic-dns-id" ||
		domain.Description != "Home network" ||
		domain.LastUpdateTime == nil ||
		*domain.LastUpdateTime != 1784046400 {
		t.Fatalf("domain = %#v", domain)
	}
}

func TestClientCreatesDynamicDNSDomainWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/DynamicDNS/create" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("domain") != "home.example.com" ||
			request.Form.Get("description") != "Home network" {
			t.Errorf("form = %v", request.Form)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":{"created_time":1784046318,"id":"dynamic-dns-id"}}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	created, err := NewClient(baseClient).Create(
		t.Context(),
		"home.example.com",
		"Home network",
	)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if created.ID != "dynamic-dns-id" ||
		created.CreatedTime != 1784046318 {
		t.Fatalf("created = %#v", created)
	}
}

func TestClientRejectsEmptyCreatedDynamicDNSID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":{"created_time":1784046318,"id":""}}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	_, err = NewClient(baseClient).Create(
		t.Context(),
		"home.example.com",
		"",
	)
	if err == nil {
		t.Fatal("Create() returned no error")
	}
}

func TestClientMutatesDynamicDNSDomainWithPOST(t *testing.T) {
	t.Parallel()

	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		paths = append(paths, request.URL.Path)
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("id") != "dynamic-dns-id" {
			t.Errorf("id = %q", request.Form.Get("id"))
		}

		switch request.URL.Path {
		case "/execute/DynamicDNS/set_description":
			if request.Form.Get("description") != "Updated" {
				t.Errorf("description = %q", request.Form.Get("description"))
			}
			_, _ = response.Write([]byte(`{"status":1,"data":null}`))
		case "/execute/DynamicDNS/recreate":
			_, _ = response.Write([]byte(
				`{"status":1,"data":{"id":"recreated-dynamic-dns-id"}}`,
			))
		case "/execute/DynamicDNS/delete":
			_, _ = response.Write([]byte(
				`{"status":1,"data":{"deleted":1}}`,
			))
		default:
			t.Errorf("unexpected path = %s", request.URL.Path)
		}
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	client := NewClient(baseClient)

	if err := client.SetDescription(
		t.Context(),
		"dynamic-dns-id",
		"Updated",
	); err != nil {
		t.Fatalf("SetDescription() error: %v", err)
	}
	recreatedID, err := client.Recreate(t.Context(), "dynamic-dns-id")
	if err != nil {
		t.Fatalf("Recreate() error: %v", err)
	}
	if recreatedID != "recreated-dynamic-dns-id" {
		t.Fatalf("recreated ID = %q", recreatedID)
	}
	deleted, err := client.Delete(t.Context(), "dynamic-dns-id")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if !deleted {
		t.Fatal("Delete() returned false")
	}

	expectedPaths := []string{
		"/execute/DynamicDNS/set_description",
		"/execute/DynamicDNS/recreate",
		"/execute/DynamicDNS/delete",
	}
	if len(paths) != len(expectedPaths) {
		t.Fatalf("paths = %v", paths)
	}
	for index, expectedPath := range expectedPaths {
		if paths[index] != expectedPath {
			t.Fatalf("paths = %v", paths)
		}
	}
}
