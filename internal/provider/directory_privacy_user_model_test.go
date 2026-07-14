package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestDirectoryPrivacyUserModelMapping(t *testing.T) {
	t.Parallel()

	apiUser := directoryprivacy.User{
		Directory:         "public_html/private",
		AbsoluteDirectory: "/home/example/public_html/private",
		Username:          "alice",
	}

	resourceModel := DirectoryPrivacyUserResourceModel{
		Password: types.StringValue("Secret-2026!"),
	}
	applyDirectoryPrivacyUserToResourceModel(&resourceModel, apiUser)
	if resourceModel.Directory.ValueString() != apiUser.Directory ||
		resourceModel.Username.ValueString() != apiUser.Username ||
		resourceModel.AbsoluteDirectory.ValueString() != apiUser.AbsoluteDirectory ||
		resourceModel.Password.ValueString() != "Secret-2026!" {
		t.Fatalf("resource model = %#v", resourceModel)
	}

	dataSourceModel := directoryPrivacyUserToDataSourceModel(apiUser)
	if dataSourceModel.Directory.ValueString() != apiUser.Directory ||
		dataSourceModel.Username.ValueString() != apiUser.Username ||
		dataSourceModel.AbsoluteDirectory.ValueString() != apiUser.AbsoluteDirectory {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
