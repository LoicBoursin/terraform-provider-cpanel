package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

var (
	_ resource.Resource                = &filesystemTextFileResource{}
	_ resource.ResourceWithConfigure   = &filesystemTextFileResource{}
	_ resource.ResourceWithImportState = &filesystemTextFileResource{}
)

func NewFilesystemTextFileResource() resource.Resource {
	return &filesystemTextFileResource{
		generateOwnershipToken: generateFilesystemTextFileOwnershipToken,
	}
}

type filesystemTextFileClient interface {
	filesystemTextFileReadClient

	LockMutations() func()
	CreateEmptyTextFile(context.Context, string) (*fileman.TextFile, error)
	SaveTextFile(context.Context, string, string) (*fileman.TextFile, error)
	DeletePath(context.Context, string) error
}

type filesystemTextFileResource struct {
	client                 filesystemTextFileClient
	generateOwnershipToken func() (string, error)
}

func (r *filesystemTextFileResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_text_file"
}

func (r *filesystemTextFileResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Creates and manages one ownership-marked UTF-8 text file below public_html in a cPanel account.",
		MarkdownDescription: "Creates and manages one ownership-marked UTF-8 text file below `public_html` in a cPanel account. Content is limited to 1 MiB. Terraform deletes only a file whose private ownership token, sidecar marker, size, and SHA-256 digest still match. Every imported file remains non-owned and is preserved, even if a matching marker already exists or appears later.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:            true,
				Description:         "The normalized file path below public_html, relative to the cPanel account home.",
				MarkdownDescription: "The normalized file path below `public_html`, relative to the cPanel account home.",
				Validators:          filesystemTextFilePathValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				Description:         "The exact UTF-8 file content, limited to 1 MiB.",
				MarkdownDescription: "The exact UTF-8 file content, limited to 1 MiB.",
				Validators:          filesystemTextFileContentValidators(),
			},
			"absolute_path": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute file path reported by cPanel.",
				MarkdownDescription: "The absolute file path reported by cPanel.",
			},
			"permissions": schema.StringAttribute{
				Computed:            true,
				Description:         "The four-digit octal file permissions reported by cPanel.",
				MarkdownDescription: "The four-digit octal file permissions reported by cPanel. New files are created with `0644`.",
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
			"owned": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the file carries verified Terraform ownership and may be updated or deleted.",
				MarkdownDescription: "Whether the file carries verified Terraform ownership and may be updated or deleted. Imported files without a marker remain `false` and are preserved.",
			},
			"content_matches_ownership_marker": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the current content size and SHA-256 digest match the verified ownership marker.",
				MarkdownDescription: "Whether the current content size and SHA-256 digest match the verified ownership marker. Terraform refuses deletion while this is `false`.",
				PlanModifiers: []planmodifier.Bool{
					filesystemTextFileOwnershipMatchPlanModifier{},
				},
			},
		},
	}
}

func (r *filesystemTextFileResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan FilesystemTextFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filePath := plan.Path.ValueString()
	content := plan.Content.ValueString()
	if err := validateFilesystemTextFilePath(filePath); err != nil {
		resp.Diagnostics.AddError("Invalid filesystem text file", err.Error())
		return
	}
	if err := validateFilesystemTextFileContent(content); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem text file content",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	existing, err := r.client.GetEntry(ctx, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to inspect filesystem text file",
			err.Error(),
		)
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Filesystem text file already exists",
			fmt.Sprintf(
				"Path %q already exists. Import it instead of taking ownership implicitly.",
				filePath,
			),
		)
		return
	}

	markerPath := filesystemTextFileMarkerPath(filePath)
	existingMarker, err := r.client.GetEntry(ctx, markerPath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to inspect filesystem text file ownership marker",
			err.Error(),
		)
		return
	}
	if existingMarker != nil {
		resp.Diagnostics.AddError(
			"Filesystem text file ownership marker already exists",
			fmt.Sprintf(
				"Reserved marker %q already exists. Inspect and remove the orphaned marker before retrying.",
				markerPath,
			),
		)
		return
	}

	token, err := r.generateOwnershipToken()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create filesystem text file ownership",
			err.Error(),
		)
		return
	}
	marker, err := newFilesystemTextFileMarker(filePath, token, content)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create filesystem text file ownership",
			err.Error(),
		)
		return
	}
	markerContent, err := filesystemTextFileMarkerContent(marker)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create filesystem text file ownership",
			err.Error(),
		)
		return
	}

	created, mutationErr := r.client.CreateEmptyTextFile(ctx, filePath)
	observed, readErr := readFilesystemTextFile(ctx, r.client, filePath)
	if mutationErr != nil {
		if readErr != nil {
			resp.Diagnostics.AddError(
				"Unable to create filesystem text file",
				errors.Join(mutationErr, readErr).Error(),
			)
			return
		}
		if observed != nil {
			resp.Diagnostics.AddError(
				"Filesystem text file creation response is ambiguous",
				fmt.Sprintf(
					"%v. Path %q now exists, but Terraform will not adopt or delete it automatically. Inspect it and import it if it is the intended file.",
					mutationErr,
					filePath,
				),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to create filesystem text file",
			mutationErr.Error(),
		)
		return
	}
	if readErr != nil || created == nil || observed == nil ||
		created.Content != "" || observed.Content != "" {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			false,
		)
		verificationErr := readErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"text file %q was not created as an empty file",
				filePath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify filesystem text file creation",
			errors.Join(verificationErr, rollbackErr).Error(),
		)
		return
	}

	saveErr := error(nil)
	if _, err := r.client.SaveTextFile(ctx, filePath, content); err != nil {
		saveErr = sensitiveMutationError(
			err,
			"filesystem text file content write",
		)
	}
	observed, readErr = readFilesystemTextFile(ctx, r.client, filePath)
	if readErr != nil || observed == nil || observed.Content != content {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			false,
		)
		verificationErr := readErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"text file %q content was not stored exactly",
				filePath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to store filesystem text file content",
			errors.Join(saveErr, verificationErr, rollbackErr).Error(),
		)
		return
	}

	createdMarker, markerCreateErr := r.client.CreateEmptyTextFile(
		ctx,
		markerPath,
	)
	observedMarker, markerReadErr := r.client.GetTextFile(ctx, markerPath)
	if markerCreateErr != nil {
		markerCreated := markerReadErr == nil &&
			observedMarker != nil &&
			observedMarker.Content == ""
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			markerCreated,
		)
		resp.Diagnostics.AddError(
			"Unable to create filesystem text file ownership marker",
			errors.Join(markerCreateErr, markerReadErr, rollbackErr).Error(),
		)
		return
	}
	if markerReadErr != nil || createdMarker == nil ||
		observedMarker == nil || createdMarker.Content != "" ||
		observedMarker.Content != "" {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		)
		verificationErr := markerReadErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"ownership marker %q was not created as an empty file",
				markerPath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify filesystem text file ownership marker",
			errors.Join(verificationErr, rollbackErr).Error(),
		)
		return
	}

	markerSaveErr := error(nil)
	if _, err := r.client.SaveTextFile(
		ctx,
		markerPath,
		markerContent,
	); err != nil {
		markerSaveErr = sensitiveMutationError(
			err,
			"filesystem ownership marker write",
		)
	}
	observedMarker, markerReadErr = r.client.GetTextFile(ctx, markerPath)
	if markerReadErr != nil || observedMarker == nil ||
		observedMarker.Content != markerContent {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		)
		verificationErr := markerReadErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"ownership marker %q was not stored exactly",
				markerPath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to establish filesystem text file ownership",
			errors.Join(
				markerSaveErr,
				verificationErr,
				rollbackErr,
			).Error(),
		)
		return
	}

	observed, readErr = readFilesystemTextFile(ctx, r.client, filePath)
	if readErr != nil || observed == nil ||
		!filesystemTextFileMarkerMatches(*observed, marker) {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		)
		verificationErr := readErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"text file %q does not match its ownership marker",
				filePath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify filesystem text file ownership",
			errors.Join(verificationErr, rollbackErr).Error(),
		)
		return
	}

	applyFilesystemTextFileToResourceModel(&plan, *observed, true, true)
	privateDiagnostics := writeFilesystemTextFileOwnershipToken(
		ctx,
		resp.Private,
		token,
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if privateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		)
		resp.Diagnostics.AddError(
			"Unable to store filesystem text file ownership",
			fmt.Sprintf("Creation rollback result: %v.", rollbackErr),
		)
		return
	}

	stateDiagnostics := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreatedTextFile(
			ctx,
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		)
		resp.Diagnostics.AddError(
			"Unable to store filesystem text file state",
			fmt.Sprintf("Creation rollback result: %v.", rollbackErr),
		)
	}
}

func (r *filesystemTextFileResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state FilesystemTextFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filePath := state.Path.ValueString()
	if err := validateFilesystemTextFilePath(filePath); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem text file in state",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	textFile, err := readFilesystemTextFile(ctx, r.client, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem text file",
			err.Error(),
		)
		return
	}
	if textFile == nil {
		if !state.Owned.IsNull() &&
			!state.Owned.IsUnknown() &&
			state.Owned.ValueBool() {
			privateDiagnostics, cleanupErr := r.cleanupMarkerForMissingTextFile(
				ctx,
				filePath,
				req.Private,
			)
			resp.Diagnostics.Append(privateDiagnostics...)
			if resp.Diagnostics.HasError() {
				return
			}
			if cleanupErr != nil {
				resp.Diagnostics.AddError(
					"Unable to clean up missing filesystem text file ownership",
					cleanupErr.Error(),
				)
				return
			}
		}
		resp.State.RemoveResource(ctx)
		return
	}
	if !state.AbsolutePath.IsNull() &&
		!state.AbsolutePath.IsUnknown() &&
		state.AbsolutePath.ValueString() != textFile.Entry.AbsolutePath {
		resp.Diagnostics.AddError(
			"Filesystem text file identity changed",
			fmt.Sprintf(
				"File %q resolved to %q instead of state path %q.",
				filePath,
				textFile.Entry.AbsolutePath,
				state.AbsolutePath.ValueString(),
			),
		)
		return
	}

	ownership, privateDiagnostics, err := r.resolveOwnership(
		ctx,
		filePath,
		req.Private,
		filesystemTextFileMayOwn(state.Owned),
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify filesystem text file ownership",
			err.Error(),
		)
		return
	}

	contentMatches := ownership.Owned &&
		filesystemTextFileMarkerMatches(*textFile, ownership.Marker)
	if ownership.Recovered && !contentMatches {
		resp.Diagnostics.AddError(
			"Filesystem text file ownership marker is stale",
			fmt.Sprintf(
				"Marker %q does not match the current size and SHA-256 digest of %q. Terraform will not adopt the file.",
				filesystemTextFileMarkerPath(filePath),
				filePath,
			),
		)
		return
	}
	if ownership.Recovered {
		resp.Diagnostics.Append(
			writeFilesystemTextFileOwnershipToken(
				ctx,
				resp.Private,
				ownership.Token,
			)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if ownership.Owned && !contentMatches {
		resp.Diagnostics.AddWarning(
			"Filesystem text file content drifted",
			fmt.Sprintf(
				"File %q no longer matches its ownership marker. Terraform can restore configured content but will refuse deletion until the marker is synchronized.",
				filePath,
			),
		)
	}

	applyFilesystemTextFileToResourceModel(
		&state,
		*textFile,
		ownership.Owned,
		contentMatches,
	)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *filesystemTextFileResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan FilesystemTextFileResourceModel
	var state FilesystemTextFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filePath := plan.Path.ValueString()
	content := plan.Content.ValueString()
	if err := validateFilesystemTextFilePath(filePath); err != nil {
		resp.Diagnostics.AddError("Invalid filesystem text file", err.Error())
		return
	}
	if err := validateFilesystemTextFileContent(content); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem text file content",
			err.Error(),
		)
		return
	}
	if !filesystemTextFileMayOwn(state.Owned) {
		resp.Diagnostics.AddError(
			"Imported filesystem text file is not owned",
			fmt.Sprintf(
				"File %q was imported without a Terraform ownership marker. Terraform will not overwrite it.",
				filePath,
			),
		)
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	textFile, err := readFilesystemTextFile(ctx, r.client, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem text file",
			err.Error(),
		)
		return
	}
	if textFile == nil {
		resp.Diagnostics.AddError(
			"Filesystem text file no longer exists",
			"Refresh the Terraform state before retrying so Terraform can recreate the missing file.",
		)
		return
	}
	if !state.AbsolutePath.IsNull() &&
		!state.AbsolutePath.IsUnknown() &&
		state.AbsolutePath.ValueString() != textFile.Entry.AbsolutePath {
		resp.Diagnostics.AddError(
			"Filesystem text file identity changed",
			fmt.Sprintf(
				"File %q resolved to %q instead of state path %q; refusing update.",
				filePath,
				textFile.Entry.AbsolutePath,
				state.AbsolutePath.ValueString(),
			),
		)
		return
	}
	if state.Content.IsNull() || state.Content.IsUnknown() ||
		textFile.Content != state.Content.ValueString() {
		resp.Diagnostics.AddError(
			"Filesystem text file changed during apply",
			fmt.Sprintf(
				"File %q no longer matches the content observed while planning. Refresh and review the new plan before retrying.",
				filePath,
			),
		)
		return
	}

	ownership, privateDiagnostics, err := r.resolveOwnership(
		ctx,
		filePath,
		req.Private,
		true,
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify filesystem text file ownership",
			err.Error(),
		)
		return
	}
	if !ownership.Owned {
		resp.Diagnostics.AddError(
			"Filesystem text file is not owned",
			fmt.Sprintf(
				"File %q has no verified Terraform ownership marker. Terraform will not overwrite it.",
				filePath,
			),
		)
		return
	}

	current, err := readFilesystemTextFile(ctx, r.client, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to recheck filesystem text file",
			err.Error(),
		)
		return
	}
	if current == nil ||
		current.Entry.AbsolutePath != textFile.Entry.AbsolutePath ||
		current.Content != state.Content.ValueString() {
		resp.Diagnostics.AddError(
			"Filesystem text file changed during apply",
			fmt.Sprintf(
				"File %q changed after ownership verification. Refresh and review the new plan before retrying.",
				filePath,
			),
		)
		return
	}
	currentMarker, err := r.client.GetTextFile(
		ctx,
		filesystemTextFileMarkerPath(filePath),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to recheck filesystem text file ownership marker",
			err.Error(),
		)
		return
	}
	if currentMarker == nil ||
		currentMarker.Content != ownership.MarkerContent {
		resp.Diagnostics.AddError(
			"Filesystem text file ownership marker changed",
			fmt.Sprintf(
				"Marker %q changed after ownership verification; refusing update.",
				filesystemTextFileMarkerPath(filePath),
			),
		)
		return
	}

	newMarker, err := newFilesystemTextFileMarker(
		filePath,
		ownership.Token,
		content,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update filesystem text file ownership",
			err.Error(),
		)
		return
	}
	newMarkerContent, err := filesystemTextFileMarkerContent(newMarker)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update filesystem text file ownership",
			err.Error(),
		)
		return
	}

	targetSaveErr := error(nil)
	if _, err := r.client.SaveTextFile(ctx, filePath, content); err != nil {
		targetSaveErr = sensitiveMutationError(
			err,
			"filesystem text file content update",
		)
	}
	updated, targetReadErr := readFilesystemTextFile(ctx, r.client, filePath)
	if targetReadErr != nil || updated == nil || updated.Content != content {
		verificationErr := targetReadErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"text file %q content was not updated exactly",
				filePath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to update filesystem text file content",
			errors.Join(targetSaveErr, verificationErr).Error(),
		)
		return
	}

	currentMarker, err = r.client.GetTextFile(
		ctx,
		filesystemTextFileMarkerPath(filePath),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to recheck filesystem text file ownership marker",
			err.Error(),
		)
		return
	}
	if currentMarker == nil ||
		currentMarker.Content != ownership.MarkerContent {
		resp.Diagnostics.AddError(
			"Filesystem text file ownership marker changed",
			fmt.Sprintf(
				"Marker %q changed while updating %q. Terraform preserved the changed marker and will require another reviewed apply.",
				filesystemTextFileMarkerPath(filePath),
				filePath,
			),
		)
		return
	}

	markerSaveErr := error(nil)
	if _, err := r.client.SaveTextFile(
		ctx,
		filesystemTextFileMarkerPath(filePath),
		newMarkerContent,
	); err != nil {
		markerSaveErr = sensitiveMutationError(
			err,
			"filesystem ownership marker update",
		)
	}
	updatedMarker, markerReadErr := r.client.GetTextFile(
		ctx,
		filesystemTextFileMarkerPath(filePath),
	)
	if markerReadErr != nil || updatedMarker == nil ||
		updatedMarker.Content != newMarkerContent {
		verificationErr := markerReadErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"ownership marker for %q was not updated exactly",
				filePath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to update filesystem text file ownership marker",
			errors.Join(markerSaveErr, verificationErr).Error(),
		)
		return
	}

	if ownership.Recovered {
		resp.Diagnostics.Append(
			writeFilesystemTextFileOwnershipToken(
				ctx,
				resp.Private,
				ownership.Token,
			)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	applyFilesystemTextFileToResourceModel(&plan, *updated, true, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *filesystemTextFileResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state FilesystemTextFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filePath := state.Path.ValueString()
	if err := validateFilesystemTextFilePath(filePath); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem text file in state",
			err.Error(),
		)
		return
	}
	if !filesystemTextFileMayOwn(state.Owned) {
		resp.Diagnostics.AddWarning(
			"Preserving imported filesystem text file",
			fmt.Sprintf(
				"File %q has no Terraform ownership marker. Terraform removed it from state without deleting the file or its content.",
				filePath,
			),
		)
		return
	}

	unlock := r.client.LockMutations()
	defer unlock()

	textFile, err := readFilesystemTextFile(ctx, r.client, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read filesystem text file",
			err.Error(),
		)
		return
	}
	if textFile == nil {
		privateDiagnostics, cleanupErr := r.cleanupMarkerForMissingTextFile(
			ctx,
			filePath,
			req.Private,
		)
		resp.Diagnostics.Append(privateDiagnostics...)
		if resp.Diagnostics.HasError() {
			return
		}
		if cleanupErr != nil {
			resp.Diagnostics.AddError(
				"Unable to clean up filesystem text file ownership marker",
				cleanupErr.Error(),
			)
		}
		return
	}
	if !state.AbsolutePath.IsNull() &&
		!state.AbsolutePath.IsUnknown() &&
		state.AbsolutePath.ValueString() != textFile.Entry.AbsolutePath {
		resp.Diagnostics.AddError(
			"Filesystem text file identity changed",
			fmt.Sprintf(
				"File %q resolved to %q instead of state path %q; refusing deletion.",
				filePath,
				textFile.Entry.AbsolutePath,
				state.AbsolutePath.ValueString(),
			),
		)
		return
	}

	ownership, privateDiagnostics, err := r.resolveOwnership(
		ctx,
		filePath,
		req.Private,
		true,
	)
	resp.Diagnostics.Append(privateDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify filesystem text file ownership",
			err.Error(),
		)
		return
	}
	if !ownership.Owned {
		resp.Diagnostics.AddError(
			"Filesystem text file is not owned",
			fmt.Sprintf(
				"File %q has no verified Terraform ownership marker; refusing deletion.",
				filePath,
			),
		)
		return
	}

	current, err := readFilesystemTextFile(ctx, r.client, filePath)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to recheck filesystem text file",
			err.Error(),
		)
		return
	}
	if current == nil ||
		current.Entry.AbsolutePath != textFile.Entry.AbsolutePath ||
		!filesystemTextFileMarkerMatches(*current, ownership.Marker) {
		resp.Diagnostics.AddError(
			"Filesystem text file content changed",
			fmt.Sprintf(
				"File %q no longer matches its verified ownership marker; refusing deletion.",
				filePath,
			),
		)
		return
	}

	targetMutationErr := r.client.DeletePath(ctx, filePath)
	remaining, targetReadErr := r.client.GetEntry(ctx, filePath)
	if targetReadErr != nil || remaining != nil {
		verificationErr := targetReadErr
		if verificationErr == nil {
			verificationErr = fmt.Errorf(
				"text file %q still exists after deletion",
				filePath,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to delete filesystem text file safely",
			errors.Join(targetMutationErr, verificationErr).Error(),
		)
		return
	}

	if err := r.deleteExactTextFile(
		ctx,
		filesystemTextFileMarkerPath(filePath),
		ownership.MarkerContent,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete filesystem text file ownership marker",
			err.Error(),
		)
	}
}

func (r *filesystemTextFileResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := validateFilesystemTextFilePath(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid filesystem text file import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("path"), req.ID)...,
	)
}

func (r *filesystemTextFileResource) Configure(
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

	r.client = client
}

func (r *filesystemTextFileResource) rollbackCreatedTextFile(
	ctx context.Context,
	filePath string,
	content string,
	markerPath string,
	markerContent string,
	markerCreated bool,
) error {
	if !markerCreated {
		return fmt.Errorf(
			"ownership marker %q was not established; preserving text file %q",
			markerPath,
			filePath,
		)
	}

	marker, err := r.client.GetTextFile(ctx, markerPath)
	if err != nil {
		return fmt.Errorf("read ownership marker before rollback: %w", err)
	}
	if marker == nil || marker.Content != markerContent {
		return fmt.Errorf(
			"ownership marker %q does not match the generated token; preserving text file %q",
			markerPath,
			filePath,
		)
	}

	if err := r.deleteCreatedTextFileIfContentMatches(
		ctx,
		filePath,
		"",
		content,
	); err != nil {
		return err
	}

	return r.deleteCreatedTextFileIfContentMatches(
		ctx,
		markerPath,
		markerContent,
	)
}

func (r *filesystemTextFileResource) deleteCreatedTextFileIfContentMatches(
	ctx context.Context,
	filePath string,
	allowedContents ...string,
) error {
	textFile, err := r.client.GetTextFile(ctx, filePath)
	if err != nil {
		return err
	}
	if textFile == nil {
		return nil
	}

	allowed := false
	for _, content := range allowedContents {
		if textFile.Content == content {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf(
			"text file %q changed during creation rollback; refusing deletion",
			filePath,
		)
	}

	mutationErr := r.client.DeletePath(ctx, filePath)
	remaining, readErr := r.client.GetEntry(ctx, filePath)
	if readErr == nil && remaining == nil {
		return nil
	}
	if readErr != nil {
		return errors.Join(mutationErr, readErr)
	}

	return errors.Join(
		mutationErr,
		fmt.Errorf(
			"text file %q still exists after creation rollback",
			filePath,
		),
	)
}

func (r *filesystemTextFileResource) deleteExactTextFile(
	ctx context.Context,
	filePath string,
	expectedContent string,
) error {
	textFile, err := r.client.GetTextFile(ctx, filePath)
	if err != nil {
		return err
	}
	if textFile == nil {
		return nil
	}
	if textFile.Content != expectedContent {
		return fmt.Errorf(
			"text file %q changed; refusing deletion",
			filePath,
		)
	}

	mutationErr := r.client.DeletePath(ctx, filePath)
	remaining, readErr := r.client.GetEntry(ctx, filePath)
	if readErr == nil && remaining == nil {
		return nil
	}
	if readErr != nil {
		return errors.Join(mutationErr, readErr)
	}

	return errors.Join(
		mutationErr,
		fmt.Errorf("text file %q still exists after deletion", filePath),
	)
}

func (r *filesystemTextFileResource) cleanupMarkerForMissingTextFile(
	ctx context.Context,
	filePath string,
	privateState filesystemTextFilePrivateStateReader,
) (diag.Diagnostics, error) {
	token, found, diagnostics := readFilesystemTextFileOwnershipToken(
		ctx,
		privateState,
	)
	if diagnostics.HasError() {
		return diagnostics, nil
	}

	markerPath := filesystemTextFileMarkerPath(filePath)
	markerFile, err := r.client.GetTextFile(ctx, markerPath)
	if err != nil {
		return diagnostics, err
	}
	if markerFile == nil {
		return diagnostics, nil
	}

	marker, err := parseFilesystemTextFileMarker(
		markerFile.Content,
		filePath,
	)
	if err != nil {
		return diagnostics, err
	}
	if found && marker.Token != token {
		return diagnostics, fmt.Errorf(
			"filesystem text file %q ownership marker does not match Terraform private state",
			filePath,
		)
	}

	return diagnostics, r.deleteExactTextFile(
		ctx,
		markerPath,
		markerFile.Content,
	)
}
