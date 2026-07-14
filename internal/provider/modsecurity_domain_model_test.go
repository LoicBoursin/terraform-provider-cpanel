package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

func TestModSecurityDomainModelMapping(t *testing.T) {
	t.Parallel()

	apiDomain := modsecurity.Domain{
		Domain:       "www.example.test",
		Enabled:      false,
		Dependencies: []string{"example.test"},
		SearchHint:   "example.test",
		Type:         modsecurity.DomainTypeSub,
	}

	model := ModSecurityDomainResourceModel{}
	diagnostics := applyModSecurityDomainToResourceModel(
		t.Context(),
		&model,
		apiDomain,
	)
	if diagnostics.HasError() {
		t.Fatalf("applyModSecurityDomainToResourceModel() diagnostics: %v", diagnostics)
	}
	if model.Domain.ValueString() != apiDomain.Domain ||
		model.Enabled.ValueBool() != apiDomain.Enabled ||
		model.DomainType.ValueString() != apiDomain.Type ||
		model.SearchHint.ValueString() != apiDomain.SearchHint {
		t.Fatalf("resource model = %#v", model)
	}

	var dependencies []string
	diagnostics = model.Dependencies.ElementsAs(
		t.Context(),
		&dependencies,
		false,
	)
	if diagnostics.HasError() ||
		len(dependencies) != 1 ||
		dependencies[0] != "example.test" {
		t.Fatalf("dependencies = %#v, diagnostics = %v", dependencies, diagnostics)
	}

	var affectedDomains []string
	diagnostics = model.AffectedDomains.ElementsAs(
		t.Context(),
		&affectedDomains,
		false,
	)
	if diagnostics.HasError() || len(affectedDomains) != 2 {
		t.Fatalf(
			"affected domains = %#v, diagnostics = %v",
			affectedDomains,
			diagnostics,
		)
	}

	dataSourceModel, diagnostics := modSecurityDomainToDataSourceModel(
		t.Context(),
		apiDomain,
	)
	if diagnostics.HasError() {
		t.Fatalf("modSecurityDomainToDataSourceModel() diagnostics: %v", diagnostics)
	}
	if dataSourceModel.Domain.ValueString() != apiDomain.Domain ||
		dataSourceModel.Enabled.ValueBool() != apiDomain.Enabled {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
