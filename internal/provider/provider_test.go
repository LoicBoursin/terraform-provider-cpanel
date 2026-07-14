// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/cron"
	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
	cpanelftp "terraform-provider-cpanel/internal/cpanel/ftp"
	"terraform-provider-cpanel/internal/cpanel/mysql"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

const providerConfig = ``

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"cpanel": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	for _, variable := range []string{"CPANEL_HOST", "CPANEL_USERNAME", "CPANEL_API_TOKEN"} {
		if os.Getenv(variable) == "" {
			t.Fatalf("%s must be set for acceptance tests", variable)
		}
	}

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := cron.NewClient(client).GetCronJobs(ctx); err != nil {
		t.Fatalf("verify Cron API access: %v", err)
	}
	if _, err := cpaneldomain.NewClient(client).ListSubdomains(ctx); err != nil {
		t.Fatalf("verify Domain API access: %v", err)
	}
	if _, err := postgresql.NewClient(client).GetDatabases(ctx); err != nil {
		t.Fatalf("verify PostgreSQL API access: %v", err)
	}
	if _, err := mysql.NewClient(client).GetRestrictions(ctx); err != nil {
		t.Fatalf("verify MySQL API access: %v", err)
	}
	if _, err := cpanelmail.NewClient(client).ListMailDomains(ctx); err != nil {
		t.Fatalf("verify Email API access: %v", err)
	}
	if _, err := cpanelftp.NewClient(client).ListAccounts(ctx); err != nil {
		t.Fatalf("verify FTP API access: %v", err)
	}
}

func testAccClient() (*cpanel.Client, error) {
	return cpanel.NewClient(
		os.Getenv("CPANEL_HOST"),
		os.Getenv("CPANEL_USERNAME"),
		os.Getenv("CPANEL_API_TOKEN"),
	)
}

func testAccPostgreSQLName(kind string) string {
	return fmt.Sprintf(
		"%s_tf%s%s",
		os.Getenv("CPANEL_USERNAME"),
		kind,
		acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum),
	)
}

func testAccMySQLName(kind string) string {
	return fmt.Sprintf(
		"%s_tf%s%s",
		os.Getenv("CPANEL_USERNAME"),
		kind,
		acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum),
	)
}

func testAccEmailAddress(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanel%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccFTPUsername(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelftp%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccFTPHomeDirectory(kind string) string {
	return fmt.Sprintf(
		"tfcpanel-ftp-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccSubdomain(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelsub%s%s.%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccDomainDocumentRoot(kind string) string {
	return fmt.Sprintf(
		"public_html/tfcpanel-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccMainDomain(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	var response struct {
		Data struct {
			MainDomain string `json:"main_domain"`
		} `json:"data"`
	}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		"DomainInfo",
		"list_domains",
		map[string]string{},
		&response,
	); err != nil {
		t.Fatalf("read cPanel main domain: %v", err)
	}
	if response.Data.MainDomain == "" {
		t.Fatal("cPanel main domain is empty")
	}

	return response.Data.MainDomain
}

func testAccCronCommand(kind string) string {
	return fmt.Sprintf(
		"/bin/true # terraform-provider-cpanel-%s-%s",
		kind,
		acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum),
	)
}

func testAccImportStateIDFromAttribute(resourceName, attribute string) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}

		value := resourceState.Primary.Attributes[attribute]
		if value == "" {
			return "", fmt.Errorf("attribute %s.%s is empty", resourceName, attribute)
		}

		return value, nil
	}
}

func testAccCheckCronJobExists(command string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		cronJobs, err := cron.NewClient(client).GetCronJobs(ctx)
		if err != nil {
			return err
		}

		for _, cronJob := range cronJobs.CpanelResult.Data {
			if cronJob.Command == command {
				return nil
			}
		}

		return fmt.Errorf("cron job with command %q was not found", command)
	}
}

func testAccCheckCronJobsDestroyed(commands ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		cronJobs, err := cron.NewClient(client).GetCronJobs(ctx)
		if err != nil {
			return err
		}

		for _, cronJob := range cronJobs.CpanelResult.Data {
			if slices.Contains(commands, cronJob.Command) {
				return fmt.Errorf("cron job with command %q still exists", cronJob.Command)
			}
		}

		return nil
	}
}

func testAccDeleteCronJobs(t *testing.T, commands ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	cronClient := cron.NewClient(client)
	cronJobs, err := cronClient.GetCronJobs(ctx)
	if err != nil {
		t.Fatalf("list cron jobs: %v", err)
	}

	for _, cronJob := range cronJobs.CpanelResult.Data {
		if !slices.Contains(commands, cronJob.Command) {
			continue
		}

		if _, err := cronClient.DeleteCronJob(
			ctx,
			cron.CronJobDeleteModel{LineKey: string(cronJob.LineKey)},
		); err != nil {
			t.Fatalf("delete cron job %q: %v", cronJob.LineKey, err)
		}
	}
}

func testAccCheckPostgreSQLUserExists(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		exists, err := postgresql.NewClient(client).UserExists(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("PostgreSQL user %q was not found", name)
		}

		return nil
	}
}

func testAccCheckPostgreSQLUsersDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		users, err := postgresql.NewClient(client).GetUsers(ctx)
		if err != nil {
			return err
		}

		for _, user := range users.Data {
			if slices.Contains(names, user) {
				return fmt.Errorf("PostgreSQL user %q still exists", user)
			}
		}

		return nil
	}
}

func testAccDeletePostgreSQLUser(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if _, err := postgresql.NewClient(client).DeleteUser(
		ctx,
		postgresql.UserDeleteModel{Name: name},
	); err != nil {
		t.Fatalf("delete PostgreSQL user %q: %v", name, err)
	}
}

func testAccCheckPostgreSQLDatabaseExists(name string, expectedUsers ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		databases, err := postgresql.NewClient(client).GetDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if database.Database != name {
				continue
			}
			if len(database.Users) != len(expectedUsers) {
				return fmt.Errorf(
					"PostgreSQL database %q users = %v, want %v",
					name,
					database.Users,
					expectedUsers,
				)
			}
			for _, expectedUser := range expectedUsers {
				if !slices.Contains(database.Users, expectedUser) {
					return fmt.Errorf(
						"PostgreSQL database %q does not grant access to %q",
						name,
						expectedUser,
					)
				}
			}

			return nil
		}

		return fmt.Errorf("PostgreSQL database %q was not found", name)
	}
}

func testAccCheckPostgreSQLDatabasesDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		databases, err := postgresql.NewClient(client).GetDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if slices.Contains(names, database.Database) {
				return fmt.Errorf("PostgreSQL database %q still exists", database.Database)
			}
		}

		return nil
	}
}

func testAccDeletePostgreSQLDatabase(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if _, err := postgresql.NewClient(client).DeleteDatabase(
		ctx,
		postgresql.DatabaseDeleteModel{Name: name},
	); err != nil {
		t.Fatalf("delete PostgreSQL database %q: %v", name, err)
	}
}

func testAccCheckMySQLUserExists(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		exists, err := mysql.NewClient(client).UserExists(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("MySQL user %q was not found", name)
		}

		return nil
	}
}

func testAccCheckMySQLUsersDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		users, err := mysql.NewClient(client).ListUsers(ctx)
		if err != nil {
			return err
		}

		for _, user := range users.Data {
			if slices.Contains(names, user.User) {
				return fmt.Errorf("MySQL user %q still exists", user.User)
			}
		}

		return nil
	}
}

func testAccDeleteMySQLUser(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := mysql.NewClient(client).DeleteUser(ctx, name); err != nil {
		t.Fatalf("delete MySQL user %q: %v", name, err)
	}
}

func testAccCheckMySQLDatabaseExists(
	name string,
	expectedUsers ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		mysqlClient := mysql.NewClient(client)
		databases, err := mysqlClient.ListDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if database.Database != name {
				continue
			}
			if len(database.Users) != len(expectedUsers) {
				return fmt.Errorf(
					"MySQL database %q users = %v, want %v",
					name,
					database.Users,
					expectedUsers,
				)
			}
			for _, expectedUser := range expectedUsers {
				if !slices.Contains(database.Users, expectedUser) {
					return fmt.Errorf(
						"MySQL database %q does not grant access to %q",
						name,
						expectedUser,
					)
				}

				privileges, err := mysqlClient.GetPrivileges(ctx, expectedUser, name)
				if err != nil {
					return err
				}
				if !slices.Contains(privileges, "ALL PRIVILEGES") {
					return fmt.Errorf(
						"MySQL database %q privileges for %q = %v",
						name,
						expectedUser,
						privileges,
					)
				}
			}

			return nil
		}

		return fmt.Errorf("MySQL database %q was not found", name)
	}
}

func testAccCheckMySQLDatabasesDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		databases, err := mysql.NewClient(client).ListDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if slices.Contains(names, database.Database) {
				return fmt.Errorf("MySQL database %q still exists", database.Database)
			}
		}

		return nil
	}
}

func testAccDeleteMySQLDatabase(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := mysql.NewClient(client).DeleteDatabase(ctx, name); err != nil {
		t.Fatalf("delete MySQL database %q: %v", name, err)
	}
}

func testAccCheckEmailAccountExists(
	address string,
	expectedQuotaMiB int64,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		user, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		account, err := cpanelmail.NewClient(client).GetAccount(ctx, user, domain)
		if err != nil {
			return err
		}
		if account == nil {
			return fmt.Errorf("email account %q was not found", address)
		}

		quotaMiB, err := account.QuotaMiB()
		if err != nil {
			return err
		}
		if quotaMiB != expectedQuotaMiB {
			return fmt.Errorf(
				"email account %q quota = %d MiB, want %d MiB",
				address,
				quotaMiB,
				expectedQuotaMiB,
			)
		}

		return nil
	}
}

func testAccCheckEmailAccountsDestroyed(addresses ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		accounts, err := cpanelmail.NewClient(client).ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, account := range accounts.Data {
			if slices.Contains(addresses, account.Email) {
				return fmt.Errorf("email account %q still exists", account.Email)
			}
		}

		return nil
	}
}

func testAccDeleteEmailAccount(t *testing.T, address string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Fatalf("split email account address: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteAccount(ctx, user, domain); err != nil {
		t.Fatalf("delete email account %q: %v", address, err)
	}
}

func testAccCheckFTPAccountExists(
	username string,
	expectedHomeDirectory string,
	expectedQuotaMiB int64,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		account, err := cpanelftp.NewClient(client).GetAccount(ctx, username)
		if err != nil {
			return err
		}
		if account == nil {
			return fmt.Errorf("FTP account %q was not found", username)
		}
		if account.RelativeDirectory != expectedHomeDirectory {
			return fmt.Errorf(
				"FTP account %q home directory = %q, want %q",
				username,
				account.RelativeDirectory,
				expectedHomeDirectory,
			)
		}

		quotaMiB, err := account.QuotaMiB()
		if err != nil {
			return err
		}
		if quotaMiB != expectedQuotaMiB {
			return fmt.Errorf(
				"FTP account %q quota = %d MiB, want %d MiB",
				username,
				quotaMiB,
				expectedQuotaMiB,
			)
		}

		return nil
	}
}

func testAccCheckFTPAccountsDestroyed(usernames ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		accounts, err := cpanelftp.NewClient(client).ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, account := range accounts.Data {
			if slices.Contains(usernames, account.Login) {
				return fmt.Errorf("FTP account %q still exists", account.Login)
			}
		}

		return nil
	}
}

func testAccDeleteFTPAccount(t *testing.T, username string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	user, domain, err := splitFTPAccountUsername(username)
	if err != nil {
		t.Fatalf("split FTP account username: %v", err)
	}
	if err := cpanelftp.NewClient(client).DeleteAccount(ctx, user, domain, true); err != nil {
		t.Fatalf("delete FTP account %q: %v", username, err)
	}
}

func testAccCheckSubdomainExists(
	domain string,
	expectedDocumentRoot string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		subdomain, err := cpaneldomain.NewClient(client).GetSubdomain(ctx, domain)
		if err != nil {
			return err
		}
		if subdomain == nil {
			return fmt.Errorf("subdomain %q was not found", domain)
		}
		if subdomain.BaseDirectory != expectedDocumentRoot {
			return fmt.Errorf(
				"subdomain %q document root = %q, want %q",
				domain,
				subdomain.BaseDirectory,
				expectedDocumentRoot,
			)
		}

		return nil
	}
}

func testAccCheckSubdomainsDestroyed(domains ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		response, err := cpaneldomain.NewClient(client).ListSubdomains(ctx)
		if err != nil {
			return err
		}
		for _, subdomain := range response.CpanelResult.Data {
			if slices.Contains(domains, subdomain.Domain) {
				return fmt.Errorf("subdomain %q still exists", subdomain.Domain)
			}
		}

		return nil
	}
}

func testAccDeleteSubdomain(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := cpaneldomain.NewClient(client).DeleteSubdomain(ctx, domain); err != nil {
		t.Fatalf("delete subdomain %q: %v", domain, err)
	}
}
