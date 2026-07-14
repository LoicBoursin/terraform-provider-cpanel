package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/ftp"
)

var (
	_ datasource.DataSource              = &ftpAccountDataSource{}
	_ datasource.DataSourceWithConfigure = &ftpAccountDataSource{}
)

func NewFTPAccountDataSource() datasource.DataSource {
	return &ftpAccountDataSource{}
}

type ftpAccountDataSource struct {
	client *ftp.Client
}

func (d *ftpAccountDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ftp_account"
}

func (d *ftpAccountDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel FTP account and its current home directory, quota, and disk usage.",
		MarkdownDescription: "Looks up a cPanel FTP account and its current home directory, quota, and disk usage.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Required:            true,
				Description:         "The complete FTP login in user@domain form.",
				MarkdownDescription: "The complete FTP login in `user@domain` form.",
				Validators:          ftpAccountValidators(),
			},
			"home_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The FTP account home directory relative to the cPanel account home.",
				MarkdownDescription: "The FTP account home directory relative to the cPanel account home.",
			},
			"quota_mib": schema.Int64Attribute{
				Computed:            true,
				Description:         "The configured disk quota in MiB, or 0 when unlimited.",
				MarkdownDescription: "The configured disk quota in MiB, or `0` when unlimited.",
			},
			"disk_used_mib": schema.Float64Attribute{
				Computed:            true,
				Description:         "The disk space currently used by the FTP account, in MiB.",
				MarkdownDescription: "The disk space currently used by the FTP account, in MiB.",
			},
		},
	}
}

func (d *ftpAccountDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config FTPAccountDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, _, err := validateFTPAccountUsername(
		ctx,
		d.client,
		config.Username.ValueString(),
	); err != nil {
		resp.Diagnostics.AddError("Invalid FTP account username", err.Error())
		return
	}

	account, err := d.client.GetAccount(ctx, config.Username.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account", err.Error())
		return
	}
	if account == nil {
		resp.Diagnostics.AddError(
			"FTP account not found",
			fmt.Sprintf("No FTP account named %q exists.", config.Username.ValueString()),
		)
		return
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account quota", err.Error())
		return
	}
	diskUsedMiB, err := account.DiskUsedMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read FTP account disk usage", err.Error())
		return
	}

	state := FTPAccountDataSourceModel{
		Username:      types.StringValue(account.Login),
		HomeDirectory: types.StringValue(account.RelativeDirectory),
		QuotaMiB:      types.Int64Value(quotaMiB),
		DiskUsedMiB:   types.Float64Value(diskUsedMiB),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *ftpAccountDataSource) Configure(
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
			fmt.Sprintf("Expected map[string]interface{}, got: %T.", req.ProviderData),
		)
		return
	}

	client, ok := providerData["ftp"].(*ftp.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected FTP Client Type",
			fmt.Sprintf("Expected *ftp.Client, got: %T.", providerData["ftp"]),
		)
		return
	}

	d.client = client
}
