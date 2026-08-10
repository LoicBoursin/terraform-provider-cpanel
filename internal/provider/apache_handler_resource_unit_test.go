package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

func TestApacheHandlerDeleteDefinitionReconcilesAmbiguousMutation(
	t *testing.T,
) {
	expected := apachehandler.Definition{
		Extension: ".example",
		Handler:   "application/x-httpd-original",
	}
	concurrent := apachehandler.Definition{
		Extension: expected.Extension,
		Handler:   "application/x-httpd-concurrent",
	}

	testCases := []struct {
		name          string
		deleteOutcome string
		wantError     bool
		wantCurrent   *apachehandler.Definition
	}{
		{
			name:          "deletion applied before ambiguous response",
			deleteOutcome: "ambiguous_deleted",
		},
		{
			name:          "deletion not applied",
			deleteOutcome: "ambiguous_unchanged",
			wantError:     true,
			wantCurrent:   &expected,
		},
		{
			name:          "concurrent definition appeared",
			deleteOutcome: "ambiguous_concurrent",
			wantError:     true,
			wantCurrent:   &concurrent,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := &apacheHandlerResourceTestState{
				current:       apacheHandlerTestHandler(expected),
				concurrent:    apacheHandlerTestHandler(concurrent),
				deleteOutcome: testCase.deleteOutcome,
			}
			resource, server := newApacheHandlerResourceTestServer(t, state)
			defer server.Close()

			err := resource.deleteApacheHandlerDefinition(
				t.Context(),
				expected,
				nil,
				"delete the previous Apache handler",
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"deleteApacheHandlerDefinition() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if state.deleteCalls != 1 {
				t.Fatalf("deleteCalls = %d, want 1", state.deleteCalls)
			}
			assertApacheHandlerTestCurrent(
				t,
				state.current,
				testCase.wantCurrent,
			)
		})
	}
}

func TestApacheHandlerRestoreAcceptsOriginalAndRefusesConcurrentDefinition(
	t *testing.T,
) {
	original := apachehandler.Definition{
		Extension: ".example",
		Handler:   "application/x-httpd-original",
	}
	attempted := apachehandler.Definition{
		Extension: original.Extension,
		Handler:   "application/x-httpd-attempted",
	}
	concurrent := apachehandler.Definition{
		Extension: original.Extension,
		Handler:   "application/x-httpd-concurrent",
	}

	testCases := []struct {
		name          string
		current       apachehandler.Definition
		deleteOutcome string
		wantError     bool
		wantAddCalls  int
		wantDeletes   int
	}{
		{
			name:    "original already restored",
			current: original,
		},
		{
			name:         "attempted replacement is rolled back",
			current:      attempted,
			wantAddCalls: 1,
			wantDeletes:  1,
		},
		{
			name:          "ambiguous deletion restored original",
			current:       attempted,
			deleteOutcome: "ambiguous_restored",
			wantDeletes:   1,
		},
		{
			name:      "concurrent definition is preserved",
			current:   concurrent,
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := &apacheHandlerResourceTestState{
				current:       apacheHandlerTestHandler(testCase.current),
				original:      apacheHandlerTestHandler(original),
				deleteOutcome: testCase.deleteOutcome,
			}
			resource, server := newApacheHandlerResourceTestServer(t, state)
			defer server.Close()

			err := resource.restoreApacheHandler(
				t.Context(),
				attempted,
				original,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"restoreApacheHandler() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if state.addCalls != testCase.wantAddCalls {
				t.Fatalf(
					"addCalls = %d, want %d",
					state.addCalls,
					testCase.wantAddCalls,
				)
			}
			if state.deleteCalls != testCase.wantDeletes {
				t.Fatalf(
					"deleteCalls = %d, want %d",
					state.deleteCalls,
					testCase.wantDeletes,
				)
			}
			if testCase.wantError {
				assertApacheHandlerTestCurrent(
					t,
					state.current,
					&concurrent,
				)
				return
			}
			assertApacheHandlerTestCurrent(t, state.current, &original)
		})
	}
}

type apacheHandlerResourceTestState struct {
	current       *apachehandler.Handler
	concurrent    *apachehandler.Handler
	original      *apachehandler.Handler
	deleteOutcome string
	addCalls      int
	deleteCalls   int
}

func newApacheHandlerResourceTestServer(
	t *testing.T,
	state *apacheHandlerResourceTestState,
) (*apacheHandlerResource, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Mime/list_handlers":
			data := []apachehandler.Handler{}
			if state.current != nil {
				data = append(data, *state.current)
			}
			writeApacheHandlerResourceTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   data,
			})
		case "/execute/Mime/add_handler":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			state.addCalls++
			state.current = &apachehandler.Handler{
				Extension: request.Form.Get("extension"),
				Handler:   request.Form.Get("handler"),
				Origin:    "user",
			}
			writeApacheHandlerResourceTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   nil,
			})
		case "/execute/Mime/delete_handler":
			state.deleteCalls++
			switch state.deleteOutcome {
			case "ambiguous_deleted":
				state.current = nil
				_, _ = response.Write([]byte("{"))
			case "ambiguous_unchanged":
				_, _ = response.Write([]byte("{"))
			case "ambiguous_concurrent":
				state.current = state.concurrent
				_, _ = response.Write([]byte("{"))
			case "ambiguous_restored":
				state.current = state.original
				_, _ = response.Write([]byte("{"))
			default:
				state.current = nil
				writeApacheHandlerResourceTestJSON(
					t,
					response,
					map[string]any{"status": 1, "data": nil},
				)
			}
		default:
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		server.Close()
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return &apacheHandlerResource{
		client: apachehandler.NewClient(baseClient),
	}, server
}

func apacheHandlerTestHandler(
	definition apachehandler.Definition,
) *apachehandler.Handler {
	return &apachehandler.Handler{
		Extension: definition.Extension,
		Handler:   definition.Handler,
		Origin:    "user",
	}
}

func assertApacheHandlerTestCurrent(
	t *testing.T,
	current *apachehandler.Handler,
	expected *apachehandler.Definition,
) {
	t.Helper()

	if expected == nil {
		if current != nil {
			t.Fatalf("current = %#v, want nil", current)
		}
		return
	}
	if !apacheHandlerMatchesDefinition(current, *expected) {
		t.Fatalf("current = %#v, want %#v", current, *expected)
	}
}

func writeApacheHandlerResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
