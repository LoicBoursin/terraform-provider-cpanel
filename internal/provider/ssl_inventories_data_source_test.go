package provider

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

const testAccSSLInstalledHostsDataSourceName = "data.cpanel_ssl_installed_hosts.all"

func TestSSLInventoryDataSourceSchemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		constructor func() datasource.DataSource
		attribute   string
		fieldCount  int
		forbidden   []string
	}{
		{
			name:        "certificates",
			constructor: NewSSLCertificatesDataSource,
			attribute:   "certificates",
			fieldCount:  17,
			forbidden: []string{
				"certificate",
				"certificate_text",
				"fingerprint_sha256",
				"modulus",
				"ecdsa_public",
			},
		},
		{
			name:        "CSRs",
			constructor: NewSSLCSRsDataSource,
			attribute:   "csrs",
			fieldCount:  8,
			forbidden: []string{
				"csr",
				"fingerprint_sha256",
				"modulus",
				"ecdsa_public",
			},
		},
		{
			name:        "keys",
			constructor: NewSSLKeysDataSource,
			attribute:   "keys",
			fieldCount:  6,
			forbidden: []string{
				"modulus",
				"ecdsa_public",
				"private_key",
			},
		},
		{
			name:        "installed hosts",
			constructor: NewSSLInstalledHostsDataSource,
			attribute:   "hosts",
			fieldCount:  7,
			forbidden: []string{
				"certificate_text",
				"docroot",
				"ip",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := &datasource.SchemaResponse{}
			test.constructor().Schema(
				t.Context(),
				datasource.SchemaRequest{},
				response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf(
					"Schema() diagnostics: %v",
					response.Diagnostics,
				)
			}
			if len(response.Schema.Attributes) != 1 {
				t.Fatalf(
					"top-level attribute count = %d, want 1",
					len(response.Schema.Attributes),
				)
			}
			inventory, ok := response.Schema.Attributes[test.attribute].(datasourceschema.ListNestedAttribute)
			if !ok || !inventory.Computed {
				t.Fatalf(
					"%s must be a computed nested list",
					test.attribute,
				)
			}
			attributes := inventory.NestedObject.Attributes
			if len(attributes) != test.fieldCount {
				t.Fatalf(
					"%s nested field count = %d, want %d",
					test.attribute,
					len(attributes),
					test.fieldCount,
				)
			}
			for _, forbidden := range test.forbidden {
				if _, exists := attributes[forbidden]; exists {
					t.Fatalf(
						"%s must not expose %q",
						test.attribute,
						forbidden,
					)
				}
			}
			if test.attribute == "hosts" {
				certificate, ok := attributes["certificate"].(datasourceschema.SingleNestedAttribute)
				if !ok || !certificate.Computed {
					t.Fatal(
						"installed host certificate must be a computed nested attribute",
					)
				}
				for _, forbidden := range []string{
					"certificate",
					"certificate_text",
					"docroot",
					"ecdsa_public",
					"ip",
					"issuer_text",
					"modulus",
					"private_key",
					"subject_text",
				} {
					if _, exists := certificate.Attributes[forbidden]; exists {
						t.Fatalf(
							"installed host certificate must not expose %q",
							forbidden,
						)
					}
				}
			}
		})
	}
}

func TestAccSSLInventoryDataSources(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	baseline := testAccReadSSLInventories(t)
	t.Cleanup(func() {
		testAccRequireSSLInventories(t, baseline)
	})

	const (
		certificatesName = "data.cpanel_ssl_certificates.all"
		csrsName         = "data.cpanel_ssl_csrs.all"
		keysName         = "data.cpanel_ssl_keys.all"
		hostsName        = testAccSSLInstalledHostsDataSourceName
	)
	installedIDs := make(map[string]struct{}, len(baseline.Hosts))
	for _, host := range baseline.Hosts {
		installedIDs[host.Certificate.ID] = struct{}{}
	}
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			certificatesName,
			"certificates.#",
			strconv.Itoa(len(baseline.Certificates)),
		),
		testresource.TestCheckResourceAttr(
			csrsName,
			"csrs.#",
			strconv.Itoa(len(baseline.CSRs)),
		),
		testresource.TestCheckResourceAttr(
			keysName,
			"keys.#",
			strconv.Itoa(len(baseline.Keys)),
		),
		testresource.TestCheckResourceAttr(
			hostsName,
			"hosts.#",
			strconv.Itoa(len(baseline.Hosts)),
		),
	}
	for index, certificate := range baseline.Certificates {
		prefix := fmt.Sprintf("certificates.%d.", index)
		_, installed := installedIDs[certificate.ID]
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"id",
				certificate.ID,
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"friendly_name",
				certificate.FriendlyName,
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"created",
				strconv.FormatInt(certificate.Created, 10),
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"not_before",
				strconv.FormatInt(certificate.NotBefore, 10),
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"not_after",
				strconv.FormatInt(certificate.NotAfter, 10),
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"is_self_signed",
				strconv.FormatBool(certificate.IsSelfSigned),
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"domain_is_configured",
				strconv.FormatBool(certificate.DomainIsConfigured),
			),
			testresource.TestCheckResourceAttr(
				certificatesName,
				prefix+"installed",
				strconv.FormatBool(installed),
			),
		)
		checks = appendSSLInventoryListChecks(
			checks,
			certificatesName,
			prefix+"domains",
			certificate.Domains,
		)
		checks = appendSSLInventoryNullableStringChecks(
			checks,
			certificatesName,
			prefix,
			map[string]string{
				"serial":              certificate.Serial,
				"signature_algorithm": certificate.SignatureAlgorithm,
				"key_algorithm":       certificate.KeyAlgorithm,
				"ecdsa_curve_name":    certificate.ECDSACurveName,
				"issuer_common_name":  certificate.IssuerCommonName,
				"subject_common_name": certificate.SubjectCommonName,
				"validation_type":     certificate.ValidationType,
			},
		)
		checks = appendSSLInventoryPositiveIntCheck(
			checks,
			certificatesName,
			prefix+"modulus_length",
			certificate.ModulusLength,
		)
	}
	for index, csr := range baseline.CSRs {
		prefix := fmt.Sprintf("csrs.%d.", index)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				csrsName,
				prefix+"id",
				csr.ID,
			),
			testresource.TestCheckResourceAttr(
				csrsName,
				prefix+"friendly_name",
				csr.FriendlyName,
			),
			testresource.TestCheckResourceAttr(
				csrsName,
				prefix+"created",
				strconv.FormatInt(csr.Created, 10),
			),
		)
		checks = appendSSLInventoryListChecks(
			checks,
			csrsName,
			prefix+"domains",
			csr.Domains,
		)
		checks = appendSSLInventoryNullableStringChecks(
			checks,
			csrsName,
			prefix,
			map[string]string{
				"common_name":      csr.CommonName,
				"key_algorithm":    csr.KeyAlgorithm,
				"ecdsa_curve_name": csr.ECDSACurveName,
			},
		)
		checks = appendSSLInventoryPointerIntCheck(
			checks,
			csrsName,
			prefix+"modulus_length",
			csr.ModulusLength,
		)
	}
	for index, key := range baseline.Keys {
		prefix := fmt.Sprintf("keys.%d.", index)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				keysName,
				prefix+"id",
				key.ID,
			),
			testresource.TestCheckResourceAttr(
				keysName,
				prefix+"friendly_name",
				key.FriendlyName,
			),
			testresource.TestCheckResourceAttr(
				keysName,
				prefix+"created",
				strconv.FormatInt(key.Created, 10),
			),
		)
		checks = appendSSLInventoryNullableStringChecks(
			checks,
			keysName,
			prefix,
			map[string]string{
				"key_algorithm":    key.KeyAlgorithm,
				"ecdsa_curve_name": key.ECDSACurveName,
			},
		)
		checks = appendSSLInventoryPointerIntCheck(
			checks,
			keysName,
			prefix+"modulus_length",
			key.ModulusLength,
		)
	}
	for index, host := range baseline.Hosts {
		prefix := fmt.Sprintf("hosts.%d.", index)
		certificatePrefix := prefix + "certificate."
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				hostsName,
				prefix+"servername",
				host.ServerName,
			),
			testresource.TestCheckResourceAttr(
				hostsName,
				certificatePrefix+"id",
				host.Certificate.ID,
			),
			testresource.TestCheckResourceAttr(
				hostsName,
				certificatePrefix+"is_self_signed",
				strconv.FormatBool(host.Certificate.IsSelfSigned),
			),
			testresource.TestCheckResourceAttr(
				hostsName,
				certificatePrefix+"not_before",
				strconv.FormatInt(host.Certificate.NotBefore, 10),
			),
			testresource.TestCheckResourceAttr(
				hostsName,
				certificatePrefix+"not_after",
				strconv.FormatInt(host.Certificate.NotAfter, 10),
			),
		)
		checks = appendSSLInventoryPointerBoolCheck(
			checks,
			prefix+"is_primary_on_ip",
			host.IsPrimaryOnIP,
		)
		checks = appendSSLInventoryPointerBoolCheck(
			checks,
			prefix+"mail_sni_status",
			host.MailSNIStatus,
		)
		checks = appendSSLInventoryPointerBoolCheck(
			checks,
			prefix+"needs_sni",
			host.NeedsSNI,
		)
		checks = appendSSLInventoryPointerBoolCheck(
			checks,
			certificatePrefix+"is_autossl",
			host.Certificate.IsAutoSSL,
		)
		checks = appendSSLInventoryListChecks(
			checks,
			hostsName,
			prefix+"domains",
			host.Domains,
		)
		checks = appendSSLInventoryListChecks(
			checks,
			hostsName,
			prefix+"fqdns",
			host.FQDNs,
		)
		checks = appendSSLInventoryListChecks(
			checks,
			hostsName,
			certificatePrefix+"domains",
			host.Certificate.Domains,
		)
		checks = appendSSLInventoryNullableStringChecks(
			checks,
			hostsName,
			certificatePrefix,
			map[string]string{
				"auto_ssl_provider": host.Certificate.AutoSSLProvider,
				"auto_ssl_provider_display_name": host.Certificate.
					AutoSSLProviderDisplayName,
				"signature_algorithm": host.Certificate.
					SignatureAlgorithm,
				"issuer_common_name": host.Certificate.
					IssuerCommonName,
				"subject_common_name": host.Certificate.
					SubjectCommonName,
				"validation_type": host.Certificate.ValidationType,
			},
		)
		checks = appendSSLInventoryPointerIntCheck(
			checks,
			hostsName,
			certificatePrefix+"modulus_length",
			host.Certificate.ModulusLength,
		)
	}

	const config = providerConfig + `
data "cpanel_ssl_certificates" "all" {}

data "cpanel_ssl_csrs" "all" {}

data "cpanel_ssl_keys" "all" {}

data "cpanel_ssl_installed_hosts" "all" {}
`
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireSSLInventories(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					checks...,
				),
			},
			{
				Config: config,
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

type testAccSSLInventoryBaseline struct {
	Certificates []sslcertificate.Certificate
	CSRs         []sslcsr.CSRMetadata
	Keys         []sslcsr.KeyMetadata
	Hosts        []sslcertificate.InstalledHost
}

func testAccReadSSLInventories(
	t *testing.T,
) testAccSSLInventoryBaseline {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	certificateClient := sslcertificate.NewClient(client)
	certificates, err := certificateClient.List(ctx)
	if err != nil {
		t.Fatalf("read stored SSL certificates: %v", err)
	}
	hosts, err := certificateClient.ListInstalledHosts(ctx)
	if err != nil {
		t.Fatalf("read installed SSL hosts: %v", err)
	}
	csrClient := sslcsr.NewClient(client)
	csrs, err := csrClient.ListMetadata(ctx)
	if err != nil {
		t.Fatalf("read stored SSL CSR metadata: %v", err)
	}
	keys, err := csrClient.ListKeys(ctx)
	if err != nil {
		t.Fatalf("read stored SSL key metadata: %v", err)
	}

	return testAccSSLInventoryBaseline{
		Certificates: certificates,
		CSRs:         csrs,
		Keys:         keys,
		Hosts:        hosts,
	}
}

func testAccRequireSSLInventories(
	t *testing.T,
	expected testAccSSLInventoryBaseline,
) {
	t.Helper()

	actual := testAccReadSSLInventories(t)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"cPanel SSL metadata inventory changed: "+
				"certificates %d/%d, CSRs %d/%d, keys %d/%d, hosts %d/%d",
			len(actual.Certificates),
			len(expected.Certificates),
			len(actual.CSRs),
			len(expected.CSRs),
			len(actual.Keys),
			len(expected.Keys),
			len(actual.Hosts),
			len(expected.Hosts),
		)
	}
}

func appendSSLInventoryListChecks(
	checks []testresource.TestCheckFunc,
	resourceName string,
	path string,
	values []string,
) []testresource.TestCheckFunc {
	if values == nil {
		return append(
			checks,
			testresource.TestCheckNoResourceAttr(resourceName, path),
		)
	}
	checks = append(
		checks,
		testresource.TestCheckResourceAttr(
			resourceName,
			path+".#",
			strconv.Itoa(len(values)),
		),
	)
	for index, value := range values {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				fmt.Sprintf("%s.%d", path, index),
				value,
			),
		)
	}

	return checks
}

func appendSSLInventoryPointerBoolCheck(
	checks []testresource.TestCheckFunc,
	path string,
	value *bool,
) []testresource.TestCheckFunc {
	if value == nil {
		return append(
			checks,
			testresource.TestCheckNoResourceAttr(
				testAccSSLInstalledHostsDataSourceName,
				path,
			),
		)
	}

	return append(
		checks,
		testresource.TestCheckResourceAttr(
			testAccSSLInstalledHostsDataSourceName,
			path,
			strconv.FormatBool(*value),
		),
	)
}

func appendSSLInventoryNullableStringChecks(
	checks []testresource.TestCheckFunc,
	resourceName string,
	prefix string,
	values map[string]string,
) []testresource.TestCheckFunc {
	for attribute, value := range values {
		path := prefix + attribute
		if value == "" {
			checks = append(
				checks,
				testresource.TestCheckNoResourceAttr(
					resourceName,
					path,
				),
			)
			continue
		}
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				path,
				value,
			),
		)
	}

	return checks
}

func appendSSLInventoryPointerIntCheck(
	checks []testresource.TestCheckFunc,
	resourceName string,
	path string,
	value *int64,
) []testresource.TestCheckFunc {
	if value == nil {
		return append(
			checks,
			testresource.TestCheckNoResourceAttr(resourceName, path),
		)
	}

	return append(
		checks,
		testresource.TestCheckResourceAttr(
			resourceName,
			path,
			strconv.FormatInt(*value, 10),
		),
	)
}

func appendSSLInventoryPositiveIntCheck(
	checks []testresource.TestCheckFunc,
	resourceName string,
	path string,
	value int64,
) []testresource.TestCheckFunc {
	if value <= 0 {
		return append(
			checks,
			testresource.TestCheckNoResourceAttr(resourceName, path),
		)
	}

	return append(
		checks,
		testresource.TestCheckResourceAttr(
			resourceName,
			path,
			strconv.FormatInt(value, 10),
		),
	)
}
