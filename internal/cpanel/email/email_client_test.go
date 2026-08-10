package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestAccountDiskValues(t *testing.T) {
	t.Parallel()

	account := Account{
		DiskQuotaRaw: json.RawMessage(`"78643200"`),
		DiskUsedRaw:  json.RawMessage(`1024`),
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		t.Fatalf("QuotaMiB() error: %v", err)
	}
	if quotaMiB != 75 {
		t.Fatalf("QuotaMiB() = %d, want 75", quotaMiB)
	}

	diskUsedBytes, err := account.DiskUsedBytes()
	if err != nil {
		t.Fatalf("DiskUsedBytes() error: %v", err)
	}
	if diskUsedBytes != 1024 {
		t.Fatalf("DiskUsedBytes() = %d, want 1024", diskUsedBytes)
	}
}

func TestAccountSuspensions(t *testing.T) {
	t.Parallel()

	account := Account{
		Email:                "terraform@example.test",
		HasSuspendedRaw:      json.RawMessage(`1`),
		HoldOutgoingRaw:      json.RawMessage(`"0"`),
		SuspendedIncomingRaw: json.RawMessage(`true`),
		SuspendedLoginRaw:    json.RawMessage(`null`),
		SuspendedOutgoingRaw: json.RawMessage(`"1"`),
	}

	suspensions, err := account.Suspensions()
	if err != nil {
		t.Fatalf("Suspensions() error: %v", err)
	}
	if suspensions.Login ||
		!suspensions.Incoming ||
		!suspensions.Outgoing ||
		suspensions.OutgoingHeld ||
		!suspensions.HasSuspended {
		t.Fatalf("suspensions = %#v", suspensions)
	}
}

func TestAccountRejectsInconsistentSuspensions(t *testing.T) {
	t.Parallel()

	account := Account{
		Email:                "terraform@example.test",
		HasSuspendedRaw:      json.RawMessage(`0`),
		HoldOutgoingRaw:      json.RawMessage(`0`),
		SuspendedIncomingRaw: json.RawMessage(`0`),
		SuspendedLoginRaw:    json.RawMessage(`1`),
		SuspendedOutgoingRaw: json.RawMessage(`0`),
	}

	if _, err := account.Suspensions(); err == nil {
		t.Fatal("Suspensions() returned no error")
	}
}

func TestAccountRejectsMissingSuspensionField(t *testing.T) {
	t.Parallel()

	account := Account{
		Email:                "terraform@example.test",
		HasSuspendedRaw:      json.RawMessage(`0`),
		HoldOutgoingRaw:      json.RawMessage(`0`),
		SuspendedIncomingRaw: json.RawMessage(`0`),
		SuspendedOutgoingRaw: json.RawMessage(`0`),
	}

	if _, err := account.Suspensions(); err == nil {
		t.Fatal("Suspensions() returned no error")
	}
}

func TestClientCreatesEmailAccountWithPOST(t *testing.T) {
	t.Parallel()

	const password = "email-password-&=?"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/add_pop" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if strings.Contains(request.URL.String(), password) || request.URL.RawQuery != "" {
			t.Errorf("request URL contains password: %s", request.URL.Redacted())
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("email"); got != "terraform" {
			t.Errorf("email = %q", got)
		}
		if got := request.Form.Get("domain"); got != "example.test" {
			t.Errorf("domain = %q", got)
		}
		if got := request.Form.Get("quota"); got != "50" {
			t.Errorf("quota = %q", got)
		}
		if got := request.Form.Get("password"); got != password {
			t.Errorf("password = %q", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":"terraform+example.test"}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.CreateAccount(
		context.Background(),
		"terraform",
		"example.test",
		password,
		50,
	); err != nil {
		t.Fatalf("CreateAccount() error: %v", err)
	}
}

func TestClientSetsEmailAccountSuspensionsWithPOST(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		mutation func(context.Context, *Client) error
	}{
		{
			name: "suspend login",
			path: "/execute/Email/suspend_login",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetLoginSuspended(
					ctx,
					"terraform@example.test",
					true,
				)
			},
		},
		{
			name: "unsuspend login",
			path: "/execute/Email/unsuspend_login",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetLoginSuspended(
					ctx,
					"terraform@example.test",
					false,
				)
			},
		},
		{
			name: "suspend incoming",
			path: "/execute/Email/suspend_incoming",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetIncomingSuspended(
					ctx,
					"terraform@example.test",
					true,
				)
			},
		},
		{
			name: "unsuspend incoming",
			path: "/execute/Email/unsuspend_incoming",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetIncomingSuspended(
					ctx,
					"terraform@example.test",
					false,
				)
			},
		},
		{
			name: "suspend outgoing",
			path: "/execute/Email/suspend_outgoing",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetOutgoingSuspended(
					ctx,
					"terraform@example.test",
					true,
				)
			},
		},
		{
			name: "unsuspend outgoing",
			path: "/execute/Email/unsuspend_outgoing",
			mutation: func(ctx context.Context, client *Client) error {
				return client.SetOutgoingSuspended(
					ctx,
					"terraform@example.test",
					false,
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
				if request.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", request.Method)
				}
				if request.URL.Path != testCase.path {
					t.Errorf("path = %s, want %s", request.URL.Path, testCase.path)
				}
				if request.URL.RawQuery != "" {
					t.Errorf("query = %q, want empty", request.URL.RawQuery)
				}
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}
				if request.Form.Get("email") != "terraform@example.test" {
					t.Errorf("email = %q", request.Form.Get("email"))
				}
				_, _ = response.Write([]byte(`{"status":1,"data":null}`))
			}))
			defer server.Close()

			if err := testCase.mutation(
				t.Context(),
				newEmailTestClient(t, server.URL),
			); err != nil {
				t.Fatalf("mutation error: %v", err)
			}
		})
	}
}

func TestClientGetsExactEmailAccount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Email/list_pops_with_disk" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("email"); got != "terraform" {
			t.Errorf("email = %q", got)
		}
		if got := request.URL.Query().Get("domain"); got != "example.test" {
			t.Errorf("domain = %q", got)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":[{"email":"other@example.test","_diskquota":"0","_diskused":0},{"email":"terraform@example.test","user":"terraform","domain":"example.test","_diskquota":"52428800","_diskused":128}]}`,
		))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	account, err := client.GetAccount(context.Background(), "terraform", "example.test")
	if err != nil {
		t.Fatalf("GetAccount() error: %v", err)
	}
	if account == nil || account.Email != "terraform@example.test" {
		t.Fatalf("account = %#v", account)
	}
}

func TestClientListsMailDomains(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_, _ = response.Write([]byte(
			`{"status":1,"data":[{"domain":"example.test"},{"domain":"mail.example.test"}]}`,
		))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	domains, err := client.ListMailDomains(context.Background())
	if err != nil {
		t.Fatalf("ListMailDomains() error: %v", err)
	}
	if len(domains) != 2 || domains[0] != "example.test" {
		t.Fatalf("domains = %v", domains)
	}
}

func newEmailTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
