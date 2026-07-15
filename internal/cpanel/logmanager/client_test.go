package logmanager

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGet(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/LogManager/get_settings",
		)
		writeJSON(
			t,
			response,
			`{"status":1,"data":{"archive_logs":"1","prune_archive":0,"retention_days":"30","using_default":0}}`,
		)
	}))
	defer server.Close()

	actual, err := newTestClient(t, server.URL).Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	want := &Settings{
		ArchiveLogs:   true,
		PruneArchive:  false,
		RetentionDays: 30,
		UsingDefault:  false,
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("Get() = %#v, want %#v", actual, want)
	}
}

func TestClientSetAndVerify(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition Definition
		readback   string
	}{
		"custom retention": {
			definition: Definition{
				ArchiveLogs:   true,
				PruneArchive:  false,
				RetentionDays: 14,
			},
			readback: `{"archive_logs":1,"prune_archive":"0","retention_days":14,"using_default":"0"}`,
		},
		"server default": {
			definition: Definition{
				ArchiveLogs:   false,
				PruneArchive:  true,
				RetentionDays: -1,
			},
			readback: `{"archive_logs":"0","prune_archive":1,"retention_days":"30","using_default":1}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch calls.Add(1) {
				case 1:
					assertRequest(
						t,
						request,
						http.MethodPost,
						"/execute/LogManager/set_settings",
					)
					if err := request.ParseForm(); err != nil {
						t.Fatalf("ParseForm() error: %v", err)
					}
					want := url.Values{
						"archive_logs": {
							booleanParameter(test.definition.ArchiveLogs),
						},
						"prune_archive": {
							booleanParameter(test.definition.PruneArchive),
						},
						"retention_days": {
							intParameter(test.definition.RetentionDays),
						},
					}
					if !reflect.DeepEqual(request.PostForm, want) {
						t.Fatalf(
							"POST form = %#v, want %#v",
							request.PostForm,
							want,
						)
					}
					writeJSON(t, response, `{"status":1,"data":null}`)
				case 2:
					assertRequest(
						t,
						request,
						http.MethodGet,
						"/execute/LogManager/get_settings",
					)
					writeJSON(
						t,
						response,
						`{"status":1,"data":`+test.readback+`}`,
					)
				default:
					t.Fatalf("unexpected request %s", request.URL.Path)
				}
			}))
			defer server.Close()

			actual, err := newTestClient(t, server.URL).Set(
				t.Context(),
				test.definition,
			)
			if err != nil {
				t.Fatalf("Set() error: %v", err)
			}
			if !SettingsMatchDefinition(*actual, test.definition) {
				t.Fatalf(
					"Set() = %#v, want matching %#v",
					actual,
					test.definition,
				)
			}
			if calls.Load() != 2 {
				t.Fatalf("request count = %d, want 2", calls.Load())
			}
		})
	}
}

func TestClientRejectsInvalidDefinitionsBeforeRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		calls.Add(1)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).Set(
		t.Context(),
		Definition{RetentionDays: -2},
	)
	if err == nil || !strings.Contains(err.Error(), "must be -1") {
		t.Fatalf("Set() error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("request count = %d, want 0", calls.Load())
	}
}

func TestClientRejectsMalformedSettings(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		data    string
		wantErr string
	}{
		"missing archive flag": {
			data:    `{"prune_archive":0,"retention_days":30,"using_default":0}`,
			wantErr: "invalid archive_logs flag",
		},
		"invalid prune flag": {
			data:    `{"archive_logs":1,"prune_archive":true,"retention_days":30,"using_default":0}`,
			wantErr: "invalid prune_archive flag",
		},
		"invalid default flag": {
			data:    `{"archive_logs":1,"prune_archive":0,"retention_days":30,"using_default":2}`,
			wantErr: "invalid using_default flag",
		},
		"null retention": {
			data:    `{"archive_logs":1,"prune_archive":0,"retention_days":null,"using_default":0}`,
			wantErr: "missing log retention period",
		},
		"negative retention": {
			data:    `{"archive_logs":1,"prune_archive":0,"retention_days":-1,"using_default":0}`,
			wantErr: "invalid log retention period",
		},
		"fractional retention": {
			data:    `{"archive_logs":1,"prune_archive":0,"retention_days":1.5,"using_default":0}`,
			wantErr: "invalid log retention period",
		},
		"whitespace retention": {
			data:    `{"archive_logs":1,"prune_archive":0,"retention_days":" 30 ","using_default":0}`,
			wantErr: "surrounding whitespace",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeJSON(
					t,
					response,
					`{"status":1,"data":`+test.data+`}`,
				)
			}))
			defer server.Close()

			_, err := newTestClient(t, server.URL).Get(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"Get() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsPostMutationMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/LogManager/set_settings":
			writeJSON(t, response, `{"status":1,"data":null}`)
		case "/execute/LogManager/get_settings":
			writeJSON(
				t,
				response,
				`{"status":1,"data":{"archive_logs":0,"prune_archive":0,"retention_days":7,"using_default":0}}`,
			)
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).Set(t.Context(), Definition{
		ArchiveLogs:   true,
		PruneArchive:  false,
		RetentionDays: 7,
	})
	if err == nil || !strings.Contains(err.Error(), "archive_logs") {
		t.Fatalf("Set() error = %v", err)
	}
}

func TestClientDoesNotRetryMutation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	baseClient, err := cpanel.NewClient(
		"https://cpanel.example.test:2083",
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	baseClient.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodPost {
				t.Fatalf("request method = %s, want POST", request.Method)
			}

			return nil, errors.New("connection closed")
		}),
	}

	_, err = NewClient(baseClient).Set(t.Context(), Definition{
		ArchiveLogs:   true,
		PruneArchive:  true,
		RetentionDays: -1,
	})
	if err == nil {
		t.Fatal("Set() returned no error")
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()

	client, err := cpanel.NewClient(
		serverURL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(client)
}

func assertRequest(
	t *testing.T,
	request *http.Request,
	method string,
	path string,
) {
	t.Helper()

	if request.Method != method {
		t.Fatalf("request method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != path {
		t.Fatalf("request path = %q, want %q", request.URL.Path, path)
	}
	if request.Header.Get("Authorization") != "cpanel username:api-token" {
		t.Fatalf(
			"Authorization header = %q",
			request.Header.Get("Authorization"),
		)
	}
}

func writeJSON(
	t *testing.T,
	response http.ResponseWriter,
	body string,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if _, err := response.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func intParameter(value int64) string {
	return strconv.FormatInt(value, 10)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return function(request)
}
