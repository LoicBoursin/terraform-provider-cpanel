package directoryprivacy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGetsProtectedDirectory(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case 2:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"file":     "public_html",
					"fullpath": "/home/example/public_html",
					"type":     "dir",
				}},
			})
		case 3:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"file":     "private",
					"fullpath": "/home/example/public_html/private",
					"type":     "dir",
				}},
			})
		case 4:
			if request.Method != http.MethodGet ||
				request.URL.Path != "/execute/DirectoryPrivacy/is_directory_protected" {
				t.Errorf("request = %s %s", request.Method, request.URL.Path)
			}
			if request.URL.Query().Get("dir") != "/home/example/public_html/private" {
				t.Errorf("query = %v", request.URL.Query())
			}
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"auth_name":   "Private files",
					"auth_type":   "Basic",
					"passwd_file": "/home/example/.htpasswds/public_html/private/passwd",
					"protected":   1,
				},
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	privacy, err := newTestClient(t, server).Get(
		t.Context(),
		"public_html/private",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if privacy == nil || !privacy.Protected ||
		privacy.AuthName != "Private files" ||
		privacy.AuthType != "Basic" {
		t.Fatalf("privacy = %#v", privacy)
	}
}

func TestClientReturnsNilWhenDirectoryDoesNotExist(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case 2:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   []map[string]any{},
			})
		default:
			t.Fatalf("unexpected request %d", requestCount)
		}
	}))
	defer server.Close()

	privacy, err := newTestClient(t, server).Get(
		t.Context(),
		"public_html",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if privacy != nil {
		t.Fatalf("Get() = %#v, want nil", privacy)
	}
}

func TestClientConfiguresDirectoryProtectionWithPOST(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case 2:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"file":     "public_html",
					"fullpath": "/home/example/public_html",
					"type":     "dir",
				}},
			})
		case 3:
			if request.Method != http.MethodPost ||
				request.URL.Path != "/execute/DirectoryPrivacy/configure_directory_protection" {
				t.Errorf("request = %s %s", request.Method, request.URL.Path)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("dir") != "/home/example/public_html" ||
				request.Form.Get("enabled") != "1" ||
				request.Form.Get("authname") != "Private files" {
				t.Errorf("form = %v", request.Form)
			}
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"auth_name":   "Private files",
					"auth_type":   "Basic",
					"passwd_file": "/home/example/.htpasswds/public_html/passwd",
					"protected":   1,
				},
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	privacy, err := newTestClient(t, server).Configure(
		t.Context(),
		"public_html",
		"Private files",
		true,
	)
	if err != nil {
		t.Fatalf("Configure() error: %v", err)
	}
	if privacy == nil || !privacy.Protected {
		t.Fatalf("privacy = %#v", privacy)
	}
}

func TestPrivacyFromResponseNormalizesDisabledDirectory(t *testing.T) {
	t.Parallel()

	privacy, err := privacyFromResponse(
		"public_html",
		"/home/example/public_html",
		ResponseData{
			AuthName:     "stale",
			AuthType:     "",
			PasswordFile: "/stale",
			Protected:    0,
		},
	)
	if err != nil {
		t.Fatalf("privacyFromResponse() error: %v", err)
	}
	if privacy.Protected ||
		privacy.AuthName != "" ||
		privacy.AuthType != "None" ||
		privacy.PasswordFile != "" {
		t.Fatalf("privacy = %#v", privacy)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func writeJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
