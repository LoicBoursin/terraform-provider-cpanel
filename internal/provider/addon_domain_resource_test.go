package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

func TestAddonDomainResourceModelPreservesManagedIdentity(t *testing.T) {
	t.Parallel()

	state := AddonDomainResourceModel{
		Domain:            types.StringValue("addon.example.test"),
		InternalSubdomain: types.StringValue("addon"),
		RootDomain:        types.StringValue("example.test"),
		FullSubdomain:     types.StringValue("addon.example.test"),
		DomainKey:         types.StringValue("addon_example_test"),
		DocumentRoot:      types.StringValue("public_html/addon"),
	}
	current := addonDomainFromResourceModel(state)
	if !addonDomainsEqual(current, addonDomainFromResourceModel(state)) {
		t.Fatal("matching addon domain state was not recognized")
	}

	changed := *current
	changed.BaseDirectory = "public_html/concurrent"
	if addonDomainsEqual(&changed, addonDomainFromResourceModel(state)) {
		t.Fatal("changed addon domain document root was accepted")
	}
}

func TestAddonDomainCreateRollbackPreservesDocumentRoot(t *testing.T) {
	t.Parallel()

	expected := cpaneldomain.AddonDomain{
		Domain:            "addon.example.test",
		InternalSubdomain: "addon",
		RootDomain:        "example.test",
		FullSubdomain:     "addon.example.test",
		DomainKey:         "addon_example.test",
		BaseDirectory:     "public_html/addon",
	}
	current := &expected
	server := newAddonDomainDeletionTestServer(t, &current, 0, nil)
	defer server.Close()

	subject := addonDomainDeletionTestResource(t, server.URL)
	err := subject.rollbackAddonDomainCreate(
		t.Context(),
		&AddonDomainResourceModel{
			Domain:            types.StringValue(expected.Domain),
			InternalSubdomain: types.StringValue(expected.InternalSubdomain),
			DocumentRoot:      types.StringValue(expected.BaseDirectory),
		},
	)
	if err != nil {
		t.Fatalf("rollbackAddonDomainCreate() error = %v", err)
	}
	if current != nil {
		t.Fatalf("current = %#v, want nil", current)
	}
}

func TestAddonDomainCreateReconcilesAmbiguousAppliedResponse(t *testing.T) {
	t.Parallel()

	expected := cpaneldomain.AddonDomain{
		Domain:            "addon.example.test",
		InternalSubdomain: "addon",
		RootDomain:        "example.test",
		FullSubdomain:     "addon.example.test",
		DomainKey:         "addon_example_test",
		BaseDirectory:     "public_html/addon",
	}
	var current *cpaneldomain.AddonDomain
	listCalls := 0
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/json-api/cpanel" {
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		switch request.Form.Get("cpanel_jsonapi_func") {
		case "listaddondomains":
			listCalls++
			data := []cpaneldomain.AddonDomain{}
			if current != nil {
				data = append(data, *current)
			}
			writeDomainResourceTestResponse(t, response, data)
		case "addaddondomain":
			createCalls++
			current = &expected
			response.WriteHeader(http.StatusGatewayTimeout)
		default:
			t.Fatalf(
				"unexpected API 2 function %q",
				request.Form.Get("cpanel_jsonapi_func"),
			)
		}
	}))
	defer server.Close()

	subject := addonDomainDeletionTestResource(t, server.URL)
	schema := addonDomainResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schema}
	model := AddonDomainResourceModel{
		Domain:             types.StringValue(expected.Domain),
		InternalSubdomain:  types.StringValue(expected.InternalSubdomain),
		RootDomain:         types.StringUnknown(),
		FullSubdomain:      types.StringUnknown(),
		DomainKey:          types.StringUnknown(),
		DocumentRoot:       types.StringValue(expected.BaseDirectory),
		DeleteDocumentRoot: types.BoolValue(false),
	}
	if diagnostics := plan.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schema},
	}
	subject.Create(
		t.Context(),
		frameworkresource.CreateRequest{Plan: plan},
		response,
	)

	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics: %v", response.Diagnostics)
	}
	if createCalls != 1 || listCalls != 2 {
		t.Fatalf(
			"API calls = create:%d list:%d, want 1 and 2",
			createCalls,
			listCalls,
		)
	}
	var state AddonDomainResourceModel
	response.Diagnostics.Append(response.State.Get(t.Context(), &state)...)
	if response.Diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", response.Diagnostics)
	}
	if state.DomainKey.ValueString() != expected.DomainKey ||
		state.DocumentRoot.ValueString() != expected.BaseDirectory {
		t.Fatalf("created state = %#v", state)
	}
}

func TestAddonDomainDeleteReconcilesRemoteAbsence(t *testing.T) {
	t.Parallel()

	expected := cpaneldomain.AddonDomain{
		Domain:            "addon.example.test",
		InternalSubdomain: "addon",
		RootDomain:        "example.test",
		FullSubdomain:     "addon.example.test",
		DomainKey:         "addon_example.test",
		BaseDirectory:     "public_html/addon",
	}
	replacement := expected
	replacement.BaseDirectory = "public_html/replacement"

	testCases := []struct {
		name             string
		deleteStatusCode int
		afterDelete      *cpaneldomain.AddonDomain
		wantError        bool
	}{
		{
			name:        "successful deletion is confirmed",
			afterDelete: nil,
		},
		{
			name:             "ambiguous error is accepted after confirmed absence",
			deleteStatusCode: http.StatusInternalServerError,
			afterDelete:      nil,
		},
		{
			name:        "false success is rejected",
			afterDelete: &expected,
			wantError:   true,
		},
		{
			name:        "concurrent replacement is preserved",
			afterDelete: &replacement,
			wantError:   true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := &expected
			server := newAddonDomainDeletionTestServer(
				t,
				&current,
				testCase.deleteStatusCode,
				testCase.afterDelete,
			)
			defer server.Close()

			subject := addonDomainDeletionTestResource(t, server.URL)
			err := subject.deleteAddonDomainAndVerify(t.Context(), &expected)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"deleteAddonDomainAndVerify() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
		})
	}
}

func TestAccAddonDomainResource(t *testing.T) {
	const resourceName = "cpanel_addon_domain.test"

	firstDomain, firstInternalSubdomain := testAccAddonDomain(t, "resource")
	secondDomain, secondInternalSubdomain := testAccAddonDomain(t, "replacement")
	firstDocumentRoot := testAccDomainDocumentRoot("addon-resource")
	updatedDocumentRoot := testAccRegisterArtifact(firstDocumentRoot + "-updated")
	secondDocumentRoot := testAccDomainDocumentRoot("addon-replacement")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckAddonDomainsDestroyed(
			firstDomain,
			secondDomain,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccAddonDomainResourceConfig(
					firstDomain,
					firstInternalSubdomain,
					firstDocumentRoot,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", firstDomain),
					resource.TestCheckResourceAttr(
						resourceName,
						"internal_subdomain",
						firstInternalSubdomain,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"document_root",
						firstDocumentRoot,
					),
					resource.TestCheckResourceAttrSet(resourceName, "root_domain"),
					resource.TestCheckResourceAttrSet(resourceName, "full_subdomain"),
					resource.TestCheckResourceAttrSet(resourceName, "domain_key"),
					resource.TestCheckResourceAttr(resourceName, "delete_document_root", "false"),
					testAccCheckAddonDomainExists(
						firstDomain,
						firstInternalSubdomain,
						firstDocumentRoot,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        firstDomain,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
				ImportStateVerifyIgnore:              []string{"delete_document_root"},
			},
			{
				Config: testAccAddonDomainResourceConfig(
					firstDomain,
					firstInternalSubdomain,
					updatedDocumentRoot,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"document_root",
						updatedDocumentRoot,
					),
					testAccCheckAddonDomainExists(
						firstDomain,
						firstInternalSubdomain,
						updatedDocumentRoot,
					),
				),
			},
			{
				Config: testAccAddonDomainResourceConfig(
					secondDomain,
					secondInternalSubdomain,
					secondDocumentRoot,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", secondDomain),
					testAccCheckAddonDomainExists(
						secondDomain,
						secondInternalSubdomain,
						secondDocumentRoot,
					),
					testAccCheckAddonDomainsDestroyed(firstDomain),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteAddonDomain(t, secondDomain)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccAddonDomainResourceConfig(
					secondDomain,
					secondInternalSubdomain,
					secondDocumentRoot,
				),
				Check: testAccCheckAddonDomainExists(
					secondDomain,
					secondInternalSubdomain,
					secondDocumentRoot,
				),
			},
		},
	})
}

func addonDomainDeletionTestResource(
	t *testing.T,
	host string,
) addonDomainResource {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return addonDomainResource{client: cpaneldomain.NewClient(baseClient)}
}

func addonDomainResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()

	response := &frameworkresource.SchemaResponse{}
	NewAddonDomainResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	return response.Schema
}

func newAddonDomainDeletionTestServer(
	t *testing.T,
	current **cpaneldomain.AddonDomain,
	deleteStatusCode int,
	afterDelete *cpaneldomain.AddonDomain,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != "/json-api/cpanel" {
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}

		switch request.Form.Get("cpanel_jsonapi_func") {
		case "deladdondomain":
			*current = afterDelete
			if deleteStatusCode != 0 {
				response.WriteHeader(deleteStatusCode)
				return
			}
			_, _ = response.Write([]byte(
				`{"cpanelresult":{"event":{"result":1},"data":[]}}`,
			))
		case "listaddondomains":
			data := []cpaneldomain.AddonDomain{}
			if *current != nil {
				data = append(data, **current)
			}
			writeDomainResourceTestResponse(t, response, data)
		default:
			t.Fatalf(
				"unexpected API 2 function %q",
				request.Form.Get("cpanel_jsonapi_func"),
			)
		}
	}))
}

func testAccAddonDomainResourceConfig(
	domain string,
	internalSubdomain string,
	documentRoot string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_addon_domain" "test" {
  domain               = %q
  internal_subdomain   = %q
  document_root        = %q
  delete_document_root = false
}
`, domain, internalSubdomain, documentRoot)
}
