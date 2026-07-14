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
