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
	"terraform-provider-cpanel/internal/cpanel/ftp"
)

func TestFTPAccountDeleteProtectsObservableIdentityAndReconcilesResponse(
	t *testing.T,
) {
	t.Parallel()

	const username = "managed@example.test"
	managed := ftp.Account{
		Login:             username,
		RelativeDirectory: "managed-home",
		AccountType:       "sub",
		DiskQuotaRaw:      json.RawMessage("50"),
	}
	replacement := managed
	replacement.RelativeDirectory = "replacement-home"

	testCases := []struct {
		name          string
		current       *ftp.Account
		deleteOutcome string
		wantError     bool
		wantDeletes   int
	}{
		{
			name:        "changed home is refused before deletion",
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

			current := cloneFTPAccount(testCase.current)
			deleteCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Ftp/list_ftp_with_disk":
					data := []ftp.Account{}
					if current != nil {
						data = append(data, *current)
					}
					writeFTPAccountResourceTestJSON(
						t,
						response,
						map[string]any{"status": 1, "data": data},
					)
				case "/execute/Ftp/delete_ftp":
					deleteCalls++
					if err := request.ParseForm(); err != nil {
						t.Fatalf("ParseForm() error: %v", err)
					}
					if request.Form.Get("destroy") != "1" {
						t.Fatalf(
							"destroy = %q, want 1",
							request.Form.Get("destroy"),
						)
					}
					switch testCase.deleteOutcome {
					case "ambiguous_deleted":
						current = nil
						_, _ = response.Write([]byte("{"))
					case "replacement":
						current = cloneFTPAccount(&replacement)
						writeFTPAccountResourceTestJSON(
							t,
							response,
							map[string]any{"status": 1, "data": nil},
						)
					case "unchanged":
						writeFTPAccountResourceTestJSON(
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
			(&ftpAccountResource{
				client: ftp.NewClient(baseClient),
			}).Delete(
				t.Context(),
				frameworkresource.DeleteRequest{
					State: testFTPAccountDeleteState(t, managed, true),
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

func TestFTPAccountDeletePreservesRemoteAccountByDefault(t *testing.T) {
	t.Parallel()

	managed := ftp.Account{
		Login:             "managed@example.test",
		RelativeDirectory: "managed-home",
		AccountType:       "sub",
		DiskQuotaRaw:      json.RawMessage("50"),
	}
	response := &frameworkresource.DeleteResponse{}
	(&ftpAccountResource{}).Delete(
		t.Context(),
		frameworkresource.DeleteRequest{
			State: testFTPAccountDeleteState(t, managed, false),
		},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if response.Diagnostics.WarningsCount() != 1 {
		t.Fatalf(
			"Delete() warning count = %d, want 1",
			response.Diagnostics.WarningsCount(),
		)
	}
}

func TestFTPAccountMatchesStateChecksObservableFields(t *testing.T) {
	t.Parallel()

	managed := ftp.Account{
		Login:             "managed@example.test",
		RelativeDirectory: "managed-home",
		AccountType:       "sub",
		DiskQuotaRaw:      json.RawMessage("50"),
	}
	state := FTPAccountResourceModel{
		Username:      types.StringValue(managed.Login),
		HomeDirectory: types.StringValue(managed.RelativeDirectory),
		QuotaMiB:      types.Int64Value(50),
	}

	testCases := map[string]func(*ftp.Account){
		"username": func(account *ftp.Account) {
			account.Login = "replacement@example.test"
		},
		"home directory": func(account *ftp.Account) {
			account.RelativeDirectory = "replacement-home"
		},
		"quota": func(account *ftp.Account) {
			account.DiskQuotaRaw = json.RawMessage("75")
		},
	}

	matches, err := ftpAccountMatchesState(managed, state)
	if err != nil || !matches {
		t.Fatalf(
			"ftpAccountMatchesState(managed) = %t, %v; want true, nil",
			matches,
			err,
		)
	}
	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			changed := *cloneFTPAccount(&managed)
			mutate(&changed)
			matches, err := ftpAccountMatchesState(changed, state)
			if err != nil {
				t.Fatalf("ftpAccountMatchesState() error: %v", err)
			}
			if matches {
				t.Fatal("ftpAccountMatchesState() = true, want false")
			}
		})
	}
}

func testFTPAccountDeleteState(
	t *testing.T,
	account ftp.Account,
	deleteOnDestroy bool,
) tfsdk.State {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewFTPAccountResource().Schema(
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
	diagnostics := state.Set(t.Context(), &FTPAccountResourceModel{
		Username:            types.StringValue(account.Login),
		Password:            types.StringValue("managed-password"),
		HomeDirectory:       types.StringValue(account.RelativeDirectory),
		QuotaMiB:            types.Int64Value(quotaMiB),
		DeleteOnDestroy:     types.BoolValue(deleteOnDestroy),
		DeleteHomeDirectory: types.BoolValue(true),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	return state
}

func cloneFTPAccount(account *ftp.Account) *ftp.Account {
	if account == nil {
		return nil
	}
	cloned := *account
	cloned.DiskQuotaRaw = append(json.RawMessage(nil), account.DiskQuotaRaw...)
	cloned.DiskUsedRaw = append(json.RawMessage(nil), account.DiskUsedRaw...)

	return &cloned
}

func writeFTPAccountResourceTestJSON(
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

func TestAccFTPAccountResource(t *testing.T) {
	const (
		resourceName = "cpanel_ftp_account.test"
		passwordOne  = "G7!ftpAccountOne-2026"
		passwordTwo  = "KZ8!ftpAccountTwo-2026"
	)

	firstUsername := testAccFTPUsername(t, "account")
	secondUsername := testAccFTPUsername(t, "replacement")
	firstHome := testAccFTPHomeDirectory("account")
	secondHome := testAccFTPHomeDirectory("replacement")
	updatedHome := testAccRegisterArtifact(firstHome + "-updated")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckFTPAccountsDestroyed(
			firstUsername,
			secondUsername,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccFTPAccountResourceConfig(
					firstUsername,
					passwordOne,
					firstHome,
					50,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "username", firstUsername),
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordOne)),
					),
					resource.TestCheckResourceAttr(resourceName, "home_directory", firstHome),
					resource.TestCheckResourceAttr(resourceName, "quota_mib", "50"),
					resource.TestCheckResourceAttr(resourceName, "delete_home_directory", "true"),
					testAccCheckFTPAccountExists(firstUsername, firstHome, 50),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        firstUsername,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "username",
				ImportStateVerifyIgnore: []string{
					"password",
					"password_version",
					"delete_on_destroy",
					"delete_home_directory",
				},
			},
			{
				Config: testAccFTPAccountResourceConfig(
					firstUsername,
					passwordTwo,
					firstHome,
					50,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordTwo)),
					),
					testAccCheckFTPAccountExists(firstUsername, firstHome, 50),
				),
			},
			{
				Config: testAccFTPAccountResourceConfig(
					firstUsername,
					passwordTwo,
					updatedHome,
					75,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "home_directory", updatedHome),
					resource.TestCheckResourceAttr(resourceName, "quota_mib", "75"),
					testAccCheckFTPAccountExists(firstUsername, updatedHome, 75),
				),
			},
			{
				Config: testAccFTPAccountResourceConfig(
					firstUsername,
					passwordTwo,
					updatedHome,
					0,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "quota_mib", "0"),
					testAccCheckFTPAccountExists(firstUsername, updatedHome, 0),
				),
			},
			{
				Config: testAccFTPAccountResourceConfig(
					secondUsername,
					passwordTwo,
					secondHome,
					75,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "username", secondUsername),
					testAccCheckFTPAccountExists(secondUsername, secondHome, 75),
					testAccCheckFTPAccountsDestroyed(firstUsername),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteFTPAccount(t, secondUsername)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccFTPAccountResourceConfig(
					secondUsername,
					passwordTwo,
					secondHome,
					75,
				),
				Check: testAccCheckFTPAccountExists(secondUsername, secondHome, 75),
			},
		},
	})
}

func testAccFTPAccountResourceConfig(
	username string,
	password string,
	homeDirectory string,
	quotaMiB int64,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_ftp_account" "test" {
  username              = %q
  password              = %q
  password_version      = %d
  home_directory        = %q
  quota_mib             = %d
  delete_on_destroy     = true
  delete_home_directory = true
}
`, username, password, testAccPasswordVersion(password), homeDirectory, quotaMiB)
}
