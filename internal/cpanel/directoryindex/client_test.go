package directoryindex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGetsDirectoryIndex(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			assertRequest(t, request, http.MethodGet, "/execute/Variables/get_user_information")
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case 2:
			assertListFilesRequest(t, request, "")
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"file":     "public_html",
					"fullpath": "/home/example/public_html",
					"type":     "dir",
				}},
			})
		case 3:
			assertListFilesRequest(t, request, "public_html")
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"file":     "downloads",
					"fullpath": "/home/example/public_html/downloads",
					"type":     "dir",
				}},
			})
		case 4:
			assertRequest(t, request, http.MethodGet, "/execute/DirectoryIndexes/get_indexing")
			if request.URL.Query().Get("dir") != "/home/example/public_html/downloads" {
				t.Errorf("dir = %q", request.URL.Query().Get("dir"))
			}
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   IndexTypeFancy,
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	index, err := client.Get(t.Context(), "public_html/downloads")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if index == nil {
		t.Fatal("Get() returned nil")
	}
	if index.Type != IndexTypeFancy ||
		index.AbsoluteDirectory != "/home/example/public_html/downloads" {
		t.Fatalf("index = %#v", index)
	}
}

func TestClientReturnsNilWhenDirectoryDoesNotExist(t *testing.T) {
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
				"data":   []map[string]any{},
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	index, err := newTestClient(t, server).Get(
		t.Context(),
		"public_html/missing",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if index != nil {
		t.Fatalf("Get() = %#v, want nil", index)
	}
}

func TestClientSetsDirectoryIndexWithPOST(t *testing.T) {
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
			assertRequest(t, request, http.MethodPost, "/execute/DirectoryIndexes/set_indexing")
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("dir") != "/home/example/public_html" ||
				request.Form.Get("type") != IndexTypeDisabled {
				t.Errorf("form = %v", request.Form)
			}
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   IndexTypeDisabled,
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	index, err := newTestClient(t, server).Set(
		t.Context(),
		"public_html",
		IndexTypeDisabled,
	)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	if index.Type != IndexTypeDisabled {
		t.Fatalf("index = %#v", index)
	}
}

func TestClientRejectsUnsupportedIndexType(t *testing.T) {
	t.Parallel()

	baseClient, err := cpanel.NewClient(
		"https://example.test",
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	if _, err := NewClient(baseClient).Set(
		t.Context(),
		"public_html",
		"unexpected",
	); err == nil {
		t.Fatal("Set() returned no error")
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

func assertListFilesRequest(
	t *testing.T,
	request *http.Request,
	directory string,
) {
	t.Helper()

	assertRequest(t, request, http.MethodGet, "/execute/Fileman/list_files")
	if request.URL.Query().Get("dir") != directory ||
		request.URL.Query().Get("show_hidden") != "1" ||
		request.URL.Query().Get("limit") != "100000" {
		t.Errorf("query = %v", request.URL.Query())
	}
}

func writeJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
