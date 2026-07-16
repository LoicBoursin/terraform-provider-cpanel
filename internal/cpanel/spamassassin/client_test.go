package spamassassin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientReadsSetsAndRemovesPreference(t *testing.T) {
	t.Parallel()

	state := &spamPreferenceTestState{}
	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	client := newTestClient(t, server.URL)
	preference, err := client.GetPreference(
		t.Context(),
		PreferenceRequiredScore,
	)
	if err != nil {
		t.Fatalf("GetPreference() error: %v", err)
	}
	if preference.Present {
		t.Fatalf("GetPreference() = %#v, want absent", preference)
	}

	preference, err = client.SetPreference(
		t.Context(),
		PreferenceRequiredScore,
		[]string{"5.5"},
	)
	if err != nil {
		t.Fatalf("SetPreference() error: %v", err)
	}
	if !PreferenceMatchesDefinition(*preference, Definition{
		Name:    PreferenceRequiredScore,
		Values:  []string{"5.5"},
		Present: true,
	}) {
		t.Fatalf("SetPreference() = %#v", preference)
	}

	preference, err = client.RemovePreference(
		t.Context(),
		PreferenceRequiredScore,
	)
	if err != nil {
		t.Fatalf("RemovePreference() error: %v", err)
	}
	if preference.Present {
		t.Fatalf("RemovePreference() = %#v, want absent", preference)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.setCalls != 1 || state.removeCalls != 1 {
		t.Fatalf(
			"mutation calls = set %d remove %d; want 1 each",
			state.setCalls,
			state.removeCalls,
		)
	}
}

func TestClientNumbersMultiplePreferenceValues(t *testing.T) {
	t.Parallel()

	state := &spamPreferenceTestState{}
	server := httptest.NewServer(http.HandlerFunc(state.handle))
	defer server.Close()

	values := []string{
		"first@example.test",
		"second@example.test",
	}
	if _, err := newTestClient(t, server.URL).SetPreference(
		t.Context(),
		PreferenceWhitelistFrom,
		values,
	); err != nil {
		t.Fatalf("SetPreference() error: %v", err)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	want := url.Values{
		"preference": {"whitelist_from"},
		"value-0":    {"first@example.test"},
		"value-1":    {"second@example.test"},
	}
	if !reflect.DeepEqual(state.lastForm, want) {
		t.Fatalf("mutation form = %#v, want %#v", state.lastForm, want)
	}
}

func TestClientRejectsMalformedPreferenceResponses(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		data      string
		wantError string
	}{
		"null": {
			data:      `null`,
			wantError: "expected an object",
		},
		"array": {
			data:      `[]`,
			wantError: "expected an object",
		},
		"scalar values": {
			data:      `{"required_score":"5.5"}`,
			wantError: "invalid values",
		},
		"empty values": {
			data:      `{"required_score":[]}`,
			wantError: "at least one value",
		},
		"duplicate values": {
			data:      `{"whitelist_from":["first@example.test","first@example.test"]}`,
			wantError: "duplicate value",
		},
		"invalid score": {
			data:      `{"required_score":["1000"]}`,
			wantError: "less than 1000",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write([]byte(
					`{"status":1,"data":` + test.data + `}`,
				))
			}))
			defer server.Close()

			preference := PreferenceRequiredScore
			if strings.Contains(test.data, "whitelist_from") {
				preference = PreferenceWhitelistFrom
			}
			_, err := newTestClient(t, server.URL).
				GetPreference(t.Context(), preference)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf(
					"GetPreference() error = %v, want containing %q",
					err,
					test.wantError,
				)
			}
		})
	}
}

func TestValidateDefinition(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		definition Definition
		wantError  bool
	}{
		"required score": {
			definition: Definition{
				Name:    PreferenceRequiredScore,
				Values:  []string{"5.5"},
				Present: true,
			},
		},
		"score rules": {
			definition: Definition{
				Name: PreferenceScore,
				Values: []string{
					"HTML_MESSAGE 1.25",
					"MISSING_MID 0.5",
				},
				Present: true,
			},
		},
		"email list": {
			definition: Definition{
				Name: PreferenceBlacklistFrom,
				Values: []string{
					"first@example.test",
					"second@example.test",
				},
				Present: true,
			},
		},
		"absent": {
			definition: Definition{
				Name:    PreferenceRequiredScore,
				Present: false,
			},
		},
		"unsupported": {
			definition: Definition{
				Name:    "custom",
				Values:  []string{"value"},
				Present: true,
			},
			wantError: true,
		},
		"zero score": {
			definition: Definition{
				Name:    PreferenceRequiredScore,
				Values:  []string{"0"},
				Present: true,
			},
			wantError: true,
		},
		"wildcard email": {
			definition: Definition{
				Name:    PreferenceWhitelistFrom,
				Values:  []string{"*@example.test"},
				Present: true,
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := ValidateDefinition(test.definition)
			if test.wantError && err == nil {
				t.Fatal("ValidateDefinition() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("ValidateDefinition() error: %v", err)
			}
		})
	}
}

type spamPreferenceTestState struct {
	mu sync.Mutex

	preferences map[string][]string
	lastForm    url.Values
	setCalls    int
	removeCalls int
}

func (s *spamPreferenceTestState) handle(
	response http.ResponseWriter,
	request *http.Request,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	response.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/execute/SpamAssassin/get_user_preferences":
		if request.Method != http.MethodGet {
			http.Error(response, "expected GET", http.StatusBadRequest)
			return
		}
		if s.preferences == nil {
			s.preferences = map[string][]string{}
		}
		writePreferencesResponse(response, s.preferences)
	case "/execute/SpamAssassin/update_user_preference":
		if request.Method != http.MethodPost {
			http.Error(response, "expected POST", http.StatusBadRequest)
			return
		}
		if err := request.ParseForm(); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		s.lastForm = cloneValues(request.PostForm)
		name := request.PostForm.Get("preference")
		values := preferenceValues(request.PostForm)
		if s.preferences == nil {
			s.preferences = map[string][]string{}
		}
		if len(values) == 0 {
			delete(s.preferences, name)
			s.removeCalls++
		} else {
			s.preferences[name] = values
			s.setCalls++
		}
		_, _ = response.Write([]byte(`{"status":1,"data":{}}`))
	default:
		http.NotFound(response, request)
	}
}

func preferenceValues(values url.Values) []string {
	if value, exists := values["value"]; exists {
		return append([]string{}, value...)
	}

	result := []string{}
	for index := 0; ; index++ {
		value, exists := values["value-"+strconv.Itoa(index)]
		if !exists {
			break
		}
		result = append(result, value...)
	}

	return result
}

func writePreferencesResponse(
	response http.ResponseWriter,
	preferences map[string][]string,
) {
	payload := struct {
		Status int                 `json:"status"`
		Data   map[string][]string `json:"data"`
	}{
		Status: 1,
		Data:   preferences,
	}
	_ = json.NewEncoder(response).Encode(payload)
}

func cloneValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for key, entries := range values {
		result[key] = append([]string{}, entries...)
	}

	return result
}

func newTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "account", "token")
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
