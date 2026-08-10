package resourceusage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGet(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/execute/ResourceUsage/get_usages" {
				t.Fatalf("unexpected path: %s", request.URL.Path)
			}
			if request.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", request.Method)
			}

			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{
				"status": 1,
				"errors": null,
				"messages": null,
				"data": [
					{
						"id": "lvecpu",
						"description": "CPU Usage",
						"formatter": null,
						"usage": 1.25,
						"maximum": 100,
						"error": null,
						"url": null
					},
					{
						"id": "bandwidth",
						"description": "Bandwidth",
						"formatter": "format_bytes",
						"usage": "18864694",
						"maximum": null,
						"error": "delayed",
						"url": "index.html?item=bandwidth"
					}
				]
			}`))
		},
	))
	defer server.Close()

	metrics, err := newTestClient(t, server).Get(t.Context())
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(metrics) != 2 || metrics[0].ID != "bandwidth" ||
		metrics[1].ID != "lvecpu" {
		t.Fatalf("metrics = %#v", metrics)
	}
	if metrics[0].Usage != "18864694" ||
		metrics[0].Maximum != nil ||
		metrics[0].Formatter == nil ||
		*metrics[0].Formatter != "format_bytes" {
		t.Fatalf("bandwidth metric = %#v", metrics[0])
	}
	if metrics[1].Usage != "1.25" ||
		metrics[1].Maximum == nil ||
		*metrics[1].Maximum != "100" ||
		metrics[1].Formatter != nil {
		t.Fatalf("CPU metric = %#v", metrics[1])
	}
}

func TestClientGetRejectsInvalidInventories(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		data      string
		wantError string
	}{
		"empty inventory": {
			data:      `[]`,
			wantError: "empty resource usage inventory",
		},
		"duplicate ID": {
			data: `[
				{"id":"disk_usage","description":"Disk","usage":1},
				{"id":"disk_usage","description":"Disk","usage":2}
			]`,
			wantError: "duplicate resource usage metric",
		},
		"invalid ID": {
			data:      `[{"id":"Disk Usage","description":"Disk","usage":1}]`,
			wantError: "invalid resource usage metric ID",
		},
		"null usage": {
			data:      `[{"id":"disk_usage","description":"Disk","usage":null}]`,
			wantError: "usage: expected a string or number",
		},
		"boolean maximum": {
			data: `[{
				"id":"disk_usage",
				"description":"Disk",
				"usage":1,
				"maximum":true
			}]`,
			wantError: "maximum: expected a string or number",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(
				func(response http.ResponseWriter, _ *http.Request) {
					response.Header().Set("Content-Type", "application/json")
					_, _ = response.Write([]byte(
						`{"status":1,"data":` + testCase.data + `}`,
					))
				},
			))
			defer server.Close()

			_, err := newTestClient(t, server).Get(t.Context())
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("Get() error = %v; want %q", err, testCase.wantError)
			}
		})
	}
}

func TestClientGetPropagatesUAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(
				`{"status":0,"errors":["resource usage unavailable"]}`,
			))
		},
	))
	defer server.Close()

	_, err := newTestClient(t, server).Get(t.Context())
	if err == nil || !strings.Contains(err.Error(), "resource usage unavailable") {
		t.Fatalf("Get() error = %v", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(server.URL, "account", "token")
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
