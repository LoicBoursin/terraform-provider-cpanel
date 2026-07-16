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
	_ datasource.DataSource              = &sslCertificatesDataSource{}
	_ datasource.DataSourceWithConfigure = &sslCertificatesDataSource{}
)

func NewSSLCertificatesDataSource() datasource.DataSource {
	return &sslCertificatesDataSource{}
}

type sslCertificatesDataSource struct {
	client *sslcertificate.Client
}

func (d *sslCertificatesDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_certificates"
}

func (d *sslCertificatesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete stored public SSL certificate metadata inventory.",
		MarkdownDescription: "Reads the complete stored public SSL certificate metadata inventory without exposing certificate PEM or private-key material.",
		Attributes: map[string]schema.Attribute{
			"certificates": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The stored certificates sorted by cPanel identifier.",
				MarkdownDescription: "The stored certificates sorted by cPanel identifier.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: sslCertificateInventoryAttributes(),
				},
			},
		},
	}
}

func sslCertificateInventoryAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			Description:         "The certificate identifier assigned by cPanel.",
			MarkdownDescription: "The certificate identifier assigned by cPanel.",
		},
		"friendly_name": schema.StringAttribute{
			Computed:            true,
			Description:         "The certificate display name.",
			MarkdownDescription: "The certificate display name.",
		},
		"domains": schema.ListAttribute{
			ElementType:         types.StringType,
			Computed:            true,
			Description:         "The certificate domains in sorted order.",
			MarkdownDescription: "The certificate domains in sorted order.",
		},
		"created": schema.Int64Attribute{
			Computed:            true,
			Description:         "The creation time as a Unix timestamp.",
			MarkdownDescription: "The creation time as a Unix timestamp.",
		},
		"not_before": schema.Int64Attribute{
			Computed:            true,
			Description:         "The validity start as a Unix timestamp.",
			MarkdownDescription: "The validity start as a Unix timestamp.",
		},
		"not_after": schema.Int64Attribute{
			Computed:            true,
			Description:         "The validity end as a Unix timestamp.",
			MarkdownDescription: "The validity end as a Unix timestamp.",
		},
		"serial": schema.StringAttribute{
			Computed:            true,
			Description:         "The certificate serial, when reported.",
			MarkdownDescription: "The certificate serial, when reported.",
		},
		"signature_algorithm": schema.StringAttribute{
			Computed:            true,
			Description:         "The signature algorithm, when reported.",
			MarkdownDescription: "The signature algorithm, when reported.",
		},
		"key_algorithm": schema.StringAttribute{
			Computed:            true,
			Description:         "The public-key algorithm, when reported.",
			MarkdownDescription: "The public-key algorithm, when reported.",
		},
		"modulus_length": schema.Int64Attribute{
			Computed:            true,
			Description:         "The RSA modulus length, or null when not applicable.",
			MarkdownDescription: "The RSA modulus length, or `null` when not applicable.",
		},
		"ecdsa_curve_name": schema.StringAttribute{
			Computed:            true,
			Description:         "The ECDSA curve name, or null when not applicable.",
			MarkdownDescription: "The ECDSA curve name, or `null` when not applicable.",
		},
		"is_self_signed": schema.BoolAttribute{
			Computed:            true,
			Description:         "Whether cPanel reports the certificate as self-signed.",
			MarkdownDescription: "Whether cPanel reports the certificate as self-signed.",
		},
		"issuer_common_name": schema.StringAttribute{
			Computed:            true,
			Description:         "The issuer common name, when reported.",
			MarkdownDescription: "The issuer common name, when reported.",
		},
		"subject_common_name": schema.StringAttribute{
			Computed:            true,
			Description:         "The subject common name, when reported.",
			MarkdownDescription: "The subject common name, when reported.",
		},
		"validation_type": schema.StringAttribute{
			Computed:            true,
			Description:         "The validation type, when reported.",
			MarkdownDescription: "The validation type, when reported.",
		},
		"domain_is_configured": schema.BoolAttribute{
			Computed:            true,
			Description:         "Whether cPanel reports a configured domain.",
			MarkdownDescription: "Whether cPanel reports a configured domain.",
		},
		"installed": schema.BoolAttribute{
			Computed:            true,
			Description:         "Whether an installed SSL host references the certificate.",
			MarkdownDescription: "Whether an installed SSL host references the certificate.",
		},
	}
}

func (d *sslCertificatesDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	certificates, err := d.client.List(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read stored SSL certificate inventory",
			err.Error(),
		)
		return
	}
	hosts, err := d.client.ListInstalledHosts(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read installed SSL host inventory",
			err.Error(),
		)
		return
	}
	installedIDs := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		installedIDs[host.Certificate.ID] = struct{}{}
	}

	model := SSLCertificatesDataSourceModel{
		Certificates: make(
			[]SSLCertificateInventoryModel,
			0,
			len(certificates),
		),
	}
	for _, certificate := range certificates {
		_, installed := installedIDs[certificate.ID]
		item, diagnostics := sslCertificateInventoryModel(
			ctx,
			certificate,
			installed,
		)
		response.Diagnostics.Append(diagnostics...)
		if response.Diagnostics.HasError() {
			return
		}
		model.Certificates = append(model.Certificates, item)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *sslCertificatesDataSource) Configure(
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
