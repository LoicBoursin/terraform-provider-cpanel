package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

func TestSSLCSRResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewSSLCSRResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	keyID, ok := response.Schema.Attributes["key_id"].(resourceschema.StringAttribute)
	if !ok || !keyID.Optional || !keyID.Computed ||
		len(keyID.PlanModifiers) != 2 {
		t.Fatal(
			"key_id must be optional and computed with state preservation and replacement",
		)
	}
	friendlyName, ok := response.Schema.Attributes["friendly_name"].(resourceschema.StringAttribute)
	if !ok || !friendlyName.Required || len(friendlyName.PlanModifiers) != 0 {
		t.Fatal("friendly_name must be a mutable required string")
	}
	domains, ok := response.Schema.Attributes["domains"].(resourceschema.ListAttribute)
	if !ok || !domains.Required || len(domains.PlanModifiers) != 1 {
		t.Fatal("domains must be a required replacement list")
	}
	for _, name := range []string{
		"country_name",
		"state_or_province_name",
		"locality_name",
		"organization_name",
	} {
		attribute, ok := response.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || len(attribute.PlanModifiers) != 1 {
			t.Fatalf("%s must be a required replacement string", name)
		}
	}
	csr, ok := response.Schema.Attributes["csr"].(resourceschema.StringAttribute)
	if !ok || !csr.Computed || csr.Sensitive {
		t.Fatal("csr must be a public computed string")
	}
}

func TestSSLCSRDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewSSLCSRDataSource().Schema(
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
	csr, ok := response.Schema.Attributes["csr"].(datasourceschema.StringAttribute)
	if !ok || !csr.Computed || csr.Sensitive {
		t.Fatal("csr must be a public computed string")
	}
}

func TestApplySSLCSRToResourceModelPreservesConfiguredDomainOrder(
	t *testing.T,
) {
	t.Parallel()

	configured := []string{
		"example.test",
		"www.example.test",
		"api.example.test",
	}
	domainValue, diagnostics := types.ListValueFrom(
		t.Context(),
		types.StringType,
		configured,
	)
	if diagnostics.HasError() {
		t.Fatalf("ListValueFrom() diagnostics: %v", diagnostics)
	}
	model := SSLCSRResourceModel{
		KeyID:   types.StringValue("key-id"),
		Domains: domainValue,
	}
	csr := testSSLCSR()
	csr.Domains = []string{
		"api.example.test",
		"example.test",
		"www.example.test",
	}

	diagnostics = applySSLCSRToResourceModel(
		t.Context(),
		&model,
		csr,
	)
	if diagnostics.HasError() {
		t.Fatalf(
			"applySSLCSRToResourceModel() diagnostics: %v",
			diagnostics,
		)
	}

	var actual []string
	diagnostics = model.Domains.ElementsAs(
		t.Context(),
		&actual,
		false,
	)
	if diagnostics.HasError() {
		t.Fatalf("ElementsAs() diagnostics: %v", diagnostics)
	}
	if strings.Join(actual, ",") != strings.Join(configured, ",") {
		t.Fatalf("domains = %v, want %v", actual, configured)
	}
	if model.KeyID.ValueString() != "key-id" {
		t.Fatalf("key_id = %q, want key-id", model.KeyID.ValueString())
	}
}

func TestPreserveSSLCSRKeyIDUsesAuthoritativeState(t *testing.T) {
	t.Parallel()

	for name, stateKeyID := range map[string]types.String{
		"created resource":  types.StringValue("created-key-id"),
		"imported resource": types.StringNull(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan := SSLCSRResourceModel{
				KeyID: types.StringValue("planned-key-id"),
			}
			preserveSSLCSRKeyID(
				&plan,
				SSLCSRResourceModel{KeyID: stateKeyID},
			)

			if !plan.KeyID.Equal(stateKeyID) {
				t.Fatalf(
					"key_id = %#v, want state value %#v",
					plan.KeyID,
					stateKeyID,
				)
			}
		})
	}
}

func TestValidateSSLCSRResourceRepresentationRejectsSparseSubjects(
	t *testing.T,
) {
	t.Parallel()

	if err := validateSSLCSRResourceRepresentation(
		testSSLCSR(),
	); err != nil {
		t.Fatalf(
			"validateSSLCSRResourceRepresentation() error: %v",
			err,
		)
	}

	testCases := map[string]func(*sslcsr.CSR){
		"country": func(csr *sslcsr.CSR) {
			csr.CountryName = ""
		},
		"state or province": func(csr *sslcsr.CSR) {
			csr.StateOrProvinceName = ""
		},
		"locality": func(csr *sslcsr.CSR) {
			csr.LocalityName = ""
		},
		"organization": func(csr *sslcsr.CSR) {
			csr.OrganizationName = ""
		},
	}
	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			csr := testSSLCSR()
			mutate(&csr)
			if err := validateSSLCSRResourceRepresentation(
				csr,
			); err == nil {
				t.Fatal(
					"validateSSLCSRResourceRepresentation() returned no error",
				)
			}
		})
	}
}

func TestSSLCSRCreateDoesNotDeleteWhenStateCannotBeSaved(
	t *testing.T,
) {
	t.Parallel()

	ctx := t.Context()
	schemaResponse := &frameworkresource.SchemaResponse{}
	NewSSLCSRResource().Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf(
			"Schema() diagnostics: %v",
			schemaResponse.Diagnostics,
		)
	}

	definition := testSSLCSRDefinition()
	domains, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		definition.Domains,
	)
	if diagnostics.HasError() {
		t.Fatalf("ListValueFrom() diagnostics: %v", diagnostics)
	}
	planModel := SSLCSRResourceModel{
		ID:                     types.StringUnknown(),
		KeyID:                  types.StringValue(definition.KeyID),
		FriendlyName:           types.StringValue(definition.FriendlyName),
		Domains:                domains,
		CountryName:            types.StringValue(definition.CountryName),
		StateOrProvinceName:    types.StringValue(definition.StateOrProvinceName),
		LocalityName:           types.StringValue(definition.LocalityName),
		OrganizationName:       types.StringValue(definition.OrganizationName),
		OrganizationalUnitName: types.StringValue(definition.OrganizationalUnitName),
		EmailAddress:           types.StringValue(definition.EmailAddress),
		CSR:                    types.StringUnknown(),
		FingerprintSHA256:      types.StringUnknown(),
		CommonName:             types.StringUnknown(),
		Created:                types.Int64Unknown(),
		KeyAlgorithm:           types.StringUnknown(),
		Modulus:                types.StringUnknown(),
		ECDSACurveName:         types.StringUnknown(),
		ECDSAPublic:            types.StringUnknown(),
	}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics = plan.Set(ctx, &planModel)
	if diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}

	generated := testSSLCSR()
	client := &fakeSSLCSRClient{generated: &generated}
	resource := &sslCSRResource{client: client}
	request := frameworkresource.CreateRequest{Plan: plan}
	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{
			Schema: resourceschema.Schema{
				Attributes: map[string]resourceschema.Attribute{},
			},
		},
	}

	resource.Create(ctx, request, response)

	if !response.Diagnostics.HasError() {
		t.Fatal("Create() returned no state persistence error")
	}
	if len(client.deleted) != 0 {
		t.Fatalf(
			"Delete() call count = %d, want 0",
			len(client.deleted),
		)
	}
	hasImportRecovery := false
	for _, diagnostic := range response.Diagnostics.Errors() {
		if strings.Contains(
			diagnostic.Detail(),
			"import this CSR by ID",
		) {
			hasImportRecovery = true
			break
		}
	}
	if !hasImportRecovery {
		t.Fatalf(
			"Create() diagnostics do not provide import recovery: %v",
			response.Diagnostics,
		)
	}
}

func TestSSLCSRRestoreFriendlyNamePreservesConcurrentRename(t *testing.T) {
	t.Parallel()

	current := testSSLCSR()
	current.FriendlyName = "Concurrent CSR"
	client := &fakeSSLCSRClient{current: &current}
	resource := &sslCSRResource{client: client}

	err := resource.restoreFriendlyName(
		t.Context(),
		current.Identity(),
		"Terraform renamed CSR",
		"Original CSR",
	)
	if err == nil {
		t.Fatal("restoreFriendlyName() returned no error")
	}
	if len(client.renames) != 0 {
		t.Fatalf("rename count = %d, want 0", len(client.renames))
	}
	if client.current.FriendlyName != "Concurrent CSR" {
		t.Fatalf(
			"friendly name = %q, want Concurrent CSR",
			client.current.FriendlyName,
		)
	}
}

func TestSSLCSRDeleteRequiresExactStateAndReconcilesResponse(t *testing.T) {
	t.Parallel()

	expected := testSSLCSR()
	replacement := expected
	replacement.FingerprintSHA256 = strings.Repeat("b", 64)
	replacement.FriendlyName = "Concurrent CSR"

	testCases := []struct {
		name        string
		current     *sslcsr.CSR
		deleteErr   error
		deleteApply bool
		replacement *sslcsr.CSR
		wantCalls   int
		wantError   bool
		wantWarning bool
	}{
		{
			name: "friendly name drift",
			current: func() *sslcsr.CSR {
				value := replacement
				value.FingerprintSHA256 = expected.FingerprintSHA256
				return &value
			}(),
			wantError: true,
		},
		{
			name:        "ambiguous applied deletion",
			current:     &expected,
			deleteErr:   errors.New("connection closed"),
			deleteApply: true,
			wantCalls:   1,
		},
		{
			name:      "ambiguous unchanged deletion",
			current:   &expected,
			deleteErr: errors.New("connection closed"),
			wantCalls: 1,
			wantError: true,
		},
		{
			name:        "concurrent replacement after deletion",
			current:     &expected,
			deleteErr:   errors.New("connection closed"),
			deleteApply: true,
			replacement: &replacement,
			wantCalls:   1,
			wantWarning: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeSSLCSRClient{
				current:           testCase.current,
				deleteErr:         testCase.deleteErr,
				deleteApplied:     testCase.deleteApply,
				deleteReplacement: testCase.replacement,
			}
			resource := &sslCSRResource{client: client}
			response := runSSLCSRDelete(t, resource, expected)

			if response.Diagnostics.HasError() != testCase.wantError {
				t.Fatalf(
					"Delete() error = %t, want %t: %v",
					response.Diagnostics.HasError(),
					testCase.wantError,
					response.Diagnostics,
				)
			}
			if (len(response.Diagnostics.Warnings()) > 0) !=
				testCase.wantWarning {
				t.Fatalf(
					"Delete() warnings = %v, wantWarning %t",
					response.Diagnostics.Warnings(),
					testCase.wantWarning,
				)
			}
			if len(client.deleted) != testCase.wantCalls {
				t.Fatalf(
					"Delete() call count = %d, want %d",
					len(client.deleted),
					testCase.wantCalls,
				)
			}
		})
	}
}

func TestSSLCSRMutationErrorClassification(t *testing.T) {
	t.Parallel()

	apiError := &cpanelapi.APIError{
		API:      "UAPI",
		Module:   "SSL",
		Function: "generate_csr",
		Messages: []string{"rejected"},
	}
	if !sslCSRMutationErrorIsDeterministic(apiError) {
		t.Fatal("API error must be deterministic")
	}
	if sslCSRMutationErrorIsDeterministic(
		errors.New(apiError.Error()),
	) {
		t.Fatal("untyped transport-style error must remain ambiguous")
	}
}

type fakeSSLCSRClient struct {
	current           *sslcsr.CSR
	generated         *sslcsr.CSR
	deleteErr         error
	deleteApplied     bool
	deleteReplacement *sslcsr.CSR
	deleted           []sslcsr.Identity
	renames           [][3]string
}

func (c *fakeSSLCSRClient) Get(
	_ context.Context,
	_ string,
) (*sslcsr.CSR, error) {
	if c.current == nil {
		return nil, nil
	}
	result := *c.current

	return &result, nil
}

func (c *fakeSSLCSRClient) Generate(
	context.Context,
	sslcsr.Definition,
) (*sslcsr.CSR, error) {
	if c.generated != nil {
		result := *c.generated

		return &result, nil
	}

	return nil, errors.New("not implemented")
}

func (c *fakeSSLCSRClient) Rename(
	_ context.Context,
	identity sslcsr.Identity,
	expected string,
	desired string,
) (*sslcsr.CSR, error) {
	c.renames = append(
		c.renames,
		[3]string{identity.ID, expected, desired},
	)
	if c.current == nil {
		return nil, errors.New("missing CSR")
	}
	c.current.FriendlyName = desired
	result := *c.current

	return &result, nil
}

func (c *fakeSSLCSRClient) Delete(
	_ context.Context,
	identity sslcsr.Identity,
) error {
	c.deleted = append(c.deleted, identity)
	if c.deleteApplied {
		c.current = c.deleteReplacement
	}

	return c.deleteErr
}

func runSSLCSRDelete(
	t *testing.T,
	resource *sslCSRResource,
	csr sslcsr.CSR,
) *frameworkresource.DeleteResponse {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewSSLCSRResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	model := SSLCSRResourceModel{KeyID: types.StringNull()}
	if diagnostics := applySSLCSRToResourceModel(
		t.Context(),
		&model,
		csr,
	); diagnostics.HasError() {
		t.Fatalf("applySSLCSRToResourceModel() diagnostics: %v", diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diagnostics := state.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.DeleteResponse{}
	resource.Delete(
		t.Context(),
		frameworkresource.DeleteRequest{State: state},
		response,
	)

	return response
}

func testSSLCSRDefinition() sslcsr.Definition {
	return sslcsr.Definition{
		KeyID:                  "key-id",
		FriendlyName:           "Terraform CSR",
		Domains:                []string{"example.test", "www.example.test"},
		CountryName:            "FR",
		StateOrProvinceName:    "Ile-de-France",
		LocalityName:           "Paris",
		OrganizationName:       "Terraform Provider cPanel",
		OrganizationalUnitName: "Acceptance",
		EmailAddress:           "csr@example.test",
	}
}

func testSSLCSR() sslcsr.CSR {
	definition := testSSLCSRDefinition()

	return sslcsr.CSR{
		ID:                     "csr-id",
		FriendlyName:           definition.FriendlyName,
		CSRPEM:                 "-----BEGIN CERTIFICATE REQUEST-----\nTEST\n-----END CERTIFICATE REQUEST-----",
		FingerprintSHA256:      strings.Repeat("a", 64),
		CommonName:             definition.Domains[0],
		CountryName:            definition.CountryName,
		StateOrProvinceName:    definition.StateOrProvinceName,
		LocalityName:           definition.LocalityName,
		OrganizationName:       definition.OrganizationName,
		OrganizationalUnitName: definition.OrganizationalUnitName,
		EmailAddress:           definition.EmailAddress,
		Created:                1700000000,
		Domains:                append([]string(nil), definition.Domains...),
		KeyAlgorithm:           "rsaEncryption",
		Modulus:                "00abcd",
	}
}
