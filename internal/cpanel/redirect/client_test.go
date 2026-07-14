package redirect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndFindsRedirects(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Mime/list_redirects" {
			t.Errorf("path = %s", request.URL.Path)
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"destination":      "https://example.net/new",
					"displaydomain":    "example.com",
					"displaysourceurl": "/old",
					"docroot":          "/home/account/public_html",
					"domain":           "example.com",
					"kind":             "rewrite",
					"matchwww":         1,
					"opts":             "L",
					"source":           "/old",
					"statuscode":       "301",
					"targeturl":        "https://example.net/new",
					"type":             "permanent",
					"urldomain":        "example.com",
					"wildcard":         0,
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	redirect, err := NewClient(baseClient).Get(
		t.Context(),
		"example.com",
		"/old",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if redirect == nil {
		t.Fatal("Get() returned nil")
	}
	if redirect.Destination != "https://example.net/new" ||
		redirect.Type != TypePermanent ||
		redirect.StatusCode != "301" ||
		redirect.MatchWWW != 1 {
		t.Fatalf("redirect = %#v", redirect)
	}
}

func TestClientRejectsAmbiguousRedirects(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":[{"domain":"example.com","source":"/old"},{"domain":"example.com","source":"/old"}]}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	_, err = NewClient(baseClient).Get(
		t.Context(),
		"example.com",
		"/old",
	)
	if err == nil {
		t.Fatal("Get() returned no error")
	}
}

func TestClientAddsRedirectsWithPOST(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		definition       Definition
		expectedType     string
		expectedWWW      string
		expectedWildcard string
	}{
		"permanent both": {
			definition: Definition{
				Domain:      "example.com",
				Source:      "/old",
				Destination: "https://example.net/new",
				Type:        TypePermanent,
				WWWMode:     WWWModeBoth,
			},
			expectedType:     "permanent",
			expectedWWW:      "0",
			expectedWildcard: "0",
		},
		"temporary without www wildcard": {
			definition: Definition{
				Domain:      "example.com",
				Source:      "/old",
				Destination: "https://example.net/new",
				Type:        TypeTemporary,
				WWWMode:     WWWModeWithout,
				Wildcard:    true,
			},
			expectedType:     "temp",
			expectedWWW:      "1",
			expectedWildcard: "1",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if request.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", request.Method)
				}
				if request.URL.Path != "/execute/Mime/add_redirect" {
					t.Errorf("path = %s", request.URL.Path)
				}
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}
				if request.Form.Get("domain") != "example.com" ||
					request.Form.Get("src") != "/old" ||
					request.Form.Get("redirect") != "https://example.net/new" ||
					request.Form.Get("type") != testCase.expectedType ||
					request.Form.Get("redirect_www") != testCase.expectedWWW ||
					request.Form.Get("redirect_wildcard") != testCase.expectedWildcard {
					t.Errorf("form = %v", request.Form)
				}

				_, _ = response.Write([]byte(`{"status":1,"data":null}`))
			}))
			defer server.Close()

			baseClient, err := cpanel.NewClient(
				server.URL,
				"username",
				"api-token",
			)
			if err != nil {
				t.Fatalf("cpanel.NewClient() error: %v", err)
			}

			if err := NewClient(baseClient).Add(
				t.Context(),
				testCase.definition,
			); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
		})
	}
}

func TestClientDeletesRedirectWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Mime/delete_redirect" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if request.Form.Get("domain") != "example.com" ||
			request.Form.Get("src") != "/old" {
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
		"example.com",
		"/old",
	); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}
