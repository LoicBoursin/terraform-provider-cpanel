package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

type DirectoryPrivacyUserResourceModel struct {
	Directory         types.String `tfsdk:"directory"`
	Username          types.String `tfsdk:"username"`
	Password          types.String `tfsdk:"password"`
	PasswordVersion   types.Int64  `tfsdk:"password_version"`
	DeleteOnDestroy   types.Bool   `tfsdk:"delete_on_destroy"`
	AbsoluteDirectory types.String `tfsdk:"absolute_directory"`
}

type DirectoryPrivacyUserDataSourceModel struct {
	Directory         types.String `tfsdk:"directory"`
	Username          types.String `tfsdk:"username"`
	AbsoluteDirectory types.String `tfsdk:"absolute_directory"`
}

func applyDirectoryPrivacyUserToResourceModel(
	model *DirectoryPrivacyUserResourceModel,
	apiUser directoryprivacy.User,
) {
	model.Directory = types.StringValue(apiUser.Directory)
	model.Username = types.StringValue(apiUser.Username)
	model.AbsoluteDirectory = types.StringValue(apiUser.AbsoluteDirectory)
}

func directoryPrivacyUserToDataSourceModel(
	apiUser directoryprivacy.User,
) *DirectoryPrivacyUserDataSourceModel {
	return &DirectoryPrivacyUserDataSourceModel{
		Directory:         types.StringValue(apiUser.Directory),
		Username:          types.StringValue(apiUser.Username),
		AbsoluteDirectory: types.StringValue(apiUser.AbsoluteDirectory),
	}
}

func directoryPrivacyUserDefinitionFromResourceModel(
	model DirectoryPrivacyUserResourceModel,
	password string,
) directoryprivacy.UserDefinition {
	return directoryprivacy.UserDefinition{
		Directory: model.Directory.ValueString(),
		Username:  model.Username.ValueString(),
		Password:  password,
	}
}
