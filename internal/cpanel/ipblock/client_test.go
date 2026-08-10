package ipblock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndNormalizesBlockedAddress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/json-api/cpanel" {
			t.Errorf("path = %s", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("cpanel_jsonapi_module") != "DenyIp" ||
			query.Get("cpanel_jsonapi_func") != "listdenyips" {
			t.Errorf("query = %v", query)
		}

		_ = json.NewEncoder(response).Encode(map[string]any{
			"cpanelresult": map[string]any{
				"event": map[string]int{"result": 1},
				"data": []map[string]string{
					{
						"ip":    "2001:0db8:0000:0000:0000:0000:0000:0123",
						"range": "2001:0db8:0000:0000:0000:0000:0000:0123",
						"start": "2001:0db8:0000:0000:0000:0000:0000:0123",
						"end":   "2001:0db8:0000:0000:0000:0000:0000:0123",
					},
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	blockedAddress, err := NewClient(baseClient).GetAddress(
		context.Background(),
		"2001:db8::123",
	)
	if err != nil {
		t.Fatalf("GetAddress() error: %v", err)
	}
	if blockedAddress == nil {
		t.Fatal("GetAddress() returned nil")
	}
	if blockedAddress.Address != "2001:db8::123" ||
		blockedAddress.Start != "2001:db8::123" ||
		blockedAddress.End != "2001:db8::123" {
		t.Fatalf("blockedAddress = %#v", blockedAddress)
	}
}

func TestClientReassemblesBlockedAddressRange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_ = json.NewEncoder(response).Encode(map[string]any{
			"cpanelresult": map[string]any{
				"event": map[string]int{"result": 1},
				"data": []map[string]string{
					{
						"ip":    "198.51.100.240/31",
						"range": "198.51.100.240-198.51.100.241",
						"start": "198.51.100.240",
						"end":   "198.51.100.241",
					},
					{
						"ip":    "198.51.100.242",
						"range": "198.51.100.242",
						"start": "198.51.100.242",
						"end":   "198.51.100.242",
					},
					{
						"ip":    "198.51.100.224/28",
						"range": "198.51.100.224-198.51.100.239",
						"start": "198.51.100.224",
						"end":   "198.51.100.239",
					},
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	blockedAddress, err := NewClient(baseClient).GetAddress(
		context.Background(),
		"198.51.100.240-198.51.100.242",
	)
	if err != nil {
		t.Fatalf("GetAddress() error: %v", err)
	}
	if blockedAddress == nil {
		t.Fatal("GetAddress() returned nil")
	}
	if blockedAddress.Address != "198.51.100.240-198.51.100.242" ||
		blockedAddress.Start != "198.51.100.240" ||
		blockedAddress.End != "198.51.100.242" {
		t.Fatalf("blockedAddress = %#v", blockedAddress)
	}
}

func TestClientRejectsIncompleteBlockedAddressRange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		_ = json.NewEncoder(response).Encode(map[string]any{
			"cpanelresult": map[string]any{
				"event": map[string]int{"result": 1},
				"data": []map[string]string{
					{
						"ip":    "198.51.100.240",
						"range": "198.51.100.240",
						"start": "198.51.100.240",
						"end":   "198.51.100.240",
					},
					{
						"ip":    "198.51.100.242",
						"range": "198.51.100.242",
						"start": "198.51.100.242",
						"end":   "198.51.100.242",
					},
				},
			},
		})
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	blockedAddress, err := NewClient(baseClient).GetAddress(
		context.Background(),
		"198.51.100.240-198.51.100.242",
	)
	if err != nil {
		t.Fatalf("GetAddress() error: %v", err)
	}
	if blockedAddress != nil {
		t.Fatalf("GetAddress() = %#v, want nil", blockedAddress)
	}
}

func TestClientMutatesBlockedAddressWithPOST(t *testing.T) {
	t.Parallel()

	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		paths = append(paths, request.URL.Path)
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("ip"); got != "198.51.100.77" {
			t.Errorf("ip = %q", got)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":["198.51.100.77"]}`,
		))
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	client := NewClient(baseClient)

	if err := client.AddAddress(context.Background(), "198.51.100.77"); err != nil {
		t.Fatalf("AddAddress() error: %v", err)
	}
	if err := client.RemoveAddress(context.Background(), "198.51.100.77"); err != nil {
		t.Fatalf("RemoveAddress() error: %v", err)
	}
	if len(paths) != 2 ||
		paths[0] != "/execute/BlockIP/add_ip" ||
		paths[1] != "/execute/BlockIP/remove_ip" {
		t.Fatalf("paths = %v", paths)
	}
}
