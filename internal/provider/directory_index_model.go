package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

type DirectoryIndexResourceModel struct {
	Directory         types.String `tfsdk:"directory"`
	Type              types.String `tfsdk:"type"`
	AbsoluteDirectory types.String `tfsdk:"absolute_directory"`
}

type DirectoryIndexDataSourceModel struct {
	Directory         types.String `tfsdk:"directory"`
	Type              types.String `tfsdk:"type"`
	AbsoluteDirectory types.String `tfsdk:"absolute_directory"`
}

func applyDirectoryIndexToResourceModel(
	model *DirectoryIndexResourceModel,
	apiIndex directoryindex.Index,
) {
	model.Directory = types.StringValue(apiIndex.Directory)
	model.Type = types.StringValue(apiIndex.Type)
	model.AbsoluteDirectory = types.StringValue(apiIndex.AbsoluteDirectory)
}

func directoryIndexToDataSourceModel(
	apiIndex directoryindex.Index,
) *DirectoryIndexDataSourceModel {
	return &DirectoryIndexDataSourceModel{
		Directory:         types.StringValue(apiIndex.Directory),
		Type:              types.StringValue(apiIndex.Type),
		AbsoluteDirectory: types.StringValue(apiIndex.AbsoluteDirectory),
	}
}

func directoryIndexDefinitionFromResourceModel(
	model DirectoryIndexResourceModel,
) directoryindex.Definition {
	return directoryindex.Definition{
		Directory: model.Directory.ValueString(),
		Type:      model.Type.ValueString(),
	}
}
