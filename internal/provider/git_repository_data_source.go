package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

var (
	_ datasource.DataSource              = &gitRepositoryDataSource{}
	_ datasource.DataSourceWithConfigure = &gitRepositoryDataSource{}
)

func NewGitRepositoryDataSource() datasource.DataSource {
	return &gitRepositoryDataSource{}
}

type gitRepositoryDataSource struct {
	client *versioncontrol.Client
}

func (d *gitRepositoryDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_git_repository"
}

func (d *gitRepositoryDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one cPanel Git Version Control repository.",
		MarkdownDescription: "Looks up one cPanel Git Version Control repository.",
		Attributes: map[string]schema.Attribute{
			"repository_root": schema.StringAttribute{
				Required:            true,
				Description:         "The repository directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized repository directory relative to the cPanel account home.",
				Validators:          gitRepositoryRootValidators(),
			},
			"name": schema.StringAttribute{
				Computed:            true,
				Description:         "The display name of the Git repository.",
				MarkdownDescription: "The display name of the Git repository.",
			},
			"source_repository_url": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The source repository URL reported by cPanel.",
				MarkdownDescription: "The source repository URL reported by cPanel.",
			},
			"absolute_repository_root": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute repository path reported by cPanel.",
				MarkdownDescription: "The absolute repository path reported by cPanel.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				Description:         "The version-control type reported by cPanel.",
				MarkdownDescription: "The version-control type reported by cPanel.",
			},
			"branch": schema.StringAttribute{
				Computed:            true,
				Description:         "The currently checked-out branch, when present.",
				MarkdownDescription: "The currently checked-out branch, when present.",
			},
			"available_branches": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The branches reported by cPanel.",
				MarkdownDescription: "The branches reported by cPanel.",
			},
			"read_only_clone_urls": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The read-only clone URLs reported by cPanel.",
				MarkdownDescription: "The read-only clone URLs reported by cPanel.",
			},
			"read_write_clone_urls": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The read-write clone URLs reported by cPanel.",
				MarkdownDescription: "The read-write clone URLs reported by cPanel.",
			},
			"source_repository_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The source remote name reported by cPanel.",
				MarkdownDescription: "The source remote name reported by cPanel.",
			},
			"deployable": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports that the repository can be deployed.",
				MarkdownDescription: "Whether cPanel reports that the repository can be deployed.",
			},
		},
	}
}

func (d *gitRepositoryDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config GitRepositoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repositoryRoot := config.RepositoryRoot.ValueString()
	if err := versioncontrol.ValidateRepositoryRoot(repositoryRoot); err != nil {
		resp.Diagnostics.AddError("Invalid Git repository root", err.Error())
		return
	}

	repository, err := d.client.Get(ctx, repositoryRoot)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Git repository",
			gitRepositoryReadError(err).Error(),
		)
		return
	}
	if repository == nil {
		resp.Diagnostics.AddError(
			"Git repository not found",
			fmt.Sprintf(
				"Git repository %q is not registered in cPanel.",
				repositoryRoot,
			),
		)
		return
	}

	model, diagnostics := gitRepositoryToDataSourceModel(ctx, *repository)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (d *gitRepositoryDataSource) Configure(
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

	client, ok := providerData["versioncontrol"].(*versioncontrol.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Version Control Client Type",
			fmt.Sprintf(
				"Expected *versioncontrol.Client, got: %T.",
				providerData["versioncontrol"],
			),
		)
		return
	}

	d.client = client
}
