package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

var (
	_ resource.Resource                = &directoryIndexResource{}
	_ resource.ResourceWithConfigure   = &directoryIndexResource{}
	_ resource.ResourceWithImportState = &directoryIndexResource{}
)

func NewDirectoryIndexResource() resource.Resource {
	return &directoryIndexResource{}
}

type directoryIndexResource struct {
	client directoryIndexClient
}

type directoryIndexClient interface {
	Get(context.Context, string) (*directoryindex.Index, error)
	Set(context.Context, string, string) (*directoryindex.Index, error)
}

func (r *directoryIndexResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_directory_index"
}

func (r *directoryIndexResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages directory indexing for an existing directory in a cPanel account.",
		MarkdownDescription: "Manages directory indexing for an existing directory in a cPanel account.",
		Attributes: map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				Required:            true,
				Description:         "The directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path relative to the cPanel account home.",
				Validators:          directoryIndexDirectoryValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				Description:         "The directory indexing mode.",
				MarkdownDescription: "The directory indexing mode: `inherit`, `disabled`, `standard`, or `fancy`.",
				Validators:          directoryIndexTypeValidators(),
			},
			"absolute_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
		},
	}
}

func (r *directoryIndexResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DirectoryIndexResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiIndex, err := r.client.Get(ctx, state.Directory.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory index",
			err.Error(),
		)
		return
	}
	if apiIndex == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyDirectoryIndexToResourceModel(&state, *apiIndex)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *directoryIndexResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DirectoryIndexResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := directoryIndexDefinitionFromResourceModel(plan)
	if err := validateDirectoryIndexDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid directory index", err.Error())
		return
	}

	current, err := r.client.Get(ctx, definition.Directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory index",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Directory not found",
			fmt.Sprintf(
				"Directory %q does not exist in the cPanel account.",
				definition.Directory,
			),
		)
		return
	}

	changed := current.Type != definition.Type
	if changed {
		if _, err := r.client.Set(
			ctx,
			definition.Directory,
			definition.Type,
		); err != nil {
			rollbackErr := r.restoreDirectoryIndexIfCurrentMatches(
				ctx,
				definition,
				*current,
			)
			resp.Diagnostics.AddError(
				"Unable to configure directory index",
				directoryIndexMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	apiIndex, err := r.verifyDirectoryIndex(ctx, definition)
	if err != nil {
		var rollbackErr error
		if changed {
			rollbackErr = r.restoreDirectoryIndexIfCurrentMatches(
				ctx,
				definition,
				*current,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify directory index",
			directoryIndexMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyDirectoryIndexToResourceModel(&plan, *apiIndex)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *directoryIndexResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DirectoryIndexResourceModel
	var state DirectoryIndexResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := directoryIndexDefinitionFromResourceModel(plan)
	if err := validateDirectoryIndexDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid directory index", err.Error())
		return
	}

	current, err := r.client.Get(ctx, state.Directory.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory index",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Directory no longer exists",
			"Refresh the Terraform state before updating the directory index.",
		)
		return
	}
	if current.Type != state.Type.ValueString() {
		resp.Diagnostics.AddError(
			"Directory index changed during update",
			fmt.Sprintf(
				"Directory %q now uses indexing mode %q instead of the mode %q stored in Terraform state. Refresh and review the change before retrying.",
				state.Directory.ValueString(),
				current.Type,
				state.Type.ValueString(),
			),
		)
		return
	}

	changed := current.Type != definition.Type
	if changed {
		if _, err := r.client.Set(
			ctx,
			definition.Directory,
			definition.Type,
		); err != nil {
			rollbackErr := r.restoreDirectoryIndexIfCurrentMatches(
				ctx,
				definition,
				*current,
			)
			resp.Diagnostics.AddError(
				"Unable to update directory index",
				directoryIndexMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	apiIndex, err := r.verifyDirectoryIndex(ctx, definition)
	if err != nil {
		var rollbackErr error
		if changed {
			rollbackErr = r.restoreDirectoryIndexIfCurrentMatches(
				ctx,
				definition,
				*current,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify directory index update",
			directoryIndexMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyDirectoryIndexToResourceModel(&plan, *apiIndex)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *directoryIndexResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DirectoryIndexResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directory := state.Directory.ValueString()
	current, err := r.client.Get(ctx, directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory index",
			err.Error(),
		)
		return
	}
	if current == nil {
		return
	}
	if current.Type != state.Type.ValueString() {
		resp.Diagnostics.AddError(
			"Unable to reset directory index",
			"The remote directory index no longer matches Terraform state, so the provider refuses to reset it. Refresh and review the drift before retrying.",
		)
		return
	}

	if current.Type != directoryindex.IndexTypeInherit {
		if err := r.resetDirectoryIndex(ctx, directory, current.Type); err != nil {
			resp.Diagnostics.AddError(
				"Unable to reset directory index",
				err.Error(),
			)
			return
		}
	}

	if _, err := r.verifyDirectoryIndex(
		ctx,
		directoryindex.Definition{
			Directory: directory,
			Type:      directoryindex.IndexTypeInherit,
		},
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify directory index reset",
			err.Error(),
		)
	}
}

func (r *directoryIndexResource) resetDirectoryIndex(
	ctx context.Context,
	directory string,
	expectedType string,
) error {
	_, mutationErr := r.client.Set(
		ctx,
		directory,
		directoryindex.IndexTypeInherit,
	)
	current, readErr := r.client.Get(ctx, directory)
	if readErr != nil {
		if mutationErr != nil {
			return fmt.Errorf(
				"restore inherited directory indexing: %v; verify reset: %w",
				mutationErr,
				readErr,
			)
		}

		return fmt.Errorf("verify inherited directory indexing: %w", readErr)
	}
	if current == nil || current.Type == directoryindex.IndexTypeInherit {
		return nil
	}
	if current.Type != expectedType {
		return fmt.Errorf(
			"the directory index changed concurrently during reset; Terraform preserved the current %q mode",
			current.Type,
		)
	}
	if mutationErr != nil {
		return fmt.Errorf(
			"restore inherited directory indexing: %w",
			mutationErr,
		)
	}

	return fmt.Errorf(
		"cPanel reported success but directory %q still uses %q indexing",
		directory,
		current.Type,
	)
}

func (r *directoryIndexResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := validateDirectoryIndexDefinition(
		directoryindex.Definition{
			Directory: req.ID,
			Type:      directoryindex.IndexTypeInherit,
		},
	); err != nil {
		resp.Diagnostics.AddError(
			"Invalid directory index import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("directory"), req.ID)...,
	)
}

func (r *directoryIndexResource) Configure(
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

	r.client = client
}

func (r *directoryIndexResource) verifyDirectoryIndex(
	ctx context.Context,
	expected directoryindex.Definition,
) (*directoryindex.Index, error) {
	apiIndex, err := r.client.Get(ctx, expected.Directory)
	if err != nil {
		return nil, fmt.Errorf("read directory index after mutation: %w", err)
	}
	if apiIndex == nil {
		return nil, fmt.Errorf(
			"directory %q was not found after indexing mutation",
			expected.Directory,
		)
	}
	if apiIndex.Type != expected.Type {
		return nil, fmt.Errorf(
			"directory %q indexing type is %q; expected %q",
			expected.Directory,
			apiIndex.Type,
			expected.Type,
		)
	}

	return apiIndex, nil
}

func (r *directoryIndexResource) restoreDirectoryIndexIfCurrentMatches(
	ctx context.Context,
	expectedCurrent directoryindex.Definition,
	original directoryindex.Index,
) error {
	current, err := r.client.Get(ctx, original.Directory)
	if err != nil {
		return fmt.Errorf("read directory index before restore: %w", err)
	}
	if current == nil {
		return fmt.Errorf(
			"directory %q no longer exists during directory index restore",
			original.Directory,
		)
	}
	if current.Type == original.Type {
		return nil
	}
	if current.Type != expectedCurrent.Type {
		return fmt.Errorf(
			"refuse to restore the previous directory index because the current mode no longer matches the Terraform transition",
		)
	}
	if _, err := r.client.Set(
		ctx,
		original.Directory,
		original.Type,
	); err != nil {
		return fmt.Errorf("restore previous directory index: %w", err)
	}
	if _, err := r.verifyDirectoryIndex(
		ctx,
		directoryindex.Definition{
			Directory: original.Directory,
			Type:      original.Type,
		},
	); err != nil {
		return fmt.Errorf("verify restored directory index: %w", err)
	}

	return nil
}

func directoryIndexMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous directory index: %v",
		primaryError,
		rollbackError,
	)
}
