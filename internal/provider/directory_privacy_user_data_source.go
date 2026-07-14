package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

var (
	_ datasource.DataSource              = &directoryPrivacyUserDataSource{}
	_ datasource.DataSourceWithConfigure = &directoryPrivacyUserDataSource{}
)

func NewDirectoryPrivacyUserDataSource() datasource.DataSource {
	return &directoryPrivacyUserDataSource{}
}

type directoryPrivacyUserDataSource struct {
	client *directoryprivacy.Client
}

func (d *directoryPrivacyUserDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_directory_privacy_user"
}

func (d *directoryPrivacyUserDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one authorized user in a cPanel Directory Privacy password file.",
		MarkdownDescription: "Looks up one authorized user in a cPanel Directory Privacy password file.",
		Attributes: map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				Required:            true,
				Description:         "The directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path relative to the cPanel account home.",
				Validators:          directoryIndexDirectoryValidators(),
			},
			"username": schema.StringAttribute{
				Required:            true,
				Description:         "The username authorized to access the directory.",
				MarkdownDescription: "The username authorized to access the directory.",
				Validators:          directoryPrivacyUsernameValidators(),
			},
			"absolute_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
		},
	}
}

func (d *directoryPrivacyUserDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config DirectoryPrivacyUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directory := config.Directory.ValueString()
	username := config.Username.ValueString()
	if err := validateDirectoryPrivacyUserIdentity(
		directory,
		username,
	); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Directory Privacy user",
			err.Error(),
		)
		return
	}

	apiUser, err := d.client.GetUser(ctx, directory, username)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Directory Privacy user",
			err.Error(),
		)
		return
	}
	if apiUser == nil {
		resp.Diagnostics.AddError(
			"Directory Privacy user not found",
			fmt.Sprintf(
				"User %q does not exist for directory %q.",
				username,
				directory,
			),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.Set(ctx, directoryPrivacyUserToDataSourceModel(*apiUser))...,
	)
}

func (d *directoryPrivacyUserDataSource) Configure(
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
