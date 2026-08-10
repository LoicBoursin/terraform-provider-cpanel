package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

func TestDirectoryIndexModelMapping(t *testing.T) {
	t.Parallel()

	apiIndex := directoryindex.Index{
		Directory:         "public_html/downloads",
		AbsoluteDirectory: "/home/example/public_html/downloads",
		Type:              directoryindex.IndexTypeDisabled,
	}

	model := DirectoryIndexResourceModel{}
	applyDirectoryIndexToResourceModel(&model, apiIndex)
	if model.Directory.ValueString() != apiIndex.Directory ||
		model.AbsoluteDirectory.ValueString() != apiIndex.AbsoluteDirectory ||
		model.Type.ValueString() != apiIndex.Type {
		t.Fatalf("resource model = %#v", model)
	}

	dataSourceModel := directoryIndexToDataSourceModel(apiIndex)
	if dataSourceModel.Directory.ValueString() != apiIndex.Directory ||
		dataSourceModel.AbsoluteDirectory.ValueString() != apiIndex.AbsoluteDirectory ||
		dataSourceModel.Type.ValueString() != apiIndex.Type {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
