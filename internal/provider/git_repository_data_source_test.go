package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelversioncontrol "terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

func TestAccGitRepositoryDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_git_repository.test"

	repositoryRoot := testAccGitRepositoryRoot("datasource")
	repositoryName := "Terraform Git data source"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			repositoryRoot,
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "fixture" {
  name                       = %q
  repository_root            = %q
  delete_contents_on_destroy = true
}

data "cpanel_git_repository" "test" {
  repository_root = cpanel_git_repository.fixture.repository_root
}
`, repositoryName, repositoryRoot),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"name",
						repositoryName,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"repository_root",
						repositoryRoot,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"type",
						"git",
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"absolute_repository_root",
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:           repositoryName,
							RepositoryRoot: repositoryRoot,
						},
					),
				),
			},
		},
	})
}
