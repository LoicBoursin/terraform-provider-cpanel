package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

type SSLCertificatesDataSourceModel struct {
	Certificates []SSLCertificateInventoryModel `tfsdk:"certificates"`
}

type SSLCertificateInventoryModel struct {
	ID                 types.String `tfsdk:"id"`
	FriendlyName       types.String `tfsdk:"friendly_name"`
	Domains            types.List   `tfsdk:"domains"`
	Created            types.Int64  `tfsdk:"created"`
	NotBefore          types.Int64  `tfsdk:"not_before"`
	NotAfter           types.Int64  `tfsdk:"not_after"`
	Serial             types.String `tfsdk:"serial"`
	SignatureAlgorithm types.String `tfsdk:"signature_algorithm"`
	KeyAlgorithm       types.String `tfsdk:"key_algorithm"`
	ModulusLength      types.Int64  `tfsdk:"modulus_length"`
	ECDSACurveName     types.String `tfsdk:"ecdsa_curve_name"`
	IsSelfSigned       types.Bool   `tfsdk:"is_self_signed"`
	IssuerCommonName   types.String `tfsdk:"issuer_common_name"`
	SubjectCommonName  types.String `tfsdk:"subject_common_name"`
	ValidationType     types.String `tfsdk:"validation_type"`
	DomainIsConfigured types.Bool   `tfsdk:"domain_is_configured"`
	Installed          types.Bool   `tfsdk:"installed"`
}

type SSLCSRsDataSourceModel struct {
	CSRs []SSLCSRInventoryModel `tfsdk:"csrs"`
}

type SSLCSRInventoryModel struct {
	ID             types.String `tfsdk:"id"`
	FriendlyName   types.String `tfsdk:"friendly_name"`
	CommonName     types.String `tfsdk:"common_name"`
	Domains        types.List   `tfsdk:"domains"`
	Created        types.Int64  `tfsdk:"created"`
	KeyAlgorithm   types.String `tfsdk:"key_algorithm"`
	ModulusLength  types.Int64  `tfsdk:"modulus_length"`
	ECDSACurveName types.String `tfsdk:"ecdsa_curve_name"`
}

type SSLKeysDataSourceModel struct {
	Keys []SSLKeyInventoryModel `tfsdk:"keys"`
}

type SSLKeyInventoryModel struct {
	ID             types.String `tfsdk:"id"`
	FriendlyName   types.String `tfsdk:"friendly_name"`
	Created        types.Int64  `tfsdk:"created"`
	KeyAlgorithm   types.String `tfsdk:"key_algorithm"`
	ModulusLength  types.Int64  `tfsdk:"modulus_length"`
	ECDSACurveName types.String `tfsdk:"ecdsa_curve_name"`
}

type SSLInstalledHostsDataSourceModel struct {
	Hosts []SSLInstalledHostModel `tfsdk:"hosts"`
}

type SSLInstalledHostModel struct {
	ServerName    types.String                 `tfsdk:"servername"`
	Domains       types.List                   `tfsdk:"domains"`
	FQDNs         types.List                   `tfsdk:"fqdns"`
	IsPrimaryOnIP types.Bool                   `tfsdk:"is_primary_on_ip"`
	MailSNIStatus types.Bool                   `tfsdk:"mail_sni_status"`
	NeedsSNI      types.Bool                   `tfsdk:"needs_sni"`
	Certificate   SSLInstalledCertificateModel `tfsdk:"certificate"`
}

type SSLInstalledCertificateModel struct {
	ID                         types.String `tfsdk:"id"`
	Domains                    types.List   `tfsdk:"domains"`
	AutoSSLProvider            types.String `tfsdk:"auto_ssl_provider"`
	AutoSSLProviderDisplayName types.String `tfsdk:"auto_ssl_provider_display_name"`
	IsAutoSSL                  types.Bool   `tfsdk:"is_autossl"`
	IsSelfSigned               types.Bool   `tfsdk:"is_self_signed"`
	NotBefore                  types.Int64  `tfsdk:"not_before"`
	NotAfter                   types.Int64  `tfsdk:"not_after"`
	SignatureAlgorithm         types.String `tfsdk:"signature_algorithm"`
	ModulusLength              types.Int64  `tfsdk:"modulus_length"`
	IssuerCommonName           types.String `tfsdk:"issuer_common_name"`
	SubjectCommonName          types.String `tfsdk:"subject_common_name"`
	ValidationType             types.String `tfsdk:"validation_type"`
}

func sslCertificateInventoryModel(
	ctx context.Context,
	certificate sslcertificate.Certificate,
	installed bool,
) (SSLCertificateInventoryModel, diag.Diagnostics) {
	domains, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		certificate.Domains,
	)

	return SSLCertificateInventoryModel{
		ID:                 types.StringValue(certificate.ID),
		FriendlyName:       types.StringValue(certificate.FriendlyName),
		Domains:            domains,
		Created:            types.Int64Value(certificate.Created),
		NotBefore:          types.Int64Value(certificate.NotBefore),
		NotAfter:           types.Int64Value(certificate.NotAfter),
		Serial:             nullableString(certificate.Serial),
		SignatureAlgorithm: nullableString(certificate.SignatureAlgorithm),
		KeyAlgorithm:       nullableString(certificate.KeyAlgorithm),
		ModulusLength: sslInventoryNullablePositiveInt64(
			certificate.ModulusLength,
		),
		ECDSACurveName:     nullableString(certificate.ECDSACurveName),
		IsSelfSigned:       types.BoolValue(certificate.IsSelfSigned),
		IssuerCommonName:   nullableString(certificate.IssuerCommonName),
		SubjectCommonName:  nullableString(certificate.SubjectCommonName),
		ValidationType:     nullableString(certificate.ValidationType),
		DomainIsConfigured: types.BoolValue(certificate.DomainIsConfigured),
		Installed:          types.BoolValue(installed),
	}, diagnostics
}

func sslCSRInventoryModel(
	ctx context.Context,
	csr sslcsr.CSRMetadata,
) (SSLCSRInventoryModel, diag.Diagnostics) {
	domains, diagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		csr.Domains,
	)

	return SSLCSRInventoryModel{
		ID:             types.StringValue(csr.ID),
		FriendlyName:   types.StringValue(csr.FriendlyName),
		CommonName:     nullableString(csr.CommonName),
		Domains:        domains,
		Created:        types.Int64Value(csr.Created),
		KeyAlgorithm:   nullableString(csr.KeyAlgorithm),
		ModulusLength:  sslInventoryNullableInt64(csr.ModulusLength),
		ECDSACurveName: nullableString(csr.ECDSACurveName),
	}, diagnostics
}

func sslKeyInventoryModel(key sslcsr.KeyMetadata) SSLKeyInventoryModel {
	return SSLKeyInventoryModel{
		ID:             types.StringValue(key.ID),
		FriendlyName:   types.StringValue(key.FriendlyName),
		Created:        types.Int64Value(key.Created),
		KeyAlgorithm:   nullableString(key.KeyAlgorithm),
		ModulusLength:  sslInventoryNullableInt64(key.ModulusLength),
		ECDSACurveName: nullableString(key.ECDSACurveName),
	}
}

func sslInstalledHostInventoryModel(
	ctx context.Context,
	host sslcertificate.InstalledHost,
) (SSLInstalledHostModel, diag.Diagnostics) {
	domains, diagnostics := sslInventoryNullableStringList(
		ctx,
		host.Domains,
	)
	fqdns, fqdnsDiagnostics := sslInventoryNullableStringList(
		ctx,
		host.FQDNs,
	)
	diagnostics.Append(fqdnsDiagnostics...)
	certificateDomains, certificateDiagnostics := types.ListValueFrom(
		ctx,
		types.StringType,
		host.Certificate.Domains,
	)
	diagnostics.Append(certificateDiagnostics...)

	return SSLInstalledHostModel{
		ServerName:    types.StringValue(host.ServerName),
		Domains:       domains,
		FQDNs:         fqdns,
		IsPrimaryOnIP: sslInventoryNullableBool(host.IsPrimaryOnIP),
		MailSNIStatus: sslInventoryNullableBool(host.MailSNIStatus),
		NeedsSNI:      sslInventoryNullableBool(host.NeedsSNI),
		Certificate: SSLInstalledCertificateModel{
			ID:              types.StringValue(host.Certificate.ID),
			Domains:         certificateDomains,
			AutoSSLProvider: nullableString(host.Certificate.AutoSSLProvider),
			AutoSSLProviderDisplayName: nullableString(
				host.Certificate.AutoSSLProviderDisplayName,
			),
			IsAutoSSL:    sslInventoryNullableBool(host.Certificate.IsAutoSSL),
			IsSelfSigned: types.BoolValue(host.Certificate.IsSelfSigned),
			NotBefore:    types.Int64Value(host.Certificate.NotBefore),
			NotAfter:     types.Int64Value(host.Certificate.NotAfter),
			SignatureAlgorithm: nullableString(
				host.Certificate.SignatureAlgorithm,
			),
			ModulusLength: sslInventoryNullableInt64(
				host.Certificate.ModulusLength,
			),
			IssuerCommonName: nullableString(
				host.Certificate.IssuerCommonName,
			),
			SubjectCommonName: nullableString(
				host.Certificate.SubjectCommonName,
			),
			ValidationType: nullableString(
				host.Certificate.ValidationType,
			),
		},
	}, diagnostics
}

func sslInventoryNullableStringList(
	ctx context.Context,
	values []string,
) (types.List, diag.Diagnostics) {
	if values == nil {
		return types.ListNull(types.StringType), nil
	}

	return types.ListValueFrom(ctx, types.StringType, values)
}

func sslInventoryNullableBool(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}

	return types.BoolValue(*value)
}

func sslInventoryNullableInt64(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}

	return types.Int64Value(*value)
}

func sslInventoryNullablePositiveInt64(value int64) types.Int64 {
	if value <= 0 {
		return types.Int64Null()
	}

	return types.Int64Value(value)
}
