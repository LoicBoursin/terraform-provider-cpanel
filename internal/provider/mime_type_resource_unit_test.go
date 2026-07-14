package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

func TestMIMETypeApplyDefinitionTracksOnlyAttributablePrefixes(
	t *testing.T,
) {
	definition := mimetype.Definition{
		Type:       "application/x-example",
		Extensions: []string{".one", ".two", ".three"},
	}

	testCases := []struct {
		name              string
		secondAddOutcome  string
		wantSecondPrefix  bool
		wantRemoteCurrent mimetype.Definition
	}{
		{
			name:             "ambiguous extension may have applied",
			secondAddOutcome: "ambiguous_applied",
			wantRemoteCurrent: mimetype.Definition{
				Type:       definition.Type,
				Extensions: []string{".one", ".three"},
			},
		},
		{
			name:             "deterministic rejection did not apply",
			secondAddOutcome: "deterministic_failure",
			wantRemoteCurrent: mimetype.Definition{
				Type:       definition.Type,
				Extensions: []string{".one"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := &mimeTypeResourceTestState{
				addOutcomes: map[string]string{
					".three": testCase.secondAddOutcome,
				},
			}
			resource, server := newMIMETypeResourceTestServer(t, state)
			defer server.Close()

			trace, err := resource.applyDefinition(t.Context(), definition)
			if err == nil {
				t.Fatal("applyDefinition() returned no error")
			}
			if strings.Join(state.addCalls, ",") != ".one,.three" {
				t.Fatalf("addCalls = %v, want [.one .three]", state.addCalls)
			}

			firstPrefix := mimetype.Definition{
				Type:       definition.Type,
				Extensions: []string{".one"},
			}
			if !trace.matches(mimeTypeResourceTestMIMEType(firstPrefix)) {
				t.Fatal("trace does not contain the confirmed first prefix")
			}
			secondPrefix := mimetype.Definition{
				Type:       definition.Type,
				Extensions: []string{".one", ".three"},
			}
			if trace.matches(mimeTypeResourceTestMIMEType(secondPrefix)) !=
				testCase.wantSecondPrefix {
				t.Fatalf(
					"second prefix attribution = %t, want %t",
					trace.matches(mimeTypeResourceTestMIMEType(secondPrefix)),
					testCase.wantSecondPrefix,
				)
			}
			unattributable := mimetype.Definition{
				Type:       definition.Type,
				Extensions: []string{".one", ".other"},
			}
			if trace.matches(mimeTypeResourceTestMIMEType(unattributable)) {
				t.Fatal("trace contains an unrelated extension set")
			}
			assertMIMETypeResourceTestCurrent(
				t,
				state.current,
				&testCase.wantRemoteCurrent,
			)
		})
	}
}

func TestMIMETypeApplyDefinitionStopsBeforeConcurrentDefinition(
	t *testing.T,
) {
	definition := mimetype.Definition{
		Type:       "application/x-example",
		Extensions: []string{".one", ".two"},
	}
	concurrent := mimetype.Definition{
		Type:       definition.Type,
		Extensions: []string{".external"},
	}
	state := &mimeTypeResourceTestState{
		concurrent:               mimeTypeResourceTestMIMEType(concurrent),
		concurrentAfterExtension: ".one",
	}
	resource, server := newMIMETypeResourceTestServer(t, state)
	defer server.Close()

	trace, err := resource.applyDefinition(t.Context(), definition)
	if err == nil {
		t.Fatal("applyDefinition() returned no error")
	}
	if strings.Join(state.addCalls, ",") != ".one" {
		t.Fatalf("addCalls = %v, want [.one]", state.addCalls)
	}
	firstPrefix := mimetype.Definition{
		Type:       definition.Type,
		Extensions: []string{".one"},
	}
	if !trace.matches(mimeTypeResourceTestMIMEType(firstPrefix)) {
		t.Fatal("trace does not contain the confirmed first prefix")
	}
	if trace.matches(state.current) {
		t.Fatal("trace attributes the concurrent definition")
	}
	assertMIMETypeResourceTestCurrent(t, state.current, &concurrent)
}

func TestMIMETypeRollbackDeletesOnlyAttributablePrefix(t *testing.T) {
	definition := mimetype.Definition{
		Type:       "application/x-example",
		Extensions: []string{".one", ".two", ".three"},
	}
	trace := mimeTypeResourceTestTrace(definition, 1, 2)
	attributable := mimetype.Definition{
		Type:       definition.Type,
		Extensions: []string{".one", ".three"},
	}
	concurrent := mimetype.Definition{
		Type:       definition.Type,
		Extensions: []string{".one", ".other"},
	}

	testCases := []struct {
		name        string
		current     mimetype.Definition
		wantError   bool
		wantDeletes int
	}{
		{
			name:        "attributable prefix",
			current:     attributable,
			wantDeletes: 1,
		},
		{
			name:      "concurrent definition",
			current:   concurrent,
			wantError: true,
		},
		{
			name:      "unrecorded longer prefix",
			current:   definition,
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			state := &mimeTypeResourceTestState{
				current: mimeTypeResourceTestMIMEType(testCase.current),
			}
			resource, server := newMIMETypeResourceTestServer(t, state)
			defer server.Close()

			err := resource.rollbackCreatedMIMEType(t.Context(), trace)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"rollbackCreatedMIMEType() error = %v, wantError %t",
					err,
					testCase.wantError,
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
				assertMIMETypeResourceTestCurrent(
					t,
					state.current,
					&testCase.current,
				)
			} else {
				assertMIMETypeResourceTestCurrent(t, state.current, nil)
			}
		})
	}
}

func TestMIMETypeRestoreAcceptsOriginalAndRefusesConcurrentDefinition(
	t *testing.T,
) {
	original := mimetype.Definition{
		Type:       "application/x-example",
		Extensions: []string{".old-one", ".old-two"},
	}
	attempted := mimetype.Definition{
		Type:       original.Type,
		Extensions: []string{".new-one", ".new-two", ".new-three"},
	}
	trace := mimeTypeResourceTestTrace(attempted, 1, 2, 3)
	partial := mimetype.Definition{
		Type:       attempted.Type,
		Extensions: []string{".new-one", ".new-three"},
	}
	concurrent := mimetype.Definition{
		Type:       attempted.Type,
		Extensions: []string{".new-one", ".other"},
	}

	testCases := []struct {
		name          string
		current       mimetype.Definition
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
			name:         "partial attempted prefix is rolled back",
			current:      partial,
			wantAddCalls: 2,
			wantDeletes:  1,
		},
		{
			name:          "ambiguous deletion restored original",
			current:       partial,
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
			state := &mimeTypeResourceTestState{
				current:       mimeTypeResourceTestMIMEType(testCase.current),
				original:      mimeTypeResourceTestMIMEType(original),
				deleteOutcome: testCase.deleteOutcome,
			}
			resource, server := newMIMETypeResourceTestServer(t, state)
			defer server.Close()

			err := resource.restoreMIMEType(t.Context(), trace, original)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"restoreMIMEType() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if len(state.addCalls) != testCase.wantAddCalls {
				t.Fatalf(
					"addCalls = %v, want %d calls",
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
				assertMIMETypeResourceTestCurrent(
					t,
					state.current,
					&concurrent,
				)
				return
			}
			assertMIMETypeResourceTestCurrent(t, state.current, &original)
		})
	}
}

func TestMIMETypeDeleteDefinitionReconcilesAmbiguousMutation(t *testing.T) {
	expected := mimetype.Definition{
		Type:       "application/x-example",
		Extensions: []string{".one", ".two"},
	}
	concurrent := mimetype.Definition{
		Type:       expected.Type,
		Extensions: []string{".other"},
	}

	testCases := []struct {
		name          string
		deleteOutcome string
		wantError     bool
		wantCurrent   *mimetype.Definition
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
			state := &mimeTypeResourceTestState{
				current:       mimeTypeResourceTestMIMEType(expected),
				concurrent:    mimeTypeResourceTestMIMEType(concurrent),
				deleteOutcome: testCase.deleteOutcome,
			}
			resource, server := newMIMETypeResourceTestServer(t, state)
			defer server.Close()

			err := resource.deleteMIMETypeDefinition(
				t.Context(),
				expected,
				nil,
				"delete the previous MIME type",
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"deleteMIMETypeDefinition() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if state.deleteCalls != 1 {
				t.Fatalf("deleteCalls = %d, want 1", state.deleteCalls)
			}
			assertMIMETypeResourceTestCurrent(
				t,
				state.current,
				testCase.wantCurrent,
			)
		})
	}
}

type mimeTypeResourceTestState struct {
	current                  *mimetype.MIMEType
	concurrent               *mimetype.MIMEType
	original                 *mimetype.MIMEType
	addOutcomes              map[string]string
	concurrentAfterExtension string
	deleteOutcome            string
	addCalls                 []string
	deleteCalls              int
}

func newMIMETypeResourceTestServer(
	t *testing.T,
	state *mimeTypeResourceTestState,
) (*mimeTypeResource, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Mime/list_mime":
			data := []mimetype.MIMEType{}
			if state.current != nil {
				data = append(data, *state.current)
			}
			writeMIMETypeResourceTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   data,
			})
		case "/execute/Mime/add_mime":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			extension := request.Form.Get("extension")
			state.addCalls = append(state.addCalls, extension)
			switch state.addOutcomes[extension] {
			case "ambiguous_applied":
				applyMIMETypeResourceTestExtension(
					state,
					request.Form.Get("type"),
					extension,
				)
				_, _ = response.Write([]byte("{"))
			case "deterministic_failure":
				writeMIMETypeResourceTestJSON(
					t,
					response,
					map[string]any{
						"status": 0,
						"errors": []string{"rejected"},
						"data":   nil,
					},
				)
			default:
				applyMIMETypeResourceTestExtension(
					state,
					request.Form.Get("type"),
					extension,
				)
				if state.concurrentAfterExtension == extension {
					state.current = state.concurrent
				}
				writeMIMETypeResourceTestJSON(
					t,
					response,
					map[string]any{"status": 1, "data": nil},
				)
			}
		case "/execute/Mime/delete_mime":
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
				writeMIMETypeResourceTestJSON(
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

	return &mimeTypeResource{
		client: mimetype.NewClient(baseClient),
	}, server
}

func applyMIMETypeResourceTestExtension(
	state *mimeTypeResourceTestState,
	mimeTypeName string,
	extension string,
) {
	extensions := []string{}
	if state.current != nil && state.current.Type == mimeTypeName {
		extensions = state.current.Extensions()
	}
	for _, existing := range extensions {
		if existing == extension {
			return
		}
	}
	extensions = append(extensions, extension)
	sort.Strings(extensions)
	state.current = &mimetype.MIMEType{
		Type:      mimeTypeName,
		Extension: strings.Join(extensions, " "),
		Origin:    "user",
	}
}

func mimeTypeResourceTestTrace(
	definition mimetype.Definition,
	prefixLengths ...int,
) mimeTypeApplyTrace {
	definition = definition.Sorted()
	trace := mimeTypeApplyTrace{definition: definition}
	for _, prefixLength := range prefixLengths {
		trace.addAttributablePrefix(mimetype.Definition{
			Type: definition.Type,
			Extensions: append(
				[]string(nil),
				definition.Extensions[:prefixLength]...,
			),
		})
	}

	return trace
}

func mimeTypeResourceTestMIMEType(
	definition mimetype.Definition,
) *mimetype.MIMEType {
	definition = definition.Sorted()

	return &mimetype.MIMEType{
		Type:      definition.Type,
		Extension: strings.Join(definition.Extensions, " "),
		Origin:    "user",
	}
}

func assertMIMETypeResourceTestCurrent(
	t *testing.T,
	current *mimetype.MIMEType,
	expected *mimetype.Definition,
) {
	t.Helper()

	if expected == nil {
		if current != nil {
			t.Fatalf("current = %#v, want nil", current)
		}
		return
	}
	if !mimeTypeMatchesDefinition(current, *expected) {
		t.Fatalf("current = %#v, want %#v", current, *expected)
	}
}

func writeMIMETypeResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
