package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailDomainForwarderDeleteForReplacementReconcilesResult(
	t *testing.T,
) {
	t.Parallel()

	const (
		domain      = "example.test"
		original    = "original.test"
		concurrent  = "concurrent.test"
		errorDetail = "ambiguous delete"
	)

	testCases := []struct {
		name                string
		destinationOnDelete string
		wantError           bool
	}{
		{
			name:                "absent continues after ambiguous error",
			destinationOnDelete: "",
		},
		{
			name:                "original still present stops",
			destinationOnDelete: original,
			wantError:           true,
		},
		{
			name:                "concurrent replacement stops",
			destinationOnDelete: concurrent,
			wantError:           true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			currentDestination := original
			deleteCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Email/delete_domain_forwarder":
					deleteCalls++
					currentDestination = testCase.destinationOnDelete
					writeEmailDomainForwarderTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{errorDetail},
						"data":   nil,
					})
				case "/execute/Email/list_domain_forwarders":
					writeEmailDomainForwarderInventory(
						t,
						response,
						domain,
						currentDestination,
					)
				default:
					t.Fatalf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			resource := emailDomainForwarderTestResource(t, server.URL)
			err := resource.deleteDomainForwarderForReplacement(
				t.Context(),
				domain,
				original,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"deleteDomainForwarderForReplacement() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if deleteCalls != 1 {
				t.Fatalf("deleteCalls = %d, want 1", deleteCalls)
			}
			if currentDestination != testCase.destinationOnDelete {
				t.Fatalf(
					"currentDestination = %q, want %q",
					currentDestination,
					testCase.destinationOnDelete,
				)
			}
		})
	}
}

func TestEmailDomainForwarderRestoreRequiresAttemptedState(t *testing.T) {
	t.Parallel()

	const (
		domain     = "example.test"
		original   = "original.test"
		attempted  = "attempted.test"
		concurrent = "concurrent.test"
	)

	testCases := []struct {
		name            string
		initial         string
		wantError       bool
		wantDestination string
		wantDeleteCalls int
		wantCreateCalls int
	}{
		{
			name:            "attempted state is restored",
			initial:         attempted,
			wantDestination: original,
			wantDeleteCalls: 1,
			wantCreateCalls: 1,
		},
		{
			name:            "original already restored is accepted",
			initial:         original,
			wantDestination: original,
		},
		{
			name:            "concurrent state is preserved",
			initial:         concurrent,
			wantError:       true,
			wantDestination: concurrent,
		},
		{
			name:      "absent attempted state is not recreated over",
			initial:   "",
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			currentDestination := testCase.initial
			deleteCalls := 0
			createCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Email/list_domain_forwarders":
					writeEmailDomainForwarderInventory(
						t,
						response,
						domain,
						currentDestination,
					)
				case "/execute/Email/delete_domain_forwarder":
					deleteCalls++
					currentDestination = ""
					writeEmailDomainForwarderTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous delete"},
						"data":   nil,
					})
				case "/execute/Email/add_domain_forwarder":
					if err := request.ParseForm(); err != nil {
						t.Fatalf("ParseForm() error: %v", err)
					}
					createCalls++
					currentDestination = request.Form.Get("destdomain")
					writeEmailDomainForwarderTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous create"},
						"data":   nil,
					})
				default:
					t.Fatalf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			resource := emailDomainForwarderTestResource(t, server.URL)
			err := resource.restoreDomainForwarder(
				t.Context(),
				domain,
				attempted,
				original,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"restoreDomainForwarder() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if currentDestination != testCase.wantDestination {
				t.Fatalf(
					"currentDestination = %q, want %q",
					currentDestination,
					testCase.wantDestination,
				)
			}
			if deleteCalls != testCase.wantDeleteCalls {
				t.Fatalf(
					"deleteCalls = %d, want %d",
					deleteCalls,
					testCase.wantDeleteCalls,
				)
			}
			if createCalls != testCase.wantCreateCalls {
				t.Fatalf(
					"createCalls = %d, want %d",
					createCalls,
					testCase.wantCreateCalls,
				)
			}
		})
	}
}

func TestAccEmailDomainForwarderResource(t *testing.T) {
	const resourceName = "cpanel_email_domain_forwarder.test"

	domain := testAccMainDomain(t)
	firstDestination := testAccEmailDomainForwarderDestination("first")
	secondDestination := testAccEmailDomainForwarderDestination("second")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEmailDomainForwarderDestroyed(domain),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailDomainForwarderResourceConfig(
					domain,
					firstDestination,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", domain),
					resource.TestCheckResourceAttr(
						resourceName,
						"destination",
						firstDestination,
					),
					testAccCheckEmailDomainForwarderExists(domain, firstDestination),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        domain,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
			},
			{
				Config: testAccEmailDomainForwarderResourceConfig(
					domain,
					secondDestination,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"destination",
						secondDestination,
					),
					testAccCheckEmailDomainForwarderExists(domain, secondDestination),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailDomainForwarder(t, domain)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailDomainForwarderResourceConfig(
					domain,
					secondDestination,
				),
				Check: testAccCheckEmailDomainForwarderExists(
					domain,
					secondDestination,
				),
			},
		},
	})
}

func emailDomainForwarderTestResource(
	t *testing.T,
	host string,
) emailDomainForwarderResource {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return emailDomainForwarderResource{
		client: cpanelmail.NewClient(baseClient),
	}
}

func writeEmailDomainForwarderInventory(
	t *testing.T,
	response http.ResponseWriter,
	domain string,
	destination string,
) {
	t.Helper()

	data := []map[string]string{}
	if destination != "" {
		data = append(data, map[string]string{
			"dest":    domain,
			"forward": destination,
		})
	}
	writeEmailDomainForwarderTestJSON(t, response, map[string]any{
		"status": 1,
		"data":   data,
	})
}

func writeEmailDomainForwarderTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func testAccEmailDomainForwarderResourceConfig(
	domain string,
	destination string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_domain_forwarder" "test" {
  domain      = %q
  destination = %q
}
`, domain, destination)
}
