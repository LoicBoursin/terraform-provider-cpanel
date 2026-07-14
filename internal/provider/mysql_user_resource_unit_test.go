package provider

import (
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestMySQLUserNameRequiresReplacement(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewMySQLUserResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	name, ok := response.Schema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || len(name.PlanModifiers) == 0 {
		t.Fatal("MySQL user name must require replacement")
	}
}
