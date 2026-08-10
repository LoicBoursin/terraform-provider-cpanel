package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

type FilesystemDirectoryResourceModel struct {
	Path         types.String `tfsdk:"path"`
	AbsolutePath types.String `tfsdk:"absolute_path"`
	Permissions  types.String `tfsdk:"permissions"`
	Owned        types.Bool   `tfsdk:"owned"`
}

type FilesystemDirectoryDataSourceModel struct {
	Path         types.String `tfsdk:"path"`
	AbsolutePath types.String `tfsdk:"absolute_path"`
	Permissions  types.String `tfsdk:"permissions"`
}

func applyFilesystemDirectoryToResourceModel(
	model *FilesystemDirectoryResourceModel,
	directory fileman.Directory,
	owned bool,
) {
	model.Path = types.StringValue(directory.Entry.Path)
	model.AbsolutePath = types.StringValue(directory.Entry.AbsolutePath)
	model.Permissions = types.StringValue(directory.Entry.Permissions)
	model.Owned = types.BoolValue(owned)
}

func filesystemDirectoryToDataSourceModel(
	directory fileman.Directory,
) *FilesystemDirectoryDataSourceModel {
	return &FilesystemDirectoryDataSourceModel{
		Path:         types.StringValue(directory.Entry.Path),
		AbsolutePath: types.StringValue(directory.Entry.AbsolutePath),
		Permissions:  types.StringValue(directory.Entry.Permissions),
	}
}

func filesystemDirectoryMayOwn(owned types.Bool) bool {
	return !owned.IsNull() &&
		!owned.IsUnknown() &&
		owned.ValueBool()
}
