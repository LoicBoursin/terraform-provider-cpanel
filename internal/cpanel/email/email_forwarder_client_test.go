package email

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreatesForwarderWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/add_forwarder" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("request URL query = %q, want empty", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}

		expected := map[string]string{
			"domain":   "example.test",
			"email":    "source@example.test",
			"fwdopt":   "fwd",
			"fwdemail": "destination@example.net",
		}
		for key, value := range expected {
			if got := request.Form.Get(key); got != value {
				t.Errorf("%s = %q, want %q", key, got, value)
			}
		}

		_, _ = response.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.CreateForwarder(
		context.Background(),
		"source@example.test",
		"example.test",
		"destination@example.net",
	); err != nil {
		t.Fatalf("CreateForwarder() error: %v", err)
	}
}

func TestClientFindsExactForwarder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Email/list_forwarders" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("domain"); got != "example.test" {
			t.Errorf("domain = %q, want example.test", got)
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]string{
				{
					"dest":    "source@example.test",
					"forward": "first@example.net",
				},
				{
					"dest":    "source@example.test",
					"forward": "second@example.net",
				},
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	forwarder, err := client.GetForwarder(
		context.Background(),
		"example.test",
		"source@example.test",
		"second@example.net",
	)
	if err != nil {
		t.Fatalf("GetForwarder() error: %v", err)
	}
	if forwarder == nil {
		t.Fatal("GetForwarder() returned nil")
	}
	if forwarder.Address != "source@example.test" ||
		forwarder.Destination != "second@example.net" {
		t.Fatalf("forwarder = %#v", forwarder)
	}
}

func TestClientDeletesForwarderWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/delete_forwarder" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("address"); got != "source@example.test" {
			t.Errorf("address = %q", got)
		}
		if got := request.Form.Get("forwarder"); got != "destination@example.net" {
			t.Errorf("forwarder = %q", got)
		}

		_, _ = fmt.Fprint(response, `{"status":1,"data":null}`)
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.DeleteForwarder(
		context.Background(),
		"source@example.test",
		"destination@example.net",
	); err != nil {
		t.Fatalf("DeleteForwarder() error: %v", err)
	}
}
