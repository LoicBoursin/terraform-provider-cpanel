package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
)

func TestAccLogSettingsResource(t *testing.T) {
	original := testAccLogSettingsOriginal(t)
	first, serverDefault, second := testAccLogSettingsTargets(original)

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckLogSettingsRemote(
			original.Definition(),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccLogSettingsResourceConfig(first),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLogSettingsResource(first, original),
					testAccCheckLogSettings(first),
					testAccCheckLogSettingsDataSource(first),
				),
			},
			{
				Config: testAccLogSettingsResourceConfig(serverDefault),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLogSettingsResource(serverDefault, original),
					testAccCheckLogSettings(serverDefault),
					testAccCheckLogSettingsDataSource(serverDefault),
				),
			},
			{
				Config: testAccLogSettingsResourceConfig(second),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLogSettingsResource(second, original),
					testAccCheckLogSettings(second),
					testAccCheckLogSettingsDataSource(second),
				),
			},
			{
				PreConfig: func() {
					testAccSetLogSettings(t, first)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccLogSettingsResourceConfig(second),
				Check:  testAccCheckLogSettings(second),
			},
		},
	})
}

func TestAccLogSettingsResourceImport(t *testing.T) {
	original := testAccLogSettingsOriginal(t)
	target, _, _ := testAccLogSettingsTargets(original)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckLogSettingsRemote(
			original.Definition(),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccLogSettingsResourceConfig(original.Definition()),
				Check: testAccCheckLogSettingsResource(
					original.Definition(),
					original,
				),
			},
			{
				ResourceName:                         "cpanel_log_settings.test",
				ImportStateId:                        cpanellogmanager.AccountIdentity,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "account",
			},
			{
				Config: testAccLogSettingsResourceConfig(target),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLogSettingsResource(target, original),
					testAccCheckLogSettings(target),
				),
			},
		},
	})
}

func testAccLogSettingsResourceConfig(
	definition cpanellogmanager.Definition,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_log_settings" "test" {
  archive_logs    = %t
  prune_archives  = %t
  retention_days = %d
}

data "cpanel_log_settings" "test" {
  depends_on = [cpanel_log_settings.test]
}
`,
		definition.ArchiveLogs,
		definition.PruneArchive,
		definition.RetentionDays,
	)
}

func testAccLogSettingsOriginal(
	t *testing.T,
) cpanellogmanager.Settings {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	client := testAccLogSettingsClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	original, err := client.Get(ctx)
	if err != nil {
		t.Fatalf("read cPanel log settings: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cleanupCancel()

		current, cleanupErr := client.Get(cleanupContext)
		if cleanupErr != nil {
			t.Errorf(
				"read cPanel log settings during cleanup: %v",
				cleanupErr,
			)
			return
		}
		if cpanellogmanager.SettingsMatchDefinition(
			*current,
			original.Definition(),
		) {
			return
		}
		if _, cleanupErr = client.Set(
			cleanupContext,
			original.Definition(),
		); cleanupErr != nil {
			t.Errorf(
				"restore cPanel log settings during cleanup: %v",
				cleanupErr,
			)
		}
	})

	return *original
}

func testAccLogSettingsTargets(
	original cpanellogmanager.Settings,
) (
	cpanellogmanager.Definition,
	cpanellogmanager.Definition,
	cpanellogmanager.Definition,
) {
	first := cpanellogmanager.Definition{
		ArchiveLogs:   !original.ArchiveLogs,
		PruneArchive:  !original.PruneArchive,
		RetentionDays: 7,
	}
	serverDefault := cpanellogmanager.Definition{
		ArchiveLogs:   original.ArchiveLogs,
		PruneArchive:  original.PruneArchive,
		RetentionDays: -1,
	}
	second := cpanellogmanager.Definition{
		ArchiveLogs:   original.ArchiveLogs,
		PruneArchive:  original.PruneArchive,
		RetentionDays: 0,
	}
	if cpanellogmanager.SettingsMatchDefinition(original, second) {
		second.RetentionDays = 14
	}
	if first == second {
		second.RetentionDays = 30
	}

	return first, serverDefault, second
}

func testAccCheckLogSettingsResource(
	expected cpanellogmanager.Definition,
	restore cpanellogmanager.Settings,
) testresource.TestCheckFunc {
	const resourceName = "cpanel_log_settings.test"

	restoreDefinition := restore.Definition()

	return testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			resourceName,
			"account",
			cpanellogmanager.AccountIdentity,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"archive_logs",
			strconv.FormatBool(expected.ArchiveLogs),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"prune_archives",
			strconv.FormatBool(expected.PruneArchive),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"retention_days",
			strconv.FormatInt(expected.RetentionDays, 10),
		),
		testresource.TestCheckResourceAttrSet(
			resourceName,
			"effective_retention_days",
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"using_default_retention",
			strconv.FormatBool(expected.RetentionDays == -1),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_archive_logs",
			strconv.FormatBool(restoreDefinition.ArchiveLogs),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_prune_archives",
			strconv.FormatBool(restoreDefinition.PruneArchive),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_retention_days",
			strconv.FormatInt(restoreDefinition.RetentionDays, 10),
		),
	)
}

func testAccCheckLogSettingsDataSource(
	expected cpanellogmanager.Definition,
) testresource.TestCheckFunc {
	const resourceName = "data.cpanel_log_settings.test"

	return testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			resourceName,
			"account",
			cpanellogmanager.AccountIdentity,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"archive_logs",
			strconv.FormatBool(expected.ArchiveLogs),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"prune_archives",
			strconv.FormatBool(expected.PruneArchive),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"retention_days",
			strconv.FormatInt(expected.RetentionDays, 10),
		),
		testresource.TestCheckResourceAttrSet(
			resourceName,
			"effective_retention_days",
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"using_default_retention",
			strconv.FormatBool(expected.RetentionDays == -1),
		),
	)
}

func testAccCheckLogSettings(
	expected cpanellogmanager.Definition,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		actual, err := testAccReadLogSettings()
		if err != nil {
			return err
		}
		if !cpanellogmanager.SettingsMatchDefinition(*actual, expected) {
			return fmt.Errorf(
				"cPanel log settings = %#v, want %#v",
				actual,
				expected,
			)
		}
		for _, resourceName := range []string{
			"cpanel_log_settings.test",
			"data.cpanel_log_settings.test",
		} {
			resourceState, ok := state.RootModule().Resources[resourceName]
			if !ok {
				return fmt.Errorf(
					"Terraform state does not contain %s",
					resourceName,
				)
			}
			attributes := resourceState.Primary.Attributes
			effectiveRetention := strconv.FormatInt(
				actual.RetentionDays,
				10,
			)
			if attributes["effective_retention_days"] != effectiveRetention {
				return fmt.Errorf(
					"%s effective_retention_days = %q, want %q",
					resourceName,
					attributes["effective_retention_days"],
					effectiveRetention,
				)
			}
			usingDefault := strconv.FormatBool(actual.UsingDefault)
			if attributes["using_default_retention"] != usingDefault {
				return fmt.Errorf(
					"%s using_default_retention = %q, want %q",
					resourceName,
					attributes["using_default_retention"],
					usingDefault,
				)
			}
		}

		return nil
	}
}

func testAccCheckLogSettingsRemote(
	expected cpanellogmanager.Definition,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		actual, err := testAccReadLogSettings()
		if err != nil {
			return err
		}
		if !cpanellogmanager.SettingsMatchDefinition(*actual, expected) {
			return fmt.Errorf(
				"cPanel log settings = %#v, want %#v",
				actual,
				expected,
			)
		}

		return nil
	}
}

func testAccReadLogSettings() (*cpanellogmanager.Settings, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}

	return cpanellogmanager.NewClient(client).Get(ctx)
}

func testAccSetLogSettings(
	t *testing.T,
	definition cpanellogmanager.Definition,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if _, err := testAccLogSettingsClient(t).Set(
		ctx,
		definition,
	); err != nil {
		t.Fatalf("set cPanel log settings: %v", err)
	}
}

func testAccLogSettingsClient(
	t *testing.T,
) *cpanellogmanager.Client {
	t.Helper()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	return cpanellogmanager.NewClient(client)
}
