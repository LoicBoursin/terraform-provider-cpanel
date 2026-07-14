package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

var (
	_ datasource.DataSource              = &mySQLRemoteHostDataSource{}
	_ datasource.DataSourceWithConfigure = &mySQLRemoteHostDataSource{}
)

func NewMySQLRemoteHostDataSource() datasource.DataSource {
	return &mySQLRemoteHostDataSource{}
}

type mySQLRemoteHostDataSource struct {
	client *mysql.Client
}

func (d *mySQLRemoteHostDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mysql_remote_host"
}

func (d *mySQLRemoteHostDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one remote host authorized to connect to the cPanel account's MySQL or MariaDB databases.",
		MarkdownDescription: "Looks up one remote host authorized to connect to the cPanel account's MySQL or MariaDB databases.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Required:            true,
				Description:         "The IPv4 address, IPv4 CIDR prefix, IPv4 percent-wildcard pattern, or hostname to look up.",
				MarkdownDescription: "The IPv4 address, IPv4 CIDR prefix, IPv4 percent-wildcard pattern, or hostname to look up.",
				Validators:          mySQLRemoteHostValidators(),
			},
			"note": schema.StringAttribute{
				Computed:            true,
				Description:         "The note stored by cPanel for the remote host, or an empty string when none exists.",
				MarkdownDescription: "The note stored by cPanel for the remote host, or an empty string when none exists.",
			},
		},
	}
}

func (d *mySQLRemoteHostDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config MySQLRemoteHostModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, err := mysql.NormalizeRemoteHost(config.Host.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid remote MySQL host", err.Error())
		return
	}

	remoteHost, err := d.client.GetRemoteHost(ctx, host)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read remote MySQL host",
			err.Error(),
		)
		return
	}
	if remoteHost == nil {
		resp.Diagnostics.AddError(
			"Remote MySQL host not found",
			fmt.Sprintf(
				"No remote MySQL host equivalent to %q is authorized.",
				host,
			),
		)
		return
	}

	applyMySQLRemoteHostToModel(&config, *remoteHost)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *mySQLRemoteHostDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["mysql"].(*mysql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected MySQL Client Type",
			fmt.Sprintf(
				"Expected *mysql.Client, got: %T.",
				providerData["mysql"],
			),
		)
		return
	}

	d.client = client
}
