package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

var (
	_ datasource.DataSource              = &mimeTypeDataSource{}
	_ datasource.DataSourceWithConfigure = &mimeTypeDataSource{}
)

func NewMIMETypeDataSource() datasource.DataSource {
	return &mimeTypeDataSource{}
}

type mimeTypeDataSource struct {
	client *mimetype.Client
}

func (d *mimeTypeDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mime_type"
}

func (d *mimeTypeDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one custom cPanel Apache MIME type.",
		MarkdownDescription: "Looks up one custom cPanel Apache MIME type.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				Description:         "The lowercase custom media type.",
				MarkdownDescription: "The lowercase custom media type, for example `application/x-example`.",
				Validators:          mimeTypeValidators(),
			},
			"extensions": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The file extensions associated with the MIME type.",
				MarkdownDescription: "The file extensions associated with the MIME type.",
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				Description:         "The MIME type owner reported by cPanel.",
				MarkdownDescription: "The MIME type owner reported by cPanel.",
			},
		},
	}
}

func (d *mimeTypeDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config MIMETypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mimeTypeName := config.Type.ValueString()
	if !mimeTypePattern.MatchString(mimeTypeName) {
		resp.Diagnostics.AddError(
			"Invalid MIME type",
			"Expected a lowercase media type such as application/x-example.",
		)
		return
	}

	apiMIMEType, err := d.client.Get(ctx, mimeTypeName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MIME type", err.Error())
		return
	}
	if apiMIMEType == nil {
		resp.Diagnostics.AddError(
			"MIME type not found",
			fmt.Sprintf("No custom MIME type %q exists.", mimeTypeName),
		)
		return
	}

	state, diagnostics := mimeTypeToDataSourceModel(ctx, *apiMIMEType)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (d *mimeTypeDataSource) Configure(
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

	client, ok := providerData["mimetype"].(*mimetype.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected MIME Type Client Type",
			fmt.Sprintf(
				"Expected *mimetype.Client, got: %T.",
				providerData["mimetype"],
			),
		)
		return
	}

	d.client = client
}
