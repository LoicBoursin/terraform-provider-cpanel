package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

func TestGitRepositoryModelMapping(t *testing.T) {
	t.Parallel()

	repository := versioncontrol.Repository{
		Name:                 "Website",
		RepositoryRoot:       "repositories/site",
		AbsoluteRoot:         "/home/example/repositories/site",
		Type:                 "git",
		Branch:               "main",
		AvailableBranches:    []string{"main", "release"},
		ReadWriteCloneURLs:   []string{"ssh://example/site"},
		SourceRepositoryName: "origin",
		SourceRepositoryURL:  "https://example.com/site.git",
		Deployable:           true,
	}

	resourceModel := GitRepositoryResourceModel{}
	diagnostics := applyGitRepositoryToResourceModel(
		t.Context(),
		&resourceModel,
		repository,
	)
	if diagnostics.HasError() {
		t.Fatalf("applyGitRepositoryToResourceModel() diagnostics: %v", diagnostics)
	}
	if resourceModel.Name.ValueString() != repository.Name ||
		resourceModel.RepositoryRoot.ValueString() != repository.RepositoryRoot ||
		resourceModel.SourceRepositoryURL.ValueString() != repository.SourceRepositoryURL ||
		resourceModel.Branch.ValueString() != repository.Branch ||
		!resourceModel.Deployable.ValueBool() {
		t.Fatalf("resource model = %#v", resourceModel)
	}

	dataSourceModel, diagnostics := gitRepositoryToDataSourceModel(
		t.Context(),
		repository,
	)
	if diagnostics.HasError() {
		t.Fatalf("gitRepositoryToDataSourceModel() diagnostics: %v", diagnostics)
	}
	if dataSourceModel.Name.ValueString() != repository.Name ||
		dataSourceModel.RepositoryRoot.ValueString() != repository.RepositoryRoot {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
