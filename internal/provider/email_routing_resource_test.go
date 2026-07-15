package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
	cpanelsslcertificate "terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

func TestEmailRoutingDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkdatasource.SchemaResponse{}
	NewEmailRoutingDataSource().Schema(
		t.Context(),
		frameworkdatasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	domain, ok := response.Schema.Attributes["domain"].(datasourceschema.StringAttribute)
	if !ok || !domain.Required {
		t.Fatal("domain must be a required string")
	}
	for _, name := range []string{
		"mode",
		"detected_mode",
		"primary_exchanger",
	} {
		if !response.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestAccEmailRoutingResource(t *testing.T) {
	const resourceName = "cpanel_email_routing.test"

	domain := testAccSubdomain(t, "emailrouting")
	documentRoot := testAccDomainDocumentRoot("routing")
	var originalMode cpanelmail.RoutingMode
	testAccRegisterEmailRoutingCleanup(t, domain, &originalMode)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeTestCheckFunc(
			testAccCheckEmailRoutingDestroyed(domain),
			testAccCheckSubdomainsDestroyed(domain),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccEmailRoutingFixtureConfig(
					domain,
					documentRoot,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSubdomainExists(domain, documentRoot),
					testAccCaptureEmailRoutingMode(domain, &originalMode),
				),
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeRemote,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"domain",
						domain,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"mode",
						string(cpanelmail.RoutingModeRemote),
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"detected_mode",
						string(cpanelmail.RoutingModeRemote),
					),
					testresource.TestCheckNoResourceAttr(
						resourceName,
						"primary_exchanger",
					),
					testAccCheckEmailRoutingResourceRestoreMode(
						resourceName,
						&originalMode,
					),
					testAccCheckEmailRouting(
						domain,
						cpanelmail.RoutingModeRemote,
					),
					testAccCheckEmailRoutingDataSource(
						domain,
						cpanelmail.RoutingModeRemote,
					),
				),
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeLocal,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailRouting(
						domain,
						cpanelmail.RoutingModeLocal,
					),
					testAccCheckEmailRoutingDataSource(
						domain,
						cpanelmail.RoutingModeLocal,
					),
				),
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeBackup,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailRouting(
						domain,
						cpanelmail.RoutingModeBackup,
					),
					testAccCheckEmailRoutingDataSource(
						domain,
						cpanelmail.RoutingModeBackup,
					),
				),
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeAuto,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailRouting(
						domain,
						cpanelmail.RoutingModeAuto,
					),
					testAccCheckEmailRoutingDataSource(
						domain,
						cpanelmail.RoutingModeAuto,
					),
				),
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeRemote,
				),
				Check: testAccCheckEmailRouting(
					domain,
					cpanelmail.RoutingModeRemote,
				),
			},
			{
				PreConfig: func() {
					testAccSetEmailRouting(
						t,
						domain,
						cpanelmail.RoutingModeLocal,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeRemote,
				),
				Check: testAccCheckEmailRouting(
					domain,
					cpanelmail.RoutingModeRemote,
				),
			},
			{
				Config: testAccEmailRoutingFixtureConfig(
					domain,
					documentRoot,
				),
				Check: testAccCheckEmailRoutingModePointer(
					domain,
					&originalMode,
				),
			},
		},
	})
}

func TestAccEmailRoutingResourceImport(t *testing.T) {
	const resourceName = "cpanel_email_routing.test"

	domain := testAccSubdomain(t, "emailroutingimport")
	documentRoot := testAccDomainDocumentRoot("routing-import")
	var originalMode cpanelmail.RoutingMode
	testAccRegisterEmailRoutingCleanup(t, domain, &originalMode)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeTestCheckFunc(
			testAccCheckEmailRoutingDestroyed(domain),
			testAccCheckSubdomainsDestroyed(domain),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccEmailRoutingFixtureConfig(
					domain,
					documentRoot,
				),
				Check: testAccCaptureEmailRoutingMode(domain, &originalMode),
			},
			{
				Config: testAccEmailRoutingResourceConfig(
					domain,
					documentRoot,
					cpanelmail.RoutingModeAuto,
				),
				Check: testAccCheckEmailRouting(
					domain,
					cpanelmail.RoutingModeAuto,
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        domain,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
			},
		},
	})
}

func testAccEmailRoutingFixtureConfig(
	domain string,
	documentRoot string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_subdomain" "routing" {
  domain               = %q
  document_root        = %q
  delete_document_root = false
}
`, domain, documentRoot)
}

func testAccEmailRoutingResourceConfig(
	domain string,
	documentRoot string,
	mode cpanelmail.RoutingMode,
) string {
	return testAccEmailRoutingFixtureConfig(
		domain,
		documentRoot,
	) + fmt.Sprintf(`
resource "cpanel_email_routing" "test" {
  domain = cpanel_subdomain.routing.domain
  mode   = %q
}

data "cpanel_email_routing" "test" {
  domain = cpanel_email_routing.test.domain
}
`, mode)
}

func testAccCaptureEmailRoutingMode(
	domain string,
	target *cpanelmail.RoutingMode,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		routing, err := testAccGetEmailRouting(domain)
		if err != nil {
			return err
		}
		if routing == nil {
			return fmt.Errorf(
				"email routing for domain %q was not found",
				domain,
			)
		}
		*target = routing.Mode

		return nil
	}
}

func testAccCheckEmailRouting(
	domain string,
	mode cpanelmail.RoutingMode,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		routing, err := testAccGetEmailRouting(domain)
		if err != nil {
			return err
		}
		if routing == nil {
			return fmt.Errorf(
				"email routing for domain %q was not found",
				domain,
			)
		}
		if routing.Mode != mode {
			return fmt.Errorf(
				"email routing mode for domain %q = %q, want %q",
				domain,
				routing.Mode,
				mode,
			)
		}
		if mode != cpanelmail.RoutingModeAuto &&
			routing.DetectedMode != mode {
			return fmt.Errorf(
				"detected email routing mode for domain %q = %q, want %q",
				domain,
				routing.DetectedMode,
				mode,
			)
		}

		return nil
	}
}

func testAccCheckEmailRoutingModePointer(
	domain string,
	mode *cpanelmail.RoutingMode,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		if *mode == "" {
			return fmt.Errorf(
				"original email routing mode for domain %q was not captured",
				domain,
			)
		}

		return testAccCheckEmailRouting(domain, *mode)(state)
	}
}

func testAccCheckEmailRoutingResourceRestoreMode(
	resourceName string,
	mode *cpanelmail.RoutingMode,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		if *mode == "" {
			return fmt.Errorf("original email routing mode was not captured")
		}

		return testresource.TestCheckResourceAttr(
			resourceName,
			"restore_mode",
			string(*mode),
		)(state)
	}
}

func testAccCheckEmailRoutingDataSource(
	domain string,
	mode cpanelmail.RoutingMode,
) testresource.TestCheckFunc {
	const dataSourceName = "data.cpanel_email_routing.test"

	return testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"domain",
			domain,
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"mode",
			string(mode),
		),
		testresource.TestCheckResourceAttrSet(
			dataSourceName,
			"detected_mode",
		),
		testresource.TestCheckNoResourceAttr(
			dataSourceName,
			"primary_exchanger",
		),
	)
}

func testAccCheckEmailRoutingDestroyed(
	domain string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		routing, err := testAccGetEmailRouting(domain)
		if err != nil {
			return err
		}
		if routing != nil {
			return fmt.Errorf(
				"email routing for domain %q still exists after destroy",
				domain,
			)
		}

		return nil
	}
}

func testAccGetEmailRouting(
	domain string,
) (*cpanelmail.Routing, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}

	return cpanelmail.NewClient(client).GetRouting(ctx, domain)
}

func testAccSetEmailRouting(
	t *testing.T,
	domain string,
	mode cpanelmail.RoutingMode,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, _, err := cpanelmail.NewClient(client).SetRouting(
		ctx,
		cpanelmail.RoutingDefinition{
			Domain: domain,
			Mode:   mode,
		},
	); err != nil {
		t.Fatalf(
			"set email routing mode %q for domain %q: %v",
			mode,
			domain,
			err,
		)
	}
}

func testAccRegisterEmailRoutingCleanup(
	t *testing.T,
	domain string,
	originalMode *cpanelmail.RoutingMode,
) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			t.Errorf("create cPanel client during routing cleanup: %v", err)
			return
		}
		emailClient := cpanelmail.NewClient(client)
		if *originalMode != "" {
			current, routingErr := emailClient.GetRouting(ctx, domain)
			if routingErr != nil {
				t.Errorf(
					"read email routing for domain %q during cleanup: %v",
					domain,
					routingErr,
				)
			} else if current != nil && current.Mode != *originalMode {
				if _, _, routingErr = emailClient.SetRouting(
					ctx,
					cpanelmail.RoutingDefinition{
						Domain: domain,
						Mode:   *originalMode,
					},
				); routingErr != nil {
					t.Errorf(
						"restore email routing for domain %q during cleanup: %v",
						domain,
						routingErr,
					)
				}
			}
		}

		testAccCleanupEmailRoutingCertificates(t, ctx, client, domain)
	})
}

func testAccCleanupEmailRoutingCertificates(
	t *testing.T,
	ctx context.Context,
	client *cpanelapi.Client,
	domain string,
) {
	t.Helper()

	sslClient := cpanelsslcertificate.NewClient(client)
	certificates, err := sslClient.List(ctx)
	if err != nil {
		t.Errorf(
			"list SSL certificates during email routing cleanup: %v",
			err,
		)
		return
	}

	for _, certificate := range certificates {
		matches := false
		safeDomains := true
		for _, certificateDomain := range certificate.Domains {
			if certificateDomain == domain ||
				certificateDomain == "www."+domain {
				matches = true
				continue
			}
			safeDomains = false
		}
		if !matches {
			continue
		}
		if !safeDomains ||
			certificate.DomainIsConfigured {
			t.Errorf(
				"refusing to delete ambiguous or configured SSL certificate %q for email routing domain %q",
				certificate.ID,
				domain,
			)
			continue
		}

		installed, err := sslClient.IsInstalled(ctx, certificate.ID)
		if err != nil {
			t.Errorf(
				"inspect SSL certificate %q installation during email routing cleanup: %v",
				certificate.ID,
				err,
			)
			continue
		}
		if installed {
			t.Errorf(
				"refusing to delete installed SSL certificate %q for email routing domain %q",
				certificate.ID,
				domain,
			)
			continue
		}
		if err := sslClient.Delete(ctx, certificate.ID); err != nil {
			t.Errorf(
				"delete SSL certificate %q for email routing domain %q: %v",
				certificate.ID,
				domain,
				err,
			)
		}
	}
}
