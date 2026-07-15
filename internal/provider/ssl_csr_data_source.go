package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

var (
	_ datasource.DataSource              = &sslCSRDataSource{}
	_ datasource.DataSourceWithConfigure = &sslCSRDataSource{}
)

func NewSSLCSRDataSource() datasource.DataSource {
	return &sslCSRDataSource{}
}

type sslCSRDataSource struct {
	client *sslcsr.Client
}

func (d *sslCSRDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_csr"
}

func (d *sslCSRDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up one public certificate signing request stored by cPanel.",
		MarkdownDescription: "Looks up one public certificate signing request stored by cPanel. The data source reads only signed public PKCS#10 material and metadata; it never reads private keys.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				Description:         "The CSR identifier assigned by cPanel.",
				MarkdownDescription: "The CSR identifier assigned by cPanel.",
				Validators:          sslCSRIDValidators(),
			},
			"friendly_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The display name stored by cPanel for the CSR.",
				MarkdownDescription: "The display name stored by cPanel for the CSR.",
			},
			"domains": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The CSR domains with the common name first.",
				MarkdownDescription: "The CSR domains with the common name first.",
			},
			"country_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The two-letter country code.",
				MarkdownDescription: "The two-letter country code.",
			},
			"state_or_province_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate subject state or province.",
				MarkdownDescription: "The certificate subject state or province.",
			},
			"locality_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate subject city or locality.",
				MarkdownDescription: "The certificate subject city or locality.",
			},
			"organization_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate subject organization.",
				MarkdownDescription: "The certificate subject organization.",
			},
			"organizational_unit_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The optional certificate subject organizational unit.",
				MarkdownDescription: "The optional certificate subject organizational unit.",
			},
			"email_address": schema.StringAttribute{
				Computed:            true,
				Description:         "The optional certificate subject email address.",
				MarkdownDescription: "The optional certificate subject email address.",
			},
			"csr": schema.StringAttribute{
				Computed:            true,
				Description:         "The canonical PEM-encoded public PKCS#10 certificate signing request.",
				MarkdownDescription: "The canonical PEM-encoded public PKCS#10 certificate signing request.",
			},
			"fingerprint_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The lowercase SHA-256 fingerprint of the PKCS#10 DER bytes.",
				MarkdownDescription: "The lowercase SHA-256 fingerprint of the PKCS#10 DER bytes.",
			},
			"common_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The CSR common name.",
				MarkdownDescription: "The CSR common name.",
			},
			"created": schema.Int64Attribute{
				Computed:            true,
				Description:         "The CSR creation time reported by cPanel as a Unix timestamp.",
				MarkdownDescription: "The CSR creation time reported by cPanel as a Unix timestamp.",
			},
			"key_algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The CSR public-key algorithm reported by cPanel.",
				MarkdownDescription: "The CSR public-key algorithm reported by cPanel.",
			},
			"modulus": schema.StringAttribute{
				Computed:            true,
				Description:         "The RSA public modulus reported by cPanel, when applicable.",
				MarkdownDescription: "The RSA public modulus reported by cPanel, when applicable.",
			},
			"ecdsa_curve_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The ECDSA curve name reported by cPanel, when applicable.",
				MarkdownDescription: "The ECDSA curve name reported by cPanel, when applicable.",
			},
			"ecdsa_public": schema.StringAttribute{
				Computed:            true,
				Description:         "The ECDSA public point reported by cPanel, when applicable.",
				MarkdownDescription: "The ECDSA public point reported by cPanel, when applicable.",
			},
		},
	}
}

func (d *sslCSRDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config SSLCSRDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if err := sslcsr.ValidateID(id); err != nil {
		response.Diagnostics.AddError("Invalid SSL CSR ID", err.Error())
		return
	}
	current, err := d.client.Get(ctx, id)
	if err != nil {
		response.Diagnostics.AddError("Unable to read SSL CSR", err.Error())
		return
	}
	if current == nil {
		response.Diagnostics.AddError(
			"SSL CSR not found",
			fmt.Sprintf("SSL CSR %q is not stored in cPanel.", id),
		)
		return
	}

	model, diagnostics := sslCSRToDataSourceModel(ctx, *current)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, model)...)
}

func (d *sslCSRDataSource) Configure(
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

	client, ok := providerData["sslcsr"].(*sslcsr.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SSL CSR Client Type",
			fmt.Sprintf(
				"Expected *sslcsr.Client, got: %T.",
				providerData["sslcsr"],
			),
		)
		return
	}

	d.client = client
}
