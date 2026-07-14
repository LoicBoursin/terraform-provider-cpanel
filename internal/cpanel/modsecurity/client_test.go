package modsecurity

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsModSecurityDomains(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/ModSecurity/list_domains",
		)
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"domain":       "www.example.test",
					"enabled":      0,
					"type":         DomainTypeSub,
					"dependencies": []string{"example.test"},
					"searchhint":   " example.test ",
				},
				{
					"domain":       "example.test",
					"enabled":      1,
					"type":         DomainTypeMain,
					"dependencies": []string{},
					"searchhint":   "",
				},
			},
		})
	}))
	defer server.Close()

	domains, err := newTestClient(t, server).List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("len(domains) = %d, want 2", len(domains))
	}
	if domains[0].Domain != "example.test" ||
		!domains[0].Enabled ||
		domains[1].Domain != "www.example.test" ||
		domains[1].Enabled ||
		domains[1].SearchHint != "example.test" {
		t.Fatalf("domains = %#v", domains)
	}
	affected := domains[1].AffectedDomains()
	if len(affected) != 2 ||
		affected[0] != "example.test" ||
		affected[1] != "www.example.test" {
		t.Fatalf("AffectedDomains() = %#v", affected)
	}
}

func TestClientGetsModSecurityInstallationStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/ModSecurity/has_modsecurity_installed",
		)
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data":   map[string]any{"installed": 1},
		})
	}))
	defer server.Close()

	installed, err := newTestClient(t, server).HasInstalled(t.Context())
	if err != nil {
		t.Fatalf("HasInstalled() error: %v", err)
	}
	if !installed {
		t.Fatal("HasInstalled() = false, want true")
	}
}

func TestClientSetsModSecurityDomainWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodPost,
			"/execute/ModSecurity/disable_domains",
		)
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("domains") != "example.test,www.example.test" {
			t.Fatalf("domains = %q", request.Form.Get("domains"))
		}
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"domain":       "www.example.test",
					"enabled":      0,
					"type":         DomainTypeSub,
					"dependencies": []string{"example.test"},
					"searchhint":   "example.test",
				},
				{
					"domain":       "example.test",
					"enabled":      0,
					"type":         DomainTypeMain,
					"dependencies": []string{},
					"searchhint":   "",
				},
			},
		})
	}))
	defer server.Close()

	domains, err := newTestClient(t, server).SetEnabled(
		t.Context(),
		[]string{"www.example.test", "example.test"},
		false,
	)
	if err != nil {
		t.Fatalf("SetEnabled() error: %v", err)
	}
	if len(domains) != 2 ||
		domains[0].Domain != "example.test" ||
		domains[1].Domain != "www.example.test" {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestClientRejectsFailedModSecurityMutationData(t *testing.T) {
	t.Parallel()

	testCases := map[string]any{
		"false": false,
		"exception": []map[string]any{{
			"domain":       "example.test",
			"enabled":      0,
			"type":         DomainTypeMain,
			"dependencies": []string{},
			"exception":    "userdata update failed",
		}},
		"missing requested domain": []map[string]any{{
			"domain":       "other.example.test",
			"enabled":      0,
			"type":         DomainTypeSub,
			"dependencies": []string{},
		}},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeJSON(t, response, map[string]any{
					"status": 1,
					"data":   data,
				})
			}))
			defer server.Close()

			if _, err := newTestClient(t, server).SetEnabled(
				t.Context(),
				[]string{"example.test"},
				false,
			); err == nil {
				t.Fatal("SetEnabled() returned no error")
			}
		})
	}
}

func TestClientRejectsInvalidModSecurityInventory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{{
				"domain":       "example.test",
				"enabled":      2,
				"type":         DomainTypeMain,
				"dependencies": []string{},
			}},
		})
	}))
	defer server.Close()

	if _, err := newTestClient(t, server).List(t.Context()); err == nil {
		t.Fatal("List() returned no error")
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func assertRequest(
	t *testing.T,
	request *http.Request,
	method string,
	requestPath string,
) {
	t.Helper()

	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != requestPath {
		t.Errorf("path = %s, want %s", request.URL.Path, requestPath)
	}
}

func writeJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
