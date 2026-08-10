package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelssh "terraform-provider-cpanel/internal/cpanel/ssh"
)

var (
	_ datasource.DataSource              = &sshPublicKeysDataSource{}
	_ datasource.DataSourceWithConfigure = &sshPublicKeysDataSource{}
)

func NewSSHPublicKeysDataSource() datasource.DataSource {
	return &sshPublicKeysDataSource{}
}

type sshPublicKeysDataSource struct {
	client *cpanelssh.Client
}

func (d *sshPublicKeysDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssh_public_keys"
}

func (d *sshPublicKeysDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete public SSH-key metadata inventory stored by cPanel.",
		MarkdownDescription: "Reads the complete public SSH-key metadata inventory stored by cPanel. The provider always selects cPanel's public-key mode, never requests private-key metadata, and does not call any SSH mutation operation.",
		Attributes: map[string]schema.Attribute{
			"keys": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The public SSH keys sorted by base filename.",
				MarkdownDescription: "The public SSH keys sorted by base filename.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							Description:         "The cPanel SSH public-key base filename without .pub.",
							MarkdownDescription: "The cPanel SSH public-key base filename without `.pub`.",
						},
						"authorized": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether cPanel reports the public key as authorized.",
							MarkdownDescription: "Whether cPanel reports the public key as authorized.",
						},
						"created_at": schema.Int64Attribute{
							Computed:            true,
							Description:         "The creation time reported by cPanel as a Unix timestamp.",
							MarkdownDescription: "The creation time reported by cPanel as a Unix timestamp.",
						},
						"modified_at": schema.Int64Attribute{
							Computed:            true,
							Description:         "The last modification time reported by cPanel as a Unix timestamp.",
							MarkdownDescription: "The last modification time reported by cPanel as a Unix timestamp.",
						},
					},
				},
			},
		},
	}
}

func (d *sshPublicKeysDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	keys, err := d.client.List(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel public SSH-key inventory",
			err.Error(),
		)
		return
	}

	model := SSHPublicKeysDataSourceModel{
		Keys: make([]SSHPublicKeyMetadataModel, 0, len(keys)),
	}
	for _, key := range keys {
		model.Keys = append(
			model.Keys,
			sshPublicKeyMetadataToModel(key),
		)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *sshPublicKeysDataSource) Configure(
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
