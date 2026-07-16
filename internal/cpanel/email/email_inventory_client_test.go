package email

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestClientListsAccountAddressesStrictly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertEmailInventoryRequest(
			t,
			request,
			"/execute/Email/list_pops",
			url.Values{
				"skip_main": {"1"},
			},
		)
		writeEmailInventoryJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"email":           "zeta@EXAMPLE.TEST.",
					"_diskquota":      52_428_800,
					"_diskused":       128,
					"suspended_login": 1,
				},
				{
					"email":  "Alpha@mail.example.test",
					"user":   "Alpha",
					"domain": "mail.example.test",
				},
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	addresses, err := client.ListAccountAddresses(context.Background())
	if err != nil {
		t.Fatalf("ListAccountAddresses() error: %v", err)
	}
	want := []string{
		"Alpha@mail.example.test",
		"zeta@example.test",
	}
	if !slices.Equal(addresses, want) {
		t.Fatalf("ListAccountAddresses() = %v, want %v", addresses, want)
	}
}

func TestClientListsMailDomainsStrictly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertEmailInventoryRequest(
			t,
			request,
			"/execute/Email/list_mail_domains",
			url.Values{},
		)
		writeEmailInventoryJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{"domain": "Z.EXAMPLE.TEST.", "select": 0},
				{"domain": "a.example.test", "select": 0},
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	domains, err := client.ListMailDomains(context.Background())
	if err != nil {
		t.Fatalf("ListMailDomains() error: %v", err)
	}
	want := []string{"z.example.test", "a.example.test"}
	if !slices.Equal(domains, want) {
		t.Fatalf("ListMailDomains() = %v, want %v", domains, want)
	}
	inventory, err := client.ListMailDomainInventory(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("ListMailDomainInventory() error: %v", err)
	}
	wantInventory := []string{"a.example.test", "z.example.test"}
	if !slices.Equal(inventory, wantInventory) {
		t.Fatalf(
			"ListMailDomainInventory() = %v, want %v",
			inventory,
			wantInventory,
		)
	}
}

func TestClientListsRoutingDefinitionsStrictly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertEmailInventoryRequest(
			t,
			request,
			"/execute/Email/list_mxs",
			url.Values{},
		)
		zeta := routingTestInventory(
			"Z.EXAMPLE.TEST.",
			RoutingModeBackup,
			RoutingModeBackup,
		)
		alpha := routingTestInventory(
			"a.example.test",
			RoutingModeRemote,
			RoutingModeRemote,
		)
		writeEmailInventoryJSON(t, response, map[string]any{
			"status": 1,
			"data":   []map[string]any{zeta, alpha},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	definitions, err := client.ListRoutingDefinitions(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("ListRoutingDefinitions() error: %v", err)
	}
	want := []RoutingDefinition{
		{Domain: "a.example.test", Mode: RoutingModeRemote},
		{Domain: "z.example.test", Mode: RoutingModeBackup},
	}
	if !reflect.DeepEqual(definitions, want) {
		t.Fatalf(
			"ListRoutingDefinitions() = %#v, want %#v",
			definitions,
			want,
		)
	}
}

func TestClientListsDomainForwardersStrictly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertEmailInventoryRequest(
			t,
			request,
			"/execute/Email/list_domain_forwarders",
			url.Values{},
		)
		writeEmailInventoryJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"dest":    "Z.EXAMPLE.TEST.",
					"forward": "TARGET.EXAMPLE.TEST.",
				},
				{
					"dest":    "a.example.test",
					"forward": "mail.example.test",
				},
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	forwarders, err := client.ListDomainForwarders(context.Background())
	if err != nil {
		t.Fatalf("ListDomainForwarders() error: %v", err)
	}
	want := []DomainForwarder{
		{
			Domain:      "a.example.test",
			Destination: "mail.example.test",
		},
		{
			Domain:      "z.example.test",
			Destination: "target.example.test",
		},
	}
	if !reflect.DeepEqual(forwarders, want) {
		t.Fatalf(
			"ListDomainForwarders() = %#v, want %#v",
			forwarders,
			want,
		)
	}
}

func TestClientListsMailingListAddressesStrictly(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertEmailInventoryRequest(
			t,
			request,
			"/execute/Email/list_lists",
			url.Values{},
		)
		writeEmailInventoryJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"list":          "z-list@EXAMPLE.TEST.",
					"listadmin":     "admin@example.test",
					"humandiskused": "42 MB",
					"diskused":      42,
				},
				{
					"list":       "A_list@mail.example.test",
					"accesstype": "private",
				},
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	addresses, err := client.ListMailingListAddresses(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("ListMailingListAddresses() error: %v", err)
	}
	want := []string{
		"A_list@mail.example.test",
		"z-list@example.test",
	}
	if !slices.Equal(addresses, want) {
		t.Fatalf(
			"ListMailingListAddresses() = %v, want %v",
			addresses,
			want,
		)
	}
}

func TestClientListsAutoResponderAddressesAcrossMailDomains(t *testing.T) {
	t.Parallel()

	calledDomains := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/list_mail_domains":
			assertEmailInventoryRequest(
				t,
				request,
				"/execute/Email/list_mail_domains",
				url.Values{},
			)
			writeEmailInventoryJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{
					{"domain": "Z.EXAMPLE.TEST."},
					{"domain": "a.example.test"},
				},
			})
		case "/execute/Email/list_auto_responders":
			domain := request.URL.Query().Get("domain")
			assertEmailInventoryRequest(
				t,
				request,
				"/execute/Email/list_auto_responders",
				url.Values{"domain": {domain}},
			)
			calledDomains = append(calledDomains, domain)
			switch domain {
			case "a.example.test":
				writeEmailInventoryJSON(t, response, map[string]any{
					"status": 1,
					"data": []map[string]any{
						{
							"email":   "Away@A.EXAMPLE.TEST.",
							"subject": "Away",
						},
					},
				})
			case "z.example.test":
				writeEmailInventoryJSON(t, response, map[string]any{
					"status": 1,
					"data": []map[string]any{
						{
							"email":   "zeta@z.example.test",
							"subject": "Back soon",
						},
					},
				})
			default:
				t.Errorf("unexpected autoresponder domain %q", domain)
				writeEmailInventoryJSON(t, response, map[string]any{
					"status": 1,
					"data":   []any{},
				})
			}
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	addresses, err := client.ListAutoResponderAddresses(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("ListAutoResponderAddresses() error: %v", err)
	}
	want := []string{
		"Away@a.example.test",
		"zeta@z.example.test",
	}
	if !slices.Equal(addresses, want) {
		t.Fatalf(
			"ListAutoResponderAddresses() = %v, want %v",
			addresses,
			want,
		)
	}
	wantDomains := []string{"a.example.test", "z.example.test"}
	if !slices.Equal(calledDomains, wantDomains) {
		t.Fatalf(
			"autoresponder domain calls = %v, want %v",
			calledDomains,
			wantDomains,
		)
	}
}

func TestClientAcceptsEmptyEmailInventories(t *testing.T) {
	t.Parallel()

	for _, testCase := range directEmailInventoryCalls() {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertEmailInventoryRequest(
					t,
					request,
					testCase.path,
					testCase.query,
				)
				_, _ = response.Write([]byte(`{"status":1,"data":[]}`))
			}))
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			length, err := testCase.call(client)
			if err != nil {
				t.Fatalf("%s error: %v", testCase.name, err)
			}
			if length != 0 {
				t.Fatalf("%s length = %d, want 0", testCase.name, length)
			}
		})
	}

	t.Run("autoresponders", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			assertEmailInventoryRequest(
				t,
				request,
				"/execute/Email/list_mail_domains",
				url.Values{},
			)
			_, _ = response.Write([]byte(`{"status":1,"data":[]}`))
		}))
		defer server.Close()

		client := newEmailTestClient(t, server.URL)
		addresses, err := client.ListAutoResponderAddresses(
			context.Background(),
		)
		if err != nil {
			t.Fatalf("ListAutoResponderAddresses() error: %v", err)
		}
		if len(addresses) != 0 {
			t.Fatalf(
				"ListAutoResponderAddresses() = %v, want empty",
				addresses,
			)
		}
	})
}

func TestClientRejectsMalformedEmailInventoryShapes(t *testing.T) {
	t.Parallel()

	shapes := []struct {
		name string
		body string
	}{
		{name: "missing data", body: `{"status":1}`},
		{name: "null data", body: `{"status":1,"data":null}`},
		{name: "object data", body: `{"status":1,"data":{}}`},
		{name: "null entry", body: `{"status":1,"data":[null]}`},
		{name: "array entry", body: `{"status":1,"data":[[]]}`},
	}

	for _, inventory := range directEmailInventoryCalls() {
		inventory := inventory
		for _, shape := range shapes {
			shape := shape
			t.Run(inventory.name+"/"+shape.name, func(t *testing.T) {
				t.Parallel()

				server := httptest.NewServer(http.HandlerFunc(func(
					response http.ResponseWriter,
					request *http.Request,
				) {
					assertEmailInventoryRequest(
						t,
						request,
						inventory.path,
						inventory.query,
					)
					_, _ = response.Write([]byte(shape.body))
				}))
				defer server.Close()

				client := newEmailTestClient(t, server.URL)
				if _, err := inventory.call(client); err == nil {
					t.Fatalf(
						"%s accepted %s",
						inventory.name,
						shape.name,
					)
				}
			})
		}
	}
}

func TestClientRejectsMalformedAutoResponderInventoryShapes(
	t *testing.T,
) {
	t.Parallel()

	shapes := []struct {
		name string
		body string
	}{
		{name: "missing data", body: `{"status":1}`},
		{name: "null data", body: `{"status":1,"data":null}`},
		{name: "object data", body: `{"status":1,"data":{}}`},
		{name: "null entry", body: `{"status":1,"data":[null]}`},
		{name: "array entry", body: `{"status":1,"data":[[]]}`},
	}

	for _, shape := range shapes {
		shape := shape
		t.Run(shape.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Email/list_mail_domains":
					_, _ = response.Write([]byte(
						`{"status":1,"data":[{"domain":"example.test"}]}`,
					))
				case "/execute/Email/list_auto_responders":
					assertEmailInventoryRequest(
						t,
						request,
						"/execute/Email/list_auto_responders",
						url.Values{"domain": {"example.test"}},
					)
					_, _ = response.Write([]byte(shape.body))
				default:
					t.Errorf("unexpected path %s", request.URL.Path)
					http.NotFound(response, request)
				}
			}))
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			if _, err := client.ListAutoResponderAddresses(
				context.Background(),
			); err == nil {
				t.Fatalf(
					"ListAutoResponderAddresses() accepted %s",
					shape.name,
				)
			}
		})
	}
}

func TestClientRejectsIncompleteEmailInventoryEntries(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		path    string
		query   url.Values
		item    string
		call    func(*Client) error
		wantErr string
	}{
		{
			name:    "account missing address",
			path:    "/execute/Email/list_pops",
			query:   accountInventoryQuery(),
			item:    `{}`,
			call:    callAccountAddressInventory,
			wantErr: "missing or null",
		},
		{
			name:    "account numeric address",
			path:    "/execute/Email/list_pops",
			query:   accountInventoryQuery(),
			item:    `{"email":123}`,
			call:    callAccountAddressInventory,
			wantErr: "expected a string",
		},
		{
			name:    "mail domain null domain",
			path:    "/execute/Email/list_mail_domains",
			query:   url.Values{},
			item:    `{"domain":null}`,
			call:    callMailDomainInventory,
			wantErr: "missing or null",
		},
		{
			name:    "mail domain numeric domain",
			path:    "/execute/Email/list_mail_domains",
			query:   url.Values{},
			item:    `{"domain":123}`,
			call:    callMailDomainInventory,
			wantErr: "expected a string",
		},
		{
			name:  "routing missing domain",
			path:  "/execute/Email/list_mxs",
			query: url.Values{},
			item: emailInventoryTestJSON(
				t,
				mutateRoutingInventory(func(item map[string]any) {
					delete(item, "domain")
				}),
			),
			call:    callRoutingDefinitionInventory,
			wantErr: "empty domain",
		},
		{
			name:  "routing missing mode",
			path:  "/execute/Email/list_mxs",
			query: url.Values{},
			item: emailInventoryTestJSON(
				t,
				mutateRoutingInventory(func(item map[string]any) {
					delete(item, "mxcheck")
				}),
			),
			call:    callRoutingDefinitionInventory,
			wantErr: "empty mxcheck",
		},
		{
			name:  "routing numeric mode",
			path:  "/execute/Email/list_mxs",
			query: url.Values{},
			item: emailInventoryTestJSON(
				t,
				mutateRoutingInventory(func(item map[string]any) {
					item["mxcheck"] = 1
				}),
			),
			call:    callRoutingDefinitionInventory,
			wantErr: "decode cPanel mxcheck",
		},
		{
			name:    "domain forwarder missing domain",
			path:    "/execute/Email/list_domain_forwarders",
			query:   url.Values{},
			item:    `{"forward":"target.test"}`,
			call:    callDomainForwarderInventory,
			wantErr: "missing or null",
		},
		{
			name:    "domain forwarder missing destination",
			path:    "/execute/Email/list_domain_forwarders",
			query:   url.Values{},
			item:    `{"dest":"example.test"}`,
			call:    callDomainForwarderInventory,
			wantErr: "missing or null",
		},
		{
			name:    "mailing list null address",
			path:    "/execute/Email/list_lists",
			query:   url.Values{},
			item:    `{"list":null}`,
			call:    callMailingListAddressInventory,
			wantErr: "missing or null",
		},
		{
			name:    "mailing list numeric address",
			path:    "/execute/Email/list_lists",
			query:   url.Values{},
			item:    `{"list":123}`,
			call:    callMailingListAddressInventory,
			wantErr: "expected a string",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertEmailInventoryRequest(
					t,
					request,
					testCase.path,
					testCase.query,
				)
				_, _ = fmt.Fprintf(
					response,
					`{"status":1,"data":[%s]}`,
					testCase.item,
				)
			}))
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			err := testCase.call(client)
			if err == nil {
				t.Fatalf("%s returned no error", testCase.name)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf(
					"%s error = %q, want substring %q",
					testCase.name,
					err,
					testCase.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsIncompleteAutoResponderEntries(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		item    string
		wantErr string
	}{
		{
			name:    "missing address",
			item:    `{}`,
			wantErr: "missing or null",
		},
		{
			name:    "null address",
			item:    `{"email":null}`,
			wantErr: "missing or null",
		},
		{
			name:    "numeric address",
			item:    `{"email":123}`,
			wantErr: "expected a string",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := autoResponderInventoryServer(
				t,
				fmt.Sprintf(
					`{"status":1,"data":[%s]}`,
					testCase.item,
				),
			)
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			_, err := client.ListAutoResponderAddresses(
				context.Background(),
			)
			if err == nil {
				t.Fatalf(
					"ListAutoResponderAddresses() accepted %s",
					testCase.name,
				)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf(
					"error = %q, want substring %q",
					err,
					testCase.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsDuplicateEmailInventoryEntries(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		path  string
		query url.Values
		data  string
		call  func(*Client) error
	}{
		{
			name:  "accounts",
			path:  "/execute/Email/list_pops",
			query: accountInventoryQuery(),
			data: `[{"email":"User@Example.TEST."},` +
				`{"email":"User@example.test"}]`,
			call: callAccountAddressInventory,
		},
		{
			name:  "mail domains",
			path:  "/execute/Email/list_mail_domains",
			query: url.Values{},
			data: `[{"domain":"Example.TEST."},` +
				`{"domain":"example.test"}]`,
			call: callMailDomainInventory,
		},
		{
			name:  "routings",
			path:  "/execute/Email/list_mxs",
			query: url.Values{},
			data: emailInventoryTestJSON(
				t,
				[]map[string]any{
					routingTestInventory(
						"Example.TEST.",
						RoutingModeLocal,
						RoutingModeLocal,
					),
					routingTestInventory(
						"example.test",
						RoutingModeRemote,
						RoutingModeRemote,
					),
				},
			),
			call: callRoutingDefinitionInventory,
		},
		{
			name:  "domain forwarders",
			path:  "/execute/Email/list_domain_forwarders",
			query: url.Values{},
			data: `[{"dest":"Example.TEST.","forward":"one.test"},` +
				`{"dest":"example.test","forward":"two.test"}]`,
			call: callDomainForwarderInventory,
		},
		{
			name:  "mailing lists",
			path:  "/execute/Email/list_lists",
			query: url.Values{},
			data: `[{"list":"List@Example.TEST."},` +
				`{"list":"List@example.test"}]`,
			call: callMailingListAddressInventory,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertEmailInventoryRequest(
					t,
					request,
					testCase.path,
					testCase.query,
				)
				_, _ = fmt.Fprintf(
					response,
					`{"status":1,"data":%s}`,
					testCase.data,
				)
			}))
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			err := testCase.call(client)
			if err == nil || !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf(
					"%s error = %v, want duplicate error",
					testCase.name,
					err,
				)
			}
		})
	}

	t.Run("autoresponders", func(t *testing.T) {
		t.Parallel()

		server := autoResponderInventoryServer(
			t,
			`{"status":1,"data":[`+
				`{"email":"Away@Example.TEST."},`+
				`{"email":"Away@example.test"}]}`,
		)
		defer server.Close()

		client := newEmailTestClient(t, server.URL)
		_, err := client.ListAutoResponderAddresses(
			context.Background(),
		)
		if err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("error = %v, want duplicate error", err)
		}
	})
}

func TestClientRejectsAutoResponderForAnotherDomain(t *testing.T) {
	t.Parallel()

	server := autoResponderInventoryServer(
		t,
		`{"status":1,"data":[{"email":"away@other.test"}]}`,
	)
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	_, err := client.ListAutoResponderAddresses(context.Background())
	if err == nil || !strings.Contains(err.Error(), "expected \"example.test\"") {
		t.Fatalf("error = %v, want domain mismatch error", err)
	}
}

func TestClientPropagatesEmailInventoryAPIErrors(t *testing.T) {
	t.Parallel()

	for _, inventory := range directEmailInventoryCalls() {
		inventory := inventory
		t.Run(inventory.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertEmailInventoryRequest(
					t,
					request,
					inventory.path,
					inventory.query,
				)
				_, _ = response.Write([]byte(
					`{"status":0,"errors":["rejected"]}`,
				))
			}))
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			if _, err := inventory.call(client); err == nil ||
				!strings.Contains(err.Error(), "rejected") {
				t.Fatalf("%s error = %v", inventory.name, err)
			}
		})
	}

	t.Run("autoresponder domain listing", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			assertEmailInventoryRequest(
				t,
				request,
				"/execute/Email/list_mail_domains",
				url.Values{},
			)
			_, _ = response.Write([]byte(
				`{"status":0,"errors":["domains rejected"]}`,
			))
		}))
		defer server.Close()

		client := newEmailTestClient(t, server.URL)
		_, err := client.ListAutoResponderAddresses(
			context.Background(),
		)
		if err == nil ||
			!strings.Contains(err.Error(), "domains rejected") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("autoresponder listing", func(t *testing.T) {
		t.Parallel()

		server := autoResponderInventoryServer(
			t,
			`{"status":0,"errors":["autoresponders rejected"]}`,
		)
		defer server.Close()

		client := newEmailTestClient(t, server.URL)
		_, err := client.ListAutoResponderAddresses(
			context.Background(),
		)
		if err == nil ||
			!strings.Contains(err.Error(), "autoresponders rejected") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestClientRejectsEmailInventoryWarnings(t *testing.T) {
	t.Parallel()

	for _, inventory := range directEmailInventoryCalls() {
		inventory := inventory
		t.Run(inventory.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertEmailInventoryRequest(
					t,
					request,
					inventory.path,
					inventory.query,
				)
				writeEmailInventoryJSON(t, response, map[string]any{
					"status":   1,
					"warnings": []string{"partial inventory"},
					"data":     []any{},
				})
			}))
			defer server.Close()

			client := newEmailTestClient(t, server.URL)
			if _, err := inventory.call(client); err == nil ||
				!strings.Contains(err.Error(), "partial inventory") {
				t.Fatalf("%s error = %v", inventory.name, err)
			}
		})
	}

	t.Run("autoresponder listing", func(t *testing.T) {
		t.Parallel()

		server := autoResponderInventoryServer(
			t,
			`{"status":1,"warnings":["partial autoresponder inventory"],"data":[]}`,
		)
		defer server.Close()

		client := newEmailTestClient(t, server.URL)
		_, err := client.ListAutoResponderAddresses(
			context.Background(),
		)
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"partial autoresponder inventory",
			) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestNormalizeEmailInventoryDomain(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		input string
		want  string
	}{
		{input: "Example.TEST.", want: "example.test"},
		{input: "MAIL.Example.TEST", want: "mail.example.test"},
		{
			input: "XN--bcher-kva.Example",
			want:  "xn--bcher-kva.example",
		},
	} {
		testCase := testCase
		t.Run(testCase.input, func(t *testing.T) {
			t.Parallel()

			actual, err := normalizeEmailInventoryDomain(testCase.input)
			if err != nil {
				t.Fatalf(
					"normalizeEmailInventoryDomain(%q) error: %v",
					testCase.input,
					err,
				)
			}
			if actual != testCase.want {
				t.Fatalf(
					"normalizeEmailInventoryDomain(%q) = %q, want %q",
					testCase.input,
					actual,
					testCase.want,
				)
			}
		})
	}

	longLabel := strings.Repeat("a", 64) + ".test"
	longDomain := strings.Repeat("a.", 126) + "test"
	for _, input := range []string{
		"",
		" example.test",
		"example.test ",
		"localhost",
		".example.test",
		"example..test",
		"example.test..",
		"-example.test",
		"example-.test",
		"éxample.test",
		longLabel,
		longDomain,
	} {
		input := input
		t.Run("invalid/"+input, func(t *testing.T) {
			t.Parallel()

			if _, err := normalizeEmailInventoryDomain(input); err == nil {
				t.Fatalf(
					"normalizeEmailInventoryDomain(%q) returned no error",
					input,
				)
			}
		})
	}
}

func TestNormalizeEmailInventoryAddress(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		input string
		want  string
	}{
		{
			input: "Local.Part@Example.TEST.",
			want:  "Local.Part@example.test",
		},
		{
			input: "_list-1@MAIL.Example.TEST",
			want:  "_list-1@mail.example.test",
		},
	} {
		testCase := testCase
		t.Run(testCase.input, func(t *testing.T) {
			t.Parallel()

			actual, err := normalizeEmailInventoryAddress(testCase.input)
			if err != nil {
				t.Fatalf(
					"normalizeEmailInventoryAddress(%q) error: %v",
					testCase.input,
					err,
				)
			}
			if actual != testCase.want {
				t.Fatalf(
					"normalizeEmailInventoryAddress(%q) = %q, want %q",
					testCase.input,
					actual,
					testCase.want,
				)
			}
		})
	}

	longLocalPart := strings.Repeat("a", 65) + "@example.test"
	longAddress := strings.Repeat("a", 64) + "@" +
		strings.Repeat("b.", 91) + "example.test"
	for _, input := range []string{
		"",
		" user@example.test",
		"user@example.test ",
		"user",
		"@example.test",
		"user@",
		"user@@example.test",
		".user@example.test",
		"user.@example.test",
		"user..name@example.test",
		"user+tag@example.test",
		"user@localhost",
		longLocalPart,
		longAddress,
	} {
		input := input
		t.Run("invalid/"+input, func(t *testing.T) {
			t.Parallel()

			if _, err := normalizeEmailInventoryAddress(input); err == nil {
				t.Fatalf(
					"normalizeEmailInventoryAddress(%q) returned no error",
					input,
				)
			}
		})
	}
}

type directEmailInventoryCall struct {
	name  string
	path  string
	query url.Values
	call  func(*Client) (int, error)
}

func directEmailInventoryCalls() []directEmailInventoryCall {
	return []directEmailInventoryCall{
		{
			name:  "accounts",
			path:  "/execute/Email/list_pops",
			query: accountInventoryQuery(),
			call: func(client *Client) (int, error) {
				values, err := client.ListAccountAddresses(
					context.Background(),
				)

				return len(values), err
			},
		},
		{
			name:  "mail domains",
			path:  "/execute/Email/list_mail_domains",
			query: url.Values{},
			call: func(client *Client) (int, error) {
				values, err := client.ListMailDomainInventory(
					context.Background(),
				)

				return len(values), err
			},
		},
		{
			name:  "routings",
			path:  "/execute/Email/list_mxs",
			query: url.Values{},
			call: func(client *Client) (int, error) {
				values, err := client.ListRoutingDefinitions(
					context.Background(),
				)

				return len(values), err
			},
		},
		{
			name:  "domain forwarders",
			path:  "/execute/Email/list_domain_forwarders",
			query: url.Values{},
			call: func(client *Client) (int, error) {
				values, err := client.ListDomainForwarders(
					context.Background(),
				)

				return len(values), err
			},
		},
		{
			name:  "mailing lists",
			path:  "/execute/Email/list_lists",
			query: url.Values{},
			call: func(client *Client) (int, error) {
				values, err := client.ListMailingListAddresses(
					context.Background(),
				)

				return len(values), err
			},
		},
	}
}

func callAccountAddressInventory(client *Client) error {
	_, err := client.ListAccountAddresses(context.Background())

	return err
}

func callMailDomainInventory(client *Client) error {
	_, err := client.ListMailDomainInventory(context.Background())

	return err
}

func callRoutingDefinitionInventory(client *Client) error {
	_, err := client.ListRoutingDefinitions(context.Background())

	return err
}

func callDomainForwarderInventory(client *Client) error {
	_, err := client.ListDomainForwarders(context.Background())

	return err
}

func callMailingListAddressInventory(client *Client) error {
	_, err := client.ListMailingListAddresses(context.Background())

	return err
}

func accountInventoryQuery() url.Values {
	return url.Values{
		"skip_main": {"1"},
	}
}

func emailInventoryTestJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode email inventory test value: %v", err)
	}

	return string(encoded)
}

func autoResponderInventoryServer(
	t *testing.T,
	autoResponderResponse string,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/list_mail_domains":
			assertEmailInventoryRequest(
				t,
				request,
				"/execute/Email/list_mail_domains",
				url.Values{},
			)
			_, _ = response.Write([]byte(
				`{"status":1,"data":[{"domain":"example.test"}]}`,
			))
		case "/execute/Email/list_auto_responders":
			assertEmailInventoryRequest(
				t,
				request,
				"/execute/Email/list_auto_responders",
				url.Values{"domain": {"example.test"}},
			)
			_, _ = response.Write([]byte(autoResponderResponse))
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			http.NotFound(response, request)
		}
	}))
}

func assertEmailInventoryRequest(
	t *testing.T,
	request *http.Request,
	path string,
	query url.Values,
) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	if request.URL.Path != path {
		t.Errorf("path = %s, want %s", request.URL.Path, path)
	}
	if actual := request.URL.Query(); !reflect.DeepEqual(actual, query) {
		t.Errorf("query = %v, want %v", actual, query)
	}
}

func writeEmailInventoryJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
