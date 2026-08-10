package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreatesDomainForwarderWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/add_domain_forwarder" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("domain"); got != "example.test" {
			t.Errorf("domain = %q, want example.test", got)
		}
		if got := request.Form.Get("destdomain"); got != "destination.test" {
			t.Errorf("destdomain = %q, want destination.test", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.CreateDomainForwarder(
		context.Background(),
		"example.test",
		"destination.test",
	); err != nil {
		t.Fatalf("CreateDomainForwarder() error: %v", err)
	}
}

func TestClientFindsDomainForwarder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Email/list_domain_forwarders" {
			t.Errorf("path = %s", request.URL.Path)
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]string{
				{"dest": "first.test", "forward": "first-target.test"},
				{"dest": "example.test", "forward": "destination.test"},
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	forwarder, err := client.GetDomainForwarder(
		context.Background(),
		"example.test",
	)
	if err != nil {
		t.Fatalf("GetDomainForwarder() error: %v", err)
	}
	if forwarder == nil {
		t.Fatal("GetDomainForwarder() returned nil")
	}
	if forwarder.Domain != "example.test" ||
		forwarder.Destination != "destination.test" {
		t.Fatalf("forwarder = %#v", forwarder)
	}
}

func TestClientDeletesDomainForwarderWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/delete_domain_forwarder" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("domain"); got != "example.test" {
			t.Errorf("domain = %q, want example.test", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.DeleteDomainForwarder(
		context.Background(),
		"example.test",
	); err != nil {
		t.Fatalf("DeleteDomainForwarder() error: %v", err)
	}
}
