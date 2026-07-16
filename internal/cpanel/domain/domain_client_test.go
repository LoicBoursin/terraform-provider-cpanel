package domain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientCreatesSubdomainWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/json-api/cpanel" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("request URL query = %q, want empty", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		expected := map[string]string{
			"cpanel_jsonapi_module": "SubDomain",
			"cpanel_jsonapi_func":   "addsubdomain",
			"domain":                "terraform",
			"rootdomain":            "example.test",
			"dir":                   "public_html/terraform",
			"disallowdot":           "0",
		}
		for key, want := range expected {
			if got := request.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"result":1}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	if err := client.CreateSubdomain(
		context.Background(),
		"terraform",
		"example.test",
		"public_html/terraform",
	); err != nil {
		t.Fatalf("CreateSubdomain() error: %v", err)
	}
}

func TestClientValidatesOptionalDomainMutationResults(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		payload   string
		wantError bool
	}{
		"failed result": {
			payload:   `{"cpanelresult":{"event":{"result":1},"data":[{"result":0,"reason":"domain already exists"}]}}`,
			wantError: true,
		},
		"empty successful result": {
			payload: `{"cpanelresult":{"event":{"result":1},"data":[]}}`,
		},
		"multiple successful results": {
			payload: `{"cpanelresult":{"event":{"result":1},"data":[{"result":1},{"result":1}]}}`,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				_, _ = response.Write([]byte(test.payload))
			}))
			defer server.Close()

			err := newDomainTestClient(t, server.URL).CreateSubdomain(
				t.Context(),
				"terraform",
				"example.test",
				"public_html/terraform",
			)
			if test.wantError && err == nil {
				t.Fatal("CreateSubdomain() error = nil, want error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("CreateSubdomain() error = %v, want nil", err)
			}
			if test.wantError &&
				!strings.Contains(err.Error(), "domain already exists") {
				t.Fatalf("CreateSubdomain() error = %v", err)
			}
		})
	}
}

func TestClientGetsExactSubdomain(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Query().Get("cpanel_jsonapi_func") != "listsubdomains" {
			t.Errorf("function = %q", request.URL.Query().Get("cpanel_jsonapi_func"))
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"domain":"other.example.test"},{"domain":"terraform.example.test","subdomain":"terraform","rootdomain":"example.test","basedir":"public_html/terraform"}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	subdomain, err := client.GetSubdomain(
		context.Background(),
		"terraform.example.test",
	)
	if err != nil {
		t.Fatalf("GetSubdomain() error: %v", err)
	}
	if subdomain == nil || subdomain.BaseDirectory != "public_html/terraform" {
		t.Fatalf("subdomain = %#v", subdomain)
	}
}

func TestClientListsBaseDomains(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/DomainInfo/list_domains" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if !strings.Contains(request.Header.Get("Authorization"), "username") {
			t.Error("authorization header does not contain username")
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":{"main_domain":"example.test","addon_domains":["addon.test"],"sub_domains":["sub.example.test"],"parked_domains":["alias.test"]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	domains, err := client.ListBaseDomains(context.Background())
	if err != nil {
		t.Fatalf("ListBaseDomains() error: %v", err)
	}
	if len(domains) != 2 || domains[0] != "example.test" || domains[1] != "addon.test" {
		t.Fatalf("domains = %v", domains)
	}
}

func TestClientListsCompleteDomainInventory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/DomainInfo/list_domains" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}
		_, _ = response.Write([]byte(
			`{"status":1,"data":{"main_domain":"Main.Example.","addon_domains":["z-addon.test","A-addon.test"],"sub_domains":["Sub.Main.Example"],"parked_domains":["Alias.Test"]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	domains, err := client.ListDomains(t.Context())
	if err != nil {
		t.Fatalf("ListDomains() error = %v", err)
	}
	expected := []InventoryDomain{
		{Name: "a-addon.test", Type: InventoryDomainTypeAddon},
		{Name: "alias.test", Type: InventoryDomainTypeAlias},
		{Name: "main.example", Type: InventoryDomainTypeMain},
		{Name: "sub.main.example", Type: InventoryDomainTypeSubdomain},
		{Name: "z-addon.test", Type: InventoryDomainTypeAddon},
	}
	if !reflect.DeepEqual(domains, expected) {
		t.Fatalf("ListDomains() = %#v; expected %#v", domains, expected)
	}
}

func TestClientRejectsAmbiguousDomainInventory(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing main domain":  `{"status":1,"data":{"addon_domains":[],"sub_domains":[],"parked_domains":[]}}`,
		"missing addon array":  `{"status":1,"data":{"main_domain":"example.test","sub_domains":[],"parked_domains":[]}}`,
		"null subdomain array": `{"status":1,"data":{"main_domain":"example.test","addon_domains":[],"sub_domains":null,"parked_domains":[]}}`,
		"wrong parked type":    `{"status":1,"data":{"main_domain":"example.test","addon_domains":[],"sub_domains":[],"parked_domains":{}}}`,
		"duplicate category":   `{"status":1,"data":{"main_domain":"example.test","addon_domains":["shared.test"],"sub_domains":["SHARED.TEST."],"parked_domains":[]}}`,
		"duplicate same type":  `{"status":1,"data":{"main_domain":"example.test","addon_domains":["shared.test","SHARED.TEST."],"sub_domains":[],"parked_domains":[]}}`,
		"blank array item":     `{"status":1,"data":{"main_domain":"example.test","addon_domains":[" "],"sub_domains":[],"parked_domains":[]}}`,
		"invalid label":        `{"status":1,"data":{"main_domain":"example.test","addon_domains":["bad_label.test"],"sub_domains":[],"parked_domains":[]}}`,
	}
	for name, payload := range tests {
		payload := payload
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				_, _ = response.Write([]byte(payload))
			}))
			defer server.Close()

			_, err := newDomainTestClient(t, server.URL).
				ListDomains(t.Context())
			if err == nil {
				t.Fatal("ListDomains() returned no error")
			}
		})
	}
}

func TestNormalizeInventoryDomainName(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		"example.test":        "example.test",
		"EXAMPLE.TEST.":       "example.test",
		"a-b.example":         "a-b.example",
		"xn--caf-dma.example": "xn--caf-dma.example",
	}
	for input, expected := range valid {
		actual, err := normalizeInventoryDomainName(input)
		if err != nil {
			t.Fatalf(
				"normalizeInventoryDomainName(%q) error: %v",
				input,
				err,
			)
		}
		if actual != expected {
			t.Fatalf(
				"normalizeInventoryDomainName(%q) = %q, want %q",
				input,
				actual,
				expected,
			)
		}
	}

	invalid := []string{
		"",
		" example.test",
		"example.test ",
		"localhost",
		"a..test",
		"-bad.test",
		"bad-.test",
		"bad_label.test",
		"foo/bar.test",
		strings.Repeat("a", 64) + ".test",
		strings.Repeat("a", 250) + ".test",
	}
	for _, input := range invalid {
		if _, err := normalizeInventoryDomainName(input); err == nil {
			t.Fatalf(
				"normalizeInventoryDomainName(%q) returned no error",
				input,
			)
		}
	}
}

func TestClientDeletesFilePathWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		expected := map[string]string{
			"cpanel_jsonapi_module": "Fileman",
			"cpanel_jsonapi_func":   "fileop",
			"op":                    "unlink",
			"sourcefiles":           "public_html/terraform",
			"doubledecode":          "0",
		}
		for key, want := range expected {
			if got := request.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"result":1}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	if err := client.DeleteFilePath(
		context.Background(),
		"public_html/terraform",
	); err != nil {
		t.Fatalf("DeleteFilePath() error: %v", err)
	}
}

func TestClientCreatesAddonDomainWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		expected := map[string]string{
			"cpanel_jsonapi_module": "AddonDomain",
			"cpanel_jsonapi_func":   "addaddondomain",
			"newdomain":             "terraform.example.test",
			"subdomain":             "terraform",
			"dir":                   "public_html/terraform-addon",
			"ftp_is_optional":       "1",
		}
		for key, want := range expected {
			if got := request.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"result":1}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	if err := client.CreateAddonDomain(
		context.Background(),
		"terraform.example.test",
		"terraform",
		"public_html/terraform-addon",
	); err != nil {
		t.Fatalf("CreateAddonDomain() error: %v", err)
	}
}

func TestClientGetsExactAddonDomain(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Query().Get("cpanel_jsonapi_func") != "listaddondomains" {
			t.Errorf("function = %q", request.URL.Query().Get("cpanel_jsonapi_func"))
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"domain":"other.example.test"},{"domain":"terraform.example.test","subdomain":"terraform","rootdomain":"main.example.test","fullsubdomain":"terraform.main.example.test","domainkey":"terraform_main.example.test","basedir":"public_html/terraform-addon"}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	addonDomain, err := client.GetAddonDomain(
		context.Background(),
		"terraform.example.test",
	)
	if err != nil {
		t.Fatalf("GetAddonDomain() error: %v", err)
	}
	if addonDomain == nil || addonDomain.DomainKey != "terraform_main.example.test" {
		t.Fatalf("addonDomain = %#v", addonDomain)
	}
}

func TestClientCreatesDomainAliasWithoutTopDomain(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		expected := map[string]string{
			"cpanel_jsonapi_module": "Park",
			"cpanel_jsonapi_func":   "park",
			"domain":                "terraform-alias.example.test",
			"disallowdot":           "0",
		}
		for key, want := range expected {
			if got := request.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		if got := request.Form.Get("topdomain"); got != "" {
			t.Errorf("topdomain = %q, want empty", got)
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"result":1}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	if err := client.CreateDomainAlias(
		context.Background(),
		"terraform-alias.example.test",
	); err != nil {
		t.Fatalf("CreateDomainAlias() error: %v", err)
	}
}

func TestClientGetsExactDomainAlias(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Query().Get("cpanel_jsonapi_module") != "Park" {
			t.Errorf("module = %q", request.URL.Query().Get("cpanel_jsonapi_module"))
		}
		if request.URL.Query().Get("cpanel_jsonapi_func") != "listparkeddomains" {
			t.Errorf("function = %q", request.URL.Query().Get("cpanel_jsonapi_func"))
		}

		_, _ = response.Write([]byte(
			`{"cpanelresult":{"event":{"result":1},"data":[{"domain":"other.example.test"},{"domain":"terraform-alias.example.test","basedir":"public_html","reldir":"home:public_html","status":"not redirected"}]}}`,
		))
	}))
	defer server.Close()

	client := newDomainTestClient(t, server.URL)
	domainAlias, err := client.GetDomainAlias(
		context.Background(),
		"terraform-alias.example.test",
	)
	if err != nil {
		t.Fatalf("GetDomainAlias() error: %v", err)
	}
	if domainAlias == nil || domainAlias.BaseDirectory != "public_html" {
		t.Fatalf("domainAlias = %#v", domainAlias)
	}
}

func newDomainTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
