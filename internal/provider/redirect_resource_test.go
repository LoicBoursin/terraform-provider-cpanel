package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

func TestRedirectDeleteForReplacementReconcilesResult(t *testing.T) {
	t.Parallel()

	original := cpanelredirect.Definition{
		Domain:      "example.test",
		Source:      "/old",
		Destination: "https://original.test/new",
		Type:        cpanelredirect.TypePermanent,
		WWWMode:     cpanelredirect.WWWModeBoth,
	}
	concurrent := original
	concurrent.Destination = "https://concurrent.test/new"

	testCases := []struct {
		name             string
		redirectOnDelete *cpanelredirect.Definition
		wantError        bool
	}{
		{
			name: "absent continues after ambiguous error",
		},
		{
			name:             "original still present stops",
			redirectOnDelete: &original,
			wantError:        true,
		},
		{
			name:             "concurrent replacement stops",
			redirectOnDelete: &concurrent,
			wantError:        true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := &original
			deleteCalls := 0
			server := newRedirectResourceTestServer(
				t,
				&current,
				func(response http.ResponseWriter, _ *http.Request) {
					deleteCalls++
					current = testCase.redirectOnDelete
					writeRedirectResourceTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous delete"},
						"data":   nil,
					})
				},
				nil,
			)
			defer server.Close()

			resource := redirectTestResource(t, server.URL)
			err := resource.deleteRedirectForReplacement(
				t.Context(),
				original,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"deleteRedirectForReplacement() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if deleteCalls != 1 {
				t.Fatalf("deleteCalls = %d, want 1", deleteCalls)
			}
			if !redirectDefinitionPointersEqual(
				current,
				testCase.redirectOnDelete,
			) {
				t.Fatalf(
					"current = %#v, want %#v",
					current,
					testCase.redirectOnDelete,
				)
			}
		})
	}
}

func TestRedirectRestoreRequiresAttemptedState(t *testing.T) {
	t.Parallel()

	original := cpanelredirect.Definition{
		Domain:      "example.test",
		Source:      "/old",
		Destination: "https://original.test/new",
		Type:        cpanelredirect.TypePermanent,
		WWWMode:     cpanelredirect.WWWModeBoth,
	}
	attempted := original
	attempted.Destination = "https://attempted.test/new"
	attempted.Type = cpanelredirect.TypeTemporary
	concurrent := attempted
	concurrent.Destination = "https://concurrent.test/new"

	testCases := []struct {
		name            string
		initial         *cpanelredirect.Definition
		wantError       bool
		wantCurrent     *cpanelredirect.Definition
		wantDeleteCalls int
		wantAddCalls    int
	}{
		{
			name:            "attempted state is restored",
			initial:         &attempted,
			wantCurrent:     &original,
			wantDeleteCalls: 1,
			wantAddCalls:    1,
		},
		{
			name:        "original already restored is accepted",
			initial:     &original,
			wantCurrent: &original,
		},
		{
			name:        "concurrent state is preserved",
			initial:     &concurrent,
			wantError:   true,
			wantCurrent: &concurrent,
		},
		{
			name:      "absent attempted state is not recreated over",
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := testCase.initial
			deleteCalls := 0
			addCalls := 0
			server := newRedirectResourceTestServer(
				t,
				&current,
				func(response http.ResponseWriter, _ *http.Request) {
					deleteCalls++
					current = nil
					writeRedirectResourceTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous delete"},
						"data":   nil,
					})
				},
				func(response http.ResponseWriter, request *http.Request) {
					addCalls++
					definition := redirectDefinitionFromTestRequest(t, request)
					current = &definition
					writeRedirectResourceTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous add"},
						"data":   nil,
					})
				},
			)
			defer server.Close()

			resource := redirectTestResource(t, server.URL)
			err := resource.restoreRedirect(
				t.Context(),
				attempted,
				original,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"restoreRedirect() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if !redirectDefinitionPointersEqual(
				current,
				testCase.wantCurrent,
			) {
				t.Fatalf(
					"current = %#v, want %#v",
					current,
					testCase.wantCurrent,
				)
			}
			if deleteCalls != testCase.wantDeleteCalls {
				t.Fatalf(
					"deleteCalls = %d, want %d",
					deleteCalls,
					testCase.wantDeleteCalls,
				)
			}
			if addCalls != testCase.wantAddCalls {
				t.Fatalf(
					"addCalls = %d, want %d",
					addCalls,
					testCase.wantAddCalls,
				)
			}
		})
	}
}

func TestRedirectResourceSchemaDefaults(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewRedirectResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"source", "type", "www_mode"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok {
			t.Fatalf(
				"%s has type %T, want schema.StringAttribute",
				attributeName,
				response.Schema.Attributes[attributeName],
			)
		}
		if !attribute.Optional || !attribute.Computed || attribute.Default == nil {
			t.Fatalf("%s must be optional, computed, and defaulted", attributeName)
		}
	}

	wildcard, ok := response.Schema.Attributes["wildcard"].(resourceschema.BoolAttribute)
	if !ok {
		t.Fatalf(
			"wildcard has type %T, want schema.BoolAttribute",
			response.Schema.Attributes["wildcard"],
		)
	}
	if !wildcard.Optional || !wildcard.Computed || wildcard.Default == nil {
		t.Fatal("wildcard must be optional, computed, and defaulted")
	}
}

func redirectTestResource(t *testing.T, host string) redirectResource {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return redirectResource{client: cpanelredirect.NewClient(baseClient)}
}

func newRedirectResourceTestServer(
	t *testing.T,
	current **cpanelredirect.Definition,
	deleteHandler func(http.ResponseWriter, *http.Request),
	addHandler func(http.ResponseWriter, *http.Request),
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Mime/list_redirects":
			data := []cpanelredirect.Redirect{}
			if *current != nil {
				data = append(
					data,
					redirectFromTestDefinition(**current),
				)
			}
			writeRedirectResourceTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   data,
			})
		case "/execute/Mime/delete_redirect":
			if deleteHandler == nil {
				t.Fatal("unexpected redirect deletion")
			}
			deleteHandler(response, request)
		case "/execute/Mime/add_redirect":
			if addHandler == nil {
				t.Fatal("unexpected redirect creation")
			}
			addHandler(response, request)
		default:
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))
}

func redirectFromTestDefinition(
	definition cpanelredirect.Definition,
) cpanelredirect.Redirect {
	statusCode := "301"
	if definition.Type == cpanelredirect.TypeTemporary {
		statusCode = "302"
	}
	matchWWW := 1
	if definition.WWWMode == cpanelredirect.WWWModeWithout {
		matchWWW = 0
	}
	wildcard := 0
	if definition.Wildcard {
		wildcard = 1
	}

	return cpanelredirect.Redirect{
		Domain:      definition.Domain,
		Source:      definition.Source,
		Destination: definition.Destination,
		Type:        definition.Type,
		MatchWWW:    matchWWW,
		Wildcard:    wildcard,
		StatusCode:  statusCode,
	}
}

func redirectDefinitionFromTestRequest(
	t *testing.T,
	request *http.Request,
) cpanelredirect.Definition {
	t.Helper()

	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}
	redirectType := cpanelredirect.TypePermanent
	if request.Form.Get("type") == "temp" {
		redirectType = cpanelredirect.TypeTemporary
	}
	wwwMode := cpanelredirect.WWWModeBoth
	if request.Form.Get("redirect_www") == "1" {
		wwwMode = cpanelredirect.WWWModeWithout
	}
	return cpanelredirect.Definition{
		Domain:      request.Form.Get("domain"),
		Source:      request.Form.Get("src"),
		Destination: request.Form.Get("redirect"),
		Type:        redirectType,
		WWWMode:     wwwMode,
		Wildcard:    request.Form.Get("redirect_wildcard") == "1",
	}
}

func redirectDefinitionPointersEqual(
	left *cpanelredirect.Definition,
	right *cpanelredirect.Definition,
) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func writeRedirectResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func TestAccRedirectResource(t *testing.T) {
	const resourceName = "cpanel_redirect.test"

	domain := testAccMainDomain(t)
	initialSource := testAccRedirectSource("resource")
	replacementSource := testAccRedirectSource("replacement")
	initialDestination := testAccRedirectDestination("initial")
	updatedDestination := testAccRedirectDestination("updated")

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckRedirectsDestroyed(
			domain,
			initialSource,
			replacementSource,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccRedirectResourceConfig(
					domain,
					initialSource,
					initialDestination,
					"",
					"",
					nil,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"domain",
						domain,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"source",
						initialSource,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"destination",
						initialDestination,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"type",
						cpanelredirect.TypePermanent,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"www_mode",
						cpanelredirect.WWWModeBoth,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"wildcard",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"status_code",
						"301",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"document_root",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"kind",
						"rewrite",
					),
					testAccCheckRedirectExists(cpanelredirect.Definition{
						Domain:      domain,
						Source:      initialSource,
						Destination: initialDestination,
						Type:        cpanelredirect.TypePermanent,
						WWWMode:     cpanelredirect.WWWModeBoth,
					}),
				),
			},
			{
				Config: testAccRedirectResourceConfig(
					domain,
					initialSource,
					updatedDestination,
					cpanelredirect.TypeTemporary,
					cpanelredirect.WWWModeWithout,
					boolPointer(true),
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"destination",
						updatedDestination,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"type",
						cpanelredirect.TypeTemporary,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"www_mode",
						cpanelredirect.WWWModeWithout,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"wildcard",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"status_code",
						"302",
					),
					testAccCheckRedirectExists(cpanelredirect.Definition{
						Domain:      domain,
						Source:      initialSource,
						Destination: updatedDestination,
						Type:        cpanelredirect.TypeTemporary,
						WWWMode:     cpanelredirect.WWWModeWithout,
						Wildcard:    true,
					}),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        domain + "|" + initialSource,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
			},
			{
				PreConfig: func() {
					testAccReplaceRedirect(
						t,
						cpanelredirect.Definition{
							Domain:      domain,
							Source:      initialSource,
							Destination: testAccRedirectDestination("external"),
							Type:        cpanelredirect.TypePermanent,
							WWWMode:     cpanelredirect.WWWModeBoth,
						},
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccRedirectResourceConfig(
					domain,
					initialSource,
					updatedDestination,
					cpanelredirect.TypeTemporary,
					cpanelredirect.WWWModeWithout,
					boolPointer(true),
				),
				Check: testAccCheckRedirectExists(cpanelredirect.Definition{
					Domain:      domain,
					Source:      initialSource,
					Destination: updatedDestination,
					Type:        cpanelredirect.TypeTemporary,
					WWWMode:     cpanelredirect.WWWModeWithout,
					Wildcard:    true,
				}),
			},
			{
				Config: testAccRedirectResourceConfig(
					domain,
					replacementSource,
					updatedDestination,
					cpanelredirect.TypeTemporary,
					cpanelredirect.WWWModeWithout,
					boolPointer(true),
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckRedirectExists(cpanelredirect.Definition{
						Domain:      domain,
						Source:      replacementSource,
						Destination: updatedDestination,
						Type:        cpanelredirect.TypeTemporary,
						WWWMode:     cpanelredirect.WWWModeWithout,
						Wildcard:    true,
					}),
					testAccCheckRedirectsDestroyed(domain, initialSource),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteRedirect(t, domain, replacementSource)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccRedirectResourceConfig(
					domain,
					replacementSource,
					updatedDestination,
					cpanelredirect.TypeTemporary,
					cpanelredirect.WWWModeWithout,
					boolPointer(true),
				),
				Check: testAccCheckRedirectExists(cpanelredirect.Definition{
					Domain:      domain,
					Source:      replacementSource,
					Destination: updatedDestination,
					Type:        cpanelredirect.TypeTemporary,
					WWWMode:     cpanelredirect.WWWModeWithout,
					Wildcard:    true,
				}),
			},
		},
	})
}

func testAccRedirectResourceConfig(
	domain string,
	source string,
	destination string,
	redirectType string,
	wwwMode string,
	wildcard *bool,
) string {
	optionalArguments := ""
	if redirectType != "" {
		optionalArguments += fmt.Sprintf("\n  type        = %q", redirectType)
	}
	if wwwMode != "" {
		optionalArguments += fmt.Sprintf("\n  www_mode    = %q", wwwMode)
	}
	if wildcard != nil {
		optionalArguments += fmt.Sprintf("\n  wildcard    = %t", *wildcard)
	}

	return providerConfig + fmt.Sprintf(`
resource "cpanel_redirect" "test" {
  domain      = %q
  source      = %q
  destination = %q%s
}
`, domain, source, destination, optionalArguments)
}

func boolPointer(value bool) *bool {
	return &value
}
