package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

type ModSecurityDomainResourceModel struct {
	Domain          types.String `tfsdk:"domain"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	DomainType      types.String `tfsdk:"domain_type"`
	Dependencies    types.Set    `tfsdk:"dependencies"`
	AffectedDomains types.Set    `tfsdk:"affected_domains"`
	SearchHint      types.String `tfsdk:"search_hint"`
}

type ModSecurityDomainDataSourceModel struct {
	Domain          types.String `tfsdk:"domain"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	DomainType      types.String `tfsdk:"domain_type"`
	Dependencies    types.Set    `tfsdk:"dependencies"`
	AffectedDomains types.Set    `tfsdk:"affected_domains"`
	SearchHint      types.String `tfsdk:"search_hint"`
}

func applyModSecurityDomainToResourceModel(
	ctx context.Context,
	model *ModSecurityDomainResourceModel,
	apiDomain modsecurity.Domain,
) diag.Diagnostics {
	dependencies, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		apiDomain.Dependencies,
	)
	affectedDomains, affectedDiagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		apiDomain.AffectedDomains(),
	)
	diagnostics.Append(affectedDiagnostics...)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Domain = types.StringValue(apiDomain.Domain)
	model.Enabled = types.BoolValue(apiDomain.Enabled)
	model.DomainType = types.StringValue(apiDomain.Type)
	model.Dependencies = dependencies
	model.AffectedDomains = affectedDomains
	model.SearchHint = types.StringValue(apiDomain.SearchHint)

	return diagnostics
}

func modSecurityDomainToDataSourceModel(
	ctx context.Context,
	apiDomain modsecurity.Domain,
) (*ModSecurityDomainDataSourceModel, diag.Diagnostics) {
	resourceModel := ModSecurityDomainResourceModel{}
	diagnostics := applyModSecurityDomainToResourceModel(
		ctx,
		&resourceModel,
		apiDomain,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &ModSecurityDomainDataSourceModel{
		Domain:          resourceModel.Domain,
		Enabled:         resourceModel.Enabled,
		DomainType:      resourceModel.DomainType,
		Dependencies:    resourceModel.Dependencies,
		AffectedDomains: resourceModel.AffectedDomains,
		SearchHint:      resourceModel.SearchHint,
	}, diagnostics
}

func modSecurityDomainDefinitionFromResourceModel(
	model ModSecurityDomainResourceModel,
) modsecurity.Definition {
	return modsecurity.Definition{
		Domain:  model.Domain.ValueString(),
		Enabled: model.Enabled.ValueBool(),
	}
}

func modSecurityDomainMatchesResourceModel(
	domain modsecurity.Domain,
	model ModSecurityDomainResourceModel,
) bool {
	return domain.Domain == model.Domain.ValueString() &&
		domain.Enabled == model.Enabled.ValueBool()
}
