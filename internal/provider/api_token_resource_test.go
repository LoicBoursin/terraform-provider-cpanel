package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/apitoken"
)

func TestAPITokenResourceSchemaProtectsSecret(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewAPITokenResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	tokenAttribute, ok := response.Schema.Attributes["token"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf(
			"token has type %T, want schema.StringAttribute",
			response.Schema.Attributes["token"],
		)
	}
	if !tokenAttribute.Sensitive || !tokenAttribute.Computed {
		t.Fatal("token must be sensitive and computed")
	}
}

func TestValidateAPITokenDeletion(t *testing.T) {
	t.Parallel()

	testValidateAPITokenMutation(t, "deletion", validateAPITokenDeletion)
}

func TestValidateAPITokenRename(t *testing.T) {
	t.Parallel()

	testValidateAPITokenMutation(t, "rename", validateAPITokenRename)
}

func TestAPITokensHaveSameIdentity(t *testing.T) {
	t.Parallel()

	expiresAt := apitoken.NullableUnixTimestamp{Value: 42, Valid: true}
	original := apitoken.Token{
		Name:          "old",
		HasFullAccess: 1,
		ExpiresAt:     expiresAt,
		CreateTime:    12,
		Features:      []string{"second", "first"},
		WhitelistIPs:  []string{"192.0.2.2", "192.0.2.1"},
	}
	renamed := original
	renamed.Name = "new"
	renamed.Features = []string{"first", "second"}
	renamed.WhitelistIPs = []string{"192.0.2.1", "192.0.2.2"}

	if !apiTokensHaveSameIdentity(original, renamed) {
		t.Fatal("renamed token with identical metadata was not recognized")
	}
	renamed.CreateTime++
	if apiTokensHaveSameIdentity(original, renamed) {
		t.Fatal("token with a different creation time was recognized")
	}
}

func TestAPITokenDeleteProtectsTokenIdentityAndReconcilesResponse(t *testing.T) {
	t.Parallel()

	const (
		name       = "managed-token"
		createTime = int64(12)
	)
	managed := apitoken.Token{
		Name:          name,
		HasFullAccess: 1,
		CreateTime:    createTime,
		Features:      []string{},
		WhitelistIPs:  []string{},
	}
	replacement := managed
	replacement.CreateTime++

	testCases := []struct {
		name          string
		current       *apitoken.Token
		revokeOutcome string
		wantError     bool
		wantRevokes   int
	}{
		{
			name:        "changed creation time is refused",
			current:     &replacement,
			wantError:   true,
			wantRevokes: 0,
		},
		{
			name:          "ambiguous response with confirmed absence succeeds",
			current:       &managed,
			revokeOutcome: "ambiguous_deleted",
			wantRevokes:   1,
		},
		{
			name:          "replacement after revoke is preserved",
			current:       &managed,
			revokeOutcome: "replacement",
			wantError:     true,
			wantRevokes:   1,
		},
		{
			name:          "false success is reported",
			current:       &managed,
			revokeOutcome: "unchanged",
			wantError:     true,
			wantRevokes:   1,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := cloneAPIToken(testCase.current)
			revokeCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Tokens/list":
					data := []map[string]any{}
					if current != nil {
						data = append(data, apiTokenResourceTestData(*current))
					}
					writeAPITokenResourceTestJSON(
						t,
						response,
						map[string]any{"status": 1, "data": data},
					)
				case "/execute/Tokens/revoke":
					revokeCalls++
					switch testCase.revokeOutcome {
					case "ambiguous_deleted":
						current = nil
						_, _ = response.Write([]byte("{"))
					case "replacement":
						current = cloneAPIToken(&replacement)
						writeAPITokenResourceTestJSON(
							t,
							response,
							map[string]any{"status": 1, "data": nil},
						)
					case "unchanged":
						writeAPITokenResourceTestJSON(
							t,
							response,
							map[string]any{"status": 1, "data": nil},
						)
					default:
						t.Fatalf(
							"unexpected revoke outcome %q",
							testCase.revokeOutcome,
						)
					}
				default:
					t.Fatalf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			baseClient, err := cpanel.NewClient(
				server.URL,
				"username",
				"provider-secret",
			)
			if err != nil {
				t.Fatalf("cpanel.NewClient() error: %v", err)
			}
			resource := &apiTokenResource{
				client:          apitoken.NewClient(baseClient),
				activeTokenName: "provider-token",
			}
			response := &frameworkresource.DeleteResponse{}
			resource.Delete(
				t.Context(),
				frameworkresource.DeleteRequest{
					State: testAPITokenDeleteState(
						t,
						managed,
					),
				},
				response,
			)

			if response.Diagnostics.HasError() != testCase.wantError {
				t.Fatalf(
					"Delete() diagnostics = %v, wantError %t",
					response.Diagnostics,
					testCase.wantError,
				)
			}
			if revokeCalls != testCase.wantRevokes {
				t.Fatalf(
					"revoke calls = %d, want %d",
					revokeCalls,
					testCase.wantRevokes,
				)
			}
		})
	}
}

func TestAPITokenUpdateRefusesRemoteDrift(t *testing.T) {
	t.Parallel()

	managed := apitoken.Token{
		Name:          "managed-token",
		HasFullAccess: 1,
		CreateTime:    12,
		Features:      []string{},
		WhitelistIPs:  []string{},
	}
	replacement := managed
	replacement.CreateTime++
	renameCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Tokens/list":
			writeAPITokenResourceTestJSON(
				t,
				response,
				map[string]any{
					"status": 1,
					"data": []map[string]any{
						apiTokenResourceTestData(replacement),
					},
				},
			)
		case "/execute/Tokens/rename":
			renameCalls++
			writeAPITokenResourceTestJSON(
				t,
				response,
				map[string]any{"status": 1, "data": nil},
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

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"provider-secret",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	resource := &apiTokenResource{
		client:          apitoken.NewClient(baseClient),
		activeTokenName: "provider-token",
	}
	stateValue := testAPITokenDeleteState(t, managed)
	var state APITokenResourceModel
	if diagnostics := stateValue.Get(
		t.Context(),
		&state,
	); diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", diagnostics)
	}
	plan := state
	plan.Name = types.StringValue("renamed-token")

	response := runSingletonUpdate(
		t,
		NewAPITokenResource(),
		state,
		plan,
		resource.Update,
	)
	assertUpdateDriftRefused(t, response.Diagnostics)
	if renameCalls != 0 {
		t.Fatalf("renameCalls = %d, want 0", renameCalls)
	}
}

func TestAPITokenMatchesStateChecksEveryObservableField(t *testing.T) {
	t.Parallel()

	managed := apitoken.Token{
		Name:          "managed-token",
		HasFullAccess: 1,
		ExpiresAt: apitoken.NullableUnixTimestamp{
			Value: 42,
			Valid: true,
		},
		CreateTime:   12,
		Features:     []string{"alpha", "beta"},
		WhitelistIPs: []string{"192.0.2.1", "192.0.2.2"},
	}
	state := testAPITokenDeleteState(t, managed)
	var stateModel APITokenResourceModel
	diagnostics := state.Get(t.Context(), &stateModel)
	if diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", diagnostics)
	}

	testCases := map[string]func(*apitoken.Token){
		"name": func(token *apitoken.Token) {
			token.Name = "replacement"
		},
		"creation time": func(token *apitoken.Token) {
			token.CreateTime++
		},
		"expiration": func(token *apitoken.Token) {
			token.ExpiresAt.Value++
		},
		"access": func(token *apitoken.Token) {
			token.HasFullAccess = 0
		},
		"features": func(token *apitoken.Token) {
			token.Features = []string{"alpha"}
		},
		"IP restrictions": func(token *apitoken.Token) {
			token.WhitelistIPs = []string{"192.0.2.1"}
		},
	}

	matches, err := apiTokenMatchesState(t.Context(), managed, stateModel)
	if err != nil || !matches {
		t.Fatalf(
			"apiTokenMatchesState(managed) = %t, %v; want true, nil",
			matches,
			err,
		)
	}
	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			changed := *cloneAPIToken(&managed)
			mutate(&changed)
			matches, err := apiTokenMatchesState(
				t.Context(),
				changed,
				stateModel,
			)
			if err != nil {
				t.Fatalf("apiTokenMatchesState() error: %v", err)
			}
			if matches {
				t.Fatal("apiTokenMatchesState() = true, want false")
			}
		})
	}
}

func testAPITokenDeleteState(
	t *testing.T,
	token apitoken.Token,
) tfsdk.State {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewAPITokenResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	features, diagnostics := types.SetValueFrom(
		t.Context(),
		types.StringType,
		token.Features,
	)
	if diagnostics.HasError() {
		t.Fatalf("features set diagnostics: %v", diagnostics)
	}
	whitelistIPs, diagnostics := types.SetValueFrom(
		t.Context(),
		types.StringType,
		token.WhitelistIPs,
	)
	if diagnostics.HasError() {
		t.Fatalf("whitelist set diagnostics: %v", diagnostics)
	}

	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics = state.Set(t.Context(), &APITokenResourceModel{
		Name:          types.StringValue(token.Name),
		ExpiresAt:     types.Int64Value(token.ExpiresAt.ValueOrZero()),
		Token:         types.StringValue("managed-secret"),
		CreatedAt:     types.Int64Value(token.CreateTime),
		HasFullAccess: types.BoolValue(token.HasFullAccess == 1),
		Features:      features,
		WhitelistIPs:  whitelistIPs,
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	return state
}

func cloneAPIToken(token *apitoken.Token) *apitoken.Token {
	if token == nil {
		return nil
	}
	cloned := *token
	cloned.Features = append([]string(nil), token.Features...)
	cloned.WhitelistIPs = append([]string(nil), token.WhitelistIPs...)

	return &cloned
}

func apiTokenResourceTestData(token apitoken.Token) map[string]any {
	var expiresAt any
	if token.ExpiresAt.Valid {
		expiresAt = token.ExpiresAt.Value
	}

	return map[string]any{
		"name":            token.Name,
		"has_full_access": token.HasFullAccess,
		"expires_at":      expiresAt,
		"create_time":     token.CreateTime,
		"features":        token.Features,
		"whitelist_ips":   token.WhitelistIPs,
	}
}

func writeAPITokenResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func testValidateAPITokenMutation(
	t *testing.T,
	action string,
	validate func(APITokenResourceModel, string, string) error,
) {
	t.Helper()

	tests := map[string]struct {
		token           types.String
		activeSecret    string
		activeTokenName string
		wantError       bool
	}{
		"managed different token": {
			token:           types.StringValue("managed-secret"),
			activeSecret:    "provider-secret",
			activeTokenName: "provider-token",
		},
		"managed active secret": {
			token:           types.StringValue("provider-secret"),
			activeSecret:    "provider-secret",
			activeTokenName: "provider-token",
			wantError:       true,
		},
		"declared active name": {
			token:           types.StringValue("different-secret"),
			activeSecret:    "provider-secret",
			activeTokenName: "resource-token",
			wantError:       true,
		},
		"imported without active name": {
			token:        types.StringNull(),
			activeSecret: "provider-secret",
			wantError:    true,
		},
		"imported active name": {
			token:           types.StringNull(),
			activeSecret:    "provider-secret",
			activeTokenName: "resource-token",
			wantError:       true,
		},
		"imported different name": {
			token:           types.StringNull(),
			activeSecret:    "provider-secret",
			activeTokenName: "provider-token",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validate(
				APITokenResourceModel{
					Name:  types.StringValue("resource-token"),
					Token: test.token,
				},
				test.activeSecret,
				test.activeTokenName,
			)
			if test.wantError && err == nil {
				t.Fatalf("validate API token %s error = nil, want error", action)
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validate API token %s error = %v, want nil",
					action,
					err,
				)
			}
		})
	}
}

func TestAccAPITokenResource(t *testing.T) {
	const resourceName = "cpanel_api_token.test"

	initialName := testAccAPITokenName("resource")
	renamedName := testAccAPITokenName("renamed")
	expiresAt := time.Now().Add(24 * time.Hour).Unix()

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckAPITokensDestroyed(
			initialName,
			renamedName,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccAPITokenResourceConfig(initialName, 0),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						initialName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"expires_at",
						"0",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"token",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"created_at",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"has_full_access",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"features.#",
						"0",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"whitelist_ips.#",
						"0",
					),
					testAccCheckAPITokenExists(initialName, 0),
				),
			},
			{
				Config: testAccAPITokenResourceConfig(renamedName, 0),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						renamedName,
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"token",
					),
					testAccCheckAPITokenExists(renamedName, 0),
					testAccCheckAPITokensDestroyed(initialName),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        renamedName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"token"},
			},
			{
				Config: testAccAPITokenResourceConfig(renamedName, expiresAt),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"expires_at",
						strconv.FormatInt(expiresAt, 10),
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"token",
					),
					testAccCheckAPITokenExists(renamedName, expiresAt),
				),
			},
			{
				PreConfig: func() {
					testAccRevokeAPIToken(t, renamedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccAPITokenResourceConfig(renamedName, expiresAt),
				Check:  testAccCheckAPITokenExists(renamedName, expiresAt),
			},
		},
	})
}

func testAccAPITokenResourceConfig(name string, expiresAt int64) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_api_token" "test" {
  name       = %q
  expires_at = %d
}
`, name, expiresAt)
}
