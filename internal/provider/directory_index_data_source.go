package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

var (
	_ datasource.DataSource              = &directoryIndexDataSource{}
	_ datasource.DataSourceWithConfigure = &directoryIndexDataSource{}
)

func NewDirectoryIndexDataSource() datasource.DataSource {
	return &directoryIndexDataSource{}
}

type directoryIndexDataSource struct {
	client *directoryindex.Client
}

func (d *directoryIndexDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_directory_index"
}

func (d *directoryIndexDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up directory indexing for an existing directory in a cPanel account.",
		MarkdownDescription: "Looks up directory indexing for an existing directory in a cPanel account.",
		Attributes: map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				Required:            true,
				Description:         "The directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path relative to the cPanel account home.",
				Validators:          directoryIndexDirectoryValidators(),
			},
			"type": schema.StringAttribute{
				Computed:            true,
				Description:         "The directory indexing mode.",
				MarkdownDescription: "The directory indexing mode: `inherit`, `disabled`, `standard`, or `fancy`.",
			},
			"absolute_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
		},
	}
}

func (d *directoryIndexDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config DirectoryIndexDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directory := config.Directory.ValueString()
	if err := validateDirectoryIndexDefinition(
		directoryindex.Definition{
			Directory: directory,
			Type:      directoryindex.IndexTypeInherit,
		},
	); err != nil {
		resp.Diagnostics.AddError("Invalid directory index", err.Error())
		return
	}

	apiIndex, err := d.client.Get(ctx, directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory index",
			err.Error(),
		)
		return
	}
	if apiIndex == nil {
		resp.Diagnostics.AddError(
			"Directory not found",
			fmt.Sprintf(
				"Directory %q does not exist in the cPanel account.",
				directory,
			),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.Set(ctx, directoryIndexToDataSourceModel(*apiIndex))...,
	)
}

func (d *directoryIndexDataSource) Configure(
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

	client, ok := providerData["directoryindex"].(*directoryindex.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Directory Index Client Type",
			fmt.Sprintf(
				"Expected *directoryindex.Client, got: %T.",
				providerData["directoryindex"],
			),
		)
		return
	}

	d.client = client
}
