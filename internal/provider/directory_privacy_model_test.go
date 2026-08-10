package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestDirectoryPrivacyModelMapping(t *testing.T) {
	t.Parallel()

	apiPrivacy := directoryprivacy.Privacy{
		Directory:         "public_html/private",
		AbsoluteDirectory: "/home/example/public_html/private",
		AuthName:          "Private files",
		AuthType:          "Basic",
		PasswordFile:      "/home/example/.htpasswds/public_html/private/passwd",
		Protected:         true,
	}

	model := DirectoryPrivacyResourceModel{}
	applyDirectoryPrivacyToResourceModel(&model, apiPrivacy)
	if model.Directory.ValueString() != apiPrivacy.Directory ||
		model.AuthName.ValueString() != apiPrivacy.AuthName ||
		model.AuthType.ValueString() != apiPrivacy.AuthType ||
		model.PasswordFile.ValueString() != apiPrivacy.PasswordFile ||
		!model.Protected.ValueBool() {
		t.Fatalf("resource model = %#v", model)
	}

	dataSourceModel := directoryPrivacyToDataSourceModel(apiPrivacy)
	if dataSourceModel.Directory.ValueString() != apiPrivacy.Directory ||
		dataSourceModel.AuthName.ValueString() != apiPrivacy.AuthName ||
		!dataSourceModel.Protected.ValueBool() {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
