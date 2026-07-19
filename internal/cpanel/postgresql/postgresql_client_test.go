package postgresql

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

type postgreSQLRequestExpectation struct {
	method     string
	path       string
	parameters url.Values
	secrets    []string
}

type postgreSQLRoundTripFunc func(*http.Request) (*http.Response, error)

func (f postgreSQLRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newPostgreSQLTestServer(
	t *testing.T,
	expectation postgreSQLRequestExpectation,
	responseBody string,
) *httptest.Server {
	t.Helper()

	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount.Add(1)
		assertPostgreSQLRequest(t, request, expectation)

		response.Header().Set("Content-Type", "application/json")
		if _, err := response.Write([]byte(responseBody)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if got := requestCount.Load(); got != 1 {
			t.Errorf("request count = %d, want 1", got)
		}
	})

	return server
}

func assertPostgreSQLRequest(
	t *testing.T,
	request *http.Request,
	expectation postgreSQLRequestExpectation,
) {
	t.Helper()

	if request.Method != expectation.method {
		t.Errorf("method = %s, want %s", request.Method, expectation.method)
	}
	if request.URL.Path != expectation.path {
		t.Errorf("path = %s, want %s", request.URL.Path, expectation.path)
	}
	if got := request.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, want application/json", got)
	}
	if got := request.Header.Get("Authorization"); got != "cpanel username:api-token" {
		t.Errorf("Authorization = %q", got)
	}

	var parameters url.Values
	switch expectation.method {
	case http.MethodGet:
		parameters = request.URL.Query()
	case http.MethodPost:
		if request.URL.RawQuery != "" {
			t.Errorf("POST query = %q, want empty", request.URL.RawQuery)
		}
		if got := request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error: %v", err)

			return
		}
		parameters = request.PostForm
	default:
		t.Errorf("unsupported expected method: %s", expectation.method)

		return
	}

	if !reflect.DeepEqual(parameters, expectation.parameters) {
		t.Errorf("parameters = %#v, want %#v", parameters, expectation.parameters)
	}

	requestURL := request.URL.String()
	for _, secret := range expectation.secrets {
		for _, representation := range []string{
			secret,
			url.QueryEscape(secret),
			url.PathEscape(secret),
		} {
			if representation != "" && strings.Contains(requestURL, representation) {
				t.Errorf("request URL contains secret: %s", request.URL.Redacted())

				break
			}
		}
	}
}

func postgreSQLValues(parameters map[string]string) url.Values {
	values := make(url.Values, len(parameters))
	for key, value := range parameters {
		values.Set(key, value)
	}

	return values
}

func newPostgreSQLTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
