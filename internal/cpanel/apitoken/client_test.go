package apitoken

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndFindsAPITokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Tokens/list" {
			t.Errorf("path = %s", request.URL.Path)
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"name":            "terraform",
					"has_full_access": 1,
					"expires_at":      "1784131088",
					"create_time":     1784044689,
					"features":        []string{},
					"whitelist_ips":   nil,
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	token, err := NewClient(baseClient).Get(context.Background(), "terraform")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if token == nil {
		t.Fatal("Get() returned nil")
	}
	if token.ExpiresAt.ValueOrZero() != 1784131088 ||
		token.CreateTime != 1784044689 ||
		token.HasFullAccess != 1 {
		t.Fatalf("token = %#v", token)
	}
}

func TestClientCreatesAPITokenWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Tokens/create_full_access" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("name") != "terraform" ||
			request.Form.Get("expires_at") != "1784131088" {
			t.Errorf("form = %v", request.Form)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":{"token":"secret","create_time":1784044689,"whitelist_ips":null}}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	created, err := NewClient(baseClient).Create(
		context.Background(),
		"terraform",
		1784131088,
	)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if created.Token != "secret" || created.CreateTime != 1784044689 {
		t.Fatalf("created = %#v", created)
	}
}

func TestClientRejectsEmptyCreatedAPIToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":{"token":"","create_time":1784044689}}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	created, err := NewClient(baseClient).Create(
		context.Background(),
		"terraform",
		0,
	)
	if err == nil {
		t.Fatal("Create() returned no error")
	}
	if created == nil || created.CreateTime != 1784044689 {
		t.Fatalf("Create() result = %#v, want creation metadata", created)
	}
}

func TestClientRenamesAndRevokesAPITokensWithPOST(t *testing.T) {
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
		if request.Form.Get("name") == "" {
			t.Error("name is empty")
		}

		_, _ = response.Write([]byte(`{"status":1,"data":1}`))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	client := NewClient(baseClient)

	if err := client.Rename(
		context.Background(),
		"terraform",
		"terraform-renamed",
	); err != nil {
		t.Fatalf("Rename() error: %v", err)
	}
	if err := client.Revoke(
		context.Background(),
		"terraform-renamed",
	); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	if len(paths) != 2 ||
		paths[0] != "/execute/Tokens/rename" ||
		paths[1] != "/execute/Tokens/revoke" {
		t.Fatalf("paths = %v", paths)
	}
}
