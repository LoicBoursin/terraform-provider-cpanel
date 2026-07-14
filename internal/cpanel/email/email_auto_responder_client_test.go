package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSetsAutoResponderWithPOST(t *testing.T) {
	t.Parallel()

	start := int64(1_800_000_000)
	stop := int64(1_800_086_400)
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/add_auto_responder" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}

		expected := map[string]string{
			"email":    "away",
			"domain":   "example.test",
			"from":     "Terraform",
			"subject":  "Away",
			"body":     "Back soon",
			"charset":  "UTF-8",
			"interval": "8",
			"is_html":  "1",
			"start":    "1800000000",
			"stop":     "1800086400",
		}
		for key, value := range expected {
			if got := request.Form.Get(key); got != value {
				t.Errorf("%s = %q, want %q", key, got, value)
			}
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.SetAutoResponder(
		context.Background(),
		"away",
		"example.test",
		AutoResponder{
			From:     "Terraform",
			Subject:  "Away",
			Body:     "Back soon",
			Charset:  "UTF-8",
			Interval: 8,
			IsHTML:   1,
			Start:    &start,
			Stop:     &stop,
		},
	); err != nil {
		t.Fatalf("SetAutoResponder() error: %v", err)
	}
}

func TestClientGetsExistingAutoResponder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/list_auto_responders":
			_ = json.NewEncoder(response).Encode(map[string]any{
				"status": 1,
				"data": []map[string]string{
					{
						"email":   "away@example.test",
						"subject": "Away",
					},
				},
			})
		case "/execute/Email/get_auto_responder":
			if got := request.URL.Query().Get("email"); got != "away@example.test" {
				t.Errorf("email = %q, want away@example.test", got)
			}
			_, _ = response.Write([]byte(
				`{"status":1,"data":{"from":"Terraform","subject":"Away","body":"Back soon\n","charset":"UTF-8","interval":8,"is_html":0,"start":null,"stop":null}}`,
			))
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	autoResponder, err := client.GetAutoResponder(
		context.Background(),
		"away@example.test",
		"example.test",
	)
	if err != nil {
		t.Fatalf("GetAutoResponder() error: %v", err)
	}
	if autoResponder == nil {
		t.Fatal("GetAutoResponder() returned nil")
	}
	if autoResponder.Email != "away@example.test" ||
		autoResponder.Subject != "Away" ||
		autoResponder.Body != "Back soon\n" ||
		autoResponder.StartUnix() != 0 ||
		autoResponder.StopUnix() != 0 {
		t.Fatalf("autoResponder = %#v", autoResponder)
	}
}

func TestClientSkipsDetailsForMissingAutoResponder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/execute/Email/list_auto_responders" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		_, _ = response.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	autoResponder, err := client.GetAutoResponder(
		context.Background(),
		"missing@example.test",
		"example.test",
	)
	if err != nil {
		t.Fatalf("GetAutoResponder() error: %v", err)
	}
	if autoResponder != nil {
		t.Fatalf("GetAutoResponder() = %#v, want nil", autoResponder)
	}
}

func TestClientDeletesAutoResponderWithFullAddress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/Email/delete_auto_responder" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("email"); got != "away@example.test" {
			t.Errorf("email = %q, want away@example.test", got)
		}

		_, _ = response.Write([]byte(`{"status":1,"data":null}`))
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	if err := client.DeleteAutoResponder(
		context.Background(),
		"away@example.test",
	); err != nil {
		t.Fatalf("DeleteAutoResponder() error: %v", err)
	}
}
