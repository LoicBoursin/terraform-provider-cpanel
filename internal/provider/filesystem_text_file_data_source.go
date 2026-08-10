package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

var (
	_ datasource.DataSource              = &filesystemTextFileDataSource{}
	_ datasource.DataSourceWithConfigure = &filesystemTextFileDataSource{}
)

func NewFilesystemTextFileDataSource() datasource.DataSource {
	return &filesystemTextFileDataSource{}
}

type filesystemTextFileDataSource struct {
	client *fileman.Client
}

func (d *filesystemTextFileDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_text_file"
}

func (d *filesystemTextFileDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Reads one UTF-8 text file below public_html in a cPanel account.",
		MarkdownDescription: "Reads one UTF-8 text file below `public_html` in a cPanel account. Files larger than 1 MiB are rejected.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:            true,
				Description:         "The normalized file path below public_html, relative to the cPanel account home.",
				MarkdownDescription: "The normalized file path below `public_html`, relative to the cPanel account home.",
				Validators:          filesystemTextFilePathValidators(),
			},
			"content": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The exact UTF-8 file content.",
				MarkdownDescription: "The exact UTF-8 file content.",
			},
			"absolute_path": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute file path reported by cPanel.",
				MarkdownDescription: "The absolute file path reported by cPanel.",
			},
			"permissions": schema.StringAttribute{
				Computed:            true,
				Description:         "The four-digit octal file permissions reported by cPanel.",
				MarkdownDescription: "The four-digit octal file permissions reported by cPanel.",
			},
			"size_bytes": schema.Int64Attribute{
				Computed:            true,
				Description:         "The exact UTF-8 content size in bytes.",
				MarkdownDescription: "The exact UTF-8 content size in bytes.",
			},
			"content_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The lowercase SHA-256 digest of the exact file content.",
				MarkdownDescription: "The lowercase SHA-256 digest of the exact file content.",
			},
		},
	}
}

func (d *filesystemTextFileDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config FilesystemTextFileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filePath := config.Path.ValueString()
	if err := validateFilesystemTextFilePath(filePath); err != nil {
		resp.Diagnostics.AddError("Invalid filesystem text file", err.Error())
		return
	}

	textFile, err := readFilesystemTextFile(ctx, d.client, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem text file",
			err.Error(),
		)
		return
	}
	if textFile == nil {
		resp.Diagnostics.AddError(
			"Filesystem text file not found",
			fmt.Sprintf(
				"Text file %q does not exist in the cPanel account.",
				filePath,
			),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.Set(
			ctx,
			filesystemTextFileToDataSourceModel(*textFile),
		)...,
	)
}

func (d *filesystemTextFileDataSource) Configure(
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

	client, ok := providerData["fileman"].(*fileman.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Fileman Client Type",
			fmt.Sprintf(
				"Expected *fileman.Client, got: %T.",
				providerData["fileman"],
			),
		)
		return
	}

	d.client = client
}
