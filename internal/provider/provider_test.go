// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/cron"
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
	if _, err := postgresql.NewClient(client).GetDatabases(ctx); err != nil {
		t.Fatalf("verify PostgreSQL API access: %v", err)
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
