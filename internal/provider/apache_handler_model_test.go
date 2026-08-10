package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

func TestApacheHandlerToDataSourceModel(t *testing.T) {
	t.Parallel()

	model := apacheHandlerToDataSourceModel(
		apachehandler.Handler{
			Extension: ".foo",
			Handler:   "example-handler",
			Origin:    "user",
		},
	)
	if model.Extension.ValueString() != ".foo" ||
		model.Handler.ValueString() != "example-handler" ||
		model.Origin.ValueString() != "user" {
		t.Fatalf("model = %#v", model)
	}
}
