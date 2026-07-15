package email

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsAndGetsEmailRoutingWithoutDomainFilter(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/Email/list_mxs" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}

		technical := routingTestInventory(
			"technical.example.test",
			RoutingModeAuto,
			RoutingModeLocal,
		)
		technical["mx"] = nil
		technical["entries"] = []map[string]any{}
		withMX := routingTestInventory(
			"example.test",
			RoutingModeAuto,
			RoutingModeLocal,
		)
		withMX["entries"] = []map[string]any{
			{
				"domain":     "example.test",
				"entrycount": 2,
				"mx":         "secondary.example.test",
				"priority":   10,
				"row":        "even",
			},
			{
				"domain":     "example.test",
				"entrycount": "1",
				"mx":         "mail.example.test",
				"priority":   "0",
				"row":        "odd",
			},
		}

		writeRoutingJSON(t, response, map[string]any{
			"status": 1,
			"data":   []map[string]any{technical, withMX},
		})
	}))
	defer server.Close()

	client := newEmailTestClient(t, server.URL)
	routings, err := client.ListRoutings(t.Context())
	if err != nil {
		t.Fatalf("ListRoutings() error: %v", err)
	}
	if len(routings) != 2 ||
		routings[0].Domain != "example.test" ||
		routings[1].Domain != "technical.example.test" {
		t.Fatalf("routings = %#v", routings)
	}
	if routings[0].PrimaryExchanger == nil ||
		*routings[0].PrimaryExchanger != "mail.example.test" ||
		len(routings[0].Entries) != 2 ||
		routings[0].Entries[0].Priority != 0 {
		t.Fatalf("routing with MX = %#v", routings[0])
	}
	if routings[1].PrimaryExchanger != nil ||
		len(routings[1].Entries) != 0 {
		t.Fatalf("technical routing = %#v", routings[1])
	}

	routing, err := client.GetRouting(t.Context(), "example.test")
	if err != nil {
		t.Fatalf("GetRouting() error: %v", err)
	}
	if routing == nil ||
		routing.Mode != RoutingModeAuto ||
		routing.DetectedMode != RoutingModeLocal ||
		!routing.Local {
		t.Fatalf("routing = %#v", routing)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
}

func TestClientReturnsNilForMissingEmailRouting(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeRoutingJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				routingTestInventory(
					"other.example.test",
					RoutingModeAuto,
					RoutingModeLocal,
				),
			},
		})
	}))
	defer server.Close()

	routing, err := newEmailTestClient(t, server.URL).GetRouting(
		t.Context(),
		"missing.example.test",
	)
	if err != nil {
		t.Fatalf("GetRouting() error: %v", err)
	}
	if routing != nil {
		t.Fatalf("routing = %#v, want nil", routing)
	}
}

func TestClientRejectsInvalidEmailRoutingInventory(t *testing.T) {
	t.Parallel()

	tests := map[string][]map[string]any{
		"duplicate domain": {
			routingTestInventory(
				"example.test",
				RoutingModeAuto,
				RoutingModeLocal,
			),
			routingTestInventory(
				"example.test",
				RoutingModeAuto,
				RoutingModeLocal,
			),
		},
		"invalid configured mode": {
			mutateRoutingInventory(func(item map[string]any) {
				item["mxcheck"] = "invalid"
			}),
		},
		"inconsistent effective flags": {
			mutateRoutingInventory(func(item map[string]any) {
				item["local"] = 0
				item["remote"] = 1
			}),
		},
		"failed row status": {
			mutateRoutingInventory(func(item map[string]any) {
				item["status"] = 0
			}),
		},
		"primary without entries": {
			mutateRoutingInventory(func(item map[string]any) {
				item["entries"] = []map[string]any{}
			}),
		},
		"entry for another domain": {
			mutateRoutingInventory(func(item map[string]any) {
				entries, ok := item["entries"].([]map[string]any)
				if !ok {
					panic("routing test inventory entries have unexpected type")
				}
				entries[0]["domain"] = "other.example.test"
			}),
		},
		"invalid entry priority": {
			mutateRoutingInventory(func(item map[string]any) {
				entries, ok := item["entries"].([]map[string]any)
				if !ok {
					panic("routing test inventory entries have unexpected type")
				}
				entries[0]["priority"] = "invalid"
			}),
		},
		"primary mismatch": {
			mutateRoutingInventory(func(item map[string]any) {
				item["mx"] = "other.example.test"
			}),
		},
	}

	for name, inventory := range tests {
		inventory := inventory
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeRoutingJSON(t, response, map[string]any{
					"status": 1,
					"data":   inventory,
				})
			}))
			defer server.Close()

			if _, err := newEmailTestClient(t, server.URL).ListRoutings(
				t.Context(),
			); err == nil {
				t.Fatal("ListRoutings() returned no error")
			}
		})
	}
}

func TestClientSetsBackupEmailRoutingWithPOSTAndVerifies(t *testing.T) {
	t.Parallel()

	mode := RoutingModeAuto
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch request.URL.Path {
		case "/execute/Email/set_always_accept":
			if request.Method != http.MethodPost {
				t.Errorf("mutation method = %s, want POST", request.Method)
			}
			if request.URL.RawQuery != "" {
				t.Errorf("mutation query = %q, want empty", request.URL.RawQuery)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			wantForm := url.Values{
				"domain":  {"example.test"},
				"mxcheck": {"secondary"},
			}
			if !reflect.DeepEqual(request.PostForm, wantForm) {
				t.Errorf(
					"mutation form = %#v, want %#v",
					request.PostForm,
					wantForm,
				)
			}
			if request.PostForm.Has("alwaysaccept") {
				t.Error("mutation form contains redundant alwaysaccept")
			}
			mode = RoutingModeBackup
			writeRoutingJSON(t, response, map[string]any{
				"status":   1,
				"warnings": []string{"outer warning"},
				"data": routingMutationTestPayload(
					RoutingModeBackup,
					RoutingModeBackup,
					[]string{"checkmx warning"},
				),
			})
		case "/execute/Email/list_mxs":
			if request.Method != http.MethodGet {
				t.Errorf("verification method = %s, want GET", request.Method)
			}
			if request.URL.RawQuery != "" {
				t.Errorf(
					"verification query = %q, want empty",
					request.URL.RawQuery,
				)
			}
			writeRoutingJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{
					routingTestInventory(
						"example.test",
						mode,
						mode,
					),
				},
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	routing, warnings, err := newEmailTestClient(t, server.URL).SetRouting(
		t.Context(),
		RoutingDefinition{
			Domain: "example.test",
			Mode:   RoutingModeBackup,
		},
	)
	if err != nil {
		t.Fatalf("SetRouting() error: %v", err)
	}
	if routing.Mode != RoutingModeBackup ||
		routing.DetectedMode != RoutingModeBackup ||
		!routing.Backup {
		t.Fatalf("routing = %#v", routing)
	}
	if !reflect.DeepEqual(
		warnings,
		[]string{"outer warning", "checkmx warning"},
	) {
		t.Fatalf("warnings = %#v", warnings)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
}

func TestClientRejectsInvalidEmailRoutingMutationResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/set_always_accept":
			payload := routingMutationTestPayload(
				RoutingModeRemote,
				RoutingModeRemote,
				nil,
			)
			payload["mxcheck"] = "local"
			writeRoutingJSON(t, response, map[string]any{
				"status": 1,
				"data":   payload,
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	if _, _, err := newEmailTestClient(t, server.URL).SetRouting(
		t.Context(),
		RoutingDefinition{
			Domain: "example.test",
			Mode:   RoutingModeRemote,
		},
	); err == nil {
		t.Fatal("SetRouting() returned no mutation response error")
	}
}

func TestClientRejectsUnverifiedEmailRoutingMutation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/set_always_accept":
			writeRoutingJSON(t, response, map[string]any{
				"status": 1,
				"data": routingMutationTestPayload(
					RoutingModeRemote,
					RoutingModeRemote,
					nil,
				),
			})
		case "/execute/Email/list_mxs":
			writeRoutingJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{
					routingTestInventory(
						"example.test",
						RoutingModeAuto,
						RoutingModeLocal,
					),
				},
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	if _, _, err := newEmailTestClient(t, server.URL).SetRouting(
		t.Context(),
		RoutingDefinition{
			Domain: "example.test",
			Mode:   RoutingModeRemote,
		},
	); err == nil {
		t.Fatal("SetRouting() returned no verification error")
	}
}

func TestValidateRoutingDefinition(t *testing.T) {
	t.Parallel()

	for _, mode := range []RoutingMode{
		RoutingModeAuto,
		RoutingModeBackup,
		RoutingModeLocal,
		RoutingModeRemote,
	} {
		if err := ValidateRoutingDefinition(RoutingDefinition{
			Domain: "example.test",
			Mode:   mode,
		}); err != nil {
			t.Errorf("ValidateRoutingDefinition(%q) error: %v", mode, err)
		}
	}

	for name, definition := range map[string]RoutingDefinition{
		"empty domain": {Mode: RoutingModeAuto},
		"spaced domain": {
			Domain: " example.test",
			Mode:   RoutingModeAuto,
		},
		"invalid mode": {
			Domain: "example.test",
			Mode:   "invalid",
		},
	} {
		if err := ValidateRoutingDefinition(definition); err == nil {
			t.Errorf("%s: ValidateRoutingDefinition() returned no error", name)
		}
	}
}

func TestClientRoutingLocksSerializePerDomain(t *testing.T) {
	t.Parallel()

	client := &Client{}
	unlockFirst := client.LockRoutingDomain("first.example.test")

	sameDomain := make(chan func(), 1)
	go func() {
		sameDomain <- client.LockRoutingDomain("first.example.test")
	}()

	select {
	case unlock := <-sameDomain:
		unlock()
		t.Fatal("same-domain lock acquired before first lock was released")
	case <-time.After(25 * time.Millisecond):
	}

	otherDomain := make(chan func(), 1)
	go func() {
		otherDomain <- client.LockRoutingDomain("other.example.test")
	}()
	select {
	case unlock := <-otherDomain:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("different-domain lock did not acquire independently")
	}

	unlockFirst()
	select {
	case unlock := <-sameDomain:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("same-domain lock did not acquire after release")
	}
}

func TestClientPropagatesEmailRoutingAPIErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeRoutingJSON(t, response, map[string]any{
			"status": 0,
			"errors": []string{"rejected"},
		})
	}))
	defer server.Close()

	_, _, err := newEmailTestClient(t, server.URL).SetRouting(
		t.Context(),
		RoutingDefinition{
			Domain: "example.test",
			Mode:   RoutingModeRemote,
		},
	)
	if err == nil {
		t.Fatal("SetRouting() returned no error")
	}
	var apiError *cpanel.APIError
	if !errors.As(err, &apiError) ||
		apiError.Error() != "UAPI Email::set_always_accept failed: rejected" {
		t.Fatalf("SetRouting() error = %T %v", err, err)
	}
}

func routingTestInventory(
	domain string,
	mode RoutingMode,
	detected RoutingMode,
) map[string]any {
	local, remote, backup := routingTestFlags(detected)
	wireMode, _ := routingModeToWire(mode)
	wireDetected, _ := routingModeToWire(detected)
	alwaysAccept := 0
	if mode == RoutingModeLocal {
		alwaysAccept = 1
	}
	exchanger := "mail." + domain

	return map[string]any{
		"alwaysaccept": alwaysAccept,
		"detected":     wireDetected,
		"domain":       domain,
		"entries": []map[string]any{
			{
				"domain":     domain,
				"entrycount": 1,
				"mx":         exchanger,
				"priority":   "0",
				"row":        "odd",
			},
		},
		"local":     local,
		"mx":        exchanger,
		"mxcheck":   wireMode,
		"remote":    remote,
		"secondary": backup,
		"status":    1,
		"statusmsg": "Fetched MX List",
	}
}

func mutateRoutingInventory(
	mutate func(map[string]any),
) map[string]any {
	item := routingTestInventory(
		"example.test",
		RoutingModeAuto,
		RoutingModeLocal,
	)
	mutate(item)

	return item
}

func routingMutationTestPayload(
	mode RoutingMode,
	detected RoutingMode,
	warnings []string,
) map[string]any {
	local, remote, backup := routingTestFlags(detected)
	wireMode, _ := routingModeToWire(mode)
	wireDetected, _ := routingModeToWire(detected)

	return map[string]any{
		"checkmx": map[string]any{
			"changed":     1,
			"detected":    wireDetected,
			"isprimary":   local,
			"issecondary": backup,
			"local":       local,
			"mxcheck":     wireMode,
			"remote":      remote,
			"secondary":   backup,
			"warnings":    warnings,
		},
		"detected":  wireDetected,
		"local":     local,
		"mxcheck":   wireMode,
		"remote":    remote,
		"results":   "Set Always Accept Status",
		"secondary": backup,
		"status":    1,
		"statusmsg": "Set Always Accept Status",
	}
}

func routingTestFlags(mode RoutingMode) (int, int, int) {
	local := 0
	remote := 0
	backup := 0
	switch mode {
	case RoutingModeLocal:
		local = 1
	case RoutingModeRemote:
		remote = 1
	case RoutingModeBackup:
		backup = 1
	}

	return local, remote, backup
}

func writeRoutingJSON(
	t *testing.T,
	response http.ResponseWriter,
	payload any,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(payload); err != nil {
		t.Fatalf("encode routing response: %v", err)
	}
}
