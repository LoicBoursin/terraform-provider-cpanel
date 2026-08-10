package contactinformation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientGetNotificationPreferences(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/ContactInformation/get_notification_preferences",
		)
		writeJSON(
			t,
			response,
			`{"status":1,"data":[{"name":"notify_disk_limit","enabled":"1","descp":"Disk quota."},{"name":"notify_ssl_expiry","enabled":0,"descp":"SSL expiry."}]}`,
		)
	}))
	defer server.Close()

	actual, err := newTestClient(t, server.URL).
		GetNotificationPreferences(t.Context())
	if err != nil {
		t.Fatalf("GetNotificationPreferences() error: %v", err)
	}
	want := &NotificationPreferences{
		Preferences: map[string]bool{
			"notify_disk_limit": true,
			"notify_ssl_expiry": false,
		},
		Descriptions: map[string]string{
			"notify_disk_limit": "Disk quota.",
			"notify_ssl_expiry": "SSL expiry.",
		},
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf(
			"GetNotificationPreferences() = %#v, want %#v",
			actual,
			want,
		)
	}
}

func TestClientSetNotificationPreferences(t *testing.T) {
	t.Parallel()

	current := map[string]bool{
		"notify_disk_limit": true,
		"notify_ssl_expiry": true,
	}
	target := map[string]bool{
		"notify_disk_limit": false,
		"notify_ssl_expiry": true,
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch calls.Add(1) {
		case 1:
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/ContactInformation/get_notification_preferences",
			)
			writeNotificationPreferences(t, response, current)
		case 2:
			assertRequest(
				t,
				request,
				http.MethodPost,
				"/execute/ContactInformation/set_notification_preferences",
			)
			if got := request.Header.Get("Content-Type"); got !=
				"application/json" {
				t.Fatalf("Content-Type = %q", got)
			}
			var input notificationPreferencesRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			want := map[string]int{
				"notify_disk_limit": 0,
				"notify_ssl_expiry": 1,
			}
			if !reflect.DeepEqual(input.Preferences, want) {
				t.Fatalf(
					"request preferences = %#v, want %#v",
					input.Preferences,
					want,
				)
			}
			current = clonePreferences(target)
			writeJSON(t, response, `{"status":1,"data":null}`)
		case 3:
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/ContactInformation/get_notification_preferences",
			)
			writeNotificationPreferences(t, response, current)
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()

	actual, err := newTestClient(t, server.URL).
		SetNotificationPreferences(t.Context(), target)
	if err != nil {
		t.Fatalf("SetNotificationPreferences() error: %v", err)
	}
	if !PreferencesMatchDefinition(*actual, target) {
		t.Fatalf(
			"SetNotificationPreferences() = %#v, want %#v",
			actual,
			target,
		)
	}
	if calls.Load() != 3 {
		t.Fatalf("request count = %d, want 3", calls.Load())
	}
}

func TestClientRejectsPreferenceNameMismatchBeforeMutation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		calls.Add(1)
		if request.Method != http.MethodGet {
			t.Fatalf("request method = %s, want GET", request.Method)
		}
		writeNotificationPreferences(
			t,
			response,
			map[string]bool{"notify_disk_limit": true},
		)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).SetNotificationPreferences(
		t.Context(),
		map[string]bool{"notify_ssl_expiry": true},
	)
	if err == nil || !strings.Contains(err.Error(), "do not exactly match") {
		t.Fatalf("SetNotificationPreferences() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestClientRejectsInvalidDefinitionsBeforeRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		calls.Add(1)
	}))
	defer server.Close()

	tests := []map[string]bool{
		{},
		{"disk_limit": true},
	}
	for _, definition := range tests {
		_, err := newTestClient(t, server.URL).SetNotificationPreferences(
			t.Context(),
			definition,
		)
		if err == nil {
			t.Fatalf(
				"SetNotificationPreferences(%#v) returned no error",
				definition,
			)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("request count = %d, want 0", calls.Load())
	}
}

func TestClientRejectsMalformedPreferences(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		data    string
		wantErr string
	}{
		"empty": {
			data:    `[]`,
			wantErr: "no account notification preferences",
		},
		"invalid name": {
			data:    `[{"name":"disk_limit","enabled":1}]`,
			wantErr: "invalid cPanel notification preference name",
		},
		"duplicate": {
			data:    `[{"name":"notify_disk_limit","enabled":1},{"name":"notify_disk_limit","enabled":0}]`,
			wantErr: "duplicate notification preference",
		},
		"invalid flag": {
			data:    `[{"name":"notify_disk_limit","enabled":true}]`,
			wantErr: "invalid enabled flag",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeJSON(
					t,
					response,
					`{"status":1,"data":`+test.data+`}`,
				)
			}))
			defer server.Close()

			_, err := newTestClient(t, server.URL).
				GetNotificationPreferences(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"GetNotificationPreferences() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsPostMutationMismatch(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch calls.Add(1) {
		case 1:
			writeNotificationPreferences(
				t,
				response,
				map[string]bool{"notify_disk_limit": true},
			)
		case 2:
			writeJSON(t, response, `{"status":1,"data":null}`)
		case 3:
			writeNotificationPreferences(
				t,
				response,
				map[string]bool{"notify_disk_limit": true},
			)
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).SetNotificationPreferences(
		t.Context(),
		map[string]bool{"notify_disk_limit": false},
	)
	if err == nil || !strings.Contains(err.Error(), "after mutation") {
		t.Fatalf("SetNotificationPreferences() error = %v", err)
	}
}

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()

	client, err := cpanel.NewClient(
		serverURL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(client)
}

func assertRequest(
	t *testing.T,
	request *http.Request,
	method string,
	path string,
) {
	t.Helper()

	if request.Method != method {
		t.Fatalf("request method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != path {
		t.Fatalf("request path = %q, want %q", request.URL.Path, path)
	}
	if request.Header.Get("Authorization") != "cpanel username:api-token" {
		t.Fatalf(
			"Authorization header = %q",
			request.Header.Get("Authorization"),
		)
	}
}

func writeNotificationPreferences(
	t *testing.T,
	response http.ResponseWriter,
	preferences map[string]bool,
) {
	t.Helper()

	type responsePreference struct {
		Name        string `json:"name"`
		Enabled     int    `json:"enabled"`
		Description string `json:"descp"`
	}
	items := make([]responsePreference, 0, len(preferences))
	for _, name := range sortedPreferenceNames(preferences) {
		enabled := 0
		if preferences[name] {
			enabled = 1
		}
		items = append(items, responsePreference{
			Name:        name,
			Enabled:     enabled,
			Description: name + " description",
		})
	}
	body, err := json.Marshal(map[string]any{
		"status": 1,
		"data":   items,
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	writeJSON(t, response, string(body))
}

func writeJSON(
	t *testing.T,
	response http.ResponseWriter,
	body string,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if _, err := response.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
