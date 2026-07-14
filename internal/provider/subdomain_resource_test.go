package provider

import (
	"encoding/json"
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

func TestSubdomainResourceModelPreservesManagedIdentity(t *testing.T) {
	t.Parallel()

	state := SubdomainResourceModel{
		Domain:       types.StringValue("sub.example.test"),
		Subdomain:    types.StringValue("sub"),
		RootDomain:   types.StringValue("example.test"),
		DocumentRoot: types.StringValue("public_html/sub"),
	}
	current := subdomainFromResourceModel(state)
	if !subdomainsEqual(current, subdomainFromResourceModel(state)) {
		t.Fatal("matching subdomain state was not recognized")
	}

	changed := *current
	changed.BaseDirectory = "public_html/concurrent"
	if subdomainsEqual(&changed, subdomainFromResourceModel(state)) {
		t.Fatal("changed subdomain document root was accepted")
	}
}

func TestSubdomainCreateRollbackPreservesDocumentRoot(t *testing.T) {
	t.Parallel()

	expected := cpaneldomain.Subdomain{
		Domain:        "sub.example.test",
		Subdomain:     "sub",
		RootDomain:    "example.test",
		BaseDirectory: "public_html/sub",
	}
	current := &expected
	server := newSubdomainDeletionTestServer(t, &current, 0, nil)
	defer server.Close()

	subject := subdomainDeletionTestResource(t, server.URL)
	err := subject.rollbackSubdomainCreate(
		t.Context(),
		expected.Domain,
		expected.BaseDirectory,
	)
	if err != nil {
		t.Fatalf("rollbackSubdomainCreate() error = %v", err)
	}
	if current != nil {
		t.Fatalf("current = %#v, want nil", current)
	}
}

func TestSubdomainCreateReconcilesAmbiguousAppliedResponse(t *testing.T) {
	t.Parallel()

	expected := cpaneldomain.Subdomain{
		Domain:        "sub.example.test",
		Subdomain:     "sub",
		RootDomain:    "example.test",
		DomainKey:     "sub_example_test",
		BaseDirectory: "public_html/sub",
	}
	var current *cpaneldomain.Subdomain
	listCalls := 0
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path == "/execute/DomainInfo/list_domains" {
			if err := json.NewEncoder(response).Encode(map[string]any{
				"status": 1,
				"data": map[string]any{
					"main_domain":    "example.test",
					"addon_domains":  []string{},
					"sub_domains":    []string{},
					"parked_domains": []string{},
				},
			}); err != nil {
				t.Fatalf("encode domain inventory response: %v", err)
			}
			return
		}
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
		case "listsubdomains":
			listCalls++
			data := []cpaneldomain.Subdomain{}
			if current != nil {
				data = append(data, *current)
			}
			writeDomainResourceTestResponse(t, response, data)
		case "addsubdomain":
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

	subject := subdomainDeletionTestResource(t, server.URL)
	schema := subdomainResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schema}
	model := SubdomainResourceModel{
		Domain:             types.StringValue(expected.Domain),
		Subdomain:          types.StringUnknown(),
		RootDomain:         types.StringUnknown(),
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
	var state SubdomainResourceModel
	response.Diagnostics.Append(response.State.Get(t.Context(), &state)...)
	if response.Diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", response.Diagnostics)
	}
	if state.Subdomain.ValueString() != expected.Subdomain ||
		state.RootDomain.ValueString() != expected.RootDomain ||
		state.DocumentRoot.ValueString() != expected.BaseDirectory {
		t.Fatalf("created state = %#v", state)
	}
}

func TestSubdomainDeleteReconcilesRemoteAbsence(t *testing.T) {
	t.Parallel()

	expected := cpaneldomain.Subdomain{
		Domain:        "sub.example.test",
		Subdomain:     "sub",
		RootDomain:    "example.test",
		BaseDirectory: "public_html/sub",
	}
	replacement := expected
	replacement.BaseDirectory = "public_html/replacement"

	testCases := []struct {
		name             string
		deleteStatusCode int
		afterDelete      *cpaneldomain.Subdomain
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
			server := newSubdomainDeletionTestServer(
				t,
				&current,
				testCase.deleteStatusCode,
				testCase.afterDelete,
			)
			defer server.Close()

			subject := subdomainDeletionTestResource(t, server.URL)
			err := subject.deleteSubdomainAndVerify(t.Context(), &expected)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"deleteSubdomainAndVerify() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
		})
	}
}

func TestAccSubdomainResource(t *testing.T) {
	const resourceName = "cpanel_subdomain.test"

	firstDomain := testAccSubdomain(t, "resource")
	secondDomain := testAccSubdomain(t, "replacement")
	firstDocumentRoot := testAccDomainDocumentRoot("sub-resource")
	updatedDocumentRoot := testAccRegisterArtifact(firstDocumentRoot + "-updated")
	secondDocumentRoot := testAccDomainDocumentRoot("sub-replacement")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSubdomainsDestroyed(
			firstDomain,
			secondDomain,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccSubdomainResourceConfig(firstDomain, firstDocumentRoot),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", firstDomain),
					resource.TestCheckResourceAttr(
						resourceName,
						"document_root",
						firstDocumentRoot,
					),
					resource.TestCheckResourceAttrSet(resourceName, "subdomain"),
					resource.TestCheckResourceAttrSet(resourceName, "root_domain"),
					resource.TestCheckResourceAttr(resourceName, "delete_document_root", "false"),
					testAccCheckSubdomainExists(firstDomain, firstDocumentRoot),
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
				Config: testAccSubdomainResourceConfig(firstDomain, updatedDocumentRoot),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"document_root",
						updatedDocumentRoot,
					),
					testAccCheckSubdomainExists(firstDomain, updatedDocumentRoot),
				),
			},
			{
				Config: testAccSubdomainResourceConfig(secondDomain, secondDocumentRoot),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", secondDomain),
					testAccCheckSubdomainExists(secondDomain, secondDocumentRoot),
					testAccCheckSubdomainsDestroyed(firstDomain),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteSubdomain(t, secondDomain)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSubdomainResourceConfig(secondDomain, secondDocumentRoot),
				Check:  testAccCheckSubdomainExists(secondDomain, secondDocumentRoot),
			},
		},
	})
}

func subdomainDeletionTestResource(
	t *testing.T,
	host string,
) subdomainResource {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return subdomainResource{client: cpaneldomain.NewClient(baseClient)}
}

func subdomainResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()

	response := &frameworkresource.SchemaResponse{}
	NewSubdomainResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	return response.Schema
}

func newSubdomainDeletionTestServer(
	t *testing.T,
	current **cpaneldomain.Subdomain,
	deleteStatusCode int,
	afterDelete *cpaneldomain.Subdomain,
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
		case "delsubdomain":
			*current = afterDelete
			if deleteStatusCode != 0 {
				response.WriteHeader(deleteStatusCode)
				return
			}
			_, _ = response.Write([]byte(
				`{"cpanelresult":{"event":{"result":1},"data":[]}}`,
			))
		case "listsubdomains":
			data := []cpaneldomain.Subdomain{}
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

func writeDomainResourceTestResponse(
	t *testing.T,
	response http.ResponseWriter,
	data any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(map[string]any{
		"cpanelresult": map[string]any{
			"event": map[string]any{"result": 1},
			"data":  data,
		},
	}); err != nil {
		t.Fatalf("encode domain resource test response: %v", err)
	}
}

func testAccSubdomainResourceConfig(domain, documentRoot string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_subdomain" "test" {
  domain               = %q
  document_root        = %q
  delete_document_root = false
}
`, domain, documentRoot)
}
