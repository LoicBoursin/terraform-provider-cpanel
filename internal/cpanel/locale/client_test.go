package locale

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGetsCurrentLocale(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Locale/get_attributes",
			)
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"locale":    "fr",
					"direction": "ltr",
					"encoding":  "UTF-8",
				},
			})
		case 2:
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Locale/list_locales",
			)
			writeJSON(t, response, localeInventory())
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	current, err := newTestClient(t, server).GetCurrent(t.Context())
	if err != nil {
		t.Fatalf("GetCurrent() error: %v", err)
	}
	if current.Code != "fr" ||
		current.Direction != DirectionLeftToRight ||
		current.Encoding != "utf-8" ||
		current.LocalName != "français" ||
		current.Name != "French" {
		t.Fatalf("current = %#v", current)
	}
}

func TestClientListsSortedLocales(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/Locale/list_locales",
		)
		writeJSON(t, response, localeInventory())
	}))
	defer server.Close()

	locales, err := newTestClient(t, server).List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(locales) != 2 ||
		locales[0].Code != "en" ||
		locales[1].Code != "fr" {
		t.Fatalf("locales = %#v", locales)
	}
}

func TestClientSetsLocaleWithPOST(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			writeJSON(t, response, localeInventory())
		case 2:
			assertRequest(
				t,
				request,
				http.MethodPost,
				"/execute/Locale/set_locale",
			)
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("locale") != "en" {
				t.Fatalf("locale = %q", request.Form.Get("locale"))
			}
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   nil,
			})
		case 3:
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"locale":    "en",
					"direction": "ltr",
					"encoding":  "utf-8",
				},
			})
		case 4:
			writeJSON(t, response, localeInventory())
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	current, err := newTestClient(t, server).Set(t.Context(), "en")
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	if current.Code != "en" || current.LocalName != "English" {
		t.Fatalf("current = %#v", current)
	}
}

func TestClientRejectsInvalidLocaleInventory(t *testing.T) {
	t.Parallel()

	testCases := map[string][]map[string]any{
		"duplicate": {
			{
				"locale":     "en",
				"direction":  "ltr",
				"name":       "English",
				"local_name": "English",
			},
			{
				"locale":     "en",
				"direction":  "ltr",
				"name":       "English",
				"local_name": "English",
			},
		},
		"invalid direction": {
			{
				"locale":     "en",
				"direction":  "sideways",
				"name":       "English",
				"local_name": "English",
			},
		},
		"missing local name": {
			{
				"locale":    "en",
				"direction": "ltr",
				"name":      "English",
			},
		},
	}

	for name, inventory := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeJSON(t, response, map[string]any{
					"status": 1,
					"data":   inventory,
				})
			}))
			defer server.Close()

			if _, err := newTestClient(t, server).List(t.Context()); err == nil {
				t.Fatal("List() returned no error")
			}
		})
	}
}

func TestValidateCode(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"en", "es_419", "i_cpanel_snowmen"} {
		if err := ValidateCode(code); err != nil {
			t.Errorf("ValidateCode(%q) error: %v", code, err)
		}
	}
	for _, code := range []string{"", "EN", " en", "en-", "en__us"} {
		if err := ValidateCode(code); err == nil {
			t.Errorf("ValidateCode(%q) returned no error", code)
		}
	}
}

func localeInventory() map[string]any {
	return map[string]any{
		"status": 1,
		"data": []map[string]any{
			{
				"locale":     "fr",
				"direction":  "ltr",
				"name":       "French",
				"local_name": "français",
			},
			{
				"locale":     "en",
				"direction":  "ltr",
				"name":       "English",
				"local_name": "English",
			},
		},
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
