package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/gpg"
)

var (
	_ datasource.DataSource              = &gpgPublicKeyDataSource{}
	_ datasource.DataSourceWithConfigure = &gpgPublicKeyDataSource{}
)

func NewGPGPublicKeyDataSource() datasource.DataSource {
	return &gpgPublicKeyDataSource{}
}

type gpgPublicKeyDataSource struct {
	client *gpg.Client
}

func (d *gpgPublicKeyDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_gpg_public_key"
}

func (d *gpgPublicKeyDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up one public OpenPGP key stored by cPanel.",
		MarkdownDescription: "Looks up one public OpenPGP key stored by cPanel. The data source exports only public material and reports whether matching secret metadata exists; it never reads or exports a private key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				Description:         "The uppercase 16-character cPanel GPG public-key identifier.",
				MarkdownDescription: "The uppercase 16-character cPanel GPG public-key identifier.",
				Validators:          gpgPublicIDValidators(),
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				Description:         "The ASCII-armored public OpenPGP key.",
				MarkdownDescription: "The ASCII-armored `PGP PUBLIC KEY BLOCK`.",
			},
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				Description:         "The full uppercase primary-key fingerprint.",
				MarkdownDescription: "The full uppercase 40-character v4 primary-key fingerprint.",
			},
			"content_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The lowercase SHA-256 of the canonical set of decoded public packets.",
				MarkdownDescription: "The lowercase SHA-256 of the canonical set of decoded public packets.",
			},
			"algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The public-key algorithm text reported by cPanel.",
				MarkdownDescription: "The public-key algorithm text reported by cPanel.",
			},
			"bits": schema.Int64Attribute{
				Computed:            true,
				Description:         "The primary public-key length in bits.",
				MarkdownDescription: "The primary public-key length in bits.",
			},
			"created": schema.Int64Attribute{
				Computed:            true,
				Description:         "The key creation time as a Unix timestamp.",
				MarkdownDescription: "The key creation time as a Unix timestamp.",
			},
			"expires": schema.Int64Attribute{
				Computed:            true,
				Description:         "The key expiration time as a Unix timestamp, when present.",
				MarkdownDescription: "The key expiration time as a Unix timestamp, or null when the key does not expire.",
			},
			"user_id": schema.StringAttribute{
				Computed:            true,
				Description:         "The primary user identity reported by cPanel.",
				MarkdownDescription: "The primary user identity reported by cPanel.",
			},
			"has_secret_key": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports a matching secret key.",
				MarkdownDescription: "Whether cPanel reports a matching secret key. No secret key material is read.",
			},
		},
	}
}

func (d *gpgPublicKeyDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config GPGPublicKeyDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}
	id := config.ID.ValueString()
	if err := gpg.ValidatePublicID(id); err != nil {
		response.Diagnostics.AddError(
			"Invalid GPG public key ID",
			err.Error(),
		)
		return
	}

	lookup, warnings, err := d.client.Lookup(ctx, id)
	addGPGWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read GPG public key",
			err.Error(),
		)
		return
	}
	if lookup.Key == nil {
		detail := fmt.Sprintf(
			"GPG public key %q is not stored in cPanel.",
			id,
		)
		if lookup.HasSecretKey {
			detail += " A matching secret-key inventory entry exists, but this data source never reads or exports private material."
		}
		response.Diagnostics.AddError("GPG public key not found", detail)
		return
	}

	model, diagnostics := gpgPublicKeyToDataSourceModel(
		*lookup.Key,
		lookup.HasSecretKey,
	)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, model)...)
}

func (d *gpgPublicKeyDataSource) Configure(
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
	client, ok := providerData["gpg"].(*gpg.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected GPG Client Type",
			fmt.Sprintf(
				"Expected *gpg.Client, got: %T.",
				providerData["gpg"],
			),
		)
		return
	}
	d.client = client
}
