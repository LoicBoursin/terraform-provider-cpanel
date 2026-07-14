package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

type GitRepositoryResourceModel struct {
	Name                    types.String `tfsdk:"name"`
	RepositoryRoot          types.String `tfsdk:"repository_root"`
	SourceRepositoryURL     types.String `tfsdk:"source_repository_url"`
	DeleteContentsOnDestroy types.Bool   `tfsdk:"delete_contents_on_destroy"`
	AbsoluteRepositoryRoot  types.String `tfsdk:"absolute_repository_root"`
	Type                    types.String `tfsdk:"type"`
	Branch                  types.String `tfsdk:"branch"`
	AvailableBranches       types.Set    `tfsdk:"available_branches"`
	ReadOnlyCloneURLs       types.Set    `tfsdk:"read_only_clone_urls"`
	ReadWriteCloneURLs      types.Set    `tfsdk:"read_write_clone_urls"`
	SourceRepositoryName    types.String `tfsdk:"source_repository_name"`
	Deployable              types.Bool   `tfsdk:"deployable"`
}

type GitRepositoryDataSourceModel struct {
	RepositoryRoot         types.String `tfsdk:"repository_root"`
	Name                   types.String `tfsdk:"name"`
	SourceRepositoryURL    types.String `tfsdk:"source_repository_url"`
	AbsoluteRepositoryRoot types.String `tfsdk:"absolute_repository_root"`
	Type                   types.String `tfsdk:"type"`
	Branch                 types.String `tfsdk:"branch"`
	AvailableBranches      types.Set    `tfsdk:"available_branches"`
	ReadOnlyCloneURLs      types.Set    `tfsdk:"read_only_clone_urls"`
	ReadWriteCloneURLs     types.Set    `tfsdk:"read_write_clone_urls"`
	SourceRepositoryName   types.String `tfsdk:"source_repository_name"`
	Deployable             types.Bool   `tfsdk:"deployable"`
}

func applyGitRepositoryToResourceModel(
	ctx context.Context,
	model *GitRepositoryResourceModel,
	repository versioncontrol.Repository,
) diag.Diagnostics {
	availableBranches, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		repository.AvailableBranches,
	)
	readOnlyCloneURLs, readOnlyDiagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		repository.ReadOnlyCloneURLs,
	)
	diagnostics.Append(readOnlyDiagnostics...)
	readWriteCloneURLs, readWriteDiagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		repository.ReadWriteCloneURLs,
	)
	diagnostics.Append(readWriteDiagnostics...)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Name = types.StringValue(repository.Name)
	model.RepositoryRoot = types.StringValue(repository.RepositoryRoot)
	model.SourceRepositoryURL = nullableString(repository.SourceRepositoryURL)
	model.AbsoluteRepositoryRoot = types.StringValue(repository.AbsoluteRoot)
	model.Type = types.StringValue(repository.Type)
	model.Branch = nullableString(repository.Branch)
	model.AvailableBranches = availableBranches
	model.ReadOnlyCloneURLs = readOnlyCloneURLs
	model.ReadWriteCloneURLs = readWriteCloneURLs
	model.SourceRepositoryName = nullableString(
		repository.SourceRepositoryName,
	)
	model.Deployable = types.BoolValue(repository.Deployable)

	return diagnostics
}

func gitRepositoryToDataSourceModel(
	ctx context.Context,
	repository versioncontrol.Repository,
) (*GitRepositoryDataSourceModel, diag.Diagnostics) {
	resourceModel := GitRepositoryResourceModel{}
	diagnostics := applyGitRepositoryToResourceModel(
		ctx,
		&resourceModel,
		repository,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &GitRepositoryDataSourceModel{
		RepositoryRoot:         resourceModel.RepositoryRoot,
		Name:                   resourceModel.Name,
		SourceRepositoryURL:    resourceModel.SourceRepositoryURL,
		AbsoluteRepositoryRoot: resourceModel.AbsoluteRepositoryRoot,
		Type:                   resourceModel.Type,
		Branch:                 resourceModel.Branch,
		AvailableBranches:      resourceModel.AvailableBranches,
		ReadOnlyCloneURLs:      resourceModel.ReadOnlyCloneURLs,
		ReadWriteCloneURLs:     resourceModel.ReadWriteCloneURLs,
		SourceRepositoryName:   resourceModel.SourceRepositoryName,
		Deployable:             resourceModel.Deployable,
	}, diagnostics
}

func gitRepositoryDefinitionFromResourceModel(
	model GitRepositoryResourceModel,
) versioncontrol.Definition {
	sourceRepositoryURL := ""
	if !model.SourceRepositoryURL.IsNull() &&
		!model.SourceRepositoryURL.IsUnknown() {
		sourceRepositoryURL = model.SourceRepositoryURL.ValueString()
	}

	return versioncontrol.Definition{
		Name:                model.Name.ValueString(),
		RepositoryRoot:      model.RepositoryRoot.ValueString(),
		SourceRepositoryURL: sourceRepositoryURL,
	}
}

func nullableString(value string) types.String {
	if value == "" {
		return types.StringNull()
	}

	return types.StringValue(value)
}
