package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

var (
	_ resource.Resource                = &filesystemDirectoryResource{}
	_ resource.ResourceWithConfigure   = &filesystemDirectoryResource{}
	_ resource.ResourceWithImportState = &filesystemDirectoryResource{}
)

func NewFilesystemDirectoryResource() resource.Resource {
	return &filesystemDirectoryResource{
		generateOwnershipToken: generateFilesystemDirectoryOwnershipToken,
	}
}

type filesystemDirectoryClient interface {
	LockMutations() func()
	GetDirectory(context.Context, string) (*fileman.Directory, error)
	ListDirectory(context.Context, string) ([]fileman.Entry, error)
	CreateDirectory(context.Context, string) (*fileman.Directory, error)
	GetTextFile(context.Context, string) (*fileman.TextFile, error)
	SaveTextFile(context.Context, string, string) (*fileman.TextFile, error)
	DeletePath(context.Context, string) error
}

type filesystemDirectoryResource struct {
	client                 filesystemDirectoryClient
	generateOwnershipToken func() (string, error)
}

func (r *filesystemDirectoryResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_directory"
}

func (r *filesystemDirectoryResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Creates one ownership-marked directory below public_html in a cPanel account.",
		MarkdownDescription: "Creates one ownership-marked directory below `public_html` in a cPanel account. Terraform deletes only a directory whose private ownership marker still matches and whose only entry is that marker. Every imported directory remains non-owned and is preserved, even if a matching marker already exists or appears later.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:            true,
				Description:         "The normalized directory path below public_html, relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path below `public_html`, relative to the cPanel account home.",
				Validators:          filesystemDirectoryPathValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"absolute_path": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
			"permissions": schema.StringAttribute{
				Computed:            true,
				Description:         "The four-digit octal directory permissions reported by cPanel.",
				MarkdownDescription: "The four-digit octal directory permissions reported by cPanel. New directories are requested with `0755`.",
			},
			"owned": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the directory carries a verified Terraform ownership marker and will be deleted on destroy.",
				MarkdownDescription: "Whether the directory carries a verified Terraform ownership marker and will be deleted on destroy. Imported directories without a marker remain `false` and are preserved.",
			},
		},
	}
}

func (r *filesystemDirectoryResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan FilesystemDirectoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directoryPath := plan.Path.ValueString()
	if err := validateFilesystemDirectoryPath(directoryPath); err != nil {
		resp.Diagnostics.AddError("Invalid filesystem directory", err.Error())
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	existing, err := r.client.GetDirectory(ctx, directoryPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to inspect filesystem directory",
			err.Error(),
		)
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Filesystem directory already exists",
			fmt.Sprintf(
				"Directory %q already exists. Import it instead of taking ownership implicitly.",
				directoryPath,
			),
		)
		return
	}

	token, err := r.generateOwnershipToken()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create filesystem directory ownership",
			err.Error(),
		)
		return
	}
	markerContent, err := filesystemDirectoryMarkerContent(
		directoryPath,
		token,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create filesystem directory ownership",
			err.Error(),
		)
		return
	}

	created, mutationErr := r.client.CreateDirectory(ctx, directoryPath)
	observed, readErr := r.client.GetDirectory(ctx, directoryPath)
	if mutationErr != nil {
		if readErr != nil {
			resp.Diagnostics.AddError(
				"Unable to create filesystem directory",
				errors.Join(mutationErr, readErr).Error(),
			)
			return
		}
		if observed != nil {
			resp.Diagnostics.AddError(
				"Filesystem directory creation response is ambiguous",
				fmt.Sprintf(
					"%v. Directory %q now exists, but Terraform will not adopt or delete it automatically. Inspect it and import it if it is the intended directory.",
					mutationErr,
					directoryPath,
				),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to create filesystem directory",
			mutationErr.Error(),
		)
		return
	}
	if readErr != nil {
		rollbackErr := r.rollbackCreatedDirectory(ctx, directoryPath, token)
		resp.Diagnostics.AddError(
			"Unable to verify filesystem directory creation",
			errors.Join(readErr, rollbackErr).Error(),
		)
		return
	}
	if created == nil || observed == nil {
		rollbackErr := r.rollbackCreatedDirectory(ctx, directoryPath, token)
		resp.Diagnostics.AddError(
			"Unable to verify filesystem directory creation",
			errors.Join(
				fmt.Errorf(
					"directory %q was not found after creation",
					directoryPath,
				),
				rollbackErr,
			).Error(),
		)
		return
	}

	markerPath := filesystemDirectoryMarkerPath(directoryPath)
	markerMutationErr := error(nil)
	if _, err := r.client.SaveTextFile(
		ctx,
		markerPath,
		markerContent,
	); err != nil {
		markerMutationErr = err
	}
	marker, markerReadErr := r.client.GetTextFile(ctx, markerPath)
	if markerReadErr != nil ||
		marker == nil ||
		marker.Content != markerContent {
		rollbackErr := r.rollbackCreatedDirectory(ctx, directoryPath, token)
		verificationErr := markerReadErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"directory %q ownership marker was not stored exactly",
				directoryPath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to establish filesystem directory ownership",
			errors.Join(
				markerMutationErr,
				verificationErr,
				rollbackErr,
			).Error(),
		)
		return
	}
	if err := r.verifyOnlyOwnershipMarker(
		ctx,
		directoryPath,
		markerContent,
	); err != nil {
		rollbackErr := r.rollbackCreatedDirectory(ctx, directoryPath, token)
		resp.Diagnostics.AddError(
			"Unable to verify filesystem directory ownership",
			errors.Join(err, rollbackErr).Error(),
		)
		return
	}

	applyFilesystemDirectoryToResourceModel(&plan, *observed, true)
	privateDiagnostics := writeFilesystemDirectoryOwnershipToken(
		ctx,
		resp.Private,
		token,
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if privateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreatedDirectory(ctx, directoryPath, token)
		resp.Diagnostics.AddError(
			"Unable to store filesystem directory ownership",
			fmt.Sprintf("Creation rollback result: %v.", rollbackErr),
		)
		return
	}

	stateDiagnostics := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreatedDirectory(ctx, directoryPath, token)
		resp.Diagnostics.AddError(
			"Unable to store filesystem directory state",
			fmt.Sprintf("Creation rollback result: %v.", rollbackErr),
		)
	}
}

func (r *filesystemDirectoryResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state FilesystemDirectoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directoryPath := state.Path.ValueString()
	if err := validateFilesystemDirectoryPath(directoryPath); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem directory in state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	directory, err := r.client.GetDirectory(ctx, directoryPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem directory",
			err.Error(),
		)
		return
	}
	if directory == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	token, owned, recovered, privateDiagnostics, err := r.resolveOwnership(
		ctx,
		directoryPath,
		req.Private,
		filesystemDirectoryMayOwn(state.Owned),
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify filesystem directory ownership",
			err.Error(),
		)
		return
	}
	if recovered {
		resp.Diagnostics.Append(
			writeFilesystemDirectoryOwnershipToken(
				ctx,
				resp.Private,
				token,
			)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	applyFilesystemDirectoryToResourceModel(&state, *directory, owned)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *filesystemDirectoryResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan FilesystemDirectoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directoryPath := plan.Path.ValueString()
	if err := validateFilesystemDirectoryPath(directoryPath); err != nil {
		resp.Diagnostics.AddError("Invalid filesystem directory", err.Error())
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	directory, err := r.client.GetDirectory(ctx, directoryPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem directory",
			err.Error(),
		)
		return
	}
	if directory == nil {
		resp.Diagnostics.AddError(
			"Filesystem directory no longer exists",
			"Refresh the Terraform state before retrying so Terraform can recreate the missing directory.",
		)
		return
	}

	token, owned, recovered, privateDiagnostics, err := r.resolveOwnership(
		ctx,
		directoryPath,
		req.Private,
		filesystemDirectoryMayOwn(plan.Owned),
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify filesystem directory ownership",
			err.Error(),
		)
		return
	}
	if recovered {
		resp.Diagnostics.Append(
			writeFilesystemDirectoryOwnershipToken(
				ctx,
				resp.Private,
				token,
			)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	applyFilesystemDirectoryToResourceModel(&plan, *directory, owned)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *filesystemDirectoryResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state FilesystemDirectoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directoryPath := state.Path.ValueString()
	if err := validateFilesystemDirectoryPath(directoryPath); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem directory in state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	directory, err := r.client.GetDirectory(ctx, directoryPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem directory",
			err.Error(),
		)
		return
	}
	if directory == nil {
		return
	}
	if !state.AbsolutePath.IsNull() &&
		!state.AbsolutePath.IsUnknown() &&
		state.AbsolutePath.ValueString() != directory.Entry.AbsolutePath {
		resp.Diagnostics.AddError(
			"Filesystem directory identity changed",
			fmt.Sprintf(
				"Directory %q resolved to %q instead of state path %q; refusing deletion.",
				directoryPath,
				directory.Entry.AbsolutePath,
				state.AbsolutePath.ValueString(),
			),
		)
		return
	}

	token, owned, _, privateDiagnostics, err := r.resolveOwnership(
		ctx,
		directoryPath,
		req.Private,
		filesystemDirectoryMayOwn(state.Owned),
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify filesystem directory ownership",
			err.Error(),
		)
		return
	}
	if !owned {
		resp.Diagnostics.AddWarning(
			"Preserving imported filesystem directory",
			fmt.Sprintf(
				"Directory %q has no Terraform ownership marker. Terraform removed it from state without deleting the directory or its contents.",
				directoryPath,
			),
		)
		return
	}

	if err := r.deleteOwnedDirectory(ctx, directoryPath, token); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete filesystem directory safely",
			err.Error(),
		)
	}
}

func (r *filesystemDirectoryResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := validateFilesystemDirectoryPath(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem directory import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("path"), req.ID)...,
	)
}

func (r *filesystemDirectoryResource) Configure(
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

	r.client = client
}
