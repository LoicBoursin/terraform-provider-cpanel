package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
)

var (
	_ datasource.DataSource              = &localeDataSource{}
	_ datasource.DataSourceWithConfigure = &localeDataSource{}
)

func NewLocaleDataSource() datasource.DataSource {
	return &localeDataSource{}
}

type localeDataSource struct {
	client *cpanellocale.Client
}

func (d *localeDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_locale"
}

func (d *localeDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Reads the display locale for the cPanel account.",
		MarkdownDescription: "Reads the display locale for the cPanel account.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
			},
			"locale": schema.StringAttribute{
				Computed:            true,
				Description:         "The abbreviated locale name returned by cPanel.",
				MarkdownDescription: "The abbreviated locale name returned by cPanel.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale name translated into the current cPanel locale.",
				MarkdownDescription: "The locale name translated into the current cPanel locale.",
			},
			"local_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale name written in its own language.",
				MarkdownDescription: "The locale name written in its own language.",
			},
			"direction": schema.StringAttribute{
				Computed:            true,
				Description:         "The locale text direction.",
				MarkdownDescription: "The locale text direction: `ltr` or `rtl`.",
			},
			"encoding": schema.StringAttribute{
				Computed:            true,
				Description:         "The character encoding reported by cPanel.",
				MarkdownDescription: "The character encoding reported by cPanel.",
			},
		},
	}
}

func (d *localeDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	current, err := d.client.GetCurrent(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cPanel locale", err.Error())
		return
	}

	resp.Diagnostics.Append(
		resp.State.Set(ctx, localeToDataSourceModel(*current))...,
	)
}

func (d *localeDataSource) Configure(
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

	client, ok := providerData["locale"].(*cpanellocale.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Locale Client Type",
			fmt.Sprintf(
				"Expected *locale.Client, got: %T.",
				providerData["locale"],
			),
		)
		return
	}

	d.client = client
}
