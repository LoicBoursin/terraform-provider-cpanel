package cpanel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClientValidatesHost(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		host    string
		wantErr string
	}{
		"absolute HTTPS": {
			host: "https://cpanel.example.test:2083/",
		},
		"loopback HTTP": {
			host: "http://127.0.0.1:8080",
		},
		"relative URL": {
			host:    "cpanel.example.test:2083",
			wantErr: "absolute URL",
		},
		"insecure remote URL": {
			host:    "http://cpanel.example.test:2083",
			wantErr: "must use HTTPS",
		},
		"credentials in URL": {
			host:    "https://user:password@cpanel.example.test:2083",
			wantErr: "must not contain credentials",
		},
		"query in URL": {
			host:    "https://cpanel.example.test:2083?token=secret",
			wantErr: "must not contain credentials",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(test.host, "username", "token")
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("NewClient() error = %v, want containing %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("NewClient() unexpected error: %v", err)
			}
			if strings.HasSuffix(client.HostURL, "/") {
				t.Fatalf("NewClient() HostURL = %q, want no trailing slash", client.HostURL)
			}
		})
	}
}

func TestExecuteUAPIOperationGET(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Postgresql/list_databases" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("name"); got != "database_name" {
			t.Errorf("name = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "cpanel username:api-token" {
			t.Errorf("Authorization = %q", got)
		}

		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":1,"data":{"value":"decoded"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	var output struct {
		Data struct {
			Value string `json:"value"`
		} `json:"data"`
	}

	err := client.ExecuteUAPIOperation(
		context.Background(),
		http.MethodGet,
		"Postgresql",
		"list_databases",
		map[string]string{"name": "database_name"},
		&output,
	)
	if err != nil {
		t.Fatalf("ExecuteUAPIOperation() error: %v", err)
	}
	if output.Data.Value != "decoded" {
		t.Fatalf("decoded value = %q", output.Data.Value)
	}
}

func TestExecuteUAPIOperationPOSTKeepsSecretsOutOfURL(t *testing.T) {
	t.Parallel()

	const password = "password-with-specials-&=?"

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if strings.Contains(request.URL.String(), password) || request.URL.RawQuery != "" {
			t.Errorf("request URL contains form data: %s", request.URL.Redacted())
		}
		if got := request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("password"); got != password {
			t.Errorf("password form value = %q", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	var output map[string]any
	err := client.ExecuteUAPIOperation(
		context.Background(),
		http.MethodPost,
		"Postgresql",
		"create_user",
		map[string]string{"name": "database_user", "password": password},
		&output,
	)
	if err != nil {
		t.Fatalf("ExecuteUAPIOperation() error: %v", err)
	}
}

func TestExecuteUAPIOperationReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"status":0,"errors":["database already exists"],"messages":["choose another name"]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ExecuteUAPIOperation(
		context.Background(),
		http.MethodPost,
		"Postgresql",
		"create_database",
		nil,
		&map[string]any{},
	)

	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if got := apiError.Error(); !strings.Contains(got, "database already exists; choose another name") {
		t.Fatalf("APIError = %q", got)
	}
}

func TestExecuteAPI2Operation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.URL.Query().Get("cpanel_jsonapi_apiversion"); got != "2" {
			t.Errorf("cpanel_jsonapi_apiversion = %q", got)
		}
		if got := request.URL.Query().Get("cpanel_jsonapi_user"); got != "username" {
			t.Errorf("cpanel_jsonapi_user = %q", got)
		}
		if got := request.URL.Query().Get("cpanel_jsonapi_module"); got != "Cron" {
			t.Errorf("cpanel_jsonapi_module = %q", got)
		}
		if got := request.URL.Query().Get("cpanel_jsonapi_func"); got != "fetchcron" {
			t.Errorf("cpanel_jsonapi_func = %q", got)
		}

		_, _ = response.Write([]byte(`{"cpanelresult":{"event":{"result":1},"data":[{"linekey":42}]}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	var output struct {
		CpanelResult struct {
			Data []struct {
				LineKey int64 `json:"linekey"`
			} `json:"data"`
		} `json:"cpanelresult"`
	}

	err := client.ExecuteAPI2Operation(
		context.Background(),
		http.MethodGet,
		"Cron",
		"fetchcron",
		nil,
		&output,
	)
	if err != nil {
		t.Fatalf("ExecuteAPI2Operation() error: %v", err)
	}
	if got := output.CpanelResult.Data[0].LineKey; got != 42 {
		t.Fatalf("linekey = %d", got)
	}
}

func TestExecuteAPI2OperationReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"cpanelresult":{"event":{"result":0},"data":[{"reason":"cron entry not found","statusmsg":"failed"}]}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ExecuteAPI2Operation(
		context.Background(),
		http.MethodPost,
		"Cron",
		"remove_line",
		nil,
		&map[string]any{},
	)

	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if got := apiError.Error(); !strings.Contains(got, "cron entry not found; failed") {
		t.Fatalf("APIError = %q", got)
	}
}

func TestExecuteReturnsSanitizedHTTPError(t *testing.T) {
	t.Parallel()

	const secret = "server-secret-response"

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = response.Write([]byte(secret))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ExecuteUAPIOperation(
		context.Background(),
		http.MethodGet,
		"Postgresql",
		"list_users",
		nil,
		&map[string]any{},
	)

	var httpError *HTTPError
	if !errors.As(err, &httpError) {
		t.Fatalf("error = %T %v, want *HTTPError", err, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("HTTPError leaks response body: %v", err)
	}
}

func TestExecuteRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(strings.Repeat("x", maxResponseSize+1)))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ExecuteUAPIOperation(
		context.Background(),
		http.MethodGet,
		"Postgresql",
		"list_users",
		nil,
		&map[string]any{},
	)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}

func TestExecuteSerializesRequests(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var maximum atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}

		time.Sleep(25 * time.Millisecond)
		active.Add(-1)
		_, _ = response.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	var waitGroup sync.WaitGroup
	errorsChannel := make(chan error, 2)

	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			errorsChannel <- client.ExecuteUAPIOperation(
				context.Background(),
				http.MethodGet,
				"Postgresql",
				"list_users",
				nil,
				&map[string]any{},
			)
		}()
	}

	waitGroup.Wait()
	close(errorsChannel)

	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("ExecuteUAPIOperation() error: %v", err)
		}
	}
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrent requests = %d, want 1", got)
	}
}

func TestExecuteHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		response.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		"Postgresql",
		"list_users",
		nil,
		&map[string]any{},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func newTestClient(t *testing.T, host string) *Client {
	t.Helper()

	client, err := NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	client.HTTPClient.Timeout = time.Second

	return client
}

func ExampleAPIError() {
	err := &APIError{
		API:      "UAPI",
		Module:   "Postgresql",
		Function: "create_database",
		Messages: []string{"database already exists"},
	}

	fmt.Println(err)
	// Output: UAPI Postgresql::create_database failed: database already exists
}
