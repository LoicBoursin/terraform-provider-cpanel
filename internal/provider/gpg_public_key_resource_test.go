package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/gpg"
)

const testAccGPGUserIDPrefix = "Terraform cPanel acceptance <tfcpanelgpg-"

func TestGPGPublicKeyResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewGPGPublicKeyResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	publicKey, ok := response.Schema.Attributes["public_key"].(resourceschema.StringAttribute)
	if !ok || !publicKey.Required || len(publicKey.PlanModifiers) != 2 {
		t.Fatal(
			"public_key must be required with semantic equality and replacement plan modifiers",
		)
	}
	for _, name := range []string{
		"id",
		"fingerprint",
		"content_sha256",
		"algorithm",
		"user_id",
	} {
		attribute, ok := response.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed string", name)
		}
	}
	hasSecret, ok := response.Schema.Attributes["has_secret_key"].(resourceschema.BoolAttribute)
	if !ok || !hasSecret.Computed {
		t.Fatal("has_secret_key must be a computed bool")
	}
}

func TestGPGPublicKeyDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewGPGPublicKeyDataSource().Schema(
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
	publicKey, ok := response.Schema.Attributes["public_key"].(datasourceschema.StringAttribute)
	if !ok || !publicKey.Computed || publicKey.Sensitive {
		t.Fatal("public_key must be a non-sensitive computed string")
	}
}

func TestApplyGPGPublicKeyPreservesEquivalentArmor(t *testing.T) {
	t.Parallel()

	armored, parsed, _ := testAccGPGPublicKeyMaterial(t, "model")
	configured := "\n" + armored + "\n"
	model := GPGPublicKeyResourceModel{
		PublicKey: types.StringValue(configured),
	}
	diagnostics := applyGPGPublicKeyToResourceModel(
		&model,
		gpg.PublicKey{
			Metadata: gpg.Metadata{
				ID:        parsed.ID,
				Algorithm: "RSA (Encrypt or Sign)",
				Bits:      parsed.Bits,
				Created:   1_700_000_000,
				UserID:    "Terraform cPanel model",
			},
			Armored:       armored,
			Fingerprint:   parsed.Fingerprint,
			ContentSHA256: strings.Repeat("a", 64),
		},
		false,
	)
	if diagnostics.HasError() {
		t.Fatalf(
			"applyGPGPublicKeyToResourceModel() diagnostics: %v",
			diagnostics,
		)
	}
	if got := model.PublicKey.ValueString(); got != configured {
		t.Fatalf("public_key = %q, want configured armor", got)
	}
	if !model.Expires.IsNull() {
		t.Fatalf("expires = %#v, want null", model.Expires)
	}
}

func TestValidateGPGResourceIdentityRejectsFingerprintChange(
	t *testing.T,
) {
	t.Parallel()

	err := validateGPGResourceIdentity(
		GPGPublicKeyResourceModel{
			Fingerprint: types.StringValue(strings.Repeat("A", 40)),
		},
		&gpg.Lookup{
			Key: &gpg.PublicKey{
				Fingerprint: strings.Repeat("B", 40),
			},
		},
	)
	if err == nil {
		t.Fatal("validateGPGResourceIdentity() returned no error")
	}
}

func TestValidateGPGResourceIdentityRejectsPacketStreamChange(
	t *testing.T,
) {
	t.Parallel()

	err := validateGPGResourceIdentity(
		GPGPublicKeyResourceModel{
			ContentSHA256: types.StringValue(strings.Repeat("a", 64)),
		},
		&gpg.Lookup{
			Key: &gpg.PublicKey{
				Metadata:      gpg.Metadata{ID: "0123456789ABCDEF"},
				ContentSHA256: strings.Repeat("b", 64),
			},
		},
	)
	if err == nil {
		t.Fatal("validateGPGResourceIdentity() returned no error")
	}
}

func TestGPGCreateStateFailurePreservesRemoteKey(t *testing.T) {
	t.Parallel()

	key := gpg.PublicKey{
		Metadata:      gpg.Metadata{ID: "0123456789ABCDEF"},
		Fingerprint:   strings.Repeat("A", 40),
		ContentSHA256: strings.Repeat("b", 64),
	}
	resource := gpgPublicKeyResource{}
	var diagnostics gpgTestDiagnostics
	resource.addCreateStateFailureDiagnostic(
		&diagnostics,
		key,
	)
	if len(diagnostics.errors) != 1 ||
		!strings.Contains(diagnostics.errors[0], key.ID) ||
		!strings.Contains(diagnostics.errors[0], "preserved") {
		t.Fatalf("diagnostics errors = %#v", diagnostics.errors)
	}
}

func TestAccGPGPublicKeyResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC is not set")
	}
	if os.Getenv("CPANEL_ALLOW_GPG_KEYPAIR_DELETE") != "1" {
		t.Fatal(
			"CPANEL_ALLOW_GPG_KEYPAIR_DELETE=1 is required for dedicated-account GPG acceptance cleanup",
		)
	}

	const (
		resourceName   = "cpanel_gpg_public_key.test"
		dataSourceName = "data.cpanel_gpg_public_key.test"
	)

	initialArmor, initialParsed, initialUserID :=
		testAccGPGPublicKeyMaterial(t, "initial")
	replacementArmor, replacementParsed, replacementUserID :=
		testAccGPGPublicKeyMaterial(t, "replacement")
	baselineSecretIDs := testAccGPGSecretIDs(t)

	t.Cleanup(func() {
		testAccCleanupGPGPublicKeys(t, baselineSecretIDs)
	})

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireGPGSecretIDs(t, baselineSecretIDs)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGPGPublicKeysPreserved(
			[]*gpg.ParsedPublicKey{
				initialParsed,
				replacementParsed,
			},
			baselineSecretIDs,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGPGPublicKeyDataSourceConfig(initialArmor),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"id",
						initialParsed.ID,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"fingerprint",
						initialParsed.Fingerprint,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"bits",
						"2048",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"user_id",
						initialUserID,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"has_secret_key",
						"false",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"content_sha256",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"created",
					),
					testresource.TestCheckResourceAttrPair(
						dataSourceName,
						"id",
						resourceName,
						"id",
					),
					testresource.TestCheckResourceAttrPair(
						dataSourceName,
						"fingerprint",
						resourceName,
						"fingerprint",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"has_secret_key",
						"false",
					),
					testAccCheckGPGPublicKeyExists(
						initialParsed,
						baselineSecretIDs,
					),
				),
			},
			{
				Config: testAccGPGPublicKeyDataSourceConfig(initialArmor),
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialParsed.ID,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
				ImportStateVerifyIgnore:              []string{"public_key"},
			},
			{
				Config: testAccGPGPublicKeyDataSourceConfig(initialArmor),
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				PreConfig: func() {
					testAccDeleteGPGPublicKey(
						t,
						initialParsed.ID,
						baselineSecretIDs,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccGPGPublicKeyDataSourceConfig(initialArmor),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckGPGPublicKeyExists(
						initialParsed,
						baselineSecretIDs,
					),
				),
			},
			{
				Config: testAccGPGPublicKeyDataSourceConfig(
					replacementArmor,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"id",
						replacementParsed.ID,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"fingerprint",
						replacementParsed.Fingerprint,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"user_id",
						replacementUserID,
					),
					testAccCheckGPGPublicKeyExists(
						replacementParsed,
						baselineSecretIDs,
					),
					testAccCheckGPGPublicKeyExists(
						initialParsed,
						baselineSecretIDs,
					),
				),
			},
		},
	})
}

func testAccGPGPublicKeyMaterial(
	t *testing.T,
	kind string,
) (string, *gpg.ParsedPublicKey, string) {
	t.Helper()

	suffix := strings.ToLower(
		acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum),
	)
	email := fmt.Sprintf(
		"tfcpanelgpg-%s-%s@example.invalid",
		kind,
		suffix,
	)
	userID := fmt.Sprintf("Terraform cPanel acceptance <%s>", email)
	testAccRegisterArtifact(userID)
	entity, err := openpgp.NewEntity(
		"Terraform cPanel acceptance",
		"",
		email,
		&packet.Config{
			RSABits: 2048,
			Time: func() time.Time {
				return time.Now().UTC().Truncate(time.Second)
			},
		},
	)
	if err != nil {
		t.Fatalf("generate GPG entity: %v", err)
	}
	var output bytes.Buffer
	writer, err := armor.Encode(&output, "PGP PUBLIC KEY BLOCK", nil)
	if err != nil {
		t.Fatalf("create GPG armor: %v", err)
	}
	if err := entity.Serialize(writer); err != nil {
		t.Fatalf("serialize GPG public key: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close GPG armor: %v", err)
	}
	armored := strings.TrimSpace(output.String())
	parsed, err := gpg.ParsePublicKey(armored)
	if err != nil {
		t.Fatalf("parse generated GPG public key: %v", err)
	}

	return armored, parsed, userID
}

func testAccGPGPublicKeyResourceConfig(armored string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_gpg_public_key" "test" {
  public_key = %q
}
`, armored)
}

func testAccGPGPublicKeyDataSourceConfig(armored string) string {
	return testAccGPGPublicKeyResourceConfig(armored) + `
data "cpanel_gpg_public_key" "test" {
  id = cpanel_gpg_public_key.test.id
}
`
}

func testAccCheckGPGPublicKeyExists(
	expected *gpg.ParsedPublicKey,
	baselineSecretIDs []string,
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
		lookup, _, err := gpg.NewClient(client).Lookup(ctx, expected.ID)
		if err != nil {
			return err
		}
		if lookup.Key == nil {
			return fmt.Errorf(
				"GPG public key %q was not found",
				expected.ID,
			)
		}
		if lookup.HasSecretKey {
			return fmt.Errorf(
				"GPG public key %q unexpectedly has secret material",
				expected.ID,
			)
		}
		if lookup.Key.Fingerprint != expected.Fingerprint {
			return fmt.Errorf(
				"GPG public key %q fingerprint is %q; expected %q",
				expected.ID,
				lookup.Key.Fingerprint,
				expected.Fingerprint,
			)
		}
		return compareGPGSecretIDs(ctx, client, baselineSecretIDs)
	}
}

func testAccDeleteGPGPublicKey(
	t *testing.T,
	id string,
	baselineSecretIDs []string,
) {
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
	gpgClient := gpg.NewClient(client)
	lookup, _, err := gpgClient.Lookup(ctx, id)
	if err != nil {
		t.Fatalf("read GPG public key %q: %v", id, err)
	}
	if lookup.Key == nil {
		return
	}
	if lookup.HasSecretKey {
		t.Fatalf(
			"refusing to delete GPG public key %q with secret material",
			id,
		)
	}
	if _, err := gpgClient.DeleteKeyPairGuarded(ctx, gpg.Ownership{
		ID:            lookup.Key.ID,
		Fingerprint:   lookup.Key.Fingerprint,
		ContentSHA256: lookup.Key.ContentSHA256,
	}); err != nil {
		t.Fatalf("delete GPG public key %q: %v", id, err)
	}
	if err := compareGPGSecretIDs(
		ctx,
		client,
		baselineSecretIDs,
	); err != nil {
		t.Fatal(err)
	}
}

func testAccCheckGPGPublicKeysPreserved(
	expected []*gpg.ParsedPublicKey,
	baselineSecretIDs []string,
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
		gpgClient := gpg.NewClient(client)
		for _, expectedKey := range expected {
			lookup, _, err := gpgClient.Lookup(ctx, expectedKey.ID)
			if err != nil {
				return err
			}
			if lookup.Key == nil {
				return fmt.Errorf(
					"preserved GPG public key %q is missing after Terraform destroy",
					expectedKey.ID,
				)
			}
			if lookup.HasSecretKey {
				return fmt.Errorf(
					"preserved GPG public key %q unexpectedly has secret material",
					expectedKey.ID,
				)
			}
			if lookup.Key.ContentSHA256 != expectedKey.ContentSHA256 {
				return fmt.Errorf(
					"preserved GPG public key %q packet SHA-256 is %q; expected %q",
					expectedKey.ID,
					lookup.Key.ContentSHA256,
					expectedKey.ContentSHA256,
				)
			}
		}

		return compareGPGSecretIDs(ctx, client, baselineSecretIDs)
	}
}

func testAccCleanupGPGPublicKeys(
	t *testing.T,
	baselineSecretIDs []string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Errorf("create cPanel client for GPG cleanup: %v", err)
		return
	}
	gpgClient := gpg.NewClient(client)
	public, _, err := gpgClient.ListPublic(ctx)
	if err != nil {
		t.Errorf("list GPG public keys for cleanup: %v", err)
		return
	}
	for _, metadata := range public {
		if !strings.HasPrefix(metadata.UserID, testAccGPGUserIDPrefix) {
			continue
		}
		lookup, _, err := gpgClient.Lookup(ctx, metadata.ID)
		if err != nil {
			t.Errorf("read test GPG public key %q: %v", metadata.ID, err)
			continue
		}
		if lookup.Key == nil {
			continue
		}
		if lookup.HasSecretKey {
			t.Errorf(
				"refusing cleanup of GPG public key %q with secret material",
				metadata.ID,
			)
			continue
		}
		if _, err := gpgClient.DeleteKeyPairGuarded(ctx, gpg.Ownership{
			ID:            lookup.Key.ID,
			Fingerprint:   lookup.Key.Fingerprint,
			ContentSHA256: lookup.Key.ContentSHA256,
		}); err != nil {
			t.Errorf(
				"delete test GPG public key %q: %v",
				metadata.ID,
				err,
			)
		}
	}
	if err := compareGPGSecretIDs(
		ctx,
		client,
		baselineSecretIDs,
	); err != nil {
		t.Error(err)
	}
}

func testAccGPGSecretIDs(t *testing.T) []string {
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
	secret, _, err := gpg.NewClient(client).ListSecret(ctx)
	if err != nil {
		t.Fatalf("list GPG secret keys: %v", err)
	}
	ids := make([]string, 0, len(secret))
	for _, key := range secret {
		ids = append(ids, key.ID)
	}

	return ids
}

func testAccRequireGPGSecretIDs(t *testing.T, expected []string) {
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
	if err := compareGPGSecretIDs(ctx, client, expected); err != nil {
		t.Fatal(err)
	}
}

func compareGPGSecretIDs(
	ctx context.Context,
	client *cpanel.Client,
	expected []string,
) error {
	secret, _, err := gpg.NewClient(client).ListSecret(ctx)
	if err != nil {
		return err
	}
	actual := make([]string, 0, len(secret))
	for _, key := range secret {
		actual = append(actual, key.ID)
	}
	if strings.Join(actual, ",") != strings.Join(expected, ",") {
		return fmt.Errorf(
			"GPG secret key inventory changed: got %v, expected %v",
			actual,
			expected,
		)
	}

	return nil
}

type gpgTestDiagnostics struct {
	errors   []string
	warnings []string
}

func (d *gpgTestDiagnostics) AddError(summary, detail string) {
	d.errors = append(d.errors, summary+": "+detail)
}

func (d *gpgTestDiagnostics) AddWarning(summary, detail string) {
	d.warnings = append(d.warnings, summary+": "+detail)
}
