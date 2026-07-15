package provider

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

func TestSSLCertificateResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewSSLCertificateResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	friendlyName, ok := response.Schema.Attributes["friendly_name"].(resourceschema.StringAttribute)
	if !ok || !friendlyName.Required {
		t.Fatal("friendly_name must be a required string")
	}
	certificate, ok := response.Schema.Attributes["certificate"].(resourceschema.StringAttribute)
	if !ok || !certificate.Required || len(certificate.PlanModifiers) != 2 {
		t.Fatal(
			"certificate must be required with semantic equality and replacement plan modifiers",
		)
	}
	id, ok := response.Schema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !id.Computed {
		t.Fatal("id must be a computed string")
	}
	for _, attributeName := range []string{
		"domain_is_configured",
		"installed",
		"is_self_signed",
	} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.BoolAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed bool", attributeName)
		}
	}
}

func TestSSLCertificateDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewSSLCertificateDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	id, ok := response.Schema.Attributes["id"].(datasourceschema.StringAttribute)
	if !ok || !id.Required {
		t.Fatal("id must be a required string")
	}
	certificate, ok := response.Schema.Attributes["certificate"].(datasourceschema.StringAttribute)
	if !ok || !certificate.Computed || certificate.Sensitive {
		t.Fatal("certificate must be a non-sensitive computed string")
	}
}

func TestApplySSLCertificateToResourceModelPreservesEquivalentPEM(t *testing.T) {
	t.Parallel()

	certificatePEM, _, _ := testAccSSLCertificateMaterial(t, "model")
	serverPEM := strings.TrimSuffix(certificatePEM, "\n")
	model := SSLCertificateResourceModel{
		Certificate: types.StringValue(certificatePEM),
	}

	diagnostics := applySSLCertificateToResourceModel(
		t.Context(),
		&model,
		sslcertificate.Certificate{
			ID:             "certificate-id",
			FriendlyName:   "certificate-name",
			CertificatePEM: serverPEM,
		},
		false,
	)
	if diagnostics.HasError() {
		t.Fatalf(
			"applySSLCertificateToResourceModel() diagnostics: %v",
			diagnostics,
		)
	}
	if got := model.Certificate.ValueString(); got != certificatePEM {
		t.Fatalf(
			"certificate = %q, want original configured PEM %q",
			got,
			certificatePEM,
		)
	}
}

func TestApplySSLCertificateToResourceModelNormalizesMissingPEM(t *testing.T) {
	t.Parallel()

	certificatePEM, _, _ := testAccSSLCertificateMaterial(t, "model")
	serverPEM := strings.TrimSuffix(certificatePEM, "\n")
	model := SSLCertificateResourceModel{}

	diagnostics := applySSLCertificateToResourceModel(
		t.Context(),
		&model,
		sslcertificate.Certificate{
			ID:             "certificate-id",
			FriendlyName:   "certificate-name",
			CertificatePEM: certificatePEM,
		},
		false,
	)
	if diagnostics.HasError() {
		t.Fatalf(
			"applySSLCertificateToResourceModel() diagnostics: %v",
			diagnostics,
		)
	}
	if got := model.Certificate.ValueString(); got != serverPEM {
		t.Fatalf(
			"certificate = %q, want normalized PEM %q",
			got,
			serverPEM,
		)
	}
}

func TestAccSSLCertificateResource(t *testing.T) {
	const resourceName = "cpanel_ssl_certificate.test"

	initialPEM, initialFingerprint, initialDomain := testAccSSLCertificateMaterial(
		t,
		"initial",
	)
	replacementPEM, replacementFingerprint, replacementDomain :=
		testAccSSLCertificateMaterial(t, "replacement")
	initialName := testAccSSLCertificateFriendlyName("initial")
	renamedName := testAccSSLCertificateFriendlyName("renamed")
	driftName := testAccSSLCertificateFriendlyName("drift")
	replacementName := testAccSSLCertificateFriendlyName("replacement")
	var initialID string
	var recreatedID string

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSSLCertificatesDestroyed(
			initialName,
			renamedName,
			driftName,
			replacementName,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccSSLCertificateResourceConfig(
					initialName,
					initialPEM,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"friendly_name",
						initialName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"fingerprint_sha256",
						initialFingerprint,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"installed",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"domain_is_configured",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"is_self_signed",
						"true",
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"domains.*",
						initialDomain,
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"id",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"created",
					),
					testAccCheckSSLCertificateExists(
						initialName,
						initialPEM,
						&initialID,
					),
				),
			},
			{
				Config: testAccSSLCertificateResourceConfig(
					initialName,
					initialPEM,
				),
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: testAccSSLCertificateResourceConfig(
					renamedName,
					initialPEM,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCertificateExists(
						renamedName,
						initialPEM,
						nil,
					),
					testAccCheckSSLCertificateID(
						renamedName,
						&initialID,
					),
					testAccCheckSSLCertificatesDestroyed(initialName),
				),
			},
			{
				ResourceName: resourceName,
				ImportStateIdFunc: testAccImportStateIDFromAttribute(
					resourceName,
					"id",
				),
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
				ImportStateVerifyIgnore:              []string{"certificate"},
			},
			{
				Config: testAccSSLCertificateResourceConfig(
					renamedName,
					initialPEM,
				),
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				PreConfig: func() {
					testAccRenameSSLCertificate(
						t,
						renamedName,
						driftName,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSSLCertificateResourceConfig(
					renamedName,
					initialPEM,
				),
				Check: testAccCheckSSLCertificateExists(
					renamedName,
					initialPEM,
					nil,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteSSLCertificate(t, renamedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSSLCertificateResourceConfig(
					renamedName,
					initialPEM,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCertificateExists(
						renamedName,
						initialPEM,
						&recreatedID,
					),
				),
			},
			{
				Config: testAccSSLCertificateResourceConfig(
					replacementName,
					replacementPEM,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"fingerprint_sha256",
						replacementFingerprint,
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"domains.*",
						replacementDomain,
					),
					testAccCheckSSLCertificateExists(
						replacementName,
						replacementPEM,
						nil,
					),
					testAccCheckSSLCertificateIDMissing(&recreatedID),
					testAccCheckSSLCertificatesDestroyed(renamedName),
				),
			},
		},
	})
}

func TestAccSSLCertificateDataSource(t *testing.T) {
	const (
		resourceName   = "cpanel_ssl_certificate.test"
		dataSourceName = "data.cpanel_ssl_certificate.test"
	)

	certificatePEM, fingerprint, domain := testAccSSLCertificateMaterial(
		t,
		"data",
	)
	friendlyName := testAccSSLCertificateFriendlyName("data")

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSSLCertificatesDestroyed(
			friendlyName,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccSSLCertificateDataSourceConfig(
					friendlyName,
					certificatePEM,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttrPair(
						dataSourceName,
						"id",
						resourceName,
						"id",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"friendly_name",
						friendlyName,
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"fingerprint_sha256",
						fingerprint,
					),
					testresource.TestCheckTypeSetElemAttr(
						dataSourceName,
						"domains.*",
						domain,
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"installed",
						"false",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"domain_is_configured",
						"false",
					),
				),
			},
		},
	})
}

func testAccSSLCertificateFriendlyName(kind string) string {
	return testAccRegisterArtifact(fmt.Sprintf(
		"tfcpanelsslcert-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(
			acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum),
		),
	))
}

func testAccSSLCertificateMaterial(
	t *testing.T,
	kind string,
) (string, string, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(
		elliptic.P256(),
		cryptorand.Reader,
	)
	if err != nil {
		t.Fatalf("generate SSL certificate key: %v", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := cryptorand.Int(cryptorand.Reader, serialLimit)
	if err != nil {
		t.Fatalf("generate SSL certificate serial: %v", err)
	}
	domain := fmt.Sprintf(
		"%s.example.test",
		testAccSSLCertificateFriendlyName(kind),
	)
	now := time.Now().UTC().Truncate(time.Second)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: domain,
		},
		DNSNames:              []string{domain},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(
		cryptorand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		t.Fatalf("create SSL certificate: %v", err)
	}
	certificatePEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificateDER,
	}))
	parsed, err := sslcertificate.ParsePEM(certificatePEM)
	if err != nil {
		t.Fatalf("parse generated SSL certificate: %v", err)
	}

	return certificatePEM, parsed.SHA256Fingerprint, domain
}

func testAccSSLCertificateResourceConfig(
	friendlyName string,
	certificatePEM string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_ssl_certificate" "test" {
  friendly_name = %q
  certificate   = %q
}
`, friendlyName, certificatePEM)
}

func testAccSSLCertificateDataSourceConfig(
	friendlyName string,
	certificatePEM string,
) string {
	return testAccSSLCertificateResourceConfig(
		friendlyName,
		certificatePEM,
	) + `
data "cpanel_ssl_certificate" "test" {
  id = cpanel_ssl_certificate.test.id
}
`
}

func testAccCheckSSLCertificateExists(
	friendlyName string,
	certificatePEM string,
	capturedID *string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		certificate, err := testAccGetSSLCertificateByFriendlyName(
			friendlyName,
		)
		if err != nil {
			return err
		}
		if certificate == nil {
			return fmt.Errorf(
				"SSL certificate %q was not found",
				friendlyName,
			)
		}
		equal, err := sslcertificate.EqualPEM(
			certificate.CertificatePEM,
			certificatePEM,
		)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf(
				"SSL certificate %q PEM does not match",
				friendlyName,
			)
		}
		if certificate.DomainIsConfigured {
			return fmt.Errorf(
				"SSL certificate %q unexpectedly configures a domain",
				friendlyName,
			)
		}
		if certificate.Created <= 0 {
			return fmt.Errorf(
				"SSL certificate %q has invalid creation time %d",
				friendlyName,
				certificate.Created,
			)
		}

		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()
		client, err := testAccClient()
		if err != nil {
			return err
		}
		installed, err := sslcertificate.NewClient(client).IsInstalled(
			ctx,
			certificate.ID,
		)
		if err != nil {
			return err
		}
		if installed {
			return fmt.Errorf(
				"SSL certificate %q unexpectedly became installed",
				friendlyName,
			)
		}
		if capturedID != nil {
			*capturedID = certificate.ID
		}

		return nil
	}
}

func testAccCheckSSLCertificatesDestroyed(
	friendlyNames ...string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()
		client, err := testAccClient()
		if err != nil {
			return err
		}
		certificates, err := sslcertificate.NewClient(client).List(ctx)
		if err != nil {
			return err
		}
		for _, certificate := range certificates {
			for _, friendlyName := range friendlyNames {
				if certificate.FriendlyName == friendlyName {
					return fmt.Errorf(
						"SSL certificate %q still exists",
						friendlyName,
					)
				}
			}
		}

		return nil
	}
}

func testAccCheckSSLCertificateID(
	friendlyName string,
	expectedID *string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if expectedID == nil || *expectedID == "" {
			return fmt.Errorf(
				"expected SSL certificate id was not captured",
			)
		}
		certificate, err := testAccGetSSLCertificateByFriendlyName(
			friendlyName,
		)
		if err != nil {
			return err
		}
		if certificate == nil {
			return fmt.Errorf(
				"SSL certificate %q was not found",
				friendlyName,
			)
		}
		if certificate.ID != *expectedID {
			return fmt.Errorf(
				"SSL certificate %q id is %q; expected %q",
				friendlyName,
				certificate.ID,
				*expectedID,
			)
		}

		return nil
	}
}

func testAccCheckSSLCertificateIDMissing(
	id *string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if id == nil || *id == "" {
			return fmt.Errorf(
				"SSL certificate id to verify was not captured",
			)
		}
		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()
		client, err := testAccClient()
		if err != nil {
			return err
		}
		certificate, err := sslcertificate.NewClient(client).Get(ctx, *id)
		if err != nil {
			return err
		}
		if certificate != nil {
			return fmt.Errorf(
				"SSL certificate id %q still exists",
				*id,
			)
		}

		return nil
	}
}

func testAccRenameSSLCertificate(
	t *testing.T,
	currentFriendlyName string,
	newFriendlyName string,
) {
	t.Helper()

	certificate, err := testAccGetSSLCertificateByFriendlyName(
		currentFriendlyName,
	)
	if err != nil {
		t.Fatalf(
			"read SSL certificate %q: %v",
			currentFriendlyName,
			err,
		)
	}
	if certificate == nil {
		t.Fatalf(
			"SSL certificate %q was not found",
			currentFriendlyName,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := sslcertificate.NewClient(client).Rename(
		ctx,
		certificate.ID,
		newFriendlyName,
	); err != nil {
		t.Fatalf(
			"rename SSL certificate %q: %v",
			currentFriendlyName,
			err,
		)
	}
}

func testAccDeleteSSLCertificate(
	t *testing.T,
	friendlyName string,
) {
	t.Helper()

	certificate, err := testAccGetSSLCertificateByFriendlyName(friendlyName)
	if err != nil {
		t.Fatalf("read SSL certificate %q: %v", friendlyName, err)
	}
	if certificate == nil {
		return
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	sslClient := sslcertificate.NewClient(client)
	installed, err := sslClient.IsInstalled(ctx, certificate.ID)
	if err != nil {
		t.Fatalf(
			"inspect SSL certificate %q installation: %v",
			friendlyName,
			err,
		)
	}
	if certificate.DomainIsConfigured || installed {
		t.Fatalf(
			"refusing to delete configured or installed SSL certificate %q",
			friendlyName,
		)
	}
	if err := sslClient.Delete(ctx, certificate.ID); err != nil {
		t.Fatalf("delete SSL certificate %q: %v", friendlyName, err)
	}
}

func testAccGetSSLCertificateByFriendlyName(
	friendlyName string,
) (*sslcertificate.Certificate, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		return nil, err
	}
	sslClient := sslcertificate.NewClient(client)
	certificates, err := sslClient.List(ctx)
	if err != nil {
		return nil, err
	}

	var match *sslcertificate.Certificate
	for _, certificate := range certificates {
		if certificate.FriendlyName != friendlyName {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf(
				"multiple SSL certificates use friendly name %q",
				friendlyName,
			)
		}
		current, err := sslClient.Get(ctx, certificate.ID)
		if err != nil {
			return nil, err
		}
		match = current
	}

	return match, nil
}
