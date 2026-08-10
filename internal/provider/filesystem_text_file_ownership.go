package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

const (
	filesystemTextFileMarkerPrefix          = ".terraform-cpanel-text-file-"
	filesystemTextFileOwnershipPrivateKey   = "filesystem_text_file_ownership_v1"
	filesystemTextFileMarkerProvider        = "terraform-provider-cpanel"
	filesystemTextFileMarkerKind            = "text_file"
	filesystemTextFileMarkerVersion         = 1
	filesystemTextFileMaximumMarkerBytes    = 4096
	filesystemTextFileOwnershipTokenHexSize = 64
)

type filesystemTextFilePrivateStateReader interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}

type filesystemTextFilePrivateStateWriter interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

type filesystemTextFileOwnershipState struct {
	Token string `json:"token"`
}

type filesystemTextFileMarker struct {
	Provider      string `json:"provider"`
	Version       int    `json:"version"`
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	Token         string `json:"token"`
	ContentSHA256 string `json:"content_sha256"`
	SizeBytes     int64  `json:"size_bytes"`
}

type filesystemTextFileOwnership struct {
	Token         string
	Marker        filesystemTextFileMarker
	MarkerContent string
	Owned         bool
	Recovered     bool
}

func generateFilesystemTextFileOwnershipToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf(
			"generate filesystem text file ownership token: %w",
			err,
		)
	}

	return hex.EncodeToString(value), nil
}

func filesystemTextFileMarkerPath(filePath string) string {
	digest := sha256.Sum256([]byte(filePath))

	return path.Join(
		path.Dir(filePath),
		filesystemTextFileMarkerPrefix+hex.EncodeToString(digest[:]),
	)
}

func filesystemTextFileContentSHA256(content string) string {
	digest := sha256.Sum256([]byte(content))

	return hex.EncodeToString(digest[:])
}

func newFilesystemTextFileMarker(
	filePath string,
	token string,
	content string,
) (filesystemTextFileMarker, error) {
	if err := validateFilesystemTextFileOwnershipToken(token); err != nil {
		return filesystemTextFileMarker{}, err
	}
	if err := validateFilesystemTextFileContent(content); err != nil {
		return filesystemTextFileMarker{}, err
	}

	return filesystemTextFileMarker{
		Provider:      filesystemTextFileMarkerProvider,
		Version:       filesystemTextFileMarkerVersion,
		Kind:          filesystemTextFileMarkerKind,
		Path:          filePath,
		Token:         token,
		ContentSHA256: filesystemTextFileContentSHA256(content),
		SizeBytes:     int64(len([]byte(content))),
	}, nil
}

func filesystemTextFileMarkerContent(
	marker filesystemTextFileMarker,
) (string, error) {
	if err := validateFilesystemTextFileMarker(marker, marker.Path); err != nil {
		return "", err
	}

	value, err := json.Marshal(marker)
	if err != nil {
		return "", fmt.Errorf(
			"encode filesystem text file ownership marker: %w",
			err,
		)
	}
	if len(value) > filesystemTextFileMaximumMarkerBytes {
		return "", fmt.Errorf(
			"filesystem text file ownership marker for %q contains %d bytes; maximum supported size is %d bytes",
			marker.Path,
			len(value),
			filesystemTextFileMaximumMarkerBytes,
		)
	}

	return string(value), nil
}

func parseFilesystemTextFileMarker(
	content string,
	expectedPath string,
) (filesystemTextFileMarker, error) {
	if len([]byte(content)) > filesystemTextFileMaximumMarkerBytes {
		return filesystemTextFileMarker{}, fmt.Errorf(
			"filesystem text file ownership marker exceeds %d bytes",
			filesystemTextFileMaximumMarkerBytes,
		)
	}

	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	var marker filesystemTextFileMarker
	if err := decoder.Decode(&marker); err != nil {
		return filesystemTextFileMarker{}, fmt.Errorf(
			"decode filesystem text file ownership marker: %w",
			err,
		)
	}
	if err := ensureFilesystemTextFileMarkerEOF(decoder); err != nil {
		return filesystemTextFileMarker{}, err
	}
	if err := validateFilesystemTextFileMarker(
		marker,
		expectedPath,
	); err != nil {
		return filesystemTextFileMarker{}, err
	}

	canonical, err := json.Marshal(marker)
	if err != nil {
		return filesystemTextFileMarker{}, fmt.Errorf(
			"encode parsed filesystem text file ownership marker: %w",
			err,
		)
	}
	if !bytes.Equal(canonical, []byte(content)) {
		return filesystemTextFileMarker{}, fmt.Errorf(
			"filesystem text file ownership marker for %q is not canonical",
			expectedPath,
		)
	}

	return marker, nil
}

func ensureFilesystemTextFileMarkerEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf(
			"decode trailing filesystem text file ownership marker data: %w",
			err,
		)
	}

	return fmt.Errorf(
		"filesystem text file ownership marker contains trailing data",
	)
}

func validateFilesystemTextFileMarker(
	marker filesystemTextFileMarker,
	expectedPath string,
) error {
	if marker.Provider != filesystemTextFileMarkerProvider ||
		marker.Version != filesystemTextFileMarkerVersion ||
		marker.Kind != filesystemTextFileMarkerKind ||
		marker.Path != expectedPath {
		return fmt.Errorf(
			"filesystem text file ownership marker does not match file %q",
			expectedPath,
		)
	}
	if err := validateFilesystemTextFileOwnershipToken(marker.Token); err != nil {
		return err
	}
	if len(marker.ContentSHA256) != sha256.Size*2 ||
		marker.ContentSHA256 != strings.ToLower(marker.ContentSHA256) {
		return fmt.Errorf(
			"filesystem text file marker content_sha256 must contain 64 lowercase hexadecimal characters",
		)
	}
	if _, err := hex.DecodeString(marker.ContentSHA256); err != nil {
		return fmt.Errorf(
			"decode filesystem text file marker content_sha256: %w",
			err,
		)
	}
	if marker.SizeBytes < 0 ||
		marker.SizeBytes > filesystemTextFileMaximumContentBytes {
		return fmt.Errorf(
			"filesystem text file marker size_bytes must be between 0 and %d",
			filesystemTextFileMaximumContentBytes,
		)
	}

	return nil
}

func validateFilesystemTextFileOwnershipToken(token string) error {
	if len(token) != filesystemTextFileOwnershipTokenHexSize {
		return fmt.Errorf(
			"filesystem text file ownership token must contain 64 hexadecimal characters",
		)
	}
	if token != strings.ToLower(token) {
		return fmt.Errorf(
			"filesystem text file ownership token must use lowercase hexadecimal characters",
		)
	}
	if _, err := hex.DecodeString(token); err != nil {
		return fmt.Errorf(
			"decode filesystem text file ownership token: %w",
			err,
		)
	}

	return nil
}

func readFilesystemTextFileOwnershipToken(
	ctx context.Context,
	privateState filesystemTextFilePrivateStateReader,
) (string, bool, diag.Diagnostics) {
	var diagnostics diag.Diagnostics
	if privateState == nil {
		return "", false, diagnostics
	}

	value, privateDiagnostics := privateState.GetKey(
		ctx,
		filesystemTextFileOwnershipPrivateKey,
	)
	diagnostics.Append(privateDiagnostics...)
	if diagnostics.HasError() || len(value) == 0 {
		return "", false, diagnostics
	}

	var state filesystemTextFileOwnershipState
	if err := json.Unmarshal(value, &state); err != nil {
		diagnostics.AddError(
			"Invalid filesystem text file private state",
			fmt.Sprintf("Decode the ownership token: %v.", err),
		)
		return "", false, diagnostics
	}
	if err := validateFilesystemTextFileOwnershipToken(state.Token); err != nil {
		diagnostics.AddError(
			"Invalid filesystem text file private state",
			err.Error(),
		)
		return "", false, diagnostics
	}

	return state.Token, true, diagnostics
}

func writeFilesystemTextFileOwnershipToken(
	ctx context.Context,
	privateState filesystemTextFilePrivateStateWriter,
	token string,
) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	if privateState == nil {
		diagnostics.AddError(
			"Unable to store filesystem text file ownership",
			"Terraform did not initialize private resource state.",
		)
		return diagnostics
	}
	if err := validateFilesystemTextFileOwnershipToken(token); err != nil {
		diagnostics.AddError(
			"Unable to store filesystem text file ownership",
			err.Error(),
		)
		return diagnostics
	}

	value, err := json.Marshal(filesystemTextFileOwnershipState{Token: token})
	if err != nil {
		diagnostics.AddError(
			"Unable to store filesystem text file ownership",
			fmt.Sprintf("Encode the ownership token: %v.", err),
		)
		return diagnostics
	}
	diagnostics.Append(
		privateState.SetKey(
			ctx,
			filesystemTextFileOwnershipPrivateKey,
			value,
		)...,
	)

	return diagnostics
}

func (r *filesystemTextFileResource) resolveOwnership(
	ctx context.Context,
	filePath string,
	privateState filesystemTextFilePrivateStateReader,
	mayOwn bool,
) (filesystemTextFileOwnership, diag.Diagnostics, error) {
	var ownership filesystemTextFileOwnership
	if !mayOwn {
		return ownership, nil, nil
	}

	token, found, diagnostics := readFilesystemTextFileOwnershipToken(
		ctx,
		privateState,
	)
	if diagnostics.HasError() {
		return ownership, diagnostics, nil
	}

	markerFile, err := r.client.GetTextFile(
		ctx,
		filesystemTextFileMarkerPath(filePath),
	)
	if err != nil {
		return ownership, diagnostics, err
	}
	if markerFile == nil {
		if found {
			return ownership, diagnostics, fmt.Errorf(
				"filesystem text file %q lost its Terraform ownership marker",
				filePath,
			)
		}

		return ownership, diagnostics, nil
	}

	marker, err := parseFilesystemTextFileMarker(
		markerFile.Content,
		filePath,
	)
	if err != nil {
		return ownership, diagnostics, err
	}
	if found && marker.Token != token {
		return ownership, diagnostics, fmt.Errorf(
			"filesystem text file %q ownership marker does not match Terraform private state",
			filePath,
		)
	}

	return filesystemTextFileOwnership{
		Token:         marker.Token,
		Marker:        marker,
		MarkerContent: markerFile.Content,
		Owned:         true,
		Recovered:     !found,
	}, diagnostics, nil
}

func filesystemTextFileMarkerMatches(
	textFile fileman.TextFile,
	marker filesystemTextFileMarker,
) bool {
	return marker.Path == textFile.Entry.Path &&
		marker.SizeBytes == textFile.Entry.SizeBytes &&
		marker.ContentSHA256 == filesystemTextFileContentSHA256(textFile.Content)
}
