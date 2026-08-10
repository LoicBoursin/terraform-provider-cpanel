package email

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

const filterTestAccount = "user@example.test"

func TestClientListsUserEmailFilters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertFilterRequest(
			t,
			request,
			http.MethodGet,
			"/execute/Email/list_filters",
			url.Values{"account": {filterTestAccount}},
		)
		writeFilterJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				filterTestInventory(1),
				{
					"filtername": "disabled",
					"enabled":    "false",
					"rules": []map[string]any{{
						"part":  "$message_body",
						"match": "contains",
						"val":   "blocked",
						"opt":   nil,
					}},
					"actions": []map[string]any{{
						"action": "fail",
						"dest":   nil,
					}},
				},
			},
		})
	}))
	defer server.Close()

	filters, err := newFilterTestClient(t, server.URL).ListFilters(
		t.Context(),
		filterTestAccount,
	)
	if err != nil {
		t.Fatalf("ListFilters() error: %v", err)
	}

	want := []Filter{
		filterTestDefinition(filterTestAccount, true),
		{
			Account: filterTestAccount,
			Name:    "disabled",
			Enabled: false,
			Rules: []FilterRule{{
				Part:  "$message_body",
				Match: "contains",
				Value: "blocked",
			}},
			Actions: []FilterAction{{
				Action: "fail",
			}},
		},
	}
	if !reflect.DeepEqual(filters, want) {
		t.Fatalf("ListFilters() = %#v, want %#v", filters, want)
	}
}

func TestClientListsAccountEmailFiltersWithoutAccountParameter(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertFilterRequest(
			t,
			request,
			http.MethodGet,
			"/execute/Email/list_filters",
			url.Values{},
		)
		writeFilterJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				filterTestInventory(1),
			},
		})
	}))
	defer server.Close()

	filters, err := newFilterTestClient(t, server.URL).ListAccountFilters(
		t.Context(),
	)
	if err != nil {
		t.Fatalf("ListAccountFilters() error: %v", err)
	}
	want := []Filter{filterTestDefinition("username", true)}
	if !reflect.DeepEqual(filters, want) {
		t.Fatalf("ListAccountFilters() = %#v, want %#v", filters, want)
	}
}

func TestClientListFiltersRequiresAccount(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		called.Store(true)
	}))
	defer server.Close()

	if _, err := newFilterTestClient(t, server.URL).ListFilters(
		t.Context(),
		"",
	); err == nil {
		t.Fatal("ListFilters() returned no error")
	}
	if called.Load() {
		t.Fatal("ListFilters() sent a request for an empty account")
	}
}

func TestClientDecodesFilterEnabledStrictly(t *testing.T) {
	t.Parallel()

	accepted := []struct {
		name    string
		value   any
		enabled bool
	}{
		{name: "integer zero", value: 0, enabled: false},
		{name: "integer one", value: 1, enabled: true},
		{name: "boolean false", value: false, enabled: false},
		{name: "boolean true", value: true, enabled: true},
		{name: "string zero", value: "0", enabled: false},
		{name: "string one", value: "1", enabled: true},
		{name: "string false", value: "false", enabled: false},
		{name: "string true", value: "true", enabled: true},
	}
	for _, testCase := range accepted {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeFilterJSON(t, response, map[string]any{
					"status": 1,
					"data": []map[string]any{
						filterTestInventory(testCase.value),
					},
				})
			}))
			defer server.Close()

			filters, err := newFilterTestClient(t, server.URL).ListFilters(
				t.Context(),
				filterTestAccount,
			)
			if err != nil {
				t.Fatalf("ListFilters() error: %v", err)
			}
			if len(filters) != 1 || filters[0].Enabled != testCase.enabled {
				t.Fatalf("filters = %#v", filters)
			}
		})
	}

	rejected := []struct {
		name    string
		value   any
		missing bool
	}{
		{name: "missing", missing: true},
		{name: "null", value: nil},
		{name: "integer two", value: 2},
		{name: "negative integer", value: -1},
		{name: "decimal zero", value: json.RawMessage(`0.0`)},
		{name: "unsupported string", value: "yes"},
		{name: "uppercase string", value: "TRUE"},
		{name: "array", value: []any{}},
		{name: "object", value: map[string]any{}},
	}
	for _, testCase := range rejected {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			inventory := filterTestInventory(testCase.value)
			if testCase.missing {
				delete(inventory, "enabled")
			}
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeFilterJSON(t, response, map[string]any{
					"status": 1,
					"data":   []map[string]any{inventory},
				})
			}))
			defer server.Close()

			if _, err := newFilterTestClient(t, server.URL).ListFilters(
				t.Context(),
				filterTestAccount,
			); err == nil {
				t.Fatal("ListFilters() returned no error")
			}
		})
	}
}

func TestClientRejectsInvalidFilterInventory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		change func(map[string]any) []map[string]any
	}{
		{
			name: "empty name",
			change: func(filter map[string]any) []map[string]any {
				filter["filtername"] = ""

				return []map[string]any{filter}
			},
		},
		{
			name: "missing rules",
			change: func(filter map[string]any) []map[string]any {
				delete(filter, "rules")

				return []map[string]any{filter}
			},
		},
		{
			name: "empty rules",
			change: func(filter map[string]any) []map[string]any {
				filter["rules"] = []map[string]any{}

				return []map[string]any{filter}
			},
		},
		{
			name: "empty rule part",
			change: func(filter map[string]any) []map[string]any {
				filterRules(filter)[0]["part"] = ""

				return []map[string]any{filter}
			},
		},
		{
			name: "empty rule match",
			change: func(filter map[string]any) []map[string]any {
				filterRules(filter)[0]["match"] = ""

				return []map[string]any{filter}
			},
		},
		{
			name: "empty rule value",
			change: func(filter map[string]any) []map[string]any {
				filterRules(filter)[0]["val"] = ""

				return []map[string]any{filter}
			},
		},
		{
			name: "non string rule opt",
			change: func(filter map[string]any) []map[string]any {
				filterRules(filter)[0]["opt"] = 1

				return []map[string]any{filter}
			},
		},
		{
			name: "missing actions",
			change: func(filter map[string]any) []map[string]any {
				delete(filter, "actions")

				return []map[string]any{filter}
			},
		},
		{
			name: "empty actions",
			change: func(filter map[string]any) []map[string]any {
				filter["actions"] = []map[string]any{}

				return []map[string]any{filter}
			},
		},
		{
			name: "empty action",
			change: func(filter map[string]any) []map[string]any {
				filterActions(filter)[0]["action"] = ""

				return []map[string]any{filter}
			},
		},
		{
			name: "non string action dest",
			change: func(filter map[string]any) []map[string]any {
				filterActions(filter)[0]["dest"] = false

				return []map[string]any{filter}
			},
		},
		{
			name: "duplicate name",
			change: func(filter map[string]any) []map[string]any {
				return []map[string]any{
					filter,
					filterTestInventory(false),
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data := testCase.change(filterTestInventory(true))
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeFilterJSON(t, response, map[string]any{
					"status": 1,
					"data":   data,
				})
			}))
			defer server.Close()

			if _, err := newFilterTestClient(t, server.URL).ListFilters(
				t.Context(),
				filterTestAccount,
			); err == nil {
				t.Fatal("ListFilters() returned no error")
			}
		})
	}
}

func TestParseNullableFilterString(t *testing.T) {
	t.Parallel()

	accepted := []struct {
		name              string
		raw               json.RawMessage
		nullStringIsEmpty bool
		want              string
	}{
		{name: "missing"},
		{name: "null", raw: json.RawMessage(`null`)},
		{name: "empty string", raw: json.RawMessage(`""`)},
		{
			name:              "null sentinel string",
			raw:               json.RawMessage(`"null"`),
			nullStringIsEmpty: true,
		},
		{
			name: "literal null string",
			raw:  json.RawMessage(`"null"`),
			want: "null",
		},
		{
			name: "ordinary string",
			raw:  json.RawMessage(`"archive@example.test"`),
			want: "archive@example.test",
		},
	}
	for _, testCase := range accepted {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseNullableFilterString(
				testCase.raw,
				testCase.nullStringIsEmpty,
			)
			if err != nil {
				t.Fatalf("parseNullableFilterString() error: %v", err)
			}
			if got != testCase.want {
				t.Fatalf(
					"parseNullableFilterString() = %q, want %q",
					got,
					testCase.want,
				)
			}
		})
	}

	for _, raw := range []json.RawMessage{
		json.RawMessage(`0`),
		json.RawMessage(`false`),
		json.RawMessage(`[]`),
		json.RawMessage(`{}`),
	} {
		if _, err := parseNullableFilterString(raw, false); err == nil {
			t.Fatalf(
				"parseNullableFilterString(%s) returned no error",
				raw,
			)
		}
	}
}

func TestNormalizeFilterActionHandlesLiteralNullByAction(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		action string
		want   string
	}{
		{name: "deliver", action: "deliver", want: "null"},
		{name: "fail", action: "fail", want: "null"},
		{name: "save", action: "save", want: "null"},
		{name: "pipe", action: "pipe", want: "null"},
		{name: "finish", action: "finish"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			action, err := normalizeFilterAction(
				filterTestAccount,
				"literal-null",
				1,
				testCase.action,
				json.RawMessage(`"null"`),
			)
			if err != nil {
				t.Fatalf("normalizeFilterAction() error: %v", err)
			}
			if action.Destination != testCase.want {
				t.Fatalf(
					"normalizeFilterAction() destination = %q, want %q",
					action.Destination,
					testCase.want,
				)
			}
		})
	}
}

func TestNormalizeSaveFilterDestination(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		account     string
		destination string
		want        string
	}{
		{
			name:        "cpanel home placeholder",
			account:     "user@example.test",
			destination: "$home/mail/example.test/user/.Terraform",
			want:        "/.Terraform",
		},
		{
			name:        "absolute home path",
			account:     "user@example.test",
			destination: "/home/cpanel/mail/example.test/user/.Terraform",
			want:        "/home/cpanel/mail/example.test/user/.Terraform",
		},
		{
			name:        "unrelated absolute path",
			account:     "user@example.test",
			destination: "/tmp/archive/mail/example.test/user/.Terraform",
			want:        "/tmp/archive/mail/example.test/user/.Terraform",
		},
		{
			name:        "already logical",
			account:     "user@example.test",
			destination: "/.Terraform",
			want:        "/.Terraform",
		},
		{
			name:        "different mailbox",
			account:     "user@example.test",
			destination: "$home/mail/example.test/other/.Terraform",
			want:        "$home/mail/example.test/other/.Terraform",
		},
		{
			name:        "relative destination",
			account:     "user@example.test",
			destination: ".Terraform",
			want:        ".Terraform",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeSaveFilterDestination(
				testCase.account,
				testCase.destination,
			)
			if got != testCase.want {
				t.Fatalf(
					"normalizeSaveFilterDestination() = %q, want %q",
					got,
					testCase.want,
				)
			}
		})
	}
}

func TestNormalizeMatchlessFilterRules(t *testing.T) {
	t.Parallel()

	for _, part := range []string{"not delivered", "error_message"} {
		rule, err := normalizeFilterRule(
			"special",
			1,
			part,
			"",
			"",
			json.RawMessage(`null`),
		)
		if err != nil {
			t.Fatalf("normalizeFilterRule(%q) error: %v", part, err)
		}
		if rule != (FilterRule{Part: part}) {
			t.Fatalf("normalizeFilterRule(%q) = %#v", part, rule)
		}
	}

	if _, err := normalizeFilterRule(
		"special",
		1,
		"not delivered",
		"",
		"unexpected",
		json.RawMessage(`null`),
	); err == nil {
		t.Fatal("normalizeFilterRule() accepted a value without a match")
	}
	if _, err := normalizeFilterRule(
		"special",
		1,
		"error_message",
		"contains",
		"",
		json.RawMessage(`null`),
	); err == nil {
		t.Fatal("normalizeFilterRule() accepted a match without a value")
	}
}

func TestClientLocksEmailFilterSequencesPerAccount(t *testing.T) {
	t.Parallel()

	client := &Client{filterLocks: make(map[string]*filterAccountLock)}
	unlockFirst := client.LockFilterAccount("first@example.test")

	sameAccount := make(chan func(), 1)
	go func() {
		sameAccount <- client.LockFilterAccount("first@example.test")
	}()

	select {
	case unlock := <-sameAccount:
		unlock()
		t.Fatal("second lock for the same account did not wait")
	case <-time.After(50 * time.Millisecond):
	}

	otherAccount := make(chan func(), 1)
	go func() {
		otherAccount <- client.LockFilterAccount("other@example.test")
	}()

	select {
	case unlock := <-otherAccount:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("lock for a different account was blocked")
	}

	unlockFirst()
	unlockFirst()

	select {
	case unlock := <-sameAccount:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second lock for the same account stayed blocked")
	}

	client.filterLocksMu.Lock()
	defer client.filterLocksMu.Unlock()
	if len(client.filterLocks) != 0 {
		t.Fatalf("filter lock count = %d, want 0", len(client.filterLocks))
	}
}

func TestClientGetsExactUserEmailFilter(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		call := calls.Add(1)
		switch request.URL.Path {
		case "/execute/Email/list_filters":
			if call != 1 {
				t.Errorf("list_filters call = %d, want 1", call)
			}
			assertFilterRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Email/list_filters",
				url.Values{"account": {filterTestAccount}},
			)
			writeFilterJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{
					filterTestInventory(true),
					{
						"filtername": "other",
						"enabled":    0,
						"rules": []map[string]any{{
							"part":  "$message_body",
							"match": "contains",
							"val":   "other",
							"opt":   nil,
						}},
						"actions": []map[string]any{{
							"action": "finish",
							"dest":   nil,
						}},
					},
				},
			})
		case "/execute/Email/get_filter":
			if call != 2 {
				t.Errorf("get_filter call = %d, want 2", call)
			}
			assertFilterRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Email/get_filter",
				url.Values{
					"account":    {filterTestAccount},
					"filtername": {"routing"},
				},
			)
			writeFilterJSON(t, response, map[string]any{
				"status": 1,
				"data":   filterTestDetailOutOfOrder(),
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	filter, err := newFilterTestClient(t, server.URL).GetFilter(
		t.Context(),
		filterTestAccount,
		"routing",
	)
	if err != nil {
		t.Fatalf("GetFilter() error: %v", err)
	}
	want := filterTestDefinition(filterTestAccount, true)
	if filter == nil || !reflect.DeepEqual(*filter, want) {
		t.Fatalf("GetFilter() = %#v, want %#v", filter, want)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestClientGetsExactAccountEmailFilterWithoutAccountParameter(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/list_filters":
			calls.Add(1)
			assertFilterRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Email/list_filters",
				url.Values{},
			)
			writeFilterJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{
					filterTestInventory(true),
				},
			})
		case "/execute/Email/get_filter":
			calls.Add(1)
			assertFilterRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Email/get_filter",
				url.Values{"filtername": {"routing"}},
			)
			writeFilterJSON(t, response, map[string]any{
				"status": 1,
				"data":   filterTestDetailOutOfOrder(),
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	filter, err := newFilterTestClient(t, server.URL).GetAccountFilter(
		t.Context(),
		"routing",
	)
	if err != nil {
		t.Fatalf("GetAccountFilter() error: %v", err)
	}
	want := filterTestDefinition("username", true)
	if filter == nil || !reflect.DeepEqual(*filter, want) {
		t.Fatalf("GetAccountFilter() = %#v, want %#v", filter, want)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestClientSkipsFilterDetailWhenMissingFromInventory(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		calls.Add(1)
		if request.URL.Path != "/execute/Email/list_filters" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		writeFilterJSON(t, response, map[string]any{
			"status": 1,
			"data":   []map[string]any{},
		})
	}))
	defer server.Close()

	filter, err := newFilterTestClient(t, server.URL).GetFilter(
		t.Context(),
		filterTestAccount,
		"missing",
	)
	if err != nil {
		t.Fatalf("GetFilter() error: %v", err)
	}
	if filter != nil {
		t.Fatalf("GetFilter() = %#v, want nil", filter)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestClientRejectsInvalidFilterDetail(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		change func(map[string]any)
	}{
		{
			name: "different name",
			change: func(detail map[string]any) {
				detail["filtername"] = "other"
			},
		},
		{
			name: "rule differs from inventory",
			change: func(detail map[string]any) {
				detailRules(detail)[0]["val"] = "different"
			},
		},
		{
			name: "action differs from inventory",
			change: func(detail map[string]any) {
				detailActions(detail)[0]["dest"] = "different@example.test"
			},
		},
		{
			name: "non consecutive rule numbers",
			change: func(detail map[string]any) {
				detailRules(detail)[1]["number"] = 3
			},
		},
		{
			name: "missing rule number",
			change: func(detail map[string]any) {
				delete(detailRules(detail)[0], "number")
			},
		},
		{
			name: "non consecutive action numbers",
			change: func(detail map[string]any) {
				detailActions(detail)[2]["number"] = 4
			},
		},
		{
			name: "duplicate action number",
			change: func(detail map[string]any) {
				detailActions(detail)[1]["number"] = 1
			},
		},
		{
			name: "non string rule opt",
			change: func(detail map[string]any) {
				detailRules(detail)[0]["opt"] = true
			},
		},
		{
			name: "non string action dest",
			change: func(detail map[string]any) {
				detailActions(detail)[0]["dest"] = 42
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detail := filterTestDetail()
			testCase.change(detail)
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Email/list_filters":
					writeFilterJSON(t, response, map[string]any{
						"status": 1,
						"data": []map[string]any{
							filterTestInventory(true),
						},
					})
				case "/execute/Email/get_filter":
					writeFilterJSON(t, response, map[string]any{
						"status": 1,
						"data":   detail,
					})
				default:
					t.Errorf("unexpected path %s", request.URL.Path)
				}
			}))
			defer server.Close()

			if _, err := newFilterTestClient(t, server.URL).GetFilter(
				t.Context(),
				filterTestAccount,
				"routing",
			); err == nil {
				t.Fatal("GetFilter() returned no error")
			}
		})
	}
}

func TestClientStoresUserEmailFilterWithPOSTForm(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		oldName string
	}{
		{name: "create"},
		{name: "rename", oldName: "old-routing"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			const sensitiveValue = "password-&=?"
			filter := filterTestDefinition("", false)
			filter.Rules[0].Value = sensitiveValue

			expectedForm := url.Values{
				"account":    {filterTestAccount},
				"filtername": {"routing"},
				"part1":      {"$header_from:"},
				"match1":     {"contains"},
				"val1":       {sensitiveValue},
				"opt1":       {"and"},
				"part2":      {"$header_subject:"},
				"match2":     {"begins"},
				"val2":       {"urgent"},
				"action1":    {"deliver"},
				"dest1":      {"archive@example.test"},
				"action2":    {"save"},
				"dest2":      {"/home/user/mail"},
				"action3":    {"finish"},
			}
			if testCase.oldName != "" {
				expectedForm.Set("oldfiltername", testCase.oldName)
			}

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if strings.Contains(request.URL.String(), sensitiveValue) {
					t.Errorf(
						"request URL contains sensitive filter value: %s",
						request.URL.Redacted(),
					)
				}
				assertFilterRequest(
					t,
					request,
					http.MethodPost,
					"/execute/Email/store_filter",
					expectedForm,
				)
				if request.Header.Get("Content-Type") !=
					"application/x-www-form-urlencoded" {
					t.Errorf(
						"Content-Type = %q",
						request.Header.Get("Content-Type"),
					)
				}
				writeFilterJSON(t, response, map[string]any{
					"status": 1,
					"data":   nil,
				})
			}))
			defer server.Close()

			if err := newFilterTestClient(t, server.URL).StoreFilter(
				t.Context(),
				filterTestAccount,
				testCase.oldName,
				filter,
			); err != nil {
				t.Fatalf("StoreFilter() error: %v", err)
			}
		})
	}
}

func TestClientStoresAccountEmailFilterWithoutAccountParameter(t *testing.T) {
	t.Parallel()

	filter := filterTestDefinition("username", false)
	expectedForm := url.Values{
		"filtername": {"routing"},
		"part1":      {"$header_from:"},
		"match1":     {"contains"},
		"val1":       {"sender@example.test"},
		"opt1":       {"and"},
		"part2":      {"$header_subject:"},
		"match2":     {"begins"},
		"val2":       {"urgent"},
		"action1":    {"deliver"},
		"dest1":      {"archive@example.test"},
		"action2":    {"save"},
		"dest2":      {"/home/user/mail"},
		"action3":    {"finish"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertFilterRequest(
			t,
			request,
			http.MethodPost,
			"/execute/Email/store_filter",
			expectedForm,
		)
		writeFilterJSON(t, response, map[string]any{
			"status": 1,
			"data":   nil,
		})
	}))
	defer server.Close()

	if err := newFilterTestClient(t, server.URL).StoreAccountFilter(
		t.Context(),
		"",
		filter,
	); err != nil {
		t.Fatalf("StoreAccountFilter() error: %v", err)
	}
}

func TestClientMutatesUserEmailFiltersWithPOST(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		mutation func(context.Context, *Client) error
	}{
		{
			name: "enable",
			path: "/execute/Email/enable_filter",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetFilterEnabled(
					ctx,
					filterTestAccount,
					"routing",
					true,
				)
			},
		},
		{
			name: "disable",
			path: "/execute/Email/disable_filter",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetFilterEnabled(
					ctx,
					filterTestAccount,
					"routing",
					false,
				)
			},
		},
		{
			name: "delete",
			path: "/execute/Email/delete_filter",
			mutation: func(ctx context.Context, client *Client) error {
				return client.DeleteFilter(
					ctx,
					filterTestAccount,
					"routing",
				)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertFilterRequest(
					t,
					request,
					http.MethodPost,
					testCase.path,
					url.Values{
						"account":    {filterTestAccount},
						"filtername": {"routing"},
					},
				)
				writeFilterJSON(t, response, map[string]any{
					"status": 1,
					"data":   nil,
				})
			}))
			defer server.Close()

			if err := testCase.mutation(
				t.Context(),
				newFilterTestClient(t, server.URL),
			); err != nil {
				t.Fatalf("mutation error: %v", err)
			}
		})
	}
}

func TestClientMutatesAccountEmailFiltersWithoutAccountParameter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		mutation func(context.Context, *Client) error
	}{
		{
			name: "enable",
			path: "/execute/Email/enable_filter",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetAccountFilterEnabled(ctx, "routing", true)
			},
		},
		{
			name: "disable",
			path: "/execute/Email/disable_filter",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetAccountFilterEnabled(ctx, "routing", false)
			},
		},
		{
			name: "delete",
			path: "/execute/Email/delete_filter",
			mutation: func(ctx context.Context, client *Client) error {
				return client.DeleteAccountFilter(ctx, "routing")
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertFilterRequest(
					t,
					request,
					http.MethodPost,
					testCase.path,
					url.Values{"filtername": {"routing"}},
				)
				writeFilterJSON(t, response, map[string]any{
					"status": 1,
					"data":   nil,
				})
			}))
			defer server.Close()

			if err := testCase.mutation(
				t.Context(),
				newFilterTestClient(t, server.URL),
			); err != nil {
				t.Fatalf("mutation error: %v", err)
			}
		})
	}
}

func filterTestDefinition(account string, enabled bool) Filter {
	return Filter{
		Account: account,
		Name:    "routing",
		Enabled: enabled,
		Rules: []FilterRule{
			{
				Part:     "$header_from:",
				Match:    "contains",
				Value:    "sender@example.test",
				Operator: "and",
			},
			{
				Part:  "$header_subject:",
				Match: "begins",
				Value: "urgent",
			},
		},
		Actions: []FilterAction{
			{
				Action:      "deliver",
				Destination: "archive@example.test",
			},
			{
				Action:      "save",
				Destination: "/home/user/mail",
			},
			{
				Action: "finish",
			},
		},
	}
}

func filterTestInventory(enabled any) map[string]any {
	return map[string]any{
		"filtername": "routing",
		"enabled":    enabled,
		"rules": []map[string]any{
			{
				"part":  "$header_from:",
				"match": "contains",
				"val":   "sender@example.test",
				"opt":   "and",
			},
			{
				"part":  "$header_subject:",
				"match": "begins",
				"val":   "urgent",
				"opt":   nil,
			},
		},
		"actions": []map[string]any{
			{
				"action": "deliver",
				"dest":   "archive@example.test",
			},
			{
				"action": "save",
				"dest":   "/home/user/mail",
			},
			{
				"action": "finish",
				"dest":   nil,
			},
		},
	}
}

func filterTestDetail() map[string]any {
	return map[string]any{
		"filtername": "routing",
		"rules": []map[string]any{
			{
				"number": 1,
				"part":   "$header_from:",
				"match":  "contains",
				"val":    "sender@example.test",
				"opt":    "and",
			},
			{
				"number": 2,
				"part":   "$header_subject:",
				"match":  "begins",
				"val":    "urgent",
				"opt":    nil,
			},
		},
		"actions": []map[string]any{
			{
				"number": 1,
				"action": "deliver",
				"dest":   "archive@example.test",
			},
			{
				"number": 2,
				"action": "save",
				"dest":   "/home/user/mail",
			},
			{
				"number": 3,
				"action": "finish",
				"dest":   nil,
			},
		},
	}
}

func filterTestDetailOutOfOrder() map[string]any {
	detail := filterTestDetail()
	rules := detailRules(detail)
	detail["rules"] = []map[string]any{rules[1], rules[0]}
	actions := detailActions(detail)
	detail["actions"] = []map[string]any{
		actions[2],
		actions[0],
		actions[1],
	}

	return detail
}

func filterRules(filter map[string]any) []map[string]any {
	return filterTestCollection(filter, "rules")
}

func filterActions(filter map[string]any) []map[string]any {
	return filterTestCollection(filter, "actions")
}

func detailRules(detail map[string]any) []map[string]any {
	return filterTestCollection(detail, "rules")
}

func detailActions(detail map[string]any) []map[string]any {
	return filterTestCollection(detail, "actions")
}

func filterTestCollection(
	value map[string]any,
	key string,
) []map[string]any {
	collection, ok := value[key].([]map[string]any)
	if !ok {
		panic(fmt.Sprintf("%q is not a filter collection", key))
	}

	return collection
}

func newFilterTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func assertFilterRequest(
	t *testing.T,
	request *http.Request,
	method string,
	requestPath string,
	expectedValues url.Values,
) {
	t.Helper()

	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != requestPath {
		t.Errorf("path = %s, want %s", request.URL.Path, requestPath)
	}

	var values url.Values
	switch method {
	case http.MethodGet:
		values = request.URL.Query()
	case http.MethodPost:
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		values = request.Form
	default:
		t.Fatalf("unsupported test method %q", method)
	}
	if !reflect.DeepEqual(values, expectedValues) {
		t.Errorf("values = %#v, want %#v", values, expectedValues)
	}
}

func writeFilterJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
