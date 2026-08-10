package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelssh "terraform-provider-cpanel/internal/cpanel/ssh"
)

var (
	_ datasource.DataSource              = &sshPublicKeyDataSource{}
	_ datasource.DataSourceWithConfigure = &sshPublicKeyDataSource{}
)

func NewSSHPublicKeyDataSource() datasource.DataSource {
	return &sshPublicKeyDataSource{}
}

type sshPublicKeyDataSource struct {
	client *cpanelssh.Client
}

func (d *sshPublicKeyDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssh_public_key"
}

func (d *sshPublicKeyDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up one public OpenSSH key stored by cPanel.",
		MarkdownDescription: "Looks up one public OpenSSH key stored by cPanel. The provider always selects cPanel's public-key mode and never reads private key material.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel SSH public-key base filename without .pub.",
				MarkdownDescription: "The cPanel SSH public-key base filename without `.pub`.",
				Validators:          sshPublicKeyNameValidators(),
			},
			"public_key": schema.StringAttribute{
				Computed:            true,
				Description:         "The canonical OpenSSH public key without its comment.",
				MarkdownDescription: "The canonical OpenSSH public key without its comment.",
			},
			"authorized": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the public key as authorized.",
				MarkdownDescription: "Whether cPanel reports the public key as authorized.",
			},
			"fingerprint_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The OpenSSH SHA-256 public-key fingerprint.",
				MarkdownDescription: "The OpenSSH SHA-256 public-key fingerprint.",
			},
			"key_type": schema.StringAttribute{
				Computed:            true,
				Description:         "The canonical OpenSSH public-key type.",
				MarkdownDescription: "The canonical OpenSSH public-key type.",
			},
			"created_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The public-key creation time reported by cPanel as a Unix timestamp.",
				MarkdownDescription: "The public-key creation time reported by cPanel as a Unix timestamp.",
			},
			"modified_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The public-key modification time reported by cPanel as a Unix timestamp.",
				MarkdownDescription: "The public-key modification time reported by cPanel as a Unix timestamp.",
			},
		},
	}
}

func (d *sshPublicKeyDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config SSHPublicKeyDataSourceModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	name := config.Name.ValueString()
	key, err := d.client.Get(ctx, name)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSH public key",
			err.Error(),
		)
		return
	}
	if key == nil {
		response.Diagnostics.AddError(
			"SSH public key not found",
			fmt.Sprintf(
				"SSH public key %q is not stored in cPanel.",
				name,
			),
		)
		return
	}
	response.Diagnostics.Append(
		response.State.Set(
			ctx,
			sshPublicKeyToDataSourceModel(*key),
		)...,
	)
}

func (d *sshPublicKeyDataSource) Configure(
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
	client, ok := providerData["ssh"].(*cpanelssh.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SSH Client Type",
			fmt.Sprintf(
				"Expected *ssh.Client, got: %T.",
				providerData["ssh"],
			),
		)
		return
	}
	d.client = client
}
