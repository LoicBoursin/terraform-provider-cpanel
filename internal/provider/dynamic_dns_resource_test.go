package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestDynamicDNSResourceSchemaProtectsWebcallCredentials(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewDynamicDNSResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"webcall_id", "webcall_url"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok {
			t.Fatalf(
				"%s has type %T, want schema.StringAttribute",
				attributeName,
				response.Schema.Attributes[attributeName],
			)
		}
		if !attribute.Sensitive || !attribute.Computed {
			t.Fatalf("%s must be sensitive and computed", attributeName)
		}
	}
}

func TestDynamicDNSMutationErrorsRedactWebcallCredentials(t *testing.T) {
	t.Parallel()

	const secret = "dynamic-dns-secret-id"

	mutationErr := dynamicDNSCredentialMutationError(
		errors.New("request failed after sending "+secret),
		"description update",
	)
	if mutationErr == nil {
		t.Fatal("dynamicDNSCredentialMutationError() = nil, want error")
	}
	if strings.Contains(mutationErr.Error(), secret) {
		t.Fatalf(
			"dynamicDNSCredentialMutationError() leaked %q",
			secret,
		)
	}

	detail := dynamicDNSMutationErrorDetail(
		errors.New("verification failed"),
		errors.New("rollback failed after sending "+secret),
	)
	if strings.Contains(detail, secret) {
		t.Fatalf("dynamicDNSMutationErrorDetail() leaked %q", secret)
	}
	if !strings.Contains(detail, "verification failed") ||
		!strings.Contains(detail, "Dynamic DNS rollback") {
		t.Fatalf(
			"dynamicDNSMutationErrorDetail() = %q, want primary and redacted rollback errors",
			detail,
		)
	}
}

func TestAccDynamicDNSResource(t *testing.T) {
	const resourceName = "cpanel_dynamic_dns.test"

	initialDomain := testAccDynamicDNSDomain(t, "resource")
	replacementDomain := testAccDynamicDNSDomain(t, "replacement")
	var initialWebcallID string

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDynamicDNSDestroyed(
			initialDomain,
			replacementDomain,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccDynamicDNSResourceConfig(
					initialDomain,
					"Initial endpoint",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"domain",
						initialDomain,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"description",
						"Initial endpoint",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"webcall_id",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"webcall_url",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"created_at",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"ipv4.#",
						"0",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"ipv6.#",
						"0",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"last_run_times.#",
						"0",
					),
					testAccCaptureDynamicDNSWebcallID(
						resourceName,
						&initialWebcallID,
					),
					testAccCheckDynamicDNSExists(
						initialDomain,
						"Initial endpoint",
					),
				),
			},
			{
				Config: testAccDynamicDNSResourceConfig(
					initialDomain,
					"Updated endpoint",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"description",
						"Updated endpoint",
					),
					testAccCheckDynamicDNSWebcallID(
						resourceName,
						&initialWebcallID,
						false,
					),
					testAccCheckDynamicDNSExists(
						initialDomain,
						"Updated endpoint",
					),
				),
			},
			{
				PreConfig: func() {
					testAccSetDynamicDNSDescription(
						t,
						initialDomain,
						"External description",
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDynamicDNSResourceConfig(
					initialDomain,
					"Updated endpoint",
				),
				Check: testAccCheckDynamicDNSExists(
					initialDomain,
					"Updated endpoint",
				),
			},
			{
				PreConfig: func() {
					testAccRecreateDynamicDNS(t, initialDomain)
				},
				Config: testAccDynamicDNSResourceConfig(
					initialDomain,
					"Updated endpoint",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckDynamicDNSWebcallID(
						resourceName,
						&initialWebcallID,
						true,
					),
					testAccCheckDynamicDNSExists(
						initialDomain,
						"Updated endpoint",
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialDomain,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
			},
			{
				Config: testAccDynamicDNSResourceConfig(
					replacementDomain,
					"Replacement endpoint",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckDynamicDNSExists(
						replacementDomain,
						"Replacement endpoint",
					),
					testAccCheckDynamicDNSDestroyed(initialDomain),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteDynamicDNS(t, replacementDomain)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDynamicDNSResourceConfig(
					replacementDomain,
					"Replacement endpoint",
				),
				Check: testAccCheckDynamicDNSExists(
					replacementDomain,
					"Replacement endpoint",
				),
			},
		},
	})
}

func testAccDynamicDNSResourceConfig(domain string, description string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_dynamic_dns" "test" {
  domain      = %q
  description = %q
}
`, domain, description)
}

func testAccCaptureDynamicDNSWebcallID(
	resourceName string,
	target *string,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}

		value := resourceState.Primary.Attributes["webcall_id"]
		if value == "" {
			return fmt.Errorf("attribute %s.webcall_id is empty", resourceName)
		}
		*target = value

		return nil
	}
}

func testAccCheckDynamicDNSWebcallID(
	resourceName string,
	previous *string,
	wantChanged bool,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}

		current := resourceState.Primary.Attributes["webcall_id"]
		if current == "" {
			return fmt.Errorf("attribute %s.webcall_id is empty", resourceName)
		}
		changed := current != *previous
		if changed != wantChanged {
			return fmt.Errorf(
				"attribute %s.webcall_id changed = %t; want %t",
				resourceName,
				changed,
				wantChanged,
			)
		}

		return nil
	}
}
