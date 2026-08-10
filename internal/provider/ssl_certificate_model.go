package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

type SSLCertificateResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	FriendlyName       types.String `tfsdk:"friendly_name"`
	Certificate        types.String `tfsdk:"certificate"`
	FingerprintSHA256  types.String `tfsdk:"fingerprint_sha256"`
	Domains            types.Set    `tfsdk:"domains"`
	Created            types.Int64  `tfsdk:"created"`
	NotBefore          types.Int64  `tfsdk:"not_before"`
	NotAfter           types.Int64  `tfsdk:"not_after"`
	Serial             types.String `tfsdk:"serial"`
	SignatureAlgorithm types.String `tfsdk:"signature_algorithm"`
	KeyAlgorithm       types.String `tfsdk:"key_algorithm"`
	ModulusLength      types.Int64  `tfsdk:"modulus_length"`
	IsSelfSigned       types.Bool   `tfsdk:"is_self_signed"`
	IssuerCommonName   types.String `tfsdk:"issuer_common_name"`
	SubjectCommonName  types.String `tfsdk:"subject_common_name"`
	ValidationType     types.String `tfsdk:"validation_type"`
	DomainIsConfigured types.Bool   `tfsdk:"domain_is_configured"`
	Installed          types.Bool   `tfsdk:"installed"`
}

type SSLCertificateDataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	FriendlyName       types.String `tfsdk:"friendly_name"`
	Certificate        types.String `tfsdk:"certificate"`
	FingerprintSHA256  types.String `tfsdk:"fingerprint_sha256"`
	Domains            types.Set    `tfsdk:"domains"`
	Created            types.Int64  `tfsdk:"created"`
	NotBefore          types.Int64  `tfsdk:"not_before"`
	NotAfter           types.Int64  `tfsdk:"not_after"`
	Serial             types.String `tfsdk:"serial"`
	SignatureAlgorithm types.String `tfsdk:"signature_algorithm"`
	KeyAlgorithm       types.String `tfsdk:"key_algorithm"`
	ModulusLength      types.Int64  `tfsdk:"modulus_length"`
	IsSelfSigned       types.Bool   `tfsdk:"is_self_signed"`
	IssuerCommonName   types.String `tfsdk:"issuer_common_name"`
	SubjectCommonName  types.String `tfsdk:"subject_common_name"`
	ValidationType     types.String `tfsdk:"validation_type"`
	DomainIsConfigured types.Bool   `tfsdk:"domain_is_configured"`
	Installed          types.Bool   `tfsdk:"installed"`
}

func applySSLCertificateToResourceModel(
	ctx context.Context,
	model *SSLCertificateResourceModel,
	certificate sslcertificate.Certificate,
	installed bool,
) diag.Diagnostics {
	domains, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		certificate.Domains,
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	parsed, err := sslcertificate.ParsePEM(certificate.CertificatePEM)
	if err != nil {
		diagnostics.AddError(
			"Unable to decode SSL certificate",
			err.Error(),
		)

		return diagnostics
	}

	certificateValue := types.StringValue(parsed.NormalizedPEM)
	if !model.Certificate.IsNull() && !model.Certificate.IsUnknown() {
		equal, comparisonErr := sslcertificate.EqualPEM(
			model.Certificate.ValueString(),
			certificate.CertificatePEM,
		)
		if comparisonErr != nil {
			diagnostics.AddError(
				"Unable to compare SSL certificate state",
				comparisonErr.Error(),
			)

			return diagnostics
		}
		if equal {
			certificateValue = model.Certificate
		}
	}

	model.ID = types.StringValue(certificate.ID)
	model.FriendlyName = types.StringValue(certificate.FriendlyName)
	model.Certificate = certificateValue
	model.FingerprintSHA256 = types.StringValue(parsed.SHA256Fingerprint)
	model.Domains = domains
	model.Created = types.Int64Value(certificate.Created)
	model.NotBefore = types.Int64Value(certificate.NotBefore)
	model.NotAfter = types.Int64Value(certificate.NotAfter)
	model.Serial = nullableString(certificate.Serial)
	model.SignatureAlgorithm = nullableString(
		certificate.SignatureAlgorithm,
	)
	model.KeyAlgorithm = nullableString(certificate.KeyAlgorithm)
	model.ModulusLength = types.Int64Value(certificate.ModulusLength)
	model.IsSelfSigned = types.BoolValue(certificate.IsSelfSigned)
	model.IssuerCommonName = nullableString(certificate.IssuerCommonName)
	model.SubjectCommonName = nullableString(certificate.SubjectCommonName)
	model.ValidationType = nullableString(certificate.ValidationType)
	model.DomainIsConfigured = types.BoolValue(
		certificate.DomainIsConfigured,
	)
	model.Installed = types.BoolValue(installed)

	return diagnostics
}

func sslCertificateToDataSourceModel(
	ctx context.Context,
	certificate sslcertificate.Certificate,
	installed bool,
) (*SSLCertificateDataSourceModel, diag.Diagnostics) {
	resourceModel := SSLCertificateResourceModel{}
	diagnostics := applySSLCertificateToResourceModel(
		ctx,
		&resourceModel,
		certificate,
		installed,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &SSLCertificateDataSourceModel{
		ID:                 resourceModel.ID,
		FriendlyName:       resourceModel.FriendlyName,
		Certificate:        resourceModel.Certificate,
		FingerprintSHA256:  resourceModel.FingerprintSHA256,
		Domains:            resourceModel.Domains,
		Created:            resourceModel.Created,
		NotBefore:          resourceModel.NotBefore,
		NotAfter:           resourceModel.NotAfter,
		Serial:             resourceModel.Serial,
		SignatureAlgorithm: resourceModel.SignatureAlgorithm,
		KeyAlgorithm:       resourceModel.KeyAlgorithm,
		ModulusLength:      resourceModel.ModulusLength,
		IsSelfSigned:       resourceModel.IsSelfSigned,
		IssuerCommonName:   resourceModel.IssuerCommonName,
		SubjectCommonName:  resourceModel.SubjectCommonName,
		ValidationType:     resourceModel.ValidationType,
		DomainIsConfigured: resourceModel.DomainIsConfigured,
		Installed:          resourceModel.Installed,
	}, diagnostics
}
