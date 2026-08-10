package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsCPDAVDDelegatesAndReusesProbeInventory(
	t *testing.T,
) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		calls.Add(1)
		assertCalendarRequest(
			t,
			request,
			http.MethodGet,
			"/execute/CPDAVD/list_delegates",
		)
		writeCalendarJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"delegator": "owner-b@example.test",
					"delegatee": "delegate@example.test",
					"calendar":  DefaultCalendar,
					"calname":   "B calendar",
					"readonly":  "0",
				},
				{
					"delegator": "owner-a@example.test",
					"delegatee": "delegate@example.test",
					"calendar":  DefaultCalendar,
					"calname":   "A calendar",
					"readonly":  1,
				},
			},
		})
	}))
	defer server.Close()

	delegates, err := newCalendarTestClient(t, server.URL).
		ListDelegates(t.Context())
	if err != nil {
		t.Fatalf("ListDelegates() error: %v", err)
	}

	want := []Delegate{
		{
			Delegator:    "owner-a@example.test",
			Delegatee:    "delegate@example.test",
			Calendar:     DefaultCalendar,
			CalendarName: "A calendar",
			ReadOnly:     true,
		},
		{
			Delegator:    "owner-b@example.test",
			Delegatee:    "delegate@example.test",
			Calendar:     DefaultCalendar,
			CalendarName: "B calendar",
			ReadOnly:     false,
		},
	}
	if !reflect.DeepEqual(delegates, want) {
		t.Fatalf("ListDelegates() = %#v, want %#v", delegates, want)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestClientGetsExactDelegate(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		calls.Add(1)
		assertCalendarRequest(
			t,
			request,
			http.MethodGet,
			"/execute/CPDAVD/list_delegates",
		)
		writeCalendarJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{{
				"delegator": "owner@example.test",
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
				"readonly":  0,
			}},
		})
	}))
	defer server.Close()

	client := newCalendarTestClient(t, server.URL)
	delegate, err := client.GetDelegate(
		t.Context(),
		"owner@example.test",
		DefaultCalendar,
		"delegate@example.test",
	)
	if err != nil {
		t.Fatalf("GetDelegate() error: %v", err)
	}
	if delegate == nil ||
		delegate.CalendarName != "Shared calendar" ||
		delegate.ReadOnly {
		t.Fatalf("GetDelegate() = %#v", delegate)
	}

	missing, err := client.GetDelegate(
		t.Context(),
		"owner@example.test",
		DefaultCalendar,
		"other@example.test",
	)
	if err != nil {
		t.Fatalf("GetDelegate() missing error: %v", err)
	}
	if missing != nil {
		t.Fatalf("GetDelegate() missing = %#v, want nil", missing)
	}
	if calls.Load() != 2 {
		t.Fatalf("request count = %d, want 2", calls.Load())
	}
}

func TestClientFallsBackToCCSAndCachesSelection(t *testing.T) {
	t.Parallel()

	var cpdavdCalls atomic.Int32
	var ccsDelegateCalls atomic.Int32
	var ccsUserCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/CPDAVD/list_delegates":
			cpdavdCalls.Add(1)
			writeMissingCPDAVDModule(t, response)
		case "/execute/CCS/list_delegates":
			ccsDelegateCalls.Add(1)
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"delegator": "owner@example.test",
					"delegatee": "delegate@example.test",
					"read_only": 1,
				}},
			})
		case "/execute/CCS/list_users":
			ccsUserCalls.Add(1)
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"owner@example.test": "0882362A-5076-11F0-8896-ACDE48001122",
				},
			})
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client := newCalendarTestClient(t, server.URL)
	delegates, err := client.ListDelegates(t.Context())
	if err != nil {
		t.Fatalf("ListDelegates() error: %v", err)
	}
	if len(delegates) != 1 ||
		delegates[0].Calendar != DefaultCalendar ||
		delegates[0].CalendarName != "" ||
		!delegates[0].ReadOnly {
		t.Fatalf("ListDelegates() = %#v", delegates)
	}

	exists, err := client.CalendarExists(
		t.Context(),
		"owner@example.test",
		DefaultCalendar,
	)
	if err != nil {
		t.Fatalf("CalendarExists() error: %v", err)
	}
	if !exists {
		t.Fatal("CalendarExists() = false, want true")
	}
	exists, err = client.CalendarExists(
		t.Context(),
		"owner@example.test",
		"other-calendar",
	)
	if err != nil {
		t.Fatalf("CalendarExists() nondefault error: %v", err)
	}
	if exists {
		t.Fatal("CalendarExists() nondefault = true, want false")
	}

	if _, err := client.ListDelegates(t.Context()); err != nil {
		t.Fatalf("second ListDelegates() error: %v", err)
	}
	if cpdavdCalls.Load() != 1 {
		t.Fatalf("CPDAVD probe count = %d, want 1", cpdavdCalls.Load())
	}
	if ccsDelegateCalls.Load() != 2 {
		t.Fatalf(
			"CCS delegate request count = %d, want 2",
			ccsDelegateCalls.Load(),
		)
	}
	if ccsUserCalls.Load() != 2 {
		t.Fatalf("CCS user request count = %d, want 2", ccsUserCalls.Load())
	}
}

func TestClientCachesModuleSafelyAcrossConcurrentCalls(t *testing.T) {
	t.Parallel()

	var cpdavdCalls atomic.Int32
	var ccsProbeCalls atomic.Int32
	var listUsersCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/CPDAVD/list_delegates":
			cpdavdCalls.Add(1)
			writeMissingCPDAVDModule(t, response)
		case "/execute/CCS/list_delegates":
			ccsProbeCalls.Add(1)
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data":   []any{},
			})
		case "/execute/CCS/list_users":
			listUsersCalls.Add(1)
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{},
			})
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client := newCalendarTestClient(t, server.URL)
	const goroutines = 8
	errs := make(chan error, goroutines)
	var waitGroup sync.WaitGroup
	for range goroutines {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			_, err := client.ListUsers(context.Background())
			errs <- err
		}()
	}
	waitGroup.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ListUsers() error: %v", err)
		}
	}
	if cpdavdCalls.Load() != 1 {
		t.Fatalf("CPDAVD probe count = %d, want 1", cpdavdCalls.Load())
	}
	if ccsProbeCalls.Load() != 1 {
		t.Fatalf("CCS probe count = %d, want 1", ccsProbeCalls.Load())
	}
	if listUsersCalls.Load() != goroutines {
		t.Fatalf(
			"list_users count = %d, want %d",
			listUsersCalls.Load(),
			goroutines,
		)
	}
}

func TestClientDoesNotFallbackFromNonModuleProbeFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		statusCode int
		body       string
	}{
		"UAPI permission error": {
			statusCode: http.StatusOK,
			body:       `{"status":0,"errors":["permission denied"]}`,
		},
		"malformed JSON": {
			statusCode: http.StatusOK,
			body:       `{"status":`,
		},
		"invalid delegate inventory": {
			statusCode: http.StatusOK,
			body:       `{"status":1,"data":{}}`,
		},
		"HTTP error": {
			statusCode: http.StatusServiceUnavailable,
			body:       `service unavailable`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var ccsCalled atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if strings.HasPrefix(request.URL.Path, "/execute/CCS/") {
					ccsCalled.Store(true)
				}
				response.WriteHeader(test.statusCode)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()

			if _, err := newCalendarTestClient(t, server.URL).
				ListDelegates(t.Context()); err == nil {
				t.Fatal("ListDelegates() returned no error")
			}
			if ccsCalled.Load() {
				t.Fatal("CCS fallback was attempted")
			}
		})
	}
}

func TestClientDoesNotFallbackFromTransportFailure(t *testing.T) {
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
			_ *http.Request,
		) (*http.Response, error) {
			calls.Add(1)

			return nil, errors.New("network unavailable")
		}),
	}

	if _, err := NewClient(baseClient).ListDelegates(t.Context()); err == nil {
		t.Fatal("ListDelegates() returned no error")
	}
	if calls.Load() != 1 {
		t.Fatalf("transport call count = %d, want 1", calls.Load())
	}
}

func TestClientReportsFailedCCSProbe(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/CPDAVD/list_delegates":
			writeMissingCPDAVDModule(t, response)
		case "/execute/CCS/list_delegates":
			writeCalendarJSON(t, response, map[string]any{
				"status": 0,
				"errors": []string{"calendar service unavailable"},
			})
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	_, err := newCalendarTestClient(t, server.URL).
		ListDelegates(t.Context())
	if err == nil {
		t.Fatal("ListDelegates() returned no error")
	}
	if !strings.Contains(err.Error(), "CPDAVD is unavailable") ||
		!strings.Contains(err.Error(), "CCS probe failed") {
		t.Fatalf("ListDelegates() error = %v", err)
	}

	var apiError *cpanel.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %T %v, want *cpanel.APIError", err, err)
	}
	if apiError.Module != cpanel.ModuleCCS ||
		apiError.Function != operationListDelegates {
		t.Fatalf("API error = %#v", apiError)
	}
}

func TestClientMutationsUsePOSTAndModuleSpecificParameters(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name             string
		module           string
		operation        string
		readOnly         bool
		expectedMutation url.Values
		call             func(context.Context, *Client, Definition) error
	}{
		{
			name:      "CPDAVD add readonly",
			module:    cpanel.ModuleCPDAVD,
			operation: operationAddDelegate,
			readOnly:  true,
			expectedMutation: url.Values{
				"delegator": {"owner@example.test"},
				"delegatee": {"delegate@example.test"},
				"calendar":  {DefaultCalendar},
				"readonly":  {"1"},
			},
			call: func(
				ctx context.Context,
				client *Client,
				definition Definition,
			) error {
				return client.AddDelegate(ctx, definition)
			},
		},
		{
			name:      "CPDAVD update writable",
			module:    cpanel.ModuleCPDAVD,
			operation: operationUpdateDelegate,
			readOnly:  false,
			expectedMutation: url.Values{
				"delegator": {"owner@example.test"},
				"delegatee": {"delegate@example.test"},
				"calendar":  {DefaultCalendar},
				"readonly":  {"0"},
			},
			call: func(
				ctx context.Context,
				client *Client,
				definition Definition,
			) error {
				return client.UpdateDelegate(ctx, definition)
			},
		},
		{
			name:      "CPDAVD remove",
			module:    cpanel.ModuleCPDAVD,
			operation: operationRemoveDelegate,
			expectedMutation: url.Values{
				"delegator": {"owner@example.test"},
				"delegatee": {"delegate@example.test"},
				"calendar":  {DefaultCalendar},
			},
			call: func(
				ctx context.Context,
				client *Client,
				definition Definition,
			) error {
				return client.RemoveDelegate(
					ctx,
					definition.Delegator,
					definition.Calendar,
					definition.Delegatee,
				)
			},
		},
		{
			name:      "CCS add readonly",
			module:    cpanel.ModuleCCS,
			operation: operationAddDelegate,
			readOnly:  true,
			expectedMutation: url.Values{
				"delegator": {"owner@example.test"},
				"delegatee": {"delegate@example.test"},
				"readonly":  {"1"},
			},
			call: func(
				ctx context.Context,
				client *Client,
				definition Definition,
			) error {
				return client.AddDelegate(ctx, definition)
			},
		},
		{
			name:      "CCS update writable",
			module:    cpanel.ModuleCCS,
			operation: operationUpdateDelegate,
			readOnly:  false,
			expectedMutation: url.Values{
				"delegator": {"owner@example.test"},
				"delegatee": {"delegate@example.test"},
				"readonly":  {"0"},
			},
			call: func(
				ctx context.Context,
				client *Client,
				definition Definition,
			) error {
				return client.UpdateDelegate(ctx, definition)
			},
		},
		{
			name:      "CCS remove",
			module:    cpanel.ModuleCCS,
			operation: operationRemoveDelegate,
			expectedMutation: url.Values{
				"delegator": {"owner@example.test"},
				"delegatee": {"delegate@example.test"},
			},
			call: func(
				ctx context.Context,
				client *Client,
				definition Definition,
			) error {
				return client.RemoveDelegate(
					ctx,
					definition.Delegator,
					definition.Calendar,
					definition.Delegatee,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var mutationCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if request.URL.Path ==
					"/execute/CPDAVD/list_delegates" {
					if test.module == cpanel.ModuleCCS {
						writeMissingCPDAVDModule(t, response)

						return
					}
					assertCalendarRequest(
						t,
						request,
						http.MethodGet,
						"/execute/CPDAVD/list_delegates",
					)
					writeCalendarJSON(t, response, map[string]any{
						"status": 1,
						"data":   []any{},
					})

					return
				}
				if request.URL.Path ==
					"/execute/CCS/list_delegates" {
					if test.module != cpanel.ModuleCCS {
						t.Fatal("unexpected CCS probe")
					}
					assertCalendarRequest(
						t,
						request,
						http.MethodGet,
						"/execute/CCS/list_delegates",
					)
					writeCalendarJSON(t, response, map[string]any{
						"status": 1,
						"data":   []any{},
					})

					return
				}

				mutationCalls.Add(1)
				assertCalendarRequest(
					t,
					request,
					http.MethodPost,
					fmt.Sprintf(
						"/execute/%s/%s",
						test.module,
						test.operation,
					),
				)
				if request.URL.RawQuery != "" {
					t.Errorf(
						"mutation query = %q, want empty",
						request.URL.RawQuery,
					)
				}
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}
				if !reflect.DeepEqual(
					request.Form,
					test.expectedMutation,
				) {
					t.Errorf(
						"mutation form = %#v, want %#v",
						request.Form,
						test.expectedMutation,
					)
				}
				writeCalendarJSON(t, response, map[string]any{
					"status": 1,
					"data":   map[string]any{},
				})
			}))
			defer server.Close()

			definition := Definition{
				Delegator: "owner@example.test",
				Delegatee: "delegate@example.test",
				Calendar:  DefaultCalendar,
				ReadOnly:  test.readOnly,
			}
			if err := test.call(
				t.Context(),
				newCalendarTestClient(t, server.URL),
				definition,
			); err != nil {
				t.Fatalf("mutation error: %v", err)
			}
			if mutationCalls.Load() != 1 {
				t.Fatalf(
					"mutation count = %d, want 1",
					mutationCalls.Load(),
				)
			}
		})
	}
}

func TestClientNeverReplaysAmbiguousMutation(t *testing.T) {
	t.Parallel()

	var ccsCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/CPDAVD/list_delegates":
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data":   []any{},
			})
		case "/execute/CPDAVD/add_delegate":
			writeMissingCPDAVDModule(t, response)
		default:
			if strings.HasPrefix(request.URL.Path, "/execute/CCS/") {
				ccsCalls.Add(1)
			}
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	err := newCalendarTestClient(t, server.URL).AddDelegate(
		t.Context(),
		Definition{
			Delegator: "owner@example.test",
			Delegatee: "delegate@example.test",
			Calendar:  DefaultCalendar,
		},
	)
	if err == nil {
		t.Fatal("AddDelegate() returned no error")
	}
	if ccsCalls.Load() != 0 {
		t.Fatalf("CCS mutation/retry count = %d, want 0", ccsCalls.Load())
	}

	var apiError *cpanel.APIError
	if !errors.As(err, &apiError) ||
		apiError.Module != cpanel.ModuleCPDAVD ||
		apiError.Function != operationAddDelegate {
		t.Fatalf("AddDelegate() error = %T %v", err, err)
	}
}

func TestClientNeverReplaysMutationTransportFailure(t *testing.T) {
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
			switch calls.Add(1) {
			case 1:
				if request.Method != http.MethodGet ||
					request.URL.Path !=
						"/execute/CPDAVD/list_delegates" {
					t.Fatalf(
						"probe request = %s %s",
						request.Method,
						request.URL.Path,
					)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(
						strings.NewReader(
							`{"status":1,"data":[]}`,
						),
					),
				}, nil
			case 2:
				if request.Method != http.MethodPost ||
					request.URL.Path !=
						"/execute/CPDAVD/add_delegate" {
					t.Fatalf(
						"mutation request = %s %s",
						request.Method,
						request.URL.Path,
					)
				}

				return nil, errors.New(
					"connection closed after request",
				)
			default:
				t.Fatalf(
					"unexpected replay request %s %s",
					request.Method,
					request.URL.Path,
				)

				return nil, errors.New("unexpected replay")
			}
		}),
	}

	err = NewClient(baseClient).AddDelegate(
		t.Context(),
		Definition{
			Delegator: "owner@example.test",
			Delegatee: "delegate@example.test",
			Calendar:  DefaultCalendar,
		},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "connection closed after request") {
		t.Fatalf("AddDelegate() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("transport call count = %d, want 2", calls.Load())
	}
}

func TestClientRejectsNonDefaultCalendarForCCSBeforeMutation(
	t *testing.T,
) {
	t.Parallel()

	var mutationCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/CPDAVD/list_delegates":
			writeMissingCPDAVDModule(t, response)
		case "/execute/CCS/list_delegates":
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data":   []any{},
			})
		default:
			mutationCalls.Add(1)
			t.Fatalf("unexpected mutation path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	err := newCalendarTestClient(t, server.URL).AddDelegate(
		t.Context(),
		Definition{
			Delegator: "owner@example.test",
			Delegatee: "delegate@example.test",
			Calendar:  "other-calendar",
		},
	)
	if err == nil ||
		!strings.Contains(err.Error(), `only the default calendar "calendar"`) {
		t.Fatalf("AddDelegate() error = %v", err)
	}
	if mutationCalls.Load() != 0 {
		t.Fatalf("mutation count = %d, want 0", mutationCalls.Load())
	}
}

func TestClientListsUsersAndRequiresVCALENDAR(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/CPDAVD/list_delegates":
			assertCalendarRequest(
				t,
				request,
				http.MethodGet,
				"/execute/CPDAVD/list_delegates",
			)
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data":   []any{},
			})
		case "/execute/CPDAVD/list_users":
			assertCalendarRequest(
				t,
				request,
				http.MethodGet,
				"/execute/CPDAVD/list_users",
			)
			writeCalendarJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"z@example.test": map[string]any{
						"tasks": map[string]any{
							"displayname": "Tasks",
							"type":        "VTODO",
						},
					},
					"owner@example.test": map[string]any{
						DefaultCalendar: map[string]any{
							"displayname": "Shared calendar",
							"type":        "VCALENDAR",
						},
						"addressbook": map[string]any{
							"displayname": "Contacts",
							"type":        "VADDRESSBOOK",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	client := newCalendarTestClient(t, server.URL)
	users, err := client.ListUsers(t.Context())
	if err != nil {
		t.Fatalf("ListUsers() error: %v", err)
	}
	want := []User{
		{
			Username: "owner@example.test",
			Collections: []Collection{
				{
					Name:        "addressbook",
					DisplayName: "Contacts",
					Type:        "VADDRESSBOOK",
				},
				{
					Name:        DefaultCalendar,
					DisplayName: "Shared calendar",
					Type:        "VCALENDAR",
				},
			},
		},
		{
			Username: "z@example.test",
			Collections: []Collection{{
				Name:        "tasks",
				DisplayName: "Tasks",
				Type:        "VTODO",
			}},
		},
	}
	if !reflect.DeepEqual(users, want) {
		t.Fatalf("ListUsers() = %#v, want %#v", users, want)
	}

	tests := []struct {
		username string
		calendar string
		want     bool
	}{
		{
			username: "owner@example.test",
			calendar: DefaultCalendar,
			want:     true,
		},
		{
			username: "owner@example.test",
			calendar: "addressbook",
			want:     false,
		},
		{
			username: "z@example.test",
			calendar: "tasks",
			want:     false,
		},
		{
			username: "missing@example.test",
			calendar: DefaultCalendar,
			want:     false,
		},
	}
	for _, test := range tests {
		exists, err := client.CalendarExists(
			t.Context(),
			test.username,
			test.calendar,
		)
		if err != nil {
			t.Fatalf(
				"CalendarExists(%q, %q) error: %v",
				test.username,
				test.calendar,
				err,
			)
		}
		if exists != test.want {
			t.Fatalf(
				"CalendarExists(%q, %q) = %t, want %t",
				test.username,
				test.calendar,
				exists,
				test.want,
			)
		}
	}
}

func TestClientRejectsInvalidDelegateInventories(t *testing.T) {
	t.Parallel()

	valid := map[string]any{
		"delegator": "owner@example.test",
		"delegatee": "delegate@example.test",
		"calendar":  DefaultCalendar,
		"calname":   "Shared calendar",
		"readonly":  1,
	}
	tests := map[string]struct {
		data    any
		wantErr string
	}{
		"null data": {
			data:    nil,
			wantErr: "missing or null data",
		},
		"object data": {
			data:    map[string]any{},
			wantErr: "data must be an array",
		},
		"missing delegator": {
			data: []map[string]any{{
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
				"readonly":  1,
			}},
			wantErr: "empty delegator",
		},
		"missing delegatee": {
			data: []map[string]any{{
				"delegator": "owner@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
				"readonly":  1,
			}},
			wantErr: "empty delegatee",
		},
		"missing CPDAVD calendar": {
			data: []map[string]any{{
				"delegator": "owner@example.test",
				"delegatee": "delegate@example.test",
				"calname":   "Shared calendar",
				"readonly":  1,
			}},
			wantErr: "empty calendar",
		},
		"missing calendar name": {
			data: []map[string]any{{
				"delegator": "owner@example.test",
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"readonly":  1,
			}},
			wantErr: "empty calendar name",
		},
		"missing readonly": {
			data: []map[string]any{{
				"delegator": "owner@example.test",
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
			}},
			wantErr: "readonly value",
		},
		"invalid readonly integer": {
			data: []map[string]any{{
				"delegator": "owner@example.test",
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
				"readonly":  2,
			}},
			wantErr: "expected 0 or 1",
		},
		"invalid readonly boolean": {
			data: []map[string]any{{
				"delegator": "owner@example.test",
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
				"readonly":  true,
			}},
			wantErr: "expected 0 or 1",
		},
		"surrounding whitespace": {
			data: []map[string]any{{
				"delegator": " owner@example.test ",
				"delegatee": "delegate@example.test",
				"calendar":  DefaultCalendar,
				"calname":   "Shared calendar",
				"readonly":  1,
			}},
			wantErr: "surrounding whitespace",
		},
		"duplicate identity": {
			data:    []map[string]any{valid, valid},
			wantErr: "duplicate calendar delegation",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeCalendarJSON(t, response, map[string]any{
					"status": 1,
					"data":   test.data,
				})
			}))
			defer server.Close()

			_, err := newCalendarTestClient(t, server.URL).
				ListDelegates(t.Context())
			if err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"ListDelegates() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsInvalidUserInventories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		data    string
		wantErr string
	}{
		"null data": {
			data:    `null`,
			wantErr: "missing or null",
		},
		"array data": {
			data:    `[]`,
			wantErr: "must be an object",
		},
		"duplicate user": {
			data: `{
				"owner@example.test": {},
				"owner@example.test": {}
			}`,
			wantErr: "duplicate key",
		},
		"collection inventory is array": {
			data:    `{"owner@example.test":[]}`,
			wantErr: "must be an object",
		},
		"duplicate collection": {
			data: `{
				"owner@example.test": {
					"calendar": {"type":"VCALENDAR"},
					"calendar": {"type":"VCALENDAR"}
				}
			}`,
			wantErr: "duplicate key",
		},
		"empty username": {
			data:    `{"":{}}`,
			wantErr: "empty calendar username",
		},
		"empty collection name": {
			data: `{
				"owner@example.test": {
					"": {"type":"VCALENDAR"}
				}
			}`,
			wantErr: "empty calendar collection name",
		},
		"missing collection type": {
			data: `{
				"owner@example.test": {
					"calendar": {"displayname":"Shared calendar"}
				}
			}`,
			wantErr: "empty calendar collection type",
		},
		"invalid collection value": {
			data: `{
				"owner@example.test": {
					"calendar": "VCALENDAR"
				}
			}`,
			wantErr: "decode calendar collection",
		},
		"whitespace display name": {
			data: `{
				"owner@example.test": {
					"calendar": {
						"displayname":" Shared calendar ",
						"type":"VCALENDAR"
					}
				}
			}`,
			wantErr: "surrounding whitespace",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/CPDAVD/list_delegates":
					writeCalendarJSON(t, response, map[string]any{
						"status": 1,
						"data":   []any{},
					})
				case "/execute/CPDAVD/list_users":
					_, _ = fmt.Fprintf(
						response,
						`{"status":1,"data":%s}`,
						test.data,
					)
				default:
					t.Fatalf(
						"unexpected request path %q",
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			_, err := newCalendarTestClient(t, server.URL).
				ListUsers(t.Context())
			if err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"ListUsers() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsInvalidLegacyCCSInventories(t *testing.T) {
	t.Parallel()

	t.Run("missing delegate read_only", func(t *testing.T) {
		t.Parallel()

		_, err := normalizeAPIDelegates(
			cpanel.ModuleCCS,
			json.RawMessage(`[{
				"delegator":"owner@example.test",
				"delegatee":"delegate@example.test"
			}]`),
		)
		if err == nil || !strings.Contains(err.Error(), "readonly value") {
			t.Fatalf(
				"normalizeAPIDelegates() error = %v, want readonly failure",
				err,
			)
		}
	})

	tests := map[string]struct {
		data    string
		wantErr string
	}{
		"non-string identifier": {
			data:    `{"owner@example.test":{"uuid":"unexpected"}}`,
			wantErr: "decode legacy CCS calendar user identifier",
		},
		"empty identifier": {
			data:    `{"owner@example.test":""}`,
			wantErr: "empty legacy CCS calendar user identifier",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := normalizeAPIUsers(
				cpanel.ModuleCCS,
				json.RawMessage(test.data),
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"normalizeAPIUsers() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientValidatesInputsBeforeRequests(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		_ http.ResponseWriter,
		_ *http.Request,
	) {
		calls.Add(1)
	}))
	defer server.Close()

	client := newCalendarTestClient(t, server.URL)
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "empty GetDelegate delegator",
			call: func() error {
				_, err := client.GetDelegate(
					t.Context(),
					"",
					DefaultCalendar,
					"delegate@example.test",
				)

				return err
			},
		},
		{
			name: "empty CalendarExists user",
			call: func() error {
				_, err := client.CalendarExists(
					t.Context(),
					"",
					DefaultCalendar,
				)

				return err
			},
		},
		{
			name: "whitespace AddDelegate calendar",
			call: func() error {
				return client.AddDelegate(t.Context(), Definition{
					Delegator: "owner@example.test",
					Delegatee: "delegate@example.test",
					Calendar:  " calendar ",
				})
			},
		},
		{
			name: "empty RemoveDelegate delegatee",
			call: func() error {
				return client.RemoveDelegate(
					t.Context(),
					"owner@example.test",
					DefaultCalendar,
					"",
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil {
				t.Fatal("call returned no error")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("request count = %d, want 0", calls.Load())
	}
}

func TestClientLocksDelegateSequencesPerIdentity(t *testing.T) {
	t.Parallel()

	client := &Client{
		delegateLocks: make(map[delegateIdentity]*delegateLock),
	}
	unlockFirst := client.LockDelegate(
		"owner@example.test",
		DefaultCalendar,
		"delegate@example.test",
	)

	sameIdentity := make(chan func(), 1)
	go func() {
		sameIdentity <- client.LockDelegate(
			"owner@example.test",
			DefaultCalendar,
			"delegate@example.test",
		)
	}()

	select {
	case unlock := <-sameIdentity:
		unlock()
		t.Fatal("same identity lock did not block")
	case <-time.After(50 * time.Millisecond):
	}

	otherIdentity := make(chan func(), 1)
	go func() {
		otherIdentity <- client.LockDelegate(
			"owner@example.test",
			DefaultCalendar,
			"other@example.test",
		)
	}()

	select {
	case unlock := <-otherIdentity:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("different identity lock blocked")
	}

	unlockFirst()
	unlockFirst()

	var unlockSame func()
	select {
	case unlockSame = <-sameIdentity:
	case <-time.After(time.Second):
		t.Fatal("same identity lock remained blocked")
	}
	unlockSame()

	client.delegateLocksMu.Lock()
	defer client.delegateLocksMu.Unlock()
	if len(client.delegateLocks) != 0 {
		t.Fatalf(
			"delegate lock registry contains %d entries",
			len(client.delegateLocks),
		)
	}
}

func TestModuleUnavailableClassificationIsStrict(t *testing.T) {
	t.Parallel()

	moduleError := func(
		module string,
		function string,
		message string,
	) error {
		return &cpanel.APIError{
			API:      "UAPI",
			Module:   module,
			Function: function,
			Messages: []string{message},
		}
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "failed to load exact module",
			err: moduleError(
				cpanel.ModuleCPDAVD,
				operationListDelegates,
				"Failed to load module “CPDAVD”.",
			),
			want: true,
		},
		{
			name: "missing exact module file",
			err: moduleError(
				cpanel.ModuleCPDAVD,
				operationListDelegates,
				"Can't locate Cpanel/API/CPDAVD.pm in @INC",
			),
			want: true,
		},
		{
			name: "generic API error",
			err: moduleError(
				cpanel.ModuleCPDAVD,
				operationListDelegates,
				"permission denied",
			),
			want: false,
		},
		{
			name: "different module mentioned",
			err: moduleError(
				cpanel.ModuleCPDAVD,
				operationListDelegates,
				"Failed to load module CCS.",
			),
			want: false,
		},
		{
			name: "different function",
			err: moduleError(
				cpanel.ModuleCPDAVD,
				operationListUsers,
				"Failed to load module CPDAVD.",
			),
			want: false,
		},
		{
			name: "transport error",
			err:  errors.New("network unavailable"),
			want: false,
		},
	}

	for _, test := range tests {
		if got := isModuleUnavailable(
			test.err,
			cpanel.ModuleCPDAVD,
			operationListDelegates,
		); got != test.want {
			t.Fatalf(
				"%s: isModuleUnavailable() = %t, want %t",
				test.name,
				got,
				test.want,
			)
		}
	}
}

func newCalendarTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func assertCalendarRequest(
	t *testing.T,
	request *http.Request,
	method string,
	path string,
) {
	t.Helper()

	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != path {
		t.Errorf("path = %s, want %s", request.URL.Path, path)
	}
	if method == http.MethodGet && request.URL.RawQuery != "" {
		t.Errorf("GET query = %q, want empty", request.URL.RawQuery)
	}
}

func writeCalendarJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func writeMissingCPDAVDModule(
	t *testing.T,
	response http.ResponseWriter,
) {
	t.Helper()

	writeCalendarJSON(t, response, map[string]any{
		"status": 0,
		"errors": []string{
			fmt.Sprintf(
				"Failed to load module %s: "+
					"Can't locate Cpanel/API/%s.pm in @INC",
				cpanel.ModuleCPDAVD,
				cpanel.ModuleCPDAVD,
			),
		},
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	return function(request)
}
