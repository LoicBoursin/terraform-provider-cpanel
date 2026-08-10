package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

var (
	_ datasource.DataSource              = &filesystemDirectoryDataSource{}
	_ datasource.DataSourceWithConfigure = &filesystemDirectoryDataSource{}
)

func NewFilesystemDirectoryDataSource() datasource.DataSource {
	return &filesystemDirectoryDataSource{}
}

type filesystemDirectoryDataSource struct {
	client *fileman.Client
}

func (d *filesystemDirectoryDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_directory"
}

func (d *filesystemDirectoryDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one directory below public_html in a cPanel account.",
		MarkdownDescription: "Looks up one directory below `public_html` in a cPanel account.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:            true,
				Description:         "The normalized directory path below public_html, relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path below `public_html`, relative to the cPanel account home.",
				Validators:          filesystemDirectoryPathValidators(),
			},
			"absolute_path": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
			"permissions": schema.StringAttribute{
				Computed:            true,
				Description:         "The four-digit octal directory permissions reported by cPanel.",
				MarkdownDescription: "The four-digit octal directory permissions reported by cPanel.",
			},
		},
	}
}

func (d *filesystemDirectoryDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config FilesystemDirectoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directoryPath := config.Path.ValueString()
	if err := validateFilesystemDirectoryPath(directoryPath); err != nil {
		resp.Diagnostics.AddError("Invalid filesystem directory", err.Error())
		return
	}

	directory, err := d.client.GetDirectory(ctx, directoryPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem directory",
			err.Error(),
		)
		return
	}
	if directory == nil {
		resp.Diagnostics.AddError(
			"Filesystem directory not found",
			fmt.Sprintf(
				"Directory %q does not exist in the cPanel account.",
				directoryPath,
			),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.Set(
			ctx,
			filesystemDirectoryToDataSourceModel(*directory),
		)...,
	)
}

func (d *filesystemDirectoryDataSource) Configure(
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
