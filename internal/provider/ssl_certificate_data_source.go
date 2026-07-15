package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

var (
	_ datasource.DataSource              = &sslCertificateDataSource{}
	_ datasource.DataSourceWithConfigure = &sslCertificateDataSource{}
)

func NewSSLCertificateDataSource() datasource.DataSource {
	return &sslCertificateDataSource{}
}

type sslCertificateDataSource struct {
	client *sslcertificate.Client
}

func (d *sslCertificateDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_certificate"
}

func (d *sslCertificateDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up one stored cPanel SSL certificate.",
		MarkdownDescription: "Looks up one stored cPanel SSL certificate. The data source reads only the public certificate and metadata; it never reads private keys.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				Description:         "The certificate identifier assigned by cPanel.",
				MarkdownDescription: "The certificate identifier assigned by cPanel.",
				Validators:          sslCertificateIDValidators(),
			},
			"friendly_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The display name stored by cPanel for the certificate.",
				MarkdownDescription: "The display name stored by cPanel for the certificate.",
			},
			"certificate": schema.StringAttribute{
				Computed:            true,
				Description:         "The PEM-encoded public X.509 certificate.",
				MarkdownDescription: "The PEM-encoded public X.509 certificate.",
			},
			"fingerprint_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The lowercase SHA-256 fingerprint of the certificate DER bytes.",
				MarkdownDescription: "The lowercase SHA-256 fingerprint of the certificate DER bytes.",
			},
			"domains": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The certificate domains reported by cPanel.",
				MarkdownDescription: "The certificate domains reported by cPanel.",
			},
			"created": schema.Int64Attribute{
				Computed:            true,
				Description:         "The certificate creation time reported by cPanel as a Unix timestamp.",
				MarkdownDescription: "The certificate creation time reported by cPanel as a Unix timestamp.",
			},
			"not_before": schema.Int64Attribute{
				Computed:            true,
				Description:         "The certificate validity start as a Unix timestamp.",
				MarkdownDescription: "The certificate validity start as a Unix timestamp.",
			},
			"not_after": schema.Int64Attribute{
				Computed:            true,
				Description:         "The certificate validity end as a Unix timestamp.",
				MarkdownDescription: "The certificate validity end as a Unix timestamp.",
			},
			"serial": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate serial reported by cPanel.",
				MarkdownDescription: "The certificate serial reported by cPanel.",
			},
			"signature_algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate signature algorithm reported by cPanel.",
				MarkdownDescription: "The certificate signature algorithm reported by cPanel.",
			},
			"key_algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate public-key algorithm reported by cPanel.",
				MarkdownDescription: "The certificate public-key algorithm reported by cPanel.",
			},
			"modulus_length": schema.Int64Attribute{
				Computed:            true,
				Description:         "The public-key modulus length reported by cPanel, when applicable.",
				MarkdownDescription: "The public-key modulus length reported by cPanel, when applicable.",
			},
			"is_self_signed": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the certificate as self-signed.",
				MarkdownDescription: "Whether cPanel reports the certificate as self-signed.",
			},
			"issuer_common_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate issuer common name.",
				MarkdownDescription: "The certificate issuer common name.",
			},
			"subject_common_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate subject common name.",
				MarkdownDescription: "The certificate subject common name.",
			},
			"validation_type": schema.StringAttribute{
				Computed:            true,
				Description:         "The validation type reported by cPanel, when present.",
				MarkdownDescription: "The validation type reported by cPanel, when present.",
			},
			"domain_is_configured": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports a configured domain for the certificate.",
				MarkdownDescription: "Whether cPanel reports a configured domain for the certificate.",
			},
			"installed": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the certificate on an installed SSL virtual host.",
				MarkdownDescription: "Whether cPanel reports the certificate on an installed SSL virtual host.",
			},
		},
	}
}

func (d *sslCertificateDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config SSLCertificateDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if err := validateSSLCertificateID(id); err != nil {
		response.Diagnostics.AddError(
			"Invalid SSL certificate ID",
			err.Error(),
		)
		return
	}

	certificate, err := d.client.Get(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSL certificate",
			err.Error(),
		)
		return
	}
	if certificate == nil {
		response.Diagnostics.AddError(
			"SSL certificate not found",
			fmt.Sprintf(
				"SSL certificate %q is not stored in cPanel.",
				id,
			),
		)
		return
	}

	installed, err := d.client.IsInstalled(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to inspect installed SSL virtual hosts",
			err.Error(),
		)
		return
	}

	model, diagnostics := sslCertificateToDataSourceModel(
		ctx,
		*certificate,
		installed,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, model)...)
}

func (d *sslCertificateDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["sslcertificate"].(*sslcertificate.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SSL Certificate Client Type",
			fmt.Sprintf(
				"Expected *sslcertificate.Client, got: %T.",
				providerData["sslcertificate"],
			),
		)
		return
	}

	d.client = client
}
