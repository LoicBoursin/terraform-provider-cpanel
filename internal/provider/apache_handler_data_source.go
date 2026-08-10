package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

var (
	_ datasource.DataSource              = &apacheHandlerDataSource{}
	_ datasource.DataSourceWithConfigure = &apacheHandlerDataSource{}
)

func NewApacheHandlerDataSource() datasource.DataSource {
	return &apacheHandlerDataSource{}
}

type apacheHandlerDataSource struct {
	client *apachehandler.Client
}

func (d *apacheHandlerDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_apache_handler"
}

func (d *apacheHandlerDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one custom cPanel Apache handler by file extension.",
		MarkdownDescription: "Looks up one custom cPanel Apache handler by file extension.",
		Attributes: map[string]schema.Attribute{
			"extension": schema.StringAttribute{
				Required:            true,
				Description:         "The file extension handled by Apache.",
				MarkdownDescription: "The file extension handled by Apache. It must begin with a dot.",
				Validators:          mimeExtensionValidators(),
			},
			"handler": schema.StringAttribute{
				Computed:            true,
				Description:         "The Apache handler name.",
				MarkdownDescription: "The Apache handler name.",
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				Description:         "The Apache handler owner reported by cPanel.",
				MarkdownDescription: "The Apache handler owner reported by cPanel.",
			},
		},
	}
}

func (d *apacheHandlerDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config ApacheHandlerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	extension := config.Extension.ValueString()
	if !mimeExtensionPattern.MatchString(extension) {
		resp.Diagnostics.AddError(
			"Invalid Apache handler extension",
			"Expected a file extension beginning with a dot.",
		)
		return
	}

	apiHandler, err := d.client.Get(ctx, extension)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Apache handler", err.Error())
		return
	}
	if apiHandler == nil {
		resp.Diagnostics.AddError(
			"Apache handler not found",
			fmt.Sprintf(
				"No custom Apache handler for extension %q exists.",
				extension,
			),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.Set(ctx, apacheHandlerToDataSourceModel(*apiHandler))...,
	)
}

func (d *apacheHandlerDataSource) Configure(
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

	client, ok := providerData["apachehandler"].(*apachehandler.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Apache Handler Client Type",
			fmt.Sprintf(
				"Expected *apachehandler.Client, got: %T.",
				providerData["apachehandler"],
			),
		)
		return
	}

	d.client = client
}
