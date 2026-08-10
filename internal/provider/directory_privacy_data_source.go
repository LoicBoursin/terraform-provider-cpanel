package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

var (
	_ datasource.DataSource              = &directoryPrivacyDataSource{}
	_ datasource.DataSourceWithConfigure = &directoryPrivacyDataSource{}
)

func NewDirectoryPrivacyDataSource() datasource.DataSource {
	return &directoryPrivacyDataSource{}
}

type directoryPrivacyDataSource struct {
	client *directoryprivacy.Client
}

func (d *directoryPrivacyDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_directory_privacy"
}

func (d *directoryPrivacyDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up password-protection status for an existing directory in a cPanel account.",
		MarkdownDescription: "Looks up password-protection status for an existing directory in a cPanel account.",
		Attributes: map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				Required:            true,
				Description:         "The directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path relative to the cPanel account home.",
				Validators:          directoryIndexDirectoryValidators(),
			},
			"auth_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The label displayed by HTTP Basic authentication.",
				MarkdownDescription: "The label displayed by HTTP Basic authentication. Empty when protection is disabled.",
			},
			"absolute_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
			"auth_type": schema.StringAttribute{
				Computed:            true,
				Description:         "The authentication type reported by cPanel.",
				MarkdownDescription: "The authentication type reported by cPanel.",
			},
			"password_file": schema.StringAttribute{
				Computed:            true,
				Description:         "The password file path reported by cPanel.",
				MarkdownDescription: "The password file path reported by cPanel. Empty when protection is disabled.",
			},
			"protected": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the directory as protected.",
				MarkdownDescription: "Whether cPanel reports the directory as protected.",
			},
		},
	}
}

func (d *directoryPrivacyDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config DirectoryPrivacyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directory := config.Directory.ValueString()
	if err := validateDirectoryPrivacyDefinition(
		directoryprivacy.Definition{
			Directory: directory,
			AuthName:  "lookup",
		},
	); err != nil {
		resp.Diagnostics.AddError("Invalid directory privacy", err.Error())
		return
	}

	apiPrivacy, err := d.client.Get(ctx, directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory privacy",
			err.Error(),
		)
		return
	}
	if apiPrivacy == nil {
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
		resp.State.Set(ctx, directoryPrivacyToDataSourceModel(*apiPrivacy))...,
	)
}

func (d *directoryPrivacyDataSource) Configure(
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

	client, ok := providerData["directoryprivacy"].(*directoryprivacy.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Directory Privacy Client Type",
			fmt.Sprintf(
				"Expected *directoryprivacy.Client, got: %T.",
				providerData["directoryprivacy"],
			),
		)
		return
	}

	d.client = client
}
