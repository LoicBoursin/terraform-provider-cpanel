package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

const (
	filesystemDirectoryMarkerName          = ".terraform-cpanel-directory"
	filesystemDirectoryOwnershipPrivateKey = "filesystem_directory_ownership_v1"
	filesystemDirectoryMarkerProvider      = "terraform-provider-cpanel"
	filesystemDirectoryMarkerVersion       = 1
)

type filesystemDirectoryPrivateStateReader interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}

type filesystemDirectoryPrivateStateWriter interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

type filesystemDirectoryOwnershipState struct {
	Token string `json:"token"`
}

type filesystemDirectoryMarker struct {
	Provider string `json:"provider"`
	Version  int    `json:"version"`
	Path     string `json:"path"`
	Token    string `json:"token"`
}

func generateFilesystemDirectoryOwnershipToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate filesystem directory ownership token: %w", err)
	}

	return hex.EncodeToString(value), nil
}

func filesystemDirectoryMarkerPath(directoryPath string) string {
	return path.Join(directoryPath, filesystemDirectoryMarkerName)
}

func filesystemDirectoryMarkerContent(
	directoryPath string,
	token string,
) (string, error) {
	if err := validateFilesystemDirectoryOwnershipToken(token); err != nil {
		return "", err
	}

	value, err := json.Marshal(filesystemDirectoryMarker{
		Provider: filesystemDirectoryMarkerProvider,
		Version:  filesystemDirectoryMarkerVersion,
		Path:     directoryPath,
		Token:    token,
	})
	if err != nil {
		return "", fmt.Errorf("encode filesystem directory ownership marker: %w", err)
	}

	return string(value), nil
}

func parseFilesystemDirectoryMarker(
	content string,
	expectedPath string,
) (string, error) {
	var marker filesystemDirectoryMarker
	if err := json.Unmarshal([]byte(content), &marker); err != nil {
		return "", fmt.Errorf("decode filesystem directory ownership marker: %w", err)
	}
	if marker.Provider != filesystemDirectoryMarkerProvider ||
		marker.Version != filesystemDirectoryMarkerVersion ||
		marker.Path != expectedPath {
		return "", fmt.Errorf(
			"filesystem directory ownership marker does not match directory %q",
			expectedPath,
		)
	}
	if err := validateFilesystemDirectoryOwnershipToken(marker.Token); err != nil {
		return "", err
	}

	return marker.Token, nil
}

func validateFilesystemDirectoryOwnershipToken(token string) error {
	if len(token) != 64 {
		return fmt.Errorf("filesystem directory ownership token must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return fmt.Errorf("decode filesystem directory ownership token: %w", err)
	}

	return nil
}

func readFilesystemDirectoryOwnershipToken(
	ctx context.Context,
	privateState filesystemDirectoryPrivateStateReader,
) (string, bool, diag.Diagnostics) {
	var diagnostics diag.Diagnostics
	if privateState == nil {
		return "", false, diagnostics
	}

	value, privateDiagnostics := privateState.GetKey(
		ctx,
		filesystemDirectoryOwnershipPrivateKey,
	)
	diagnostics.Append(privateDiagnostics...)
	if diagnostics.HasError() || len(value) == 0 {
		return "", false, diagnostics
	}

	var state filesystemDirectoryOwnershipState
	if err := json.Unmarshal(value, &state); err != nil {
		diagnostics.AddError(
			"Invalid filesystem directory private state",
			fmt.Sprintf("Decode the ownership token: %v.", err),
		)
		return "", false, diagnostics
	}
	if err := validateFilesystemDirectoryOwnershipToken(state.Token); err != nil {
		diagnostics.AddError(
			"Invalid filesystem directory private state",
			err.Error(),
		)
		return "", false, diagnostics
	}

	return state.Token, true, diagnostics
}

func writeFilesystemDirectoryOwnershipToken(
	ctx context.Context,
	privateState filesystemDirectoryPrivateStateWriter,
	token string,
) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	if privateState == nil {
		diagnostics.AddError(
			"Unable to store filesystem directory ownership",
			"Terraform did not initialize private resource state.",
		)
		return diagnostics
	}
	if err := validateFilesystemDirectoryOwnershipToken(token); err != nil {
		diagnostics.AddError(
			"Unable to store filesystem directory ownership",
			err.Error(),
		)
		return diagnostics
	}

	value, err := json.Marshal(filesystemDirectoryOwnershipState{Token: token})
	if err != nil {
		diagnostics.AddError(
			"Unable to store filesystem directory ownership",
			fmt.Sprintf("Encode the ownership token: %v.", err),
		)
		return diagnostics
	}
	diagnostics.Append(
		privateState.SetKey(
			ctx,
			filesystemDirectoryOwnershipPrivateKey,
			value,
		)...,
	)

	return diagnostics
}

func (r *filesystemDirectoryResource) resolveOwnership(
	ctx context.Context,
	directoryPath string,
	privateState filesystemDirectoryPrivateStateReader,
	mayOwn bool,
) (string, bool, bool, diag.Diagnostics, error) {
	if !mayOwn {
		return "", false, false, nil, nil
	}

	token, found, diagnostics := readFilesystemDirectoryOwnershipToken(
		ctx,
		privateState,
	)
	if diagnostics.HasError() {
		return "", false, false, diagnostics, nil
	}

	marker, err := r.client.GetTextFile(
		ctx,
		filesystemDirectoryMarkerPath(directoryPath),
	)
	if err != nil {
		return "", false, false, diagnostics, err
	}
	if marker == nil {
		if found {
			return "", false, false, diagnostics, fmt.Errorf(
				"filesystem directory %q lost its Terraform ownership marker",
				directoryPath,
			)
		}

		return "", false, false, diagnostics, nil
	}

	markerToken, err := parseFilesystemDirectoryMarker(
		marker.Content,
		directoryPath,
	)
	if err != nil {
		return "", false, false, diagnostics, err
	}
	if found && markerToken != token {
		return "", false, false, diagnostics, fmt.Errorf(
			"filesystem directory %q ownership marker does not match Terraform private state",
			directoryPath,
		)
	}

	return markerToken, true, !found, diagnostics, nil
}

func (r *filesystemDirectoryResource) verifyOnlyOwnershipMarker(
	ctx context.Context,
	directoryPath string,
	expectedContent string,
) error {
	entries, err := r.client.ListDirectory(ctx, directoryPath)
	if err != nil {
		return err
	}
	if len(entries) != 1 ||
		entries[0].Path != filesystemDirectoryMarkerPath(directoryPath) ||
		entries[0].Type != fileman.EntryTypeFile {
		return fmt.Errorf(
			"directory %q contains entries outside Terraform ownership; refusing deletion",
			directoryPath,
		)
	}

	marker, err := r.client.GetTextFile(
		ctx,
		filesystemDirectoryMarkerPath(directoryPath),
	)
	if err != nil {
		return err
	}
	if marker == nil || marker.Content != expectedContent {
		return fmt.Errorf(
			"directory %q ownership marker changed; refusing deletion",
			directoryPath,
		)
	}

	return nil
}

func (r *filesystemDirectoryResource) restoreOwnershipMarker(
	ctx context.Context,
	directoryPath string,
	content string,
) error {
	directory, err := r.client.GetDirectory(ctx, directoryPath)
	if err != nil {
		return err
	}
	if directory == nil {
		return fmt.Errorf(
			"directory %q no longer exists while restoring its ownership marker",
			directoryPath,
		)
	}

	markerPath := filesystemDirectoryMarkerPath(directoryPath)
	marker, err := r.client.GetTextFile(ctx, markerPath)
	if err != nil {
		return err
	}
	if marker != nil {
		if marker.Content == content {
			return nil
		}

		return fmt.Errorf(
			"refusing to overwrite changed ownership marker in directory %q",
			directoryPath,
		)
	}

	mutationErr := error(nil)
	if _, err := r.client.SaveTextFile(ctx, markerPath, content); err != nil {
		mutationErr = err
	}
	marker, readErr := r.client.GetTextFile(ctx, markerPath)
	if readErr != nil {
		return errors.Join(mutationErr, readErr)
	}
	if marker == nil || marker.Content != content {
		return errors.Join(
			mutationErr,
			fmt.Errorf(
				"ownership marker for directory %q was not restored",
				directoryPath,
			),
		)
	}

	return nil
}

func (r *filesystemDirectoryResource) deleteOwnedDirectory(
	ctx context.Context,
	directoryPath string,
	token string,
) error {
	expectedContent, err := filesystemDirectoryMarkerContent(
		directoryPath,
		token,
	)
	if err != nil {
		return err
	}
	if err := r.verifyOnlyOwnershipMarker(
		ctx,
		directoryPath,
		expectedContent,
	); err != nil {
		return err
	}

	markerPath := filesystemDirectoryMarkerPath(directoryPath)
	markerMutationErr := r.client.DeletePath(ctx, markerPath)
	marker, markerReadErr := r.client.GetTextFile(ctx, markerPath)
	if markerReadErr != nil {
		return errors.Join(markerMutationErr, markerReadErr)
	}
	if marker != nil {
		return errors.Join(
			markerMutationErr,
			fmt.Errorf(
				"ownership marker for directory %q still exists after deletion",
				directoryPath,
			),
		)
	}

	entries, listErr := r.client.ListDirectory(ctx, directoryPath)
	if listErr != nil {
		restoreErr := r.restoreOwnershipMarker(
			ctx,
			directoryPath,
			expectedContent,
		)
		return errors.Join(markerMutationErr, listErr, restoreErr)
	}
	if len(entries) != 0 {
		restoreErr := r.restoreOwnershipMarker(
			ctx,
			directoryPath,
			expectedContent,
		)
		return errors.Join(
			markerMutationErr,
			fmt.Errorf(
				"directory %q changed during deletion; refusing to remove it",
				directoryPath,
			),
			restoreErr,
		)
	}

	directoryMutationErr := r.client.DeletePath(ctx, directoryPath)
	directory, directoryReadErr := r.client.GetDirectory(ctx, directoryPath)
	if directoryReadErr == nil && directory == nil {
		return nil
	}

	restoreErr := r.restoreOwnershipMarker(
		ctx,
		directoryPath,
		expectedContent,
	)
	if directoryReadErr != nil {
		return errors.Join(
			markerMutationErr,
			directoryMutationErr,
			directoryReadErr,
			restoreErr,
		)
	}

	return errors.Join(
		markerMutationErr,
		directoryMutationErr,
		fmt.Errorf(
			"directory %q still exists after deletion",
			directoryPath,
		),
		restoreErr,
	)
}

func (r *filesystemDirectoryResource) rollbackCreatedDirectory(
	ctx context.Context,
	directoryPath string,
	token string,
) error {
	return r.deleteOwnedDirectory(ctx, directoryPath, token)
}
