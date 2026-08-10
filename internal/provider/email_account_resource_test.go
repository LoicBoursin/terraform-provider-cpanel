package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailAccountDeleteProtectsObservableIdentityAndReconcilesResponse(
	t *testing.T,
) {
	t.Parallel()

	const address = "managed@example.test"
	managed := cpanelmail.Account{
		Email:        address,
		User:         "managed",
		Domain:       "example.test",
		DiskQuotaRaw: json.RawMessage("52428800"),
	}
	replacement := managed
	replacement.DiskQuotaRaw = json.RawMessage("78643200")

	testCases := []struct {
		name          string
		current       *cpanelmail.Account
		deleteOutcome string
		wantError     bool
		wantDeletes   int
	}{
		{
			name:        "changed quota is refused before deletion",
			current:     &replacement,
			wantError:   true,
			wantDeletes: 0,
		},
		{
			name:          "ambiguous response with confirmed absence succeeds",
			current:       &managed,
			deleteOutcome: "ambiguous_deleted",
			wantDeletes:   1,
		},
		{
			name:          "replacement after deletion is preserved",
			current:       &managed,
			deleteOutcome: "replacement",
			wantError:     true,
			wantDeletes:   1,
		},
		{
			name:          "false success is reported",
			current:       &managed,
			deleteOutcome: "unchanged",
			wantError:     true,
			wantDeletes:   1,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := cloneEmailAccount(testCase.current)
			deleteCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Email/list_pops_with_disk":
					data := []cpanelmail.Account{}
					if current != nil {
						data = append(data, *current)
					}
					writeEmailAccountResourceTestJSON(
						t,
						response,
						map[string]any{"status": 1, "data": data},
					)
				case "/execute/Email/delete_pop":
					deleteCalls++
					switch testCase.deleteOutcome {
					case "ambiguous_deleted":
						current = nil
						_, _ = response.Write([]byte("{"))
					case "replacement":
						current = cloneEmailAccount(&replacement)
						writeEmailAccountResourceTestJSON(
							t,
							response,
							map[string]any{"status": 1, "data": nil},
						)
					case "unchanged":
						writeEmailAccountResourceTestJSON(
							t,
							response,
							map[string]any{"status": 1, "data": nil},
						)
					default:
						t.Fatalf(
							"unexpected delete outcome %q",
							testCase.deleteOutcome,
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
				"api-token",
			)
			if err != nil {
				t.Fatalf("cpanel.NewClient() error: %v", err)
			}
			response := &frameworkresource.DeleteResponse{}
			(&emailAccountResource{
				client: cpanelmail.NewClient(baseClient),
			}).Delete(
				t.Context(),
				frameworkresource.DeleteRequest{
					State: testEmailAccountDeleteState(t, managed, true),
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
			if deleteCalls != testCase.wantDeletes {
				t.Fatalf(
					"delete calls = %d, want %d",
					deleteCalls,
					testCase.wantDeletes,
				)
			}
		})
	}
}

func TestEmailAccountDeletePreservesRemoteAccountByDefault(t *testing.T) {
	t.Parallel()

	account := cpanelmail.Account{
		Email:        "managed@example.test",
		DiskQuotaRaw: json.RawMessage("52428800"),
	}
	response := &frameworkresource.DeleteResponse{}
	(&emailAccountResource{}).Delete(
		t.Context(),
		frameworkresource.DeleteRequest{
			State: testEmailAccountDeleteState(t, account, false),
		},
		response,
	)

	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if response.Diagnostics.WarningsCount() != 1 {
		t.Fatalf(
			"Delete() warnings = %d; want 1",
			response.Diagnostics.WarningsCount(),
		)
	}
}

func TestEmailAccountMatchesStateChecksObservableFields(t *testing.T) {
	t.Parallel()

	managed := cpanelmail.Account{
		Email:        "managed@example.test",
		DiskQuotaRaw: json.RawMessage("52428800"),
	}
	state := EmailAccountResourceModel{
		Email:    types.StringValue(managed.Email),
		QuotaMiB: types.Int64Value(50),
	}

	testCases := map[string]func(*cpanelmail.Account){
		"address": func(account *cpanelmail.Account) {
			account.Email = "replacement@example.test"
		},
		"quota": func(account *cpanelmail.Account) {
			account.DiskQuotaRaw = json.RawMessage("78643200")
		},
	}

	matches, err := emailAccountMatchesState(managed, state)
	if err != nil || !matches {
		t.Fatalf(
			"emailAccountMatchesState(managed) = %t, %v; want true, nil",
			matches,
			err,
		)
	}
	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			changed := *cloneEmailAccount(&managed)
			mutate(&changed)
			matches, err := emailAccountMatchesState(changed, state)
			if err != nil {
				t.Fatalf("emailAccountMatchesState() error: %v", err)
			}
			if matches {
				t.Fatal("emailAccountMatchesState() = true, want false")
			}
		})
	}
}

func testEmailAccountDeleteState(
	t *testing.T,
	account cpanelmail.Account,
	deleteOnDestroy bool,
) tfsdk.State {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewEmailAccountResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		t.Fatalf("QuotaMiB() error: %v", err)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(t.Context(), &EmailAccountResourceModel{
		Email:           types.StringValue(account.Email),
		Password:        types.StringNull(),
		PasswordVersion: types.Int64Value(1),
		QuotaMiB:        types.Int64Value(quotaMiB),
		DeleteOnDestroy: types.BoolValue(deleteOnDestroy),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	return state
}

func cloneEmailAccount(account *cpanelmail.Account) *cpanelmail.Account {
	if account == nil {
		return nil
	}
	cloned := *account
	cloned.DiskQuotaRaw = append(json.RawMessage(nil), account.DiskQuotaRaw...)
	cloned.DiskUsedRaw = append(json.RawMessage(nil), account.DiskUsedRaw...)

	return &cloned
}

func writeEmailAccountResourceTestJSON(
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

func TestAccEmailAccountResource(t *testing.T) {
	const (
		resourceName = "cpanel_email_account.test"
		passwordOne  = "G7!emailAccountOne-2026"
		passwordTwo  = "KZ8!emailAccountTwo-2026"
	)

	firstAddress := testAccEmailAddress(t, "account")
	secondAddress := testAccEmailAddress(t, "replacement")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckEmailAccountsDestroyed(
			firstAddress,
			secondAddress,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailAccountResourceConfig(firstAddress, passwordOne, 50),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "email", firstAddress),
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordOne)),
					),
					resource.TestCheckResourceAttr(resourceName, "quota_mib", "50"),
					testAccCheckEmailAccountExists(firstAddress, 50),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        firstAddress,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "email",
				ImportStateVerifyIgnore: []string{
					"password",
					"password_version",
					"delete_on_destroy",
				},
			},
			{
				Config: testAccEmailAccountResourceConfig(firstAddress, passwordTwo, 50),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordTwo)),
					),
					testAccCheckEmailAccountExists(firstAddress, 50),
				),
			},
			{
				Config: testAccEmailAccountResourceConfig(firstAddress, passwordTwo, 75),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "quota_mib", "75"),
					testAccCheckEmailAccountExists(firstAddress, 75),
				),
			},
			{
				Config: testAccEmailAccountResourceConfig(secondAddress, passwordTwo, 75),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "email", secondAddress),
					testAccCheckEmailAccountExists(secondAddress, 75),
					testAccCheckEmailAccountsDestroyed(firstAddress),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailAccount(t, secondAddress)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailAccountResourceConfig(secondAddress, passwordTwo, 75),
				Check:  testAccCheckEmailAccountExists(secondAddress, 75),
			},
		},
	})
}

func testAccEmailAccountResourceConfig(
	address string,
	password string,
	quotaMiB int64,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "test" {
  email             = %q
  password          = %q
  password_version  = %d
  quota_mib         = %d
  delete_on_destroy = true
}
`, address, password, testAccPasswordVersion(password), quotaMiB)
}
