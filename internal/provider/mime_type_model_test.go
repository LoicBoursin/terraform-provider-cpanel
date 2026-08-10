package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

func TestMIMETypeToDataSourceModel(t *testing.T) {
	t.Parallel()

	model, diagnostics := mimeTypeToDataSourceModel(
		t.Context(),
		mimetype.MIMEType{
			Extension: ".foo .bar",
			Origin:    "user",
			Type:      "application/x-example",
		},
	)
	if diagnostics.HasError() {
		t.Fatalf("mimeTypeToDataSourceModel() diagnostics: %v", diagnostics)
	}
	if model == nil {
		t.Fatal("mimeTypeToDataSourceModel() returned nil")
	}
	if model.Type.ValueString() != "application/x-example" ||
		model.Origin.ValueString() != "user" ||
		len(model.Extensions.Elements()) != 2 {
		t.Fatalf("model = %#v", model)
	}
}

func TestMIMETypeDefinitionFromResourceModelSortsExtensions(t *testing.T) {
	t.Parallel()

	extensions, diagnostics := types.SetValueFrom(
		t.Context(),
		types.StringType,
		[]string{".foo", ".bar"},
	)
	if diagnostics.HasError() {
		t.Fatalf("types.SetValueFrom() diagnostics: %v", diagnostics)
	}

	definition, diagnostics := mimeTypeDefinitionFromResourceModel(
		t.Context(),
		MIMETypeResourceModel{
			Type:       types.StringValue("application/x-example"),
			Extensions: extensions,
		},
	)
	if diagnostics.HasError() {
		t.Fatalf(
			"mimeTypeDefinitionFromResourceModel() diagnostics: %v",
			diagnostics,
		)
	}
	if len(definition.Extensions) != 2 ||
		definition.Extensions[0] != ".bar" ||
		definition.Extensions[1] != ".foo" {
		t.Fatalf("extensions = %v", definition.Extensions)
	}
}
