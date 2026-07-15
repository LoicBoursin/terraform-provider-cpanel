package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsMailingListsWithGET(t *testing.T) {
	t.Parallel()

	publicList := mailingListTestInventory(
		"public@example.test",
		"public",
		1,
	)
	publicList["listadmin"] = "z@example.test, alpha@example.test, beta@example.test"
	publicList["humandiskused"] = "19.9\u00a0KB"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Email/list_lists" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("domain"); got != "example.test" {
			t.Errorf("domain = %q, want example.test", got)
		}

		writeMailingListJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				publicList,
				mailingListTestInventory(
					"private-confirm@example.test",
					"private",
					3,
				),
				mailingListTestInventory(
					"private-no-confirm@example.test",
					"private",
					2,
				),
			},
		})
	}))
	defer server.Close()

	mailingLists, err := newEmailTestClient(t, server.URL).ListMailingLists(
		t.Context(),
		"example.test",
	)
	if err != nil {
		t.Fatalf("ListMailingLists() error: %v", err)
	}

	want := []MailingList{
		{
			Address:         "public@example.test",
			ID:              "public_example.test",
			Advertised:      true,
			SubscribePolicy: 1,
			Administrators: []string{
				"alpha@example.test",
				"beta@example.test",
				"z@example.test",
			},
			HumanDiskUsed: "19.9\u00a0KB",
		},
		{
			Address:         "private-confirm@example.test",
			ID:              "private-confirm_example.test",
			Private:         true,
			ArchivePrivate:  true,
			SubscribePolicy: 3,
			Administrators:  []string{"admin@example.test"},
			HumanDiskUsed:   "0 bytes",
		},
		{
			Address:         "private-no-confirm@example.test",
			ID:              "private-no-confirm_example.test",
			Private:         true,
			ArchivePrivate:  true,
			SubscribePolicy: 2,
			Administrators:  []string{"admin@example.test"},
			HumanDiskUsed:   "0 bytes",
		},
	}
	if !reflect.DeepEqual(mailingLists, want) {
		t.Fatalf("mailing lists = %#v, want %#v", mailingLists, want)
	}
}

func TestClientAcceptsMixedPublicMailingListAccessOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		advertised      int
		archivePrivate  int
		subscribePolicy int64
	}{
		{
			name:            "not advertised with public archive and open subscriptions",
			advertised:      0,
			archivePrivate:  0,
			subscribePolicy: 1,
		},
		{
			name:            "not advertised with public archive and moderated subscriptions",
			advertised:      0,
			archivePrivate:  0,
			subscribePolicy: 2,
		},
		{
			name:            "not advertised with public archive and confirmed moderated subscriptions",
			advertised:      0,
			archivePrivate:  0,
			subscribePolicy: 3,
		},
		{
			name:            "not advertised with private archive and open subscriptions",
			advertised:      0,
			archivePrivate:  1,
			subscribePolicy: 1,
		},
		{
			name:            "advertised with public archive and open subscriptions",
			advertised:      1,
			archivePrivate:  0,
			subscribePolicy: 1,
		},
		{
			name:            "advertised with public archive and moderated subscriptions",
			advertised:      1,
			archivePrivate:  0,
			subscribePolicy: 2,
		},
		{
			name:            "advertised with public archive and confirmed moderated subscriptions",
			advertised:      1,
			archivePrivate:  0,
			subscribePolicy: 3,
		},
		{
			name:            "advertised with private archive and open subscriptions",
			advertised:      1,
			archivePrivate:  1,
			subscribePolicy: 1,
		},
		{
			name:            "advertised with private archive and moderated subscriptions",
			advertised:      1,
			archivePrivate:  1,
			subscribePolicy: 2,
		},
		{
			name:            "advertised with private archive and confirmed moderated subscriptions",
			advertised:      1,
			archivePrivate:  1,
			subscribePolicy: 3,
		},
	}

	inventory := make([]map[string]any, 0, len(testCases))
	for index, testCase := range testCases {
		item := mailingListTestInventory(
			fmt.Sprintf("mixed-%d@example.test", index),
			"public",
			testCase.subscribePolicy,
		)
		item["advertised"] = testCase.advertised
		item["archive_private"] = testCase.archivePrivate
		inventory = append(inventory, item)
	}

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeMailingListJSON(t, response, map[string]any{
			"status": 1,
			"data":   inventory,
		})
	}))
	defer server.Close()

	mailingLists, err := newEmailTestClient(t, server.URL).ListMailingLists(
		t.Context(),
		"example.test",
	)
	if err != nil {
		t.Fatalf("ListMailingLists() error: %v", err)
	}
	if len(mailingLists) != len(testCases) {
		t.Fatalf(
			"mailing list count = %d, want %d",
			len(mailingLists),
			len(testCases),
		)
	}
	for index, mailingList := range mailingLists {
		if mailingList.Private {
			t.Errorf(
				"%s normalized as private",
				testCases[index].name,
			)
		}
	}
}

func TestClientGetsExactMailingListAndReturnsNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Email/list_lists" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("domain"); got != "example.test" {
			t.Errorf("domain = %q, want example.test", got)
		}

		writeMailingListJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				mailingListTestInventory(
					"other@example.test",
					"public",
					1,
				),
				mailingListTestInventory(
					"target@example.test",
					"private",
					2,
				),
			},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	mailingList, err := client.GetMailingList(
		t.Context(),
		"target@example.test",
		"example.test",
	)
	if err != nil {
		t.Fatalf("GetMailingList() error: %v", err)
	}
	if mailingList == nil ||
		mailingList.Address != "target@example.test" ||
		!mailingList.Private {
		t.Fatalf("mailing list = %#v", mailingList)
	}

	missing, err := client.GetMailingList(
		t.Context(),
		"Target@example.test",
		"example.test",
	)
	if err != nil {
		t.Fatalf("GetMailingList() missing error: %v", err)
	}
	if missing != nil {
		t.Fatalf("GetMailingList() missing = %#v, want nil", missing)
	}
}

func TestClientCreatesMailingListWithPOSTAndKeepsPasswordOutOfURL(
	t *testing.T,
) {
	t.Parallel()

	const password = "mailing-password-&=?#"

	for _, private := range []bool{false, true} {
		private := private
		t.Run(map[bool]string{false: "public", true: "private"}[private], func(
			t *testing.T,
		) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if request.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", request.Method)
				}
				if request.URL.Path != "/execute/Email/add_list" {
					t.Errorf("path = %s", request.URL.Path)
				}
				if request.URL.RawQuery != "" {
					t.Errorf(
						"request URL query = %q, want empty",
						request.URL.RawQuery,
					)
				}
				if strings.Contains(request.RequestURI, password) ||
					strings.Contains(
						request.RequestURI,
						url.QueryEscape(password),
					) {
					t.Errorf(
						"request URL contains password: %s",
						request.URL.Redacted(),
					)
				}
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}

				expectedPrivate := "0"
				if private {
					expectedPrivate = "1"
				}
				expected := map[string]string{
					"list":     "terraform",
					"domain":   "example.test",
					"password": password,
					"private":  expectedPrivate,
				}
				for key, value := range expected {
					if got := request.Form.Get(key); got != value {
						t.Errorf(
							"%s = %q, want %q",
							key,
							got,
							value,
						)
					}
				}

				writeMailingListJSON(t, response, map[string]any{
					"status": 1,
					"data":   nil,
				})
			}))
			defer server.Close()

			if err := newEmailTestClient(t, server.URL).CreateMailingList(
				t.Context(),
				"terraform",
				"example.test",
				password,
				private,
			); err != nil {
				t.Fatalf("CreateMailingList() error: %v", err)
			}
		})
	}
}

func TestClientChangesMailingListPasswordWithPOSTAndKeepsPasswordOutOfURL(
	t *testing.T,
) {
	t.Parallel()

	const password = "changed-mailing-password-&=?#"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/passwd_list" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf(
				"request URL query = %q, want empty",
				request.URL.RawQuery,
			)
		}
		if strings.Contains(request.RequestURI, password) ||
			strings.Contains(request.RequestURI, url.QueryEscape(password)) {
			t.Errorf(
				"request URL contains password: %s",
				request.URL.Redacted(),
			)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		expected := url.Values{
			"list":     {"terraform@example.test"},
			"password": {password},
		}
		if !reflect.DeepEqual(request.Form, expected) {
			t.Errorf("form = %#v, want %#v", request.Form, expected)
		}

		writeMailingListJSON(t, response, map[string]any{
			"status": 1,
			"data":   nil,
		})
	}))
	defer server.Close()

	if err := newEmailTestClient(t, server.URL).ChangeMailingListPassword(
		t.Context(),
		"terraform@example.test",
		password,
	); err != nil {
		t.Fatalf("ChangeMailingListPassword() error: %v", err)
	}
}

func TestClientSetsMailingListPrivacyOptionsWithPOST(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options MailingListPrivacyOptions
		values  url.Values
	}{
		{
			name: "private",
			options: MailingListPrivacyOptions{
				ArchivePrivate:  true,
				SubscribePolicy: 3,
			},
			values: url.Values{
				"advertised":       {"0"},
				"archive_private":  {"1"},
				"subscribe_policy": {"3"},
			},
		},
		{
			name: "mixed public not advertised",
			options: MailingListPrivacyOptions{
				SubscribePolicy: 2,
			},
			values: url.Values{
				"advertised":       {"0"},
				"archive_private":  {"0"},
				"subscribe_policy": {"2"},
			},
		},
		{
			name: "mixed public advertised private archive",
			options: MailingListPrivacyOptions{
				Advertised:      true,
				ArchivePrivate:  true,
				SubscribePolicy: 1,
			},
			values: url.Values{
				"advertised":       {"1"},
				"archive_private":  {"1"},
				"subscribe_policy": {"1"},
			},
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
				if request.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", request.Method)
				}
				if request.URL.Path !=
					"/execute/Email/set_list_privacy_options" {
					t.Errorf("path = %s", request.URL.Path)
				}
				if request.URL.RawQuery != "" {
					t.Errorf(
						"request URL query = %q, want empty",
						request.URL.RawQuery,
					)
				}
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}
				expected := testCase.values
				expected.Set("list", "terraform@example.test")
				if !reflect.DeepEqual(request.Form, expected) {
					t.Errorf(
						"form = %#v, want %#v",
						request.Form,
						expected,
					)
				}

				writeMailingListJSON(t, response, map[string]any{
					"status": 1,
					"data":   nil,
				})
			}))
			defer server.Close()

			if err := newEmailTestClient(
				t,
				server.URL,
			).SetMailingListPrivacyOptions(
				t.Context(),
				"terraform@example.test",
				testCase.options,
			); err != nil {
				t.Fatalf("SetMailingListPrivacyOptions() error: %v", err)
			}
		})
	}
}

func TestClientRejectsInvalidMailingListMutationInputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		call      func(*Client) error
		errorText string
	}{
		{
			name: "password change empty address",
			call: func(client *Client) error {
				return client.ChangeMailingListPassword(
					t.Context(),
					"",
					"password",
				)
			},
			errorText: "address must not be empty",
		},
		{
			name: "password change address with surrounding whitespace",
			call: func(client *Client) error {
				return client.ChangeMailingListPassword(
					t.Context(),
					" list@example.test ",
					"password",
				)
			},
			errorText: "surrounding whitespace",
		},
		{
			name: "empty password",
			call: func(client *Client) error {
				return client.ChangeMailingListPassword(
					t.Context(),
					"list@example.test",
					"",
				)
			},
			errorText: "password must not be empty",
		},
		{
			name: "privacy empty address",
			call: func(client *Client) error {
				return client.SetMailingListPrivacyOptions(
					t.Context(),
					"",
					MailingListPrivacyOptions{SubscribePolicy: 1},
				)
			},
			errorText: "address must not be empty",
		},
		{
			name: "privacy address with surrounding whitespace",
			call: func(client *Client) error {
				return client.SetMailingListPrivacyOptions(
					t.Context(),
					" list@example.test ",
					MailingListPrivacyOptions{SubscribePolicy: 1},
				)
			},
			errorText: "surrounding whitespace",
		},
		{
			name: "subscribe policy below range",
			call: func(client *Client) error {
				return client.SetMailingListPrivacyOptions(
					t.Context(),
					"list@example.test",
					MailingListPrivacyOptions{SubscribePolicy: 0},
				)
			},
			errorText: "from 1 through 3",
		},
		{
			name: "subscribe policy above range",
			call: func(client *Client) error {
				return client.SetMailingListPrivacyOptions(
					t.Context(),
					"list@example.test",
					MailingListPrivacyOptions{SubscribePolicy: 4},
				)
			},
			errorText: "from 1 through 3",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.call(&Client{})
			if err == nil || !strings.Contains(err.Error(), testCase.errorText) {
				t.Fatalf("error = %v, want text %q", err, testCase.errorText)
			}
		})
	}
}

func TestClientDeletesMailingListWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/delete_list" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("request URL query = %q, want empty", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("list"); got != "terraform@example.test" {
			t.Errorf("list = %q, want terraform@example.test", got)
		}

		writeMailingListJSON(t, response, map[string]any{
			"status": 1,
			"data":   nil,
		})
	}))
	defer server.Close()

	if err := newEmailTestClient(t, server.URL).DeleteMailingList(
		t.Context(),
		"terraform@example.test",
	); err != nil {
		t.Fatalf("DeleteMailingList() error: %v", err)
	}
}

func TestClientRejectsInvalidMailingListInventoryData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response map[string]any
	}{
		{
			name:     "missing data",
			response: map[string]any{"status": 1},
		},
		{
			name: "null data",
			response: map[string]any{
				"status": 1,
				"data":   nil,
			},
		},
		{
			name: "object data",
			response: map[string]any{
				"status": 1,
				"data":   map[string]any{},
			},
		},
		{
			name: "string data",
			response: map[string]any{
				"status": 1,
				"data":   "not-an-array",
			},
		},
		{
			name: "non-object item",
			response: map[string]any{
				"status": 1,
				"data":   []any{42},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeMailingListJSON(t, response, testCase.response)
			}))
			defer server.Close()

			if _, err := newEmailTestClient(
				t,
				server.URL,
			).ListMailingLists(t.Context(), "example.test"); err == nil {
				t.Fatal("ListMailingLists() returned no error")
			}
		})
	}
}

func TestClientRejectsInvalidMailingListFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		change func(map[string]any)
	}{
		{
			name: "missing address",
			change: func(item map[string]any) {
				delete(item, "list")
			},
		},
		{
			name: "null address",
			change: func(item map[string]any) {
				item["list"] = nil
			},
		},
		{
			name: "empty address",
			change: func(item map[string]any) {
				item["list"] = ""
			},
		},
		{
			name: "numeric address",
			change: func(item map[string]any) {
				item["list"] = 42
			},
		},
		{
			name: "address with surrounding whitespace",
			change: func(item map[string]any) {
				item["list"] = " list@example.test "
			},
		},
		{
			name: "missing id",
			change: func(item map[string]any) {
				delete(item, "listid")
			},
		},
		{
			name: "null id",
			change: func(item map[string]any) {
				item["listid"] = nil
			},
		},
		{
			name: "empty id",
			change: func(item map[string]any) {
				item["listid"] = ""
			},
		},
		{
			name: "boolean id",
			change: func(item map[string]any) {
				item["listid"] = true
			},
		},
		{
			name: "id with surrounding whitespace",
			change: func(item map[string]any) {
				item["listid"] = " list_example.test "
			},
		},
		{
			name: "missing access type",
			change: func(item map[string]any) {
				delete(item, "accesstype")
			},
		},
		{
			name: "null access type",
			change: func(item map[string]any) {
				item["accesstype"] = nil
			},
		},
		{
			name: "numeric access type",
			change: func(item map[string]any) {
				item["accesstype"] = 1
			},
		},
		{
			name: "unsupported access type",
			change: func(item map[string]any) {
				item["accesstype"] = "members-only"
			},
		},
		{
			name: "missing advertised",
			change: func(item map[string]any) {
				delete(item, "advertised")
			},
		},
		{
			name: "null advertised",
			change: func(item map[string]any) {
				item["advertised"] = nil
			},
		},
		{
			name: "string advertised",
			change: func(item map[string]any) {
				item["advertised"] = "1"
			},
		},
		{
			name: "boolean advertised",
			change: func(item map[string]any) {
				item["advertised"] = true
			},
		},
		{
			name: "out of range advertised",
			change: func(item map[string]any) {
				item["advertised"] = 2
			},
		},
		{
			name: "decimal advertised",
			change: func(item map[string]any) {
				item["advertised"] = json.RawMessage(`1.0`)
			},
		},
		{
			name: "missing archive private",
			change: func(item map[string]any) {
				delete(item, "archive_private")
			},
		},
		{
			name: "null archive private",
			change: func(item map[string]any) {
				item["archive_private"] = nil
			},
		},
		{
			name: "string archive private",
			change: func(item map[string]any) {
				item["archive_private"] = "0"
			},
		},
		{
			name: "boolean archive private",
			change: func(item map[string]any) {
				item["archive_private"] = false
			},
		},
		{
			name: "out of range archive private",
			change: func(item map[string]any) {
				item["archive_private"] = 2
			},
		},
		{
			name: "missing subscribe policy",
			change: func(item map[string]any) {
				delete(item, "subscribe_policy")
			},
		},
		{
			name: "null subscribe policy",
			change: func(item map[string]any) {
				item["subscribe_policy"] = nil
			},
		},
		{
			name: "string subscribe policy",
			change: func(item map[string]any) {
				item["subscribe_policy"] = "1"
			},
		},
		{
			name: "boolean subscribe policy",
			change: func(item map[string]any) {
				item["subscribe_policy"] = true
			},
		},
		{
			name: "subscribe policy below range",
			change: func(item map[string]any) {
				item["subscribe_policy"] = 0
			},
		},
		{
			name: "subscribe policy above range",
			change: func(item map[string]any) {
				item["subscribe_policy"] = 4
			},
		},
		{
			name: "decimal subscribe policy",
			change: func(item map[string]any) {
				item["subscribe_policy"] = json.RawMessage(`1.0`)
			},
		},
		{
			name: "missing administrators",
			change: func(item map[string]any) {
				delete(item, "listadmin")
			},
		},
		{
			name: "null administrators",
			change: func(item map[string]any) {
				item["listadmin"] = nil
			},
		},
		{
			name: "array administrators",
			change: func(item map[string]any) {
				item["listadmin"] = []string{"admin@example.test"}
			},
		},
		{
			name: "missing human disk used",
			change: func(item map[string]any) {
				delete(item, "humandiskused")
			},
		},
		{
			name: "null human disk used",
			change: func(item map[string]any) {
				item["humandiskused"] = nil
			},
		},
		{
			name: "numeric human disk used",
			change: func(item map[string]any) {
				item["humandiskused"] = 0
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			item := mailingListTestInventory(
				"list@example.test",
				"public",
				1,
			)
			testCase.change(item)

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeMailingListJSON(t, response, map[string]any{
					"status": 1,
					"data":   []map[string]any{item},
				})
			}))
			defer server.Close()

			if _, err := newEmailTestClient(
				t,
				server.URL,
			).ListMailingLists(t.Context(), "example.test"); err == nil {
				t.Fatal("ListMailingLists() returned no error")
			}
		})
	}
}

func TestClientRejectsContradictoryMailingListAccessSettings(
	t *testing.T,
) {
	t.Parallel()

	testCases := []struct {
		name            string
		accessType      string
		advertised      int
		archivePrivate  int
		subscribePolicy int64
	}{
		{
			name:            "public with private settings and moderated subscriptions",
			accessType:      "public",
			advertised:      0,
			archivePrivate:  1,
			subscribePolicy: 2,
		},
		{
			name:            "public with private settings and confirmed moderated subscriptions",
			accessType:      "public",
			advertised:      0,
			archivePrivate:  1,
			subscribePolicy: 3,
		},
		{
			name:            "private advertised",
			accessType:      "private",
			advertised:      1,
			archivePrivate:  1,
			subscribePolicy: 2,
		},
		{
			name:            "private public archive",
			accessType:      "private",
			advertised:      0,
			archivePrivate:  0,
			subscribePolicy: 2,
		},
		{
			name:            "private open subscriptions",
			accessType:      "private",
			advertised:      0,
			archivePrivate:  1,
			subscribePolicy: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			item := mailingListTestInventory(
				"list@example.test",
				testCase.accessType,
				testCase.subscribePolicy,
			)
			item["advertised"] = testCase.advertised
			item["archive_private"] = testCase.archivePrivate

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeMailingListJSON(t, response, map[string]any{
					"status": 1,
					"data":   []map[string]any{item},
				})
			}))
			defer server.Close()

			_, err := newEmailTestClient(t, server.URL).ListMailingLists(
				t.Context(),
				"example.test",
			)
			if err == nil || !strings.Contains(err.Error(), "inconsistent") {
				t.Fatalf(
					"ListMailingLists() error = %v, want inconsistency",
					err,
				)
			}
		})
	}
}

func TestClientRejectsDuplicateMailingListAddresses(t *testing.T) {
	t.Parallel()

	first := mailingListTestInventory(
		"duplicate@example.test",
		"public",
		1,
	)
	second := mailingListTestInventory(
		"duplicate@example.test",
		"private",
		2,
	)
	second["listid"] = "different_example.test"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeMailingListJSON(t, response, map[string]any{
			"status": 1,
			"data":   []map[string]any{first, second},
		})
	}))
	defer server.Close()

	_, err := newEmailTestClient(t, server.URL).GetMailingList(
		t.Context(),
		"duplicate@example.test",
		"example.test",
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("GetMailingList() error = %v, want duplicate error", err)
	}
}

func TestClientPropagatesMailingListAPIErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		function string
		call     func(context.Context, *Client) error
	}{
		{
			name:     "list",
			function: operationListMailingLists,
			call: func(ctx context.Context, client *Client) error {
				_, err := client.ListMailingLists(ctx, "example.test")

				return err
			},
		},
		{
			name:     "create",
			function: operationAddMailingList,
			call: func(ctx context.Context, client *Client) error {
				return client.CreateMailingList(
					ctx,
					"terraform",
					"example.test",
					"password",
					true,
				)
			},
		},
		{
			name:     "delete",
			function: operationDeleteMailingList,
			call: func(ctx context.Context, client *Client) error {
				return client.DeleteMailingList(
					ctx,
					"terraform@example.test",
				)
			},
		},
		{
			name:     "change password",
			function: operationChangeMailingListPassword,
			call: func(ctx context.Context, client *Client) error {
				return client.ChangeMailingListPassword(
					ctx,
					"terraform@example.test",
					"password",
				)
			},
		},
		{
			name:     "set privacy options",
			function: operationSetMailingListPrivacyOptions,
			call: func(ctx context.Context, client *Client) error {
				return client.SetMailingListPrivacyOptions(
					ctx,
					"terraform@example.test",
					MailingListPrivacyOptions{
						ArchivePrivate:  true,
						SubscribePolicy: 2,
					},
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
				if request.URL.Path !=
					"/execute/Email/"+testCase.function {
					t.Errorf("path = %s", request.URL.Path)
				}
				writeMailingListJSON(t, response, map[string]any{
					"status": 0,
					"errors": []string{"Mailman is unavailable"},
				})
			}))
			defer server.Close()

			err := testCase.call(
				t.Context(),
				newEmailTestClient(t, server.URL),
			)
			var apiError *cpanel.APIError
			if !errors.As(err, &apiError) {
				t.Fatalf("error = %v, want *cpanel.APIError", err)
			}
			if apiError.Function != testCase.function ||
				!strings.Contains(apiError.Error(), "Mailman is unavailable") {
				t.Fatalf("API error = %#v", apiError)
			}
		})
	}
}

func TestClientLocksMailingListSequencesPerAddress(t *testing.T) {
	t.Parallel()

	client := NewClient(nil)
	unlockFirst := client.LockMailingList("first@example.test")

	sameAddress := make(chan func(), 1)
	go func() {
		sameAddress <- client.LockMailingList("first@example.test")
	}()

	select {
	case unlock := <-sameAddress:
		unlock()
		t.Fatal("second lock for the same mailing list did not wait")
	case <-time.After(50 * time.Millisecond):
	}

	otherAddress := make(chan func(), 1)
	go func() {
		otherAddress <- client.LockMailingList("other@example.test")
	}()

	select {
	case unlock := <-otherAddress:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("lock for a different mailing list was blocked")
	}

	filterAccount := make(chan func(), 1)
	go func() {
		filterAccount <- client.LockFilterAccount("first@example.test")
	}()

	select {
	case unlock := <-filterAccount:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("mailing list lock blocked an email filter lock")
	}

	unlockFirst()
	unlockFirst()

	select {
	case unlock := <-sameAddress:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second lock for the same mailing list stayed blocked")
	}

	client.mailingListLocksMu.Lock()
	defer client.mailingListLocksMu.Unlock()
	if len(client.mailingListLocks) != 0 {
		t.Fatalf(
			"mailing list lock count = %d, want 0",
			len(client.mailingListLocks),
		)
	}

	client.filterLocksMu.Lock()
	defer client.filterLocksMu.Unlock()
	if len(client.filterLocks) != 0 {
		t.Fatalf("filter lock count = %d, want 0", len(client.filterLocks))
	}
}

func mailingListTestInventory(
	address string,
	accessType string,
	subscribePolicy int64,
) map[string]any {
	advertised := 1
	archivePrivate := 0
	if accessType == "private" {
		advertised = 0
		archivePrivate = 1
	}

	return map[string]any{
		"list":             address,
		"listid":           strings.Replace(address, "@", "_", 1),
		"accesstype":       accessType,
		"advertised":       advertised,
		"archive_private":  archivePrivate,
		"subscribe_policy": subscribePolicy,
		"listadmin":        "admin@example.test",
		"humandiskused":    "0 bytes",
	}
}

func writeMailingListJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
