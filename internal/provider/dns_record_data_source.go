package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpaneldns "terraform-provider-cpanel/internal/cpanel/dns"
)

var (
	_ datasource.DataSource              = &dnsRecordDataSource{}
	_ datasource.DataSourceWithConfigure = &dnsRecordDataSource{}
)

func NewDNSRecordDataSource() datasource.DataSource {
	return &dnsRecordDataSource{}
}

type dnsRecordDataSource struct {
	client *cpaneldns.Client
}

func (d *dnsRecordDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (d *dnsRecordDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one DNS record by zone and cPanel line index.",
		MarkdownDescription: "Looks up one DNS record by zone and cPanel line index.",
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel-managed DNS zone without a trailing dot.",
				MarkdownDescription: "The cPanel-managed DNS zone without a trailing dot.",
				Validators:          domainNameValidators(),
			},
			"line_index": schema.Int64Attribute{
				Required:            true,
				Description:         "The current zero-based line index of the record in the cPanel DNS zone.",
				MarkdownDescription: "The current zero-based line index of the record in the cPanel DNS zone.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				Description:         "The record name relative to the zone, or @ for the zone apex.",
				MarkdownDescription: "The record name relative to the zone, or `@` for the zone apex.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				Description:         "The DNS record type.",
				MarkdownDescription: "The DNS record type.",
			},
			"ttl": schema.Int64Attribute{
				Computed:            true,
				Description:         "The DNS record time to live in seconds.",
				MarkdownDescription: "The DNS record time to live in seconds.",
			},
			"data": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The ordered DNS record data fields.",
				MarkdownDescription: "The ordered DNS record data fields.",
			},
		},
	}
}

func (d *dnsRecordDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config DNSRecordModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := d.client.ParseZone(ctx, config.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS zone", err.Error())
		return
	}
	record := zone.RecordAt(config.LineIndex.ValueInt64())
	if record == nil {
		resp.Diagnostics.AddError(
			"DNS record not found",
			fmt.Sprintf(
				"No DNS record exists at line %d in zone %q.",
				config.LineIndex.ValueInt64(),
				config.Zone.ValueString(),
			),
		)
		return
	}

	resp.Diagnostics.Append(applyDNSRecordToModel(ctx, &config, *record)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *dnsRecordDataSource) Configure(
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

	client, ok := providerData["dns"].(*cpaneldns.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DNS Client Type",
			fmt.Sprintf("Expected *dns.Client, got: %T.", providerData["dns"]),
		)
		return
	}

	d.client = client
}
