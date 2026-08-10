package mimetype

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndFindsUserMIMETypes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Mime/list_mime" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.Query().Get("type") != "user" {
			t.Errorf("query = %v", request.URL.Query())
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"extension": ".foo .bar",
					"origin":    "user",
					"type":      "application/x-example",
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	mimeType, err := NewClient(baseClient).Get(
		t.Context(),
		"application/x-example",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if mimeType == nil {
		t.Fatal("Get() returned nil")
	}
	if mimeType.Origin != "user" {
		t.Fatalf("origin = %q, want user", mimeType.Origin)
	}
	extensions := mimeType.Extensions()
	if len(extensions) != 2 ||
		extensions[0] != ".bar" ||
		extensions[1] != ".foo" {
		t.Fatalf("extensions = %v", extensions)
	}
}

func TestClientRejectsAmbiguousMIMEType(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":[{"type":"application/x-example"},{"type":"application/x-example"}]}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	_, err = NewClient(baseClient).Get(
		t.Context(),
		"application/x-example",
	)
	if err == nil {
		t.Fatal("Get() returned no error")
	}
}

func TestClientAddsMIMEExtensionWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Mime/add_mime" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("type") != "application/x-example" ||
			request.Form.Get("extension") != ".foo" {
			t.Errorf("form = %v", request.Form)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	if err := NewClient(baseClient).AddExtension(
		t.Context(),
		"application/x-example",
		".foo",
	); err != nil {
		t.Fatalf("AddExtension() error: %v", err)
	}
}

func TestClientDeletesMIMETypeWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Mime/delete_mime" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("type") != "application/x-example" {
			t.Errorf("form = %v", request.Form)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	if err := NewClient(baseClient).Delete(
		t.Context(),
		"application/x-example",
	); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}
