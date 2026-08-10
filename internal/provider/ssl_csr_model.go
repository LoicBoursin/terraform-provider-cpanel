package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

type SSLCSRResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	KeyID                  types.String `tfsdk:"key_id"`
	FriendlyName           types.String `tfsdk:"friendly_name"`
	Domains                types.List   `tfsdk:"domains"`
	CountryName            types.String `tfsdk:"country_name"`
	StateOrProvinceName    types.String `tfsdk:"state_or_province_name"`
	LocalityName           types.String `tfsdk:"locality_name"`
	OrganizationName       types.String `tfsdk:"organization_name"`
	OrganizationalUnitName types.String `tfsdk:"organizational_unit_name"`
	EmailAddress           types.String `tfsdk:"email_address"`
	CSR                    types.String `tfsdk:"csr"`
	FingerprintSHA256      types.String `tfsdk:"fingerprint_sha256"`
	CommonName             types.String `tfsdk:"common_name"`
	Created                types.Int64  `tfsdk:"created"`
	KeyAlgorithm           types.String `tfsdk:"key_algorithm"`
	Modulus                types.String `tfsdk:"modulus"`
	ECDSACurveName         types.String `tfsdk:"ecdsa_curve_name"`
	ECDSAPublic            types.String `tfsdk:"ecdsa_public"`
}

type SSLCSRDataSourceModel struct {
	ID                     types.String `tfsdk:"id"`
	FriendlyName           types.String `tfsdk:"friendly_name"`
	Domains                types.List   `tfsdk:"domains"`
	CountryName            types.String `tfsdk:"country_name"`
	StateOrProvinceName    types.String `tfsdk:"state_or_province_name"`
	LocalityName           types.String `tfsdk:"locality_name"`
	OrganizationName       types.String `tfsdk:"organization_name"`
	OrganizationalUnitName types.String `tfsdk:"organizational_unit_name"`
	EmailAddress           types.String `tfsdk:"email_address"`
	CSR                    types.String `tfsdk:"csr"`
	FingerprintSHA256      types.String `tfsdk:"fingerprint_sha256"`
	CommonName             types.String `tfsdk:"common_name"`
	Created                types.Int64  `tfsdk:"created"`
	KeyAlgorithm           types.String `tfsdk:"key_algorithm"`
	Modulus                types.String `tfsdk:"modulus"`
	ECDSACurveName         types.String `tfsdk:"ecdsa_curve_name"`
	ECDSAPublic            types.String `tfsdk:"ecdsa_public"`
}

func sslCSRDefinitionFromResourceModel(
	ctx context.Context,
	model SSLCSRResourceModel,
) (sslcsr.Definition, diag.Diagnostics) {
	var domains []string
	diagnostics := model.Domains.ElementsAs(ctx, &domains, false)
	if diagnostics.HasError() {
		return sslcsr.Definition{}, diagnostics
	}

	definition := sslcsr.Definition{
		KeyID:                  model.KeyID.ValueString(),
		FriendlyName:           model.FriendlyName.ValueString(),
		Domains:                domains,
		CountryName:            model.CountryName.ValueString(),
		StateOrProvinceName:    model.StateOrProvinceName.ValueString(),
		LocalityName:           model.LocalityName.ValueString(),
		OrganizationName:       model.OrganizationName.ValueString(),
		OrganizationalUnitName: model.OrganizationalUnitName.ValueString(),
		EmailAddress:           model.EmailAddress.ValueString(),
	}
	if err := sslcsr.ValidateDefinition(definition); err != nil {
		diagnostics.AddError("Invalid SSL CSR", err.Error())
	}

	return definition, diagnostics
}

func applySSLCSRToResourceModel(
	ctx context.Context,
	model *SSLCSRResourceModel,
	csr sslcsr.CSR,
) diag.Diagnostics {
	domains := sslCSRDomainsForState(csr)
	domainValue, diagnostics := sslCSRDomainValue(
		ctx,
		model.Domains,
		domains,
		csr.CommonName,
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.ID = types.StringValue(csr.ID)
	model.FriendlyName = types.StringValue(csr.FriendlyName)
	model.Domains = domainValue
	model.CountryName = types.StringValue(csr.CountryName)
	model.StateOrProvinceName = types.StringValue(
		csr.StateOrProvinceName,
	)
	model.LocalityName = types.StringValue(csr.LocalityName)
	model.OrganizationName = types.StringValue(csr.OrganizationName)
	model.OrganizationalUnitName = nullableString(
		csr.OrganizationalUnitName,
	)
	model.EmailAddress = nullableString(csr.EmailAddress)
	model.CSR = types.StringValue(csr.CSRPEM)
	model.FingerprintSHA256 = types.StringValue(csr.FingerprintSHA256)
	model.CommonName = types.StringValue(csr.CommonName)
	model.Created = types.Int64Value(csr.Created)
	model.KeyAlgorithm = types.StringValue(csr.KeyAlgorithm)
	model.Modulus = nullableString(csr.Modulus)
	model.ECDSACurveName = nullableString(csr.ECDSACurveName)
	model.ECDSAPublic = nullableString(csr.ECDSAPublic)

	return diagnostics
}

func sslCSRToDataSourceModel(
	ctx context.Context,
	csr sslcsr.CSR,
) (*SSLCSRDataSourceModel, diag.Diagnostics) {
	resourceModel := SSLCSRResourceModel{
		KeyID: types.StringNull(),
	}
	diagnostics := applySSLCSRToResourceModel(
		ctx,
		&resourceModel,
		csr,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &SSLCSRDataSourceModel{
		ID:                     resourceModel.ID,
		FriendlyName:           resourceModel.FriendlyName,
		Domains:                resourceModel.Domains,
		CountryName:            resourceModel.CountryName,
		StateOrProvinceName:    resourceModel.StateOrProvinceName,
		LocalityName:           resourceModel.LocalityName,
		OrganizationName:       resourceModel.OrganizationName,
		OrganizationalUnitName: resourceModel.OrganizationalUnitName,
		EmailAddress:           resourceModel.EmailAddress,
		CSR:                    resourceModel.CSR,
		FingerprintSHA256:      resourceModel.FingerprintSHA256,
		CommonName:             resourceModel.CommonName,
		Created:                resourceModel.Created,
		KeyAlgorithm:           resourceModel.KeyAlgorithm,
		Modulus:                resourceModel.Modulus,
		ECDSACurveName:         resourceModel.ECDSACurveName,
		ECDSAPublic:            resourceModel.ECDSAPublic,
	}, diagnostics
}

func sslCSRDomainValue(
	ctx context.Context,
	current types.List,
	canonical []string,
	commonName string,
) (types.List, diag.Diagnostics) {
	if !current.IsNull() && !current.IsUnknown() {
		var configured []string
		diagnostics := current.ElementsAs(ctx, &configured, false)
		if diagnostics.HasError() {
			return types.ListNull(types.StringType), diagnostics
		}
		if len(configured) > 0 &&
			configured[0] == commonName &&
			sslCSRDomainSetsEqual(configured, canonical) {
			return current, nil
		}
	}

	return types.ListValueFrom(ctx, types.StringType, canonical)
}

func sslCSRDomainsForState(csr sslcsr.CSR) []string {
	domains := make([]string, 0, len(csr.Domains))
	domains = append(domains, csr.CommonName)
	for _, domain := range csr.Domains {
		if domain != csr.CommonName {
			domains = append(domains, domain)
		}
	}

	return domains
}

func sslCSRDomainSetsEqual(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}

	firstCopy := append([]string(nil), first...)
	secondCopy := append([]string(nil), second...)
	slices.Sort(firstCopy)
	slices.Sort(secondCopy)

	return slices.Equal(firstCopy, secondCopy)
}

func sslCSRIdentityFromResourceModel(
	model SSLCSRResourceModel,
) (sslcsr.Identity, error) {
	identity := sslcsr.Identity{
		ID:                model.ID.ValueString(),
		FingerprintSHA256: model.FingerprintSHA256.ValueString(),
	}
	if err := sslcsr.ValidateIdentity(identity); err != nil {
		return sslcsr.Identity{}, fmt.Errorf(
			"invalid SSL CSR resource identity: %w",
			err,
		)
	}

	return identity, nil
}
