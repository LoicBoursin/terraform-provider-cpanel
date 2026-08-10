package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/apitoken"
)

var (
	_ datasource.DataSource              = &apiTokenDataSource{}
	_ datasource.DataSourceWithConfigure = &apiTokenDataSource{}
)

func NewAPITokenDataSource() datasource.DataSource {
	return &apiTokenDataSource{}
}

type apiTokenDataSource struct {
	client *apitoken.Client
}

func (d *apiTokenDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_api_token"
}

func (d *apiTokenDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up cPanel API token metadata by name without exposing its secret.",
		MarkdownDescription: "Looks up cPanel API token metadata by name without exposing its secret.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The API token name.",
				MarkdownDescription: "The API token name.",
				Validators:          apiTokenNameValidators(),
			},
			"expires_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The Unix timestamp when the token expires, or 0 for no expiration.",
				MarkdownDescription: "The Unix timestamp when the token expires, or `0` for no expiration.",
			},
			"created_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The token creation time as a Unix timestamp.",
				MarkdownDescription: "The token creation time as a Unix timestamp.",
			},
			"has_full_access": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports that the token has full account access.",
				MarkdownDescription: "Whether cPanel reports that the token has full account access.",
			},
			"features": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The feature names attached to a limited token.",
				MarkdownDescription: "The feature names attached to a limited token.",
			},
			"whitelist_ips": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The source IP restrictions reported by cPanel.",
				MarkdownDescription: "The source IP restrictions reported by cPanel.",
			},
		},
	}
}

func (d *apiTokenDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config APITokenDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := d.client.Get(ctx, config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API token", err.Error())
		return
	}
	if token == nil {
		resp.Diagnostics.AddError(
			"API token not found",
			fmt.Sprintf(
				"No API token named %q exists.",
				config.Name.ValueString(),
			),
		)
		return
	}

	state, diagnostics := apiTokenToDataSourceModel(ctx, *token)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (d *apiTokenDataSource) Configure(
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

	client, ok := providerData["apitoken"].(*apitoken.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected API Token Client Type",
			fmt.Sprintf("Expected *apitoken.Client, got: %T.", providerData["apitoken"]),
		)
		return
	}

	d.client = client
}
