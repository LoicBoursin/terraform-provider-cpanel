package apachehandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndFindsUserApacheHandlers(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Mime/list_handlers" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.Query().Get("type") != "user" {
			t.Errorf("query = %v", request.URL.Query())
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"extension": ".foo",
					"handler":   "example-handler",
					"origin":    "user",
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	handler, err := NewClient(baseClient).Get(t.Context(), ".foo")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if handler == nil {
		t.Fatal("Get() returned nil")
	}
	if handler.Handler != "example-handler" || handler.Origin != "user" {
		t.Fatalf("handler = %#v", handler)
	}
}

func TestClientRejectsAmbiguousApacheHandler(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":[{"extension":".foo"},{"extension":".foo"}]}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	_, err = NewClient(baseClient).Get(t.Context(), ".foo")
	if err == nil {
		t.Fatal("Get() returned no error")
	}
}

func TestClientAddsApacheHandlerWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Mime/add_handler" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("extension") != ".foo" ||
			request.Form.Get("handler") != "example-handler" {
			t.Errorf("form = %v", request.Form)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	if err := NewClient(baseClient).Add(
		t.Context(),
		Definition{
			Extension: ".foo",
			Handler:   "example-handler",
		},
	); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
}

func TestClientDeletesApacheHandlerWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Mime/delete_handler" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("extension") != ".foo" {
			t.Errorf("form = %v", request.Form)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	if err := NewClient(baseClient).Delete(t.Context(), ".foo"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}
