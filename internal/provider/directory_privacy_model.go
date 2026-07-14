package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

type DirectoryPrivacyResourceModel struct {
	Directory         types.String `tfsdk:"directory"`
	AuthName          types.String `tfsdk:"auth_name"`
	AbsoluteDirectory types.String `tfsdk:"absolute_directory"`
	AuthType          types.String `tfsdk:"auth_type"`
	PasswordFile      types.String `tfsdk:"password_file"`
	Protected         types.Bool   `tfsdk:"protected"`
}

type DirectoryPrivacyDataSourceModel struct {
	Directory         types.String `tfsdk:"directory"`
	AuthName          types.String `tfsdk:"auth_name"`
	AbsoluteDirectory types.String `tfsdk:"absolute_directory"`
	AuthType          types.String `tfsdk:"auth_type"`
	PasswordFile      types.String `tfsdk:"password_file"`
	Protected         types.Bool   `tfsdk:"protected"`
}

func applyDirectoryPrivacyToResourceModel(
	model *DirectoryPrivacyResourceModel,
	apiPrivacy directoryprivacy.Privacy,
) {
	model.Directory = types.StringValue(apiPrivacy.Directory)
	model.AuthName = types.StringValue(apiPrivacy.AuthName)
	model.AbsoluteDirectory = types.StringValue(apiPrivacy.AbsoluteDirectory)
	model.AuthType = types.StringValue(apiPrivacy.AuthType)
	model.PasswordFile = types.StringValue(apiPrivacy.PasswordFile)
	model.Protected = types.BoolValue(apiPrivacy.Protected)
}

func directoryPrivacyToDataSourceModel(
	apiPrivacy directoryprivacy.Privacy,
) *DirectoryPrivacyDataSourceModel {
	return &DirectoryPrivacyDataSourceModel{
		Directory:         types.StringValue(apiPrivacy.Directory),
		AuthName:          types.StringValue(apiPrivacy.AuthName),
		AbsoluteDirectory: types.StringValue(apiPrivacy.AbsoluteDirectory),
		AuthType:          types.StringValue(apiPrivacy.AuthType),
		PasswordFile:      types.StringValue(apiPrivacy.PasswordFile),
		Protected:         types.BoolValue(apiPrivacy.Protected),
	}
}

func directoryPrivacyDefinitionFromResourceModel(
	model DirectoryPrivacyResourceModel,
) directoryprivacy.Definition {
	return directoryprivacy.Definition{
		Directory: model.Directory.ValueString(),
		AuthName:  model.AuthName.ValueString(),
	}
}
