package provider

import (
	"fmt"
	"path"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelversioncontrol "terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

func TestGitRepositoryResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewGitRepositoryResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"name", "repository_root"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required string", attributeName)
		}
	}

	sourceURL, ok := response.Schema.Attributes["source_repository_url"].(resourceschema.StringAttribute)
	if !ok || !sourceURL.Optional || !sourceURL.Computed || !sourceURL.Sensitive {
		t.Fatal("source_repository_url must be optional, computed, and sensitive")
	}

	deleteContents, ok := response.Schema.Attributes["delete_contents_on_destroy"].(resourceschema.BoolAttribute)
	if !ok || !deleteContents.Optional || !deleteContents.Computed {
		t.Fatal("delete_contents_on_destroy must be optional and computed")
	}
}

func TestAccGitRepositoryResource(t *testing.T) {
	const resourceName = "cpanel_git_repository.test"

	initialRoot := testAccGitRepositoryRoot("resource")
	replacementRoot := testAccGitRepositoryRoot("replacement")
	initialName := "Terraform Git initial"
	updatedName := "Terraform Git updated"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			initialRoot,
			replacementRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					initialName,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						initialName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"repository_root",
						initialRoot,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"type",
						"git",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"deployable",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"delete_contents_on_destroy",
						"true",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_repository_root",
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:           initialName,
							RepositoryRoot: initialRoot,
						},
					),
				),
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: initialRoot,
					},
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialRoot,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "repository_root",
				ImportStateVerifyIgnore: []string{
					"delete_contents_on_destroy",
				},
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: initialRoot,
					},
				),
			},
			{
				PreConfig: func() {
					testAccSetGitRepositoryName(
						t,
						initialRoot,
						"Terraform Git external",
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: initialRoot,
					},
				),
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					replacementRoot,
					updatedName,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckGitRepositoryMissing(initialRoot),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:           updatedName,
							RepositoryRoot: replacementRoot,
						},
					),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, replacementRoot)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					replacementRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: replacementRoot,
					},
				),
			},
		},
	})
}

func TestAccGitRepositoryRetainsContentsByDefault(t *testing.T) {
	repositoryRoot := testAccGitRepositoryRoot("retain")
	repositoryName := "Terraform Git retained"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			repositoryRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositoryResourceConfig(
					repositoryRoot,
					repositoryName,
					false,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           repositoryName,
						RepositoryRoot: repositoryRoot,
					},
				),
			},
			{
				Config: `provider "cpanel" {}`,
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           repositoryName,
						RepositoryRoot: repositoryRoot,
					},
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, repositoryRoot)
				},
				Config: `provider "cpanel" {}`,
				Check:  testAccCheckGitRepositoryMissing(repositoryRoot),
			},
		},
	})
}

func TestAccGitRepositorySourceClone(t *testing.T) {
	const (
		resourceName = "cpanel_git_repository.test"
		sourceURL    = "https://github.com/octocat/Hello-World.git"
	)

	repositoryRoot := path.Join(
		testAccGitRepositoryRoot("source-parent"),
		testAccGitRepositoryRoot("source"),
	)
	repositoryName := "Terraform Git source clone"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			repositoryRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					sourceURL,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"source_repository_url",
						sourceURL,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"source_repository_name",
						"origin",
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:                repositoryName,
							RepositoryRoot:      repositoryRoot,
							SourceRepositoryURL: sourceURL,
						},
					),
					testAccCheckDirectoryEntryExists(
						repositoryRoot,
						"README",
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        repositoryRoot,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "repository_root",
				ImportStateVerifyIgnore: []string{
					"branch",
					"delete_contents_on_destroy",
				},
			},
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					sourceURL,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:                repositoryName,
						RepositoryRoot:      repositoryRoot,
						SourceRepositoryURL: sourceURL,
					},
				),
			},
		},
	})
}

func testAccGitRepositoryResourceConfig(
	repositoryRoot string,
	name string,
	deleteContentsOnDestroy bool,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "test" {
  name                       = %q
  repository_root            = %q
  delete_contents_on_destroy = %t
}
`, name, repositoryRoot, deleteContentsOnDestroy)
}

func testAccGitRepositorySourceConfig(
	repositoryRoot string,
	name string,
	sourceURL string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "test" {
  name                       = %q
  repository_root            = %q
  source_repository_url      = %q
  delete_contents_on_destroy = true
}
`, name, repositoryRoot, sourceURL)
}
