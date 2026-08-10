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
	_ datasource.DataSource              = &sslInstalledHostsDataSource{}
	_ datasource.DataSourceWithConfigure = &sslInstalledHostsDataSource{}
)

func NewSSLInstalledHostsDataSource() datasource.DataSource {
	return &sslInstalledHostsDataSource{}
}

type sslInstalledHostsDataSource struct {
	client *sslcertificate.Client
}

func (d *sslInstalledHostsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_installed_hosts"
}

func (d *sslInstalledHostsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete installed SSL virtual-host metadata inventory.",
		MarkdownDescription: "Reads the complete installed SSL virtual-host metadata inventory without exposing certificate PEM, document roots, IP addresses, or private-key material.",
		Attributes: map[string]schema.Attribute{
			"hosts": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The installed SSL hosts in stable identity order.",
				MarkdownDescription: "The installed SSL hosts sorted by server name, certificate identifier, and FQDNs.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"servername": schema.StringAttribute{
							Computed:            true,
							Description:         "The installed virtual-host server name.",
							MarkdownDescription: "The installed virtual-host server name.",
						},
						"domains": schema.ListAttribute{
							ElementType:         types.StringType,
							Computed:            true,
							Description:         "The host domains in sorted order, when cPanel reports them.",
							MarkdownDescription: "The host domains in sorted order, or `null` when cPanel's dedicated-IP endpoint does not report them.",
						},
						"fqdns": schema.ListAttribute{
							ElementType:         types.StringType,
							Computed:            true,
							Description:         "The host FQDNs in sorted order, when cPanel reports them.",
							MarkdownDescription: "The host FQDNs in sorted order, or `null` when cPanel's dedicated-IP endpoint does not report them.",
						},
						"is_primary_on_ip": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether cPanel marks the host as primary on its IP, when reported.",
							MarkdownDescription: "Whether cPanel marks the host as primary on its IP, or `null` when the selected endpoint does not report the flag.",
						},
						"mail_sni_status": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether cPanel reports mail SNI as enabled, when reported.",
							MarkdownDescription: "Whether cPanel reports mail SNI as enabled, or `null` when the selected endpoint does not report the flag.",
						},
						"needs_sni": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether the host requires SNI, when reported.",
							MarkdownDescription: "Whether the host requires SNI, or `null` when the selected endpoint does not report the flag.",
						},
						"certificate": schema.SingleNestedAttribute{
							Computed:            true,
							Description:         "Safe public metadata for the installed certificate.",
							MarkdownDescription: "Safe public metadata for the installed certificate.",
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									Computed:            true,
									Description:         "The installed certificate identifier.",
									MarkdownDescription: "The installed certificate identifier.",
								},
								"domains": schema.ListAttribute{
									ElementType:         types.StringType,
									Computed:            true,
									Description:         "The certificate domains in sorted order.",
									MarkdownDescription: "The certificate domains in sorted order.",
								},
								"auto_ssl_provider": schema.StringAttribute{
									Computed:            true,
									Description:         "The AutoSSL provider identifier, when reported.",
									MarkdownDescription: "The AutoSSL provider identifier, when reported.",
								},
								"auto_ssl_provider_display_name": schema.StringAttribute{
									Computed:            true,
									Description:         "The AutoSSL provider display name, when reported.",
									MarkdownDescription: "The AutoSSL provider display name, when reported.",
								},
								"is_autossl": schema.BoolAttribute{
									Computed:            true,
									Description:         "Whether cPanel identifies this as an AutoSSL certificate, when reported.",
									MarkdownDescription: "Whether cPanel identifies this as an AutoSSL certificate, or `null` when the dedicated-IP endpoint does not report the flag.",
								},
								"is_self_signed": schema.BoolAttribute{
									Computed:            true,
									Description:         "Whether cPanel reports the certificate as self-signed.",
									MarkdownDescription: "Whether cPanel reports the certificate as self-signed.",
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
								"signature_algorithm": schema.StringAttribute{
									Computed:            true,
									Description:         "The signature algorithm, when reported.",
									MarkdownDescription: "The signature algorithm, when reported.",
								},
								"modulus_length": schema.Int64Attribute{
									Computed:            true,
									Description:         "The RSA modulus length, or null when not applicable.",
									MarkdownDescription: "The RSA modulus length, or `null` when not applicable.",
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
							},
						},
					},
				},
			},
		},
	}
}

func (d *sslInstalledHostsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	hosts, err := d.client.ListInstalledHosts(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read installed SSL host inventory",
			err.Error(),
		)
		return
	}

	model := SSLInstalledHostsDataSourceModel{
		Hosts: make([]SSLInstalledHostModel, 0, len(hosts)),
	}
	for _, host := range hosts {
		item, diagnostics := sslInstalledHostInventoryModel(ctx, host)
		response.Diagnostics.Append(diagnostics...)
		if response.Diagnostics.HasError() {
			return
		}
		model.Hosts = append(model.Hosts, item)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *sslInstalledHostsDataSource) Configure(
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
