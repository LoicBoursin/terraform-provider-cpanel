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
	_ datasource.DataSource              = &sslCSRsDataSource{}
	_ datasource.DataSourceWithConfigure = &sslCSRsDataSource{}
)

func NewSSLCSRsDataSource() datasource.DataSource {
	return &sslCSRsDataSource{}
}

type sslCSRsDataSource struct {
	client *sslcsr.Client
}

func (d *sslCSRsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_csrs"
}

func (d *sslCSRsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete stored SSL certificate signing request metadata inventory.",
		MarkdownDescription: "Reads the complete stored SSL certificate signing request metadata inventory without exposing CSR PEM, modulus values, or ECDSA public parameters.",
		Attributes: map[string]schema.Attribute{
			"csrs": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The stored CSRs sorted by cPanel identifier.",
				MarkdownDescription: "The stored CSRs sorted by cPanel identifier.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "The CSR identifier assigned by cPanel.",
							MarkdownDescription: "The CSR identifier assigned by cPanel.",
						},
						"friendly_name": schema.StringAttribute{
							Computed:            true,
							Description:         "The CSR display name.",
							MarkdownDescription: "The CSR display name.",
						},
						"common_name": schema.StringAttribute{
							Computed:            true,
							Description:         "The CSR common name, when reported.",
							MarkdownDescription: "The CSR common name, when reported.",
						},
						"domains": schema.ListAttribute{
							ElementType:         types.StringType,
							Computed:            true,
							Description:         "The CSR domains in sorted order.",
							MarkdownDescription: "The CSR domains in sorted order.",
						},
						"created": schema.Int64Attribute{
							Computed:            true,
							Description:         "The creation time as a Unix timestamp.",
							MarkdownDescription: "The creation time as a Unix timestamp.",
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
					},
				},
			},
		},
	}
}

func (d *sslCSRsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	csrs, err := d.client.ListMetadata(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read stored SSL CSR metadata inventory",
			err.Error(),
		)
		return
	}

	model := SSLCSRsDataSourceModel{
		CSRs: make([]SSLCSRInventoryModel, 0, len(csrs)),
	}
	for _, csr := range csrs {
		item, diagnostics := sslCSRInventoryModel(ctx, csr)
		response.Diagnostics.Append(diagnostics...)
		if response.Diagnostics.HasError() {
			return
		}
		model.CSRs = append(model.CSRs, item)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *sslCSRsDataSource) Configure(
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
