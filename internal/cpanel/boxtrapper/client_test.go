package boxtrapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

const boxTrapperTestAccount = "box@example.test"

func TestClientListsBoxTrapperAccountsSorted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertAPI2InventoryRequest(t, request)
		writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
			boxTrapperInventoryEntry("z@example.test", "1"),
			boxTrapperInventoryEntry("a@example.test", 0),
		))
	}))
	defer server.Close()

	accounts, err := newBoxTrapperTestClient(
		t,
		server.URL,
	).listAccounts(t.Context())
	if err != nil {
		t.Fatalf("listAccounts() error: %v", err)
	}
	want := []accountInventoryEntry{
		{Account: "a@example.test", Enabled: false},
		{Account: "z@example.test", Enabled: true},
	}
	if !reflect.DeepEqual(accounts, want) {
		t.Fatalf("listAccounts() = %#v, want %#v", accounts, want)
	}
}

func TestClientRejectsDuplicateBoxTrapperAccounts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
			boxTrapperInventoryEntry(boxTrapperTestAccount, 0),
			boxTrapperInventoryEntry(boxTrapperTestAccount, "0"),
		))
	}))
	defer server.Close()

	_, err := newBoxTrapperTestClient(
		t,
		server.URL,
	).listAccounts(t.Context())
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("listAccounts() error = %v, want duplicate error", err)
	}
}

func TestClientRejectsInvalidBoxTrapperInventorySchema(t *testing.T) {
	t.Parallel()

	validEntry := `{"account":"box@example.test","accounturi":"box%40example.test","bg":"even","enabled":0,"status":"Localized status"}`
	tests := map[string]struct {
		body    string
		wantErr string
	}{
		"missing entry field": {
			body: api2InventoryRaw(
				`[{"account":"box@example.test","accounturi":"box%40example.test","bg":"even","enabled":0}]`,
			),
			wantErr: "missing required field",
		},
		"unexpected entry field": {
			body: api2InventoryRaw(
				`[{"account":"box@example.test","accounturi":"box%40example.test","bg":"even","enabled":0,"status":"Localized","extra":true}]`,
			),
			wantErr: "unexpected field",
		},
		"duplicate entry field": {
			body: api2InventoryRaw(
				`[{"account":"box@example.test","account":"other@example.test","accounturi":"box%40example.test","bg":"even","enabled":0,"status":"Localized"}]`,
			),
			wantErr: "duplicate field",
		},
		"invalid enabled flag": {
			body: api2InventoryRaw(
				`[{"account":"box@example.test","accounturi":"box%40example.test","bg":"even","enabled":2,"status":"Localized"}]`,
			),
			wantErr: "integer or string 0 or 1",
		},
		"data is not an array": {
			body:    api2InventoryRaw(`{}`),
			wantErr: "decode API 2 response envelope",
		},
		"wrong function identity": {
			body: strings.Replace(
				api2InventoryRaw("["+validEntry+"]"),
				`"func":"accountmanagelist"`,
				`"func":"other"`,
				1,
			),
			wantErr: "func is",
		},
		"unexpected event field": {
			body: strings.Replace(
				api2InventoryRaw("["+validEntry+"]"),
				`"event":{"result":1}`,
				`"event":{"result":1,"reason":"ok"}`,
				1,
			),
			wantErr: "unexpected field",
		},
		"invalid preevent shape": {
			body: strings.Replace(
				api2InventoryRaw("["+validEntry+"]"),
				`"module":"BoxTrapper"`,
				`"module":"BoxTrapper","preevent":[]`,
				1,
			),
			wantErr: "must be an object",
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
				writeBoxTrapperRaw(t, response, test.body)
			}))
			defer server.Close()

			_, err := newBoxTrapperTestClient(
				t,
				server.URL,
			).listAccounts(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"listAccounts() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientReturnsNilWhenBoxTrapperAccountIsAbsent(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		assertAPI2InventoryRequest(t, request)
		writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
			boxTrapperInventoryEntry("other@example.test", 0),
		))
	}))
	defer server.Close()

	settings, err := newBoxTrapperTestClient(t, server.URL).Get(
		t.Context(),
		boxTrapperTestAccount,
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if settings != nil {
		t.Fatalf("Get() = %#v, want nil", settings)
	}
	if requests.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requests.Load())
	}
}

func TestClientGetsCompleteBoxTrapperSettingsWithWireTypes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		inventoryEnabled any
		statusData       any
		configuration    map[string]any
		definition       Definition
		fromName         *string
	}{
		"new mailbox integer values and null from name": {
			inventoryEnabled: 0,
			statusData:       0,
			configuration: boxTrapperConfigurationData(
				1,
				boxTrapperTestAccount,
				nil,
				15,
				-2.5,
				1,
			),
			definition: Definition{
				Enabled:                false,
				EnableAutoWhitelist:    true,
				FromAddresses:          boxTrapperTestAccount,
				QueueDays:              15,
				SpamScore:              -2.5,
				WhitelistByAssociation: true,
			},
		},
		"post-mutation string values and empty from name": {
			inventoryEnabled: "1",
			statusData:       "1",
			configuration: boxTrapperConfigurationData(
				"0",
				"sender@example.test",
				"",
				"9",
				"3.7",
				"0",
			),
			definition: Definition{
				Enabled:                true,
				EnableAutoWhitelist:    false,
				FromAddresses:          "sender@example.test",
				QueueDays:              9,
				SpamScore:              3.7,
				WhitelistByAssociation: false,
			},
			fromName: stringPointer(""),
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				requests.Add(1)
				switch request.URL.Path {
				case "/json-api/cpanel":
					assertAPI2InventoryRequest(t, request)
					writeBoxTrapperJSON(
						t,
						response,
						api2InventoryEnvelope(
							boxTrapperInventoryEntry(
								boxTrapperTestAccount,
								test.inventoryEnabled,
							),
						),
					)
				case "/execute/BoxTrapper/get_status":
					assertUAPIRequest(
						t,
						request,
						http.MethodGet,
						"/execute/BoxTrapper/get_status",
						url.Values{"email": {boxTrapperTestAccount}},
					)
					writeBoxTrapperJSON(
						t,
						response,
						uapiEnvelopePayload(test.statusData, nil),
					)
				case "/execute/BoxTrapper/get_configuration":
					assertUAPIRequest(
						t,
						request,
						http.MethodGet,
						"/execute/BoxTrapper/get_configuration",
						url.Values{"email": {boxTrapperTestAccount}},
					)
					writeBoxTrapperJSON(
						t,
						response,
						uapiEnvelopePayload(test.configuration, nil),
					)
				default:
					t.Errorf("unexpected path %s", request.URL.Path)
				}
			}))
			defer server.Close()

			settings, err := newBoxTrapperTestClient(
				t,
				server.URL,
			).Get(t.Context(), boxTrapperTestAccount)
			if err != nil {
				t.Fatalf("Get() error: %v", err)
			}
			if settings == nil {
				t.Fatal("Get() returned nil")
			}
			if settings.Account != boxTrapperTestAccount ||
				!SettingsMatchDefinition(*settings, test.definition) {
				t.Fatalf("Get() = %#v, want matching %#v", settings, test.definition)
			}
			if !stringPointersEqual(settings.FromName, test.fromName) {
				t.Fatalf(
					"FromName = %#v, want %#v",
					settings.FromName,
					test.fromName,
				)
			}
			if requests.Load() != 3 {
				t.Fatalf("request count = %d, want 3", requests.Load())
			}
		})
	}
}

func TestClientRejectsBoxTrapperInventoryStatusMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/json-api/cpanel":
			writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
				boxTrapperInventoryEntry(boxTrapperTestAccount, 1),
			))
		case "/execute/BoxTrapper/get_status":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload("0", nil),
			)
		case "/execute/BoxTrapper/get_configuration":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(defaultBoxTrapperConfiguration(), nil),
			)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	_, err := newBoxTrapperTestClient(t, server.URL).Get(
		t.Context(),
		boxTrapperTestAccount,
	)
	if err == nil || !strings.Contains(err.Error(), "disagrees") {
		t.Fatalf("Get() error = %v, want API mismatch", err)
	}
}

func TestClientRejectsInvalidBoxTrapperUAPISchema(t *testing.T) {
	t.Parallel()

	validStatus := uapiRaw(`0`)
	validConfiguration := uapiRaw(
		`{"enable_auto_whitelist":1,"from_addresses":"box@example.test","from_name":null,"queue_days":15,"spam_score":-2.5,"whitelist_by_association":1}`,
	)
	tests := map[string]struct {
		statusBody        string
		configurationBody string
		wantErr           string
	}{
		"invalid status flag": {
			statusBody:        uapiRaw(`2`),
			configurationBody: validConfiguration,
			wantErr:           "integer or string 0 or 1",
		},
		"status envelope unknown field": {
			statusBody:        `{"status":1,"data":0,"errors":null,"messages":null,"metadata":{},"warnings":null,"unknown":true}`,
			configurationBody: validConfiguration,
			wantErr:           "unexpected field",
		},
		"status envelope duplicate field": {
			statusBody:        `{"status":1,"status":1,"data":0,"errors":null,"messages":null,"metadata":{},"warnings":null}`,
			configurationBody: validConfiguration,
			wantErr:           "duplicate field",
		},
		"warnings wrong type": {
			statusBody:        `{"status":1,"data":0,"errors":null,"messages":null,"metadata":{},"warnings":"warning"}`,
			configurationBody: validConfiguration,
			wantErr:           "must be an array",
		},
		"configuration missing from name": {
			statusBody: validStatus,
			configurationBody: uapiRaw(
				`{"enable_auto_whitelist":1,"from_addresses":"box@example.test","queue_days":15,"spam_score":-2.5,"whitelist_by_association":1}`,
			),
			wantErr: "missing required field",
		},
		"configuration unexpected field": {
			statusBody: validStatus,
			configurationBody: uapiRaw(
				`{"enable_auto_whitelist":1,"from_addresses":"box@example.test","from_name":null,"queue_days":15,"spam_score":-2.5,"whitelist_by_association":1,"extra":true}`,
			),
			wantErr: "unexpected field",
		},
		"fractional queue days": {
			statusBody: validStatus,
			configurationBody: uapiRaw(
				`{"enable_auto_whitelist":1,"from_addresses":"box@example.test","from_name":null,"queue_days":9.5,"spam_score":-2.5,"whitelist_by_association":1}`,
			),
			wantErr: "integer or decimal integer string",
		},
		"spam score above one decimal": {
			statusBody: validStatus,
			configurationBody: uapiRaw(
				`{"enable_auto_whitelist":1,"from_addresses":"box@example.test","from_name":null,"queue_days":15,"spam_score":3.75,"whitelist_by_association":1}`,
			),
			wantErr: "one decimal place",
		},
		"from name control character": {
			statusBody: validStatus,
			configurationBody: uapiRaw(
				`{"enable_auto_whitelist":1,"from_addresses":"box@example.test","from_name":"Sender\nName","queue_days":15,"spam_score":-2.5,"whitelist_by_association":1}`,
			),
			wantErr: "control characters",
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/json-api/cpanel":
					writeBoxTrapperJSON(
						t,
						response,
						api2InventoryEnvelope(
							boxTrapperInventoryEntry(
								boxTrapperTestAccount,
								0,
							),
						),
					)
				case "/execute/BoxTrapper/get_status":
					writeBoxTrapperRaw(t, response, test.statusBody)
				case "/execute/BoxTrapper/get_configuration":
					writeBoxTrapperRaw(
						t,
						response,
						test.configurationBody,
					)
				default:
					t.Errorf("unexpected path %s", request.URL.Path)
				}
			}))
			defer server.Close()

			_, err := newBoxTrapperTestClient(t, server.URL).Get(
				t.Context(),
				boxTrapperTestAccount,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"Get() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientSetsBoxTrapperStatusWithExactPOSTAndVerifies(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		switch request.URL.Path {
		case "/execute/BoxTrapper/set_status":
			assertUAPIRequest(
				t,
				request,
				http.MethodPost,
				"/execute/BoxTrapper/set_status",
				url.Values{
					"email":   {boxTrapperTestAccount},
					"enabled": {"1"},
				},
			)
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(
					map[string]any{"accepted": true},
					[]string{"status warning"},
				),
			)
		case "/json-api/cpanel":
			writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
				boxTrapperInventoryEntry(boxTrapperTestAccount, 1),
			))
		case "/execute/BoxTrapper/get_status":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload("1", nil),
			)
		case "/execute/BoxTrapper/get_configuration":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(defaultBoxTrapperConfiguration(), nil),
			)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	settings, warnings, err := newBoxTrapperTestClient(
		t,
		server.URL,
	).SetStatus(t.Context(), boxTrapperTestAccount, true)
	if err != nil {
		t.Fatalf("SetStatus() error: %v", err)
	}
	if settings == nil || !settings.Enabled {
		t.Fatalf("SetStatus() settings = %#v", settings)
	}
	if !reflect.DeepEqual(warnings, []string{"status warning"}) {
		t.Fatalf("SetStatus() warnings = %#v", warnings)
	}
	if requests.Load() != 4 {
		t.Fatalf("request count = %d, want 4", requests.Load())
	}
}

func TestClientSavesBoxTrapperConfigurationWithPreservedFromNameAndVerifies(
	t *testing.T,
) {
	t.Parallel()

	fromName := "Preserved Sender"
	definition := Definition{
		Enabled:                true,
		EnableAutoWhitelist:    false,
		FromAddresses:          "sender@example.test,not-an-address",
		QueueDays:              9,
		SpamScore:              3.7,
		WhitelistByAssociation: true,
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requests.Add(1)
		switch request.URL.Path {
		case "/execute/BoxTrapper/save_configuration":
			assertUAPIRequest(
				t,
				request,
				http.MethodPost,
				"/execute/BoxTrapper/save_configuration",
				url.Values{
					"email":                    {boxTrapperTestAccount},
					"enable_auto_whitelist":    {"0"},
					"from_addresses":           {definition.FromAddresses},
					"from_name":                {fromName},
					"queue_days":               {"9"},
					"spam_score":               {"3.7"},
					"whitelist_by_association": {"1"},
				},
			)
			if request.PostForm.Has("enabled") {
				t.Fatal("save_configuration POST contains enabled")
			}
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(nil, []string{"configuration warning"}),
			)
		case "/json-api/cpanel":
			writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
				boxTrapperInventoryEntry(boxTrapperTestAccount, 0),
			))
		case "/execute/BoxTrapper/get_status":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(0, nil),
			)
		case "/execute/BoxTrapper/get_configuration":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(
					boxTrapperConfigurationData(
						0,
						definition.FromAddresses,
						fromName,
						"9",
						3.7,
						1,
					),
					nil,
				),
			)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	settings, warnings, err := newBoxTrapperTestClient(
		t,
		server.URL,
	).SaveConfiguration(
		t.Context(),
		boxTrapperTestAccount,
		definition,
		&fromName,
	)
	if err != nil {
		t.Fatalf("SaveConfiguration() error: %v", err)
	}
	if settings == nil ||
		settings.Enabled ||
		settings.FromName == nil ||
		*settings.FromName != fromName {
		t.Fatalf("SaveConfiguration() settings = %#v", settings)
	}
	if SettingsMatchDefinition(*settings, definition) {
		t.Fatal("SettingsMatchDefinition() ignored the differing Enabled field")
	}
	if !reflect.DeepEqual(
		warnings,
		[]string{"configuration warning"},
	) {
		t.Fatalf("SaveConfiguration() warnings = %#v", warnings)
	}
	if requests.Load() != 4 {
		t.Fatalf("request count = %d, want 4", requests.Load())
	}
}

func TestClientRejectsMalformedBoxTrapperMutationResponses(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"array data": uapiRaw(`[]`),
		"string data": uapiRaw(
			`"saved"`,
		),
		"duplicate envelope field": `{"status":1,"status":1,"data":null,"errors":null,"messages":null,"metadata":{},"warnings":null}`,
		"unknown envelope field":   `{"status":1,"data":null,"errors":null,"messages":null,"metadata":{},"warnings":null,"extra":true}`,
		"duplicate mutation object field": uapiRaw(
			`{"accepted":true,"accepted":false}`,
		),
	}

	for name, body := range tests {
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				requests.Add(1)
				writeBoxTrapperRaw(t, response, body)
			}))
			defer server.Close()

			_, _, err := newBoxTrapperTestClient(
				t,
				server.URL,
			).SetStatus(t.Context(), boxTrapperTestAccount, true)
			if err == nil {
				t.Fatal("SetStatus() returned no error")
			}
			if requests.Load() != 1 {
				t.Fatalf("request count = %d, want 1", requests.Load())
			}
		})
	}
}

func TestClientReturnsWarningsWithMalformedMutationData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeBoxTrapperJSON(t, response, uapiEnvelopePayload(
			[]any{},
			[]string{"malformed data warning"},
		))
	}))
	defer server.Close()

	_, warnings, err := newBoxTrapperTestClient(
		t,
		server.URL,
	).SetStatus(t.Context(), boxTrapperTestAccount, true)
	if err == nil {
		t.Fatal("SetStatus() returned no mutation data error")
	}
	if !reflect.DeepEqual(
		warnings,
		[]string{"malformed data warning"},
	) {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestClientRejectsUnverifiedBoxTrapperMutations(t *testing.T) {
	t.Parallel()

	t.Run("status mismatch", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			switch request.URL.Path {
			case "/execute/BoxTrapper/set_status":
				writeBoxTrapperJSON(
					t,
					response,
					uapiEnvelopePayload(nil, nil),
				)
			case "/json-api/cpanel":
				writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
					boxTrapperInventoryEntry(boxTrapperTestAccount, 0),
				))
			case "/execute/BoxTrapper/get_status":
				writeBoxTrapperJSON(
					t,
					response,
					uapiEnvelopePayload(0, nil),
				)
			case "/execute/BoxTrapper/get_configuration":
				writeBoxTrapperJSON(
					t,
					response,
					uapiEnvelopePayload(
						defaultBoxTrapperConfiguration(),
						nil,
					),
				)
			default:
				t.Errorf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()

		_, _, err := newBoxTrapperTestClient(
			t,
			server.URL,
		).SetStatus(t.Context(), boxTrapperTestAccount, true)
		if err == nil || !strings.Contains(err.Error(), "expected true") {
			t.Fatalf("SetStatus() error = %v, want verification error", err)
		}
		var verificationError *MutationVerificationError
		if !errors.As(err, &verificationError) {
			t.Fatalf(
				"SetStatus() error = %T %v, want MutationVerificationError",
				err,
				err,
			)
		}
	})

	t.Run("configuration mismatch preserves warnings", func(t *testing.T) {
		t.Parallel()

		definition := Definition{
			EnableAutoWhitelist:    true,
			FromAddresses:          boxTrapperTestAccount,
			QueueDays:              9,
			SpamScore:              -2.5,
			WhitelistByAssociation: true,
		}
		fromName := "Preserved Sender"
		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			switch request.URL.Path {
			case "/execute/BoxTrapper/save_configuration":
				writeBoxTrapperJSON(
					t,
					response,
					uapiEnvelopePayload(
						nil,
						[]string{"save warning"},
					),
				)
			case "/json-api/cpanel":
				writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
					boxTrapperInventoryEntry(boxTrapperTestAccount, 0),
				))
			case "/execute/BoxTrapper/get_status":
				writeBoxTrapperJSON(
					t,
					response,
					uapiEnvelopePayload(0, nil),
				)
			case "/execute/BoxTrapper/get_configuration":
				configuration := defaultBoxTrapperConfiguration()
				configuration["queue_days"] = "10"
				configuration["from_name"] = fromName
				writeBoxTrapperJSON(
					t,
					response,
					uapiEnvelopePayload(configuration, nil),
				)
			default:
				t.Errorf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()

		_, warnings, err := newBoxTrapperTestClient(
			t,
			server.URL,
		).SaveConfiguration(
			t.Context(),
			boxTrapperTestAccount,
			definition,
			&fromName,
		)
		if err == nil || !strings.Contains(err.Error(), "queue_days") {
			t.Fatalf(
				"SaveConfiguration() error = %v, want verification error",
				err,
			)
		}
		if !reflect.DeepEqual(warnings, []string{"save warning"}) {
			t.Fatalf("warnings = %#v", warnings)
		}
		var verificationError *MutationVerificationError
		if !errors.As(err, &verificationError) {
			t.Fatalf(
				"SaveConfiguration() error = %T %v, want MutationVerificationError",
				err,
				err,
			)
		}
	})
}

func TestClientReturnsMutationWarningsWhenRereadFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/BoxTrapper/set_status":
			writeBoxTrapperJSON(
				t,
				response,
				uapiEnvelopePayload(nil, []string{"mutation warning"}),
			)
		case "/json-api/cpanel":
			writeBoxTrapperJSON(t, response, map[string]any{
				"cpanelresult": map[string]any{
					"event": map[string]any{"result": 0},
					"data": []map[string]any{{
						"reason":    "inventory failed",
						"statusmsg": "failed",
					}},
				},
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	_, warnings, err := newBoxTrapperTestClient(
		t,
		server.URL,
	).SetStatus(t.Context(), boxTrapperTestAccount, true)
	if err == nil {
		t.Fatal("SetStatus() returned no reread error")
	}
	if !reflect.DeepEqual(warnings, []string{"mutation warning"}) {
		t.Fatalf("warnings = %#v", warnings)
	}
	var verificationError *MutationVerificationError
	if !errors.As(err, &verificationError) {
		t.Fatalf(
			"SetStatus() error = %T %v, want MutationVerificationError",
			err,
			err,
		)
	}
	var apiError *cpanel.APIError
	if !errors.As(err, &apiError) || apiError.API != "API 2" {
		t.Fatalf("SetStatus() error = %T %v", err, err)
	}
}

func TestClientPropagatesBoxTrapperAPI2AndUAPIErrors(t *testing.T) {
	t.Parallel()

	t.Run("API 2 inventory", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			_ *http.Request,
		) {
			writeBoxTrapperJSON(t, response, map[string]any{
				"cpanelresult": map[string]any{
					"event": map[string]any{"result": 0},
					"data": []map[string]any{{
						"reason": "inventory rejected",
					}},
				},
			})
		}))
		defer server.Close()

		_, err := newBoxTrapperTestClient(t, server.URL).Get(
			t.Context(),
			boxTrapperTestAccount,
		)
		var apiError *cpanel.APIError
		if !errors.As(err, &apiError) ||
			apiError.API != "API 2" ||
			apiError.Module != cpanel.ModuleBoxTrapper {
			t.Fatalf("Get() error = %T %v", err, err)
		}
	})

	t.Run("UAPI read", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			switch request.URL.Path {
			case "/json-api/cpanel":
				writeBoxTrapperJSON(t, response, api2InventoryEnvelope(
					boxTrapperInventoryEntry(boxTrapperTestAccount, 0),
				))
			case "/execute/BoxTrapper/get_status":
				writeBoxTrapperJSON(t, response, map[string]any{
					"status": 0,
					"errors": []string{"status rejected"},
				})
			default:
				t.Errorf("unexpected path %s", request.URL.Path)
			}
		}))
		defer server.Close()

		_, err := newBoxTrapperTestClient(t, server.URL).Get(
			t.Context(),
			boxTrapperTestAccount,
		)
		var apiError *cpanel.APIError
		if !errors.As(err, &apiError) ||
			apiError.API != "UAPI" ||
			apiError.Function != operationGetStatus {
			t.Fatalf("Get() error = %T %v", err, err)
		}
	})

	t.Run("UAPI mutation", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			_ *http.Request,
		) {
			writeBoxTrapperJSON(t, response, map[string]any{
				"status": 0,
				"errors": []string{"mutation rejected"},
			})
		}))
		defer server.Close()

		_, _, err := newBoxTrapperTestClient(
			t,
			server.URL,
		).SetStatus(t.Context(), boxTrapperTestAccount, true)
		var apiError *cpanel.APIError
		if !errors.As(err, &apiError) ||
			apiError.API != "UAPI" ||
			apiError.Function != operationSetStatus {
			t.Fatalf("SetStatus() error = %T %v", err, err)
		}
	})
}

func TestClientBoxTrapperAccountLocksSerializeAndCleanUp(t *testing.T) {
	t.Parallel()

	client := &Client{}
	unlockFirst := client.LockAccount(boxTrapperTestAccount)

	sameAccount := make(chan func(), 1)
	go func() {
		sameAccount <- client.LockAccount(boxTrapperTestAccount)
	}()

	select {
	case unlock := <-sameAccount:
		unlock()
		t.Fatal("same-account lock acquired before release")
	case <-time.After(25 * time.Millisecond):
	}

	otherAccount := make(chan func(), 1)
	go func() {
		otherAccount <- client.LockAccount("other@example.test")
	}()
	select {
	case unlock := <-otherAccount:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("different-account lock was blocked")
	}

	unlockFirst()
	unlockFirst()

	select {
	case unlock := <-sameAccount:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("same-account lock remained blocked after release")
	}

	client.accountLocksMu.Lock()
	defer client.accountLocksMu.Unlock()
	if len(client.accountLocks) != 0 {
		t.Fatalf(
			"account lock registry contains %d entries",
			len(client.accountLocks),
		)
	}
}

func newBoxTrapperTestClient(
	t *testing.T,
	host string,
) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func newCountingTestClient(
	t *testing.T,
	requests *atomic.Int32,
) (*Client, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		requests.Add(1)
	}))

	return newBoxTrapperTestClient(t, server.URL), server.Close
}

func assertAPI2InventoryRequest(
	t *testing.T,
	request *http.Request,
) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", request.Method)
	}
	if request.URL.Path != "/json-api/cpanel" {
		t.Errorf("path = %s, want /json-api/cpanel", request.URL.Path)
	}
	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}
	want := url.Values{
		"cpanel_jsonapi_apiversion": {"2"},
		"cpanel_jsonapi_func":       {operationAccountManageList},
		"cpanel_jsonapi_module":     {cpanel.ModuleBoxTrapper},
		"cpanel_jsonapi_user":       {"username"},
	}
	if !reflect.DeepEqual(request.Form, want) {
		t.Errorf("API 2 form = %#v, want %#v", request.Form, want)
	}
}

func assertUAPIRequest(
	t *testing.T,
	request *http.Request,
	method string,
	path string,
	want url.Values,
) {
	t.Helper()

	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != path {
		t.Errorf("path = %s, want %s", request.URL.Path, path)
	}
	if method == http.MethodPost && request.URL.RawQuery != "" {
		t.Errorf("POST query = %q, want empty", request.URL.RawQuery)
	}
	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}
	actual := request.Form
	if method == http.MethodPost {
		actual = request.PostForm
	}
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("form = %#v, want %#v", actual, want)
	}
}

func boxTrapperInventoryEntry(
	account string,
	enabled any,
) map[string]any {
	return map[string]any{
		"account":    account,
		"accounturi": strings.ReplaceAll(account, "@", "%40"),
		"bg":         "even",
		"enabled":    enabled,
		"status":     "Localized status that is not interpreted",
	}
}

func api2InventoryEnvelope(
	entries ...map[string]any,
) map[string]any {
	return map[string]any{
		"cpanelresult": map[string]any{
			"apiversion": 2,
			"data":       entries,
			"event":      map[string]any{"result": 1},
			"func":       operationAccountManageList,
			"module":     cpanel.ModuleBoxTrapper,
			"postevent":  map[string]any{},
			"preevent":   nil,
		},
	}
}

func api2InventoryRaw(data string) string {
	return fmt.Sprintf(
		`{"cpanelresult":{"apiversion":2,"data":%s,"event":{"result":1},"func":"accountmanagelist","module":"BoxTrapper"}}`,
		data,
	)
}

func uapiEnvelopePayload(
	data any,
	warnings []string,
) map[string]any {
	return map[string]any{
		"data":     data,
		"errors":   nil,
		"messages": nil,
		"metadata": map[string]any{},
		"status":   1,
		"warnings": warnings,
	}
}

func uapiRaw(data string) string {
	return fmt.Sprintf(
		`{"status":1,"data":%s,"errors":null,"messages":null,"metadata":{},"warnings":null}`,
		data,
	)
}

func boxTrapperConfigurationData(
	enableAutoWhitelist any,
	fromAddresses string,
	fromName any,
	queueDays any,
	spamScore any,
	whitelistByAssociation any,
) map[string]any {
	return map[string]any{
		"enable_auto_whitelist":    enableAutoWhitelist,
		"from_addresses":           fromAddresses,
		"from_name":                fromName,
		"queue_days":               queueDays,
		"spam_score":               spamScore,
		"whitelist_by_association": whitelistByAssociation,
	}
}

func defaultBoxTrapperConfiguration() map[string]any {
	return boxTrapperConfigurationData(
		1,
		boxTrapperTestAccount,
		nil,
		15,
		-2.5,
		1,
	)
}

func writeBoxTrapperJSON(
	t *testing.T,
	response http.ResponseWriter,
	payload any,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func writeBoxTrapperRaw(
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

func stringPointer(value string) *string {
	return &value
}

func stringPointersEqual(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}
