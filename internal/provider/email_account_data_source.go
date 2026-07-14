package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailAccountDataSource{}
	_ datasource.DataSourceWithConfigure = &emailAccountDataSource{}
)

func NewEmailAccountDataSource() datasource.DataSource {
	return &emailAccountDataSource{}
}

type emailAccountDataSource struct {
	client *cpanelmail.Client
}

func (d *emailAccountDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_account"
}

func (d *emailAccountDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up a cPanel email account and its current quota and disk usage.",
		MarkdownDescription: "Looks up a cPanel email account and its current quota and disk usage.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:            true,
				Description:         "The complete email address on a domain owned by the cPanel account.",
				MarkdownDescription: "The complete email address on a domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
			},
			"quota_mib": schema.Int64Attribute{
				Computed:            true,
				Description:         "The configured disk quota in MiB, or 0 when unlimited.",
				MarkdownDescription: "The configured disk quota in MiB, or `0` when unlimited.",
			},
			"disk_used_bytes": schema.Int64Attribute{
				Computed:            true,
				Description:         "The disk space currently used by the email account, in bytes.",
				MarkdownDescription: "The disk space currently used by the email account, in bytes.",
			},
		},
	}
}

func (d *emailAccountDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config EmailAccountDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		d.client,
		config.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid email account address", err.Error())
		return
	}

	account, err := d.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account", err.Error())
		return
	}
	if account == nil {
		resp.Diagnostics.AddError(
			"Email account not found",
			fmt.Sprintf("No email account named %q exists.", config.Email.ValueString()),
		)
		return
	}

	quotaMiB, err := account.QuotaMiB()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account quota", err.Error())
		return
	}
	diskUsedBytes, err := account.DiskUsedBytes()
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email account disk usage", err.Error())
		return
	}

	state := EmailAccountDataSourceModel{
		Email:         types.StringValue(account.Email),
		QuotaMiB:      types.Int64Value(quotaMiB),
		DiskUsedBytes: types.Int64Value(diskUsedBytes),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *emailAccountDataSource) Configure(
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

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf("Expected *email.Client, got: %T.", providerData["email"]),
		)
		return
	}

	d.client = client
}
