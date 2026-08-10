package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

func TestModSecurityDomainResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewModSecurityDomainResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	domain, ok := response.Schema.Attributes["domain"].(resourceschema.StringAttribute)
	if !ok || !domain.Required {
		t.Fatal("domain must be a required string")
	}
	enabled, ok := response.Schema.Attributes["enabled"].(resourceschema.BoolAttribute)
	if !ok || !enabled.Required {
		t.Fatal("enabled must be a required bool")
	}
	if !strings.Contains(
		response.Schema.MarkdownDescription,
		"re-enables ModSecurity",
	) {
		t.Fatal("schema must document the reset-to-enabled destroy behavior")
	}
	for _, attributeName := range []string{"dependencies", "affected_domains"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.SetAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed set", attributeName)
		}
	}
}

func TestModSecurityDomainResourceModelDetectsManagedDrift(t *testing.T) {
	t.Parallel()

	state := ModSecurityDomainResourceModel{
		Domain:  types.StringValue("sub.example.test"),
		Enabled: types.BoolValue(true),
	}
	current := modsecurity.Domain{
		Domain:  state.Domain.ValueString(),
		Enabled: true,
	}
	if !modSecurityDomainMatchesResourceModel(current, state) {
		t.Fatal("matching ModSecurity state was not recognized")
	}
	current.Enabled = false
	if modSecurityDomainMatchesResourceModel(current, state) {
		t.Fatal("changed ModSecurity status was accepted")
	}
}

func TestAccModSecurityDomainResource(t *testing.T) {
	const resourceName = "cpanel_modsecurity_domain.test"

	firstDomain := testAccSubdomain(t, "modsecurityprimary")
	secondDomain := testAccSubdomain(t, "modsecurityreplacement")
	firstDocumentRoot := testAccDomainDocumentRoot("modsecurity-primary")
	secondDocumentRoot := testAccDomainDocumentRoot("modsecurity-replacement")

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccModSecurityPreCheck(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeTestCheckFunc(
			testAccCheckModSecurityDomainsDestroyed(firstDomain, secondDomain),
			testAccCheckSubdomainsDestroyed(firstDomain, secondDomain),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccModSecurityFixturesConfig(
					firstDomain,
					firstDocumentRoot,
					secondDomain,
					secondDocumentRoot,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSubdomainExists(firstDomain, firstDocumentRoot),
					testAccCheckSubdomainExists(secondDomain, secondDocumentRoot),
					testAccCheckModSecurityDomainIsolation(firstDomain),
					testAccCheckModSecurityDomainIsolation(secondDomain),
					testAccCheckModSecurityDomain(firstDomain, true),
					testAccCheckModSecurityDomain(secondDomain, true),
				),
			},
			{
				Config: testAccModSecurityResourceConfig(
					firstDomain,
					firstDocumentRoot,
					secondDomain,
					secondDocumentRoot,
					"primary",
					false,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"domain",
						firstDomain,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"enabled",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"domain_type",
						modsecurity.DomainTypeSub,
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"affected_domains.*",
						firstDomain,
					),
					testAccCheckModSecurityDomain(firstDomain, false),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        firstDomain,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
			},
			{
				PreConfig: func() {
					testAccSetModSecurityDomain(t, firstDomain, true)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccModSecurityResourceConfig(
					firstDomain,
					firstDocumentRoot,
					secondDomain,
					secondDocumentRoot,
					"primary",
					false,
				),
				Check: testAccCheckModSecurityDomain(firstDomain, false),
			},
			{
				Config: testAccModSecurityResourceConfig(
					firstDomain,
					firstDocumentRoot,
					secondDomain,
					secondDocumentRoot,
					"replacement",
					false,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckModSecurityDomain(firstDomain, true),
					testAccCheckModSecurityDomain(secondDomain, false),
				),
			},
			{
				Config: testAccModSecurityResourceConfig(
					firstDomain,
					firstDocumentRoot,
					secondDomain,
					secondDocumentRoot,
					"replacement",
					true,
				),
				Check: testAccCheckModSecurityDomain(secondDomain, true),
			},
			{
				PreConfig: func() {
					testAccDeleteSubdomain(t, secondDomain)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccModSecurityResourceConfig(
					firstDomain,
					firstDocumentRoot,
					secondDomain,
					secondDocumentRoot,
					"replacement",
					false,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckModSecurityDomainIsolation(secondDomain),
					testAccCheckModSecurityDomain(secondDomain, false),
				),
			},
		},
	})
}

func testAccModSecurityFixturesConfig(
	firstDomain string,
	firstDocumentRoot string,
	secondDomain string,
	secondDocumentRoot string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_subdomain" "primary" {
  domain               = %q
  document_root        = %q
  delete_document_root = false
}

resource "cpanel_subdomain" "replacement" {
  domain               = %q
  document_root        = %q
  delete_document_root = false
}
`, firstDomain, firstDocumentRoot, secondDomain, secondDocumentRoot)
}

func testAccModSecurityResourceConfig(
	firstDomain string,
	firstDocumentRoot string,
	secondDomain string,
	secondDocumentRoot string,
	target string,
	enabled bool,
) string {
	return testAccModSecurityFixturesConfig(
		firstDomain,
		firstDocumentRoot,
		secondDomain,
		secondDocumentRoot,
	) + fmt.Sprintf(`
resource "cpanel_modsecurity_domain" "test" {
  domain  = cpanel_subdomain.%s.domain
  enabled = %t
}
`, target, enabled)
}

func testAccModSecurityPreCheck(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	modSecurityClient := modsecurity.NewClient(client)
	installed, err := modSecurityClient.HasInstalled(ctx)
	if err != nil {
		t.Fatalf("verify ModSecurity installation: %v", err)
	}
	if !installed {
		t.Fatal("ModSecurity is not installed")
	}
	if _, err := modSecurityClient.List(ctx); err != nil {
		t.Fatalf("verify ModSecurity domain access: %v", err)
	}
}

func testAccCheckModSecurityDomainIsolation(
	domain string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		apiDomain, err := testAccGetModSecurityDomain(domain)
		if err != nil {
			return err
		}
		if apiDomain == nil {
			return fmt.Errorf("ModSecurity domain %q was not found", domain)
		}
		for _, affectedDomain := range apiDomain.AffectedDomains() {
			if !strings.HasPrefix(
				affectedDomain,
				"tfcpanelsubmodsecurity",
			) {
				return fmt.Errorf(
					"refusing to mutate ModSecurity domain %q because it affects non-test domain %q",
					domain,
					affectedDomain,
				)
			}
		}

		return nil
	}
}

func testAccCheckModSecurityDomain(
	domain string,
	enabled bool,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		apiDomain, err := testAccGetModSecurityDomain(domain)
		if err != nil {
			return err
		}
		if apiDomain == nil {
			return fmt.Errorf("ModSecurity domain %q was not found", domain)
		}
		if apiDomain.Enabled != enabled {
			return fmt.Errorf(
				"ModSecurity domain %q enabled = %t, want %t",
				domain,
				apiDomain.Enabled,
				enabled,
			)
		}

		return nil
	}
}

func testAccCheckModSecurityDomainsDestroyed(
	domains ...string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		for _, domain := range domains {
			apiDomain, err := testAccGetModSecurityDomain(domain)
			if err != nil {
				return err
			}
			if apiDomain != nil {
				return fmt.Errorf(
					"ModSecurity domain %q still exists after destroy",
					domain,
				)
			}
		}

		return nil
	}
}

func testAccGetModSecurityDomain(
	domain string,
) (*modsecurity.Domain, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}

	return modsecurity.NewClient(client).Get(ctx, domain)
}

func testAccSetModSecurityDomain(
	t *testing.T,
	domain string,
	enabled bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, err := modsecurity.NewClient(client).SetEnabled(
		ctx,
		[]string{domain},
		enabled,
	); err != nil {
		t.Fatalf(
			"set ModSecurity domain %q enabled=%t: %v",
			domain,
			enabled,
			err,
		)
	}
}
