package ftp

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
		DiskQuotaRaw: json.RawMessage(`"75.00"`),
		DiskUsedRaw:  json.RawMessage(`"0.50"`),
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		t.Fatalf("QuotaMiB() error: %v", err)
	}
	if quotaMiB != 75 {
		t.Fatalf("QuotaMiB() = %d, want 75", quotaMiB)
	}

	diskUsedMiB, err := account.DiskUsedMiB()
	if err != nil {
		t.Fatalf("DiskUsedMiB() error: %v", err)
	}
	if diskUsedMiB != 0.5 {
		t.Fatalf("DiskUsedMiB() = %v, want 0.5", diskUsedMiB)
	}
}

func TestAccountUnlimitedQuota(t *testing.T) {
	t.Parallel()

	for _, value := range []json.RawMessage{json.RawMessage(`"0.00"`), json.RawMessage(`"unlimited"`)} {
		account := Account{DiskQuotaRaw: value}
		quotaMiB, err := account.QuotaMiB()
		if err != nil {
			t.Fatalf("QuotaMiB() error: %v", err)
		}
		if quotaMiB != 0 {
			t.Fatalf("QuotaMiB() = %d, want 0", quotaMiB)
		}
	}
}

func TestClientCreatesFTPAccountWithPOST(t *testing.T) {
	t.Parallel()

	const password = "ftp-password-&=?"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Ftp/add_ftp" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if strings.Contains(request.URL.String(), password) || request.URL.RawQuery != "" {
			t.Errorf("request URL contains password: %s", request.URL.Redacted())
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("user"); got != "terraform" {
			t.Errorf("user = %q", got)
		}
		if got := request.Form.Get("domain"); got != "example.test" {
			t.Errorf("domain = %q", got)
		}
		if got := request.Form.Get("homedir"); got != "sites/terraform" {
			t.Errorf("homedir = %q", got)
		}
		if got := request.Form.Get("quota"); got != "50" {
			t.Errorf("quota = %q", got)
		}
		if got := request.Form.Get("pass"); got != password {
			t.Errorf("pass = %q", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newFTPTestClient(t, server.URL)
	if err := client.CreateAccount(
		context.Background(),
		"terraform",
		"example.test",
		password,
		"sites/terraform",
		50,
	); err != nil {
		t.Fatalf("CreateAccount() error: %v", err)
	}
}

func TestClientGetsExactFTPAccount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Ftp/list_ftp_with_disk" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("include_acct_types"); got != "sub" {
			t.Errorf("include_acct_types = %q", got)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":[{"login":"other@example.test","accttype":"sub"},{"login":"terraform@example.test","reldir":"sites/terraform","accttype":"sub","_diskquota":"50.00","_diskused":"0.00"}]}`,
		))
	}))
	defer server.Close()

	client := newFTPTestClient(t, server.URL)
	account, err := client.GetAccount(context.Background(), "terraform@example.test")
	if err != nil {
		t.Fatalf("GetAccount() error: %v", err)
	}
	if account == nil || account.Login != "terraform@example.test" {
		t.Fatalf("account = %#v", account)
	}
}

func TestClientSetsUnlimitedQuota(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Ftp/set_quota" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("quota"); got != "0" {
			t.Errorf("quota = %q", got)
		}
		if got := request.Form.Get("kill"); got != "1" {
			t.Errorf("kill = %q", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newFTPTestClient(t, server.URL)
	if err := client.SetQuota(
		context.Background(),
		"terraform",
		"example.test",
		0,
	); err != nil {
		t.Fatalf("SetQuota() error: %v", err)
	}
}

func TestClientListsFTPAccountDomains(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/DomainInfo/list_domains" {
			t.Errorf("path = %s", request.URL.Path)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":{"main_domain":"example.test","addon_domains":["addon.test"],"sub_domains":["sub.example.test"],"parked_domains":["alias.test"]}}`,
		))
	}))
	defer server.Close()

	client := newFTPTestClient(t, server.URL)
	domains, err := client.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("ListDomains() error: %v", err)
	}
	if len(domains) != 4 || domains[0] != "example.test" || domains[3] != "alias.test" {
		t.Fatalf("domains = %v", domains)
	}
}

func newFTPTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
