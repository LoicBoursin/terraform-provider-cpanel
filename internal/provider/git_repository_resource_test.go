package provider

import (
	"fmt"
	"path"
	"regexp"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

func TestGitSourceRepositoryURLChanged(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		state types.String
		plan  types.String
		want  bool
	}{
		"same URL": {
			state: types.StringValue("https://github.com/octocat/Hello-World.git"),
			plan:  types.StringValue("https://github.com/octocat/Hello-World.git"),
		},
		"different URL": {
			state: types.StringValue("https://github.com/octocat/Hello-World.git"),
			plan:  types.StringValue("https://github.com/octocat/Spoon-Knife.git"),
			want:  true,
		},
		"unknown state": {
			state: types.StringUnknown(),
			plan:  types.StringValue("https://github.com/octocat/Hello-World.git"),
		},
		"unknown plan": {
			state: types.StringValue("https://github.com/octocat/Hello-World.git"),
			plan:  types.StringUnknown(),
		},
		"null to URL": {
			state: types.StringNull(),
			plan:  types.StringValue("https://github.com/octocat/Hello-World.git"),
			want:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := gitSourceRepositoryURLChanged(
				GitRepositoryResourceModel{
					SourceRepositoryURL: test.state,
				},
				GitRepositoryResourceModel{
					SourceRepositoryURL: test.plan,
				},
			)
			if got != test.want {
				t.Fatalf(
					"gitSourceRepositoryURLChanged() = %t, want %t",
					got,
					test.want,
				)
			}
		})
	}
}

func TestGitSourceReplacementRequiresDeletion(t *testing.T) {
	t.Parallel()

	sourceURL := types.StringValue(
		"https://github.com/octocat/Hello-World.git",
	)
	replacementURL := types.StringValue(
		"https://github.com/octocat/Spoon-Knife.git",
	)

	tests := map[string]struct {
		stateRoot types.String
		planRoot  types.String
		stateURL  types.String
		planURL   types.String
		want      bool
	}{
		"same root and changed source": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringValue("repositories/website"),
			stateURL:  sourceURL,
			planURL:   replacementURL,
			want:      true,
		},
		"changed root and changed source": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringValue("repositories/replacement"),
			stateURL:  sourceURL,
			planURL:   replacementURL,
		},
		"same root and same source": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringValue("repositories/website"),
			stateURL:  sourceURL,
			planURL:   sourceURL,
		},
		"unknown planned root remains conservative": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringUnknown(),
			stateURL:  sourceURL,
			planURL:   replacementURL,
			want:      true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := gitSourceReplacementRequiresDeletion(
				GitRepositoryResourceModel{
					RepositoryRoot:      test.stateRoot,
					SourceRepositoryURL: test.stateURL,
				},
				GitRepositoryResourceModel{
					RepositoryRoot:      test.planRoot,
					SourceRepositoryURL: test.planURL,
				},
			)
			if got != test.want {
				t.Fatalf(
					"gitSourceReplacementRequiresDeletion() = %t, want %t",
					got,
					test.want,
				)
			}
		})
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
		resourceName         = "cpanel_git_repository.test"
		sourceURL            = "https://github.com/octocat/Hello-World.git"
		replacementSourceURL = "https://github.com/octocat/Spoon-Knife.git"
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
					true,
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
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:                repositoryName,
						RepositoryRoot:      repositoryRoot,
						SourceRepositoryURL: sourceURL,
					},
				),
			},
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					replacementSourceURL,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"source_repository_url",
						replacementSourceURL,
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:                repositoryName,
							RepositoryRoot:      repositoryRoot,
							SourceRepositoryURL: replacementSourceURL,
						},
					),
				),
			},
		},
	})
}

func TestAccGitRepositorySourceReplacementRequiresDeletion(t *testing.T) {
	const (
		sourceURL            = "https://github.com/octocat/Hello-World.git"
		replacementSourceURL = "https://github.com/octocat/Spoon-Knife.git"
	)

	repositoryRoot := path.Join(
		testAccGitRepositoryRoot("source-guard-parent"),
		testAccGitRepositoryRoot("source-guard"),
	)
	repositoryName := "Terraform Git source guard"

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
					false,
				),
			},
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					replacementSourceURL,
					false,
				),
				ExpectError: regexp.MustCompile(
					"Git source replacement requires prior destructive deletion",
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, repositoryRoot)
				},
				Config: `provider "cpanel" {}`,
			},
		},
	})
}

func TestAccGitRepositorySourceAndRootReplacement(t *testing.T) {
	const (
		sourceURL            = "https://github.com/octocat/Hello-World.git"
		replacementSourceURL = "https://github.com/octocat/Spoon-Knife.git"
	)

	initialRoot := testAccGitRepositoryRoot("source-root-initial")
	replacementRoot := testAccGitRepositoryRoot("source-root-replacement")
	repositoryName := "Terraform Git source and root replacement"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			initialRoot,
			replacementRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositorySourceConfig(
					initialRoot,
					repositoryName,
					sourceURL,
					false,
				),
			},
			{
				Config: testAccGitRepositorySourceConfig(
					replacementRoot,
					repositoryName,
					replacementSourceURL,
					false,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:                repositoryName,
						RepositoryRoot:      replacementRoot,
						SourceRepositoryURL: replacementSourceURL,
					},
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, initialRoot)
					testAccDeleteGitRepository(t, replacementRoot)
				},
				Config: `provider "cpanel" {}`,
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
	deleteContentsOnDestroy bool,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "test" {
  name                       = %q
  repository_root            = %q
  source_repository_url      = %q
  delete_contents_on_destroy = %t
}
`, name, repositoryRoot, sourceURL, deleteContentsOnDestroy)
}
