package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

var (
	_ resource.Resource                = &gitRepositoryResource{}
	_ resource.ResourceWithConfigure   = &gitRepositoryResource{}
	_ resource.ResourceWithImportState = &gitRepositoryResource{}
	_ resource.ResourceWithModifyPlan  = &gitRepositoryResource{}
)

func NewGitRepositoryResource() resource.Resource {
	return &gitRepositoryResource{}
}

type gitRepositoryResource struct {
	client *versioncontrol.Client
}

func (r *gitRepositoryResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_git_repository"
}

func (r *gitRepositoryResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one cPanel Git Version Control repository.",
		MarkdownDescription: "Manages one cPanel Git Version Control repository.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The display name of the Git repository.",
				MarkdownDescription: "The display name of the Git repository.",
				Validators:          gitRepositoryNameValidators(),
			},
			"repository_root": schema.StringAttribute{
				Required:            true,
				Description:         "The repository directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized repository directory relative to the cPanel account home.",
				Validators:          gitRepositoryRootValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source_repository_url": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				Description:         "The HTTPS or SSH URL cloned when cPanel creates the repository. Changing it at the same repository root requires delete_contents_on_destroy to already be enabled in state.",
				MarkdownDescription: "The HTTPS or SSH URL cloned when cPanel creates the repository. Changing it at the same `repository_root` requires `delete_contents_on_destroy` to already be enabled in state.",
				Validators:          gitSourceRepositoryURLValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"delete_contents_on_destroy": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Whether destroying the Terraform resource permanently deletes the repository directory and all contents.",
				MarkdownDescription: "Whether destroying the Terraform resource permanently deletes the repository directory and all contents. Defaults to `false`, which removes only the Terraform state and leaves the cPanel-managed repository intact.",
			},
			"absolute_repository_root": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute repository path reported by cPanel.",
				MarkdownDescription: "The absolute repository path reported by cPanel.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				Description:         "The version-control type reported by cPanel.",
				MarkdownDescription: "The version-control type reported by cPanel. Managed repositories use `git`.",
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
				MarkdownDescription: "The source remote name reported by cPanel, usually `origin`.",
			},
			"deployable": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports that the repository can be deployed.",
				MarkdownDescription: "Whether cPanel reports that the repository can be deployed.",
			},
		},
	}
}

func (r *gitRepositoryResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state GitRepositoryResourceModel
	var plan GitRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !gitSourceReplacementRequiresDeletion(state, plan) {
		return
	}
	if state.DeleteContentsOnDestroy.IsUnknown() ||
		state.DeleteContentsOnDestroy.IsNull() ||
		!state.DeleteContentsOnDestroy.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("source_repository_url"),
			"Git source replacement requires prior destructive deletion",
			"cPanel cannot change a registered repository source URL in place. Set delete_contents_on_destroy = true and apply that change first, then change source_repository_url in a second apply so Terraform can remove the old repository directory before cloning the replacement.",
		)
	}
}

func gitSourceReplacementRequiresDeletion(
	state GitRepositoryResourceModel,
	plan GitRepositoryResourceModel,
) bool {
	return gitSourceRepositoryURLChanged(state, plan) &&
		!gitRepositoryRootChanged(state, plan)
}

func gitSourceRepositoryURLChanged(
	state GitRepositoryResourceModel,
	plan GitRepositoryResourceModel,
) bool {
	if state.SourceRepositoryURL.IsUnknown() ||
		plan.SourceRepositoryURL.IsUnknown() {
		return false
	}

	return !state.SourceRepositoryURL.Equal(plan.SourceRepositoryURL)
}

func gitRepositoryRootChanged(
	state GitRepositoryResourceModel,
	plan GitRepositoryResourceModel,
) bool {
	if state.RepositoryRoot.IsUnknown() ||
		plan.RepositoryRoot.IsUnknown() {
		return false
	}

	return !state.RepositoryRoot.Equal(plan.RepositoryRoot)
}

func (r *gitRepositoryResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state GitRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repository, err := r.client.Get(
		ctx,
		state.RepositoryRoot.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Git repository", err.Error())
		return
	}
	if repository == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	if state.DeleteContentsOnDestroy.IsNull() ||
		state.DeleteContentsOnDestroy.IsUnknown() {
		state.DeleteContentsOnDestroy = types.BoolValue(false)
	}
	resp.Diagnostics.Append(
		applyGitRepositoryToResourceModel(ctx, &state, *repository)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gitRepositoryResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan GitRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := gitRepositoryDefinitionFromResourceModel(plan)
	if err := validateGitRepositoryDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid Git repository", err.Error())
		return
	}

	existing, err := r.client.Get(ctx, definition.RepositoryRoot)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Git repository", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Git repository already exists",
			fmt.Sprintf(
				"Git repository %q is already managed by cPanel. Import it instead of replacing it implicitly.",
				definition.RepositoryRoot,
			),
		)
		return
	}

	rootExists, err := r.client.RootExists(ctx, definition.RepositoryRoot)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to inspect Git repository root",
			err.Error(),
		)
		return
	}
	if rootExists {
		resp.Diagnostics.AddError(
			"Git repository root already exists",
			fmt.Sprintf(
				"Directory %q already exists but is not registered in cPanel Git Version Control. Register it outside Terraform and import it to avoid taking ownership implicitly.",
				definition.RepositoryRoot,
			),
		)
		return
	}

	if _, err := r.client.Create(ctx, definition); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Git repository",
			gitRepositoryCreationError(
				err,
				definition.SourceRepositoryURL,
			).Error(),
		)
		return
	}

	repository, err := r.verifyRepository(ctx, definition)
	if err != nil {
		rollbackErr := r.deleteRepositoryAndContents(
			ctx,
			definition.RepositoryRoot,
		)
		resp.Diagnostics.AddError(
			"Unable to verify Git repository",
			gitRepositoryMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyGitRepositoryToResourceModel(ctx, &plan, *repository)...,
	)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.deleteRepositoryAndContents(
			ctx,
			definition.RepositoryRoot,
		)
		resp.Diagnostics.AddError(
			"Unable to decode Git repository",
			gitRepositoryMutationErrorDetail(
				fmt.Errorf("convert Git repository metadata"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *gitRepositoryResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan GitRepositoryResourceModel
	var state GitRepositoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := gitRepositoryDefinitionFromResourceModel(plan)
	if err := validateGitRepositoryDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid Git repository", err.Error())
		return
	}

	current, err := r.client.Get(
		ctx,
		state.RepositoryRoot.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Git repository", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Git repository no longer exists",
			"Refresh the Terraform state before updating the Git repository.",
		)
		return
	}

	nameChanged := current.Name != definition.Name
	if nameChanged {
		if _, err := r.client.Update(
			ctx,
			definition.RepositoryRoot,
			definition.Name,
		); err != nil {
			resp.Diagnostics.AddError(
				"Unable to update Git repository",
				err.Error(),
			)
			return
		}
	}

	repository, err := r.verifyRepository(ctx, definition)
	if err != nil {
		var rollbackErr error
		if nameChanged {
			_, rollbackErr = r.client.Update(
				ctx,
				current.RepositoryRoot,
				current.Name,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify Git repository update",
			gitRepositoryMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyGitRepositoryToResourceModel(ctx, &plan, *repository)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *gitRepositoryResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state GitRepositoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.DeleteContentsOnDestroy.ValueBool() {
		return
	}

	repositoryRoot := state.RepositoryRoot.ValueString()
	if err := r.deleteRepositoryAndContents(ctx, repositoryRoot); err != nil {
		resp.Diagnostics.AddError("Unable to delete Git repository", err.Error())
		return
	}

	remaining, err := r.client.Get(ctx, repositoryRoot)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify Git repository deletion",
			err.Error(),
		)
		return
	}
	if remaining != nil {
		resp.Diagnostics.AddError(
			"Unable to verify Git repository deletion",
			fmt.Sprintf(
				"Git repository %q still exists after deletion.",
				repositoryRoot,
			),
		)
		return
	}

	rootExists, err := r.client.RootExists(ctx, repositoryRoot)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify Git repository directory deletion",
			err.Error(),
		)
		return
	}
	if rootExists {
		resp.Diagnostics.AddError(
			"Unable to verify Git repository directory deletion",
			fmt.Sprintf(
				"Directory %q still exists after destructive repository deletion.",
				repositoryRoot,
			),
		)
	}
}

func (r *gitRepositoryResource) deleteRepositoryAndContents(
	ctx context.Context,
	repositoryRoot string,
) error {
	current, err := r.client.Get(ctx, repositoryRoot)
	if err != nil {
		return fmt.Errorf("read Git repository before deletion: %w", err)
	}
	if current != nil {
		if err := r.client.Delete(ctx, repositoryRoot); err != nil {
			return fmt.Errorf("unregister Git repository: %w", err)
		}
	}

	if err := r.client.DeleteDirectory(ctx, repositoryRoot); err != nil {
		return fmt.Errorf("delete Git repository directory: %w", err)
	}

	return nil
}

func (r *gitRepositoryResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := versioncontrol.ValidateRepositoryRoot(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Git repository import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("repository_root"), req.ID)...,
	)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(
			ctx,
			path.Root("delete_contents_on_destroy"),
			false,
		)...,
	)
}

func (r *gitRepositoryResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
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

	r.client = client
}

func (r *gitRepositoryResource) verifyRepository(
	ctx context.Context,
	expected versioncontrol.Definition,
) (*versioncontrol.Repository, error) {
	repository, err := r.client.Get(ctx, expected.RepositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("read Git repositories after mutation: %w", err)
	}
	if repository == nil {
		return nil, fmt.Errorf(
			"git repository %q was not found after mutation",
			expected.RepositoryRoot,
		)
	}
	if repository.Name != expected.Name {
		return nil, fmt.Errorf(
			"git repository %q name is %q; expected %q",
			expected.RepositoryRoot,
			repository.Name,
			expected.Name,
		)
	}
	if repository.SourceRepositoryURL != expected.SourceRepositoryURL {
		return nil, fmt.Errorf(
			"git repository %q source URL does not match the configured value",
			expected.RepositoryRoot,
		)
	}

	return repository, nil
}

func gitRepositoryMutationErrorDetail(
	mutationErr error,
	rollbackErr error,
) string {
	if rollbackErr == nil {
		return mutationErr.Error() + ". The previous remote state was restored."
	}

	return fmt.Sprintf(
		"%v. Automatic rollback also failed: %v",
		mutationErr,
		rollbackErr,
	)
}

func gitRepositoryCreationError(
	err error,
	sourceRepositoryURL string,
) error {
	if sourceRepositoryURL == "" {
		return err
	}

	return sensitiveMutationError(
		err,
		"Git repository source clone",
	)
}
