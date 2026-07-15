package capabilities

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGetsAccountCapabilities(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Features/list_features":
			writeCapabilitiesJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"autossl":       "0",
					"passengerapps": 1,
					"ssh-whitelist": true,
				},
			})
		case "/execute/StatsBar/get_stats":
			if request.URL.Query().Get("display") != "cpanelversion" {
				t.Errorf("query = %v", request.URL.Query())
			}
			writeCapabilitiesJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"name":  "cpanelversion",
					"value": "134.0 (build 45)",
				}},
			})
		case "/execute/Variables/get_user_information":
			writeCapabilitiesJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"user":                             "example",
					"home":                             "/home/example",
					"domain":                           "example.test",
					"plan":                             "test",
					"theme":                            "paper_lantern",
					"shell":                            "/bin/bash",
					"maximum_addon_domains":            "unlimited",
					"maximum_databases":                "10",
					"maximum_defer_fail_percentage":    "100",
					"maximum_email_account_disk_quota": "unlimited",
					"maximum_emails_per_hour":          "250",
					"maximum_ftp_accounts":             "unlimited",
					"maximum_mail_accounts":            "unlimited",
					"maximum_mailing_lists":            "5",
					"maximum_parked_domains":           "unlimited",
					"maximum_passenger_apps":           "4",
					"maximum_subdomains":               "unlimited",
					"max_team_users":                   "0",
					"contact_email":                    "ignored@example.test",
					"uuid":                             "ignored",
				},
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	capabilities, err := newCapabilitiesTestClient(t, server).Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if capabilities.Version != "134.0 (build 45)" ||
		capabilities.Username != "example" ||
		capabilities.HomeDirectory != "/home/example" ||
		capabilities.PrimaryDomain != "example.test" ||
		!capabilities.Features["passengerapps"] ||
		capabilities.Features["autossl"] ||
		capabilities.Limits["passenger_apps"] != "4" {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}

func TestClientRejectsInvalidFeatureFlag(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Features/list_features" {
			t.Fatalf("unexpected request: %s", request.URL)
		}
		writeCapabilitiesJSON(t, response, map[string]any{
			"status": 1,
			"data":   map[string]any{"passengerapps": 2},
		})
	}))
	defer server.Close()

	if _, err := newCapabilitiesTestClient(t, server).Get(
		t.Context(),
	); err == nil {
		t.Fatal("Get() succeeded")
	}
}

func newCapabilitiesTestClient(
	t *testing.T,
	server *httptest.Server,
) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func writeCapabilitiesJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
