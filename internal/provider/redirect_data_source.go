package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

var (
	_ datasource.DataSource              = &redirectDataSource{}
	_ datasource.DataSourceWithConfigure = &redirectDataSource{}
)

func NewRedirectDataSource() datasource.DataSource {
	return &redirectDataSource{}
}

type redirectDataSource struct {
	client *cpanelredirect.Client
}

func (d *redirectDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_redirect"
}

func (d *redirectDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one cPanel HTTP redirect rule.",
		MarkdownDescription: "Looks up one cPanel HTTP redirect rule.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel account domain that owns the redirect.",
				MarkdownDescription: "The cPanel account domain that owns the redirect.",
				Validators:          domainNameValidators(),
			},
			"source": schema.StringAttribute{
				Required:            true,
				Description:         "The absolute source URL path.",
				MarkdownDescription: "The absolute source URL path.",
				Validators:          redirectSourceValidators(),
			},
			"destination": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute HTTP or HTTPS destination URL.",
				MarkdownDescription: "The absolute HTTP or HTTPS destination URL.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				Description:         "Whether the redirect is permanent or temporary.",
				MarkdownDescription: "Whether the redirect is `permanent` (`301`) or `temporary` (`302`).",
			},
			"www_mode": schema.StringAttribute{
				Computed:            true,
				Description:         "Whether the redirect matches both www and non-www requests, or only requests without www.",
				MarkdownDescription: "Whether the redirect matches `both` www and non-www requests, or only requests `without` www.",
			},
			"wildcard": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether matching files below the source path keep their relative path below the destination.",
				MarkdownDescription: "Whether matching files below the source path keep their relative path below the destination.",
			},
			"status_code": schema.Int64Attribute{
				Computed:            true,
				Description:         "The HTTP status code reported by cPanel.",
				MarkdownDescription: "The HTTP status code reported by cPanel.",
			},
			"document_root": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute document root whose .htaccess file contains the redirect.",
				MarkdownDescription: "The absolute document root whose `.htaccess` file contains the redirect.",
			},
			"kind": schema.StringAttribute{
				Computed:            true,
				Description:         "The redirect directive kind reported by cPanel.",
				MarkdownDescription: "The redirect directive kind reported by cPanel.",
			},
		},
	}
}

func (d *redirectDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config RedirectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDomainName(config.Domain.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid redirect domain", err.Error())
		return
	}
	if err := validateRedirectSource(config.Source.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid redirect source", err.Error())
		return
	}

	redirect, err := d.client.Get(
		ctx,
		config.Domain.ValueString(),
		config.Source.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read redirect", err.Error())
		return
	}
	if redirect == nil {
		resp.Diagnostics.AddError(
			"Redirect not found",
			fmt.Sprintf(
				"No redirect for %s%s exists.",
				config.Domain.ValueString(),
				config.Source.ValueString(),
			),
		)
		return
	}

	state, diagnostics := redirectToDataSourceModel(*redirect)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (d *redirectDataSource) Configure(
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

	client, ok := providerData["redirect"].(*cpanelredirect.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Redirect Client Type",
			fmt.Sprintf(
				"Expected *redirect.Client, got: %T.",
				providerData["redirect"],
			),
		)
		return
	}

	d.client = client
}
