package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/ipblock"
)

var (
	_ datasource.DataSource              = &ipBlockDataSource{}
	_ datasource.DataSourceWithConfigure = &ipBlockDataSource{}
)

func NewIPBlockDataSource() datasource.DataSource {
	return &ipBlockDataSource{}
}

type ipBlockDataSource struct {
	client *ipblock.Client
}

func (d *ipBlockDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ip_block"
}

func (d *ipBlockDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one cPanel IP address, CIDR prefix, or address-range block.",
		MarkdownDescription: "Looks up one cPanel IP address, CIDR prefix, or address-range block.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Required:            true,
				Description:         "The IP address, CIDR prefix, or start-end range to look up.",
				MarkdownDescription: "The IP address, CIDR prefix, or `start-end` range to look up.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(2, 255),
				},
			},
			"start_address": schema.StringAttribute{
				Computed:            true,
				Description:         "The normalized first address in the blocked range.",
				MarkdownDescription: "The normalized first address in the blocked range.",
			},
			"end_address": schema.StringAttribute{
				Computed:            true,
				Description:         "The normalized last address in the blocked range.",
				MarkdownDescription: "The normalized last address in the blocked range.",
			},
		},
	}
}

func (d *ipBlockDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config IPBlockModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address, err := ipblock.NormalizeAddress(config.Address.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid blocked address", err.Error())
		return
	}

	blockedAddress, err := d.client.GetAddress(ctx, address)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read IP block", err.Error())
		return
	}
	if blockedAddress == nil {
		resp.Diagnostics.AddError(
			"IP block not found",
			fmt.Sprintf("No IP block equivalent to %q exists.", address),
		)
		return
	}

	if err := applyIPBlockToModel(&config, *blockedAddress); err != nil {
		resp.Diagnostics.AddError("Unable to decode IP block", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *ipBlockDataSource) Configure(
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

	client, ok := providerData["ipblock"].(*ipblock.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected IP Block Client Type",
			fmt.Sprintf("Expected *ipblock.Client, got: %T.", providerData["ipblock"]),
		)
		return
	}

	d.client = client
}
