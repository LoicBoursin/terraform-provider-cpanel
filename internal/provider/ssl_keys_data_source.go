package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

var (
	_ datasource.DataSource              = &sslKeysDataSource{}
	_ datasource.DataSourceWithConfigure = &sslKeysDataSource{}
)

func NewSSLKeysDataSource() datasource.DataSource {
	return &sslKeysDataSource{}
}

type sslKeysDataSource struct {
	client *sslcsr.Client
}

func (d *sslKeysDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_keys"
}

func (d *sslKeysDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete stored SSL key metadata inventory.",
		MarkdownDescription: "Reads the complete stored SSL key metadata inventory without exposing modulus values, ECDSA public parameters, or private-key material.",
		Attributes: map[string]schema.Attribute{
			"keys": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The stored SSL keys sorted by cPanel identifier.",
				MarkdownDescription: "The stored SSL keys sorted by cPanel identifier.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							Description:         "The key identifier assigned by cPanel.",
							MarkdownDescription: "The key identifier assigned by cPanel.",
						},
						"friendly_name": schema.StringAttribute{
							Computed:            true,
							Description:         "The key display name.",
							MarkdownDescription: "The key display name.",
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

func (d *sslKeysDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	keys, err := d.client.ListKeys(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read stored SSL key metadata inventory",
			err.Error(),
		)
		return
	}

	model := SSLKeysDataSourceModel{
		Keys: make([]SSLKeyInventoryModel, 0, len(keys)),
	}
	for _, key := range keys {
		model.Keys = append(model.Keys, sslKeyInventoryModel(key))
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *sslKeysDataSource) Configure(
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
