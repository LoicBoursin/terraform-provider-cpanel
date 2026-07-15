package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

type FilesystemTextFileResourceModel struct {
	Path                          types.String `tfsdk:"path"`
	Content                       types.String `tfsdk:"content"`
	AbsolutePath                  types.String `tfsdk:"absolute_path"`
	Permissions                   types.String `tfsdk:"permissions"`
	SizeBytes                     types.Int64  `tfsdk:"size_bytes"`
	ContentSHA256                 types.String `tfsdk:"content_sha256"`
	Owned                         types.Bool   `tfsdk:"owned"`
	ContentMatchesOwnershipMarker types.Bool   `tfsdk:"content_matches_ownership_marker"`
}

type FilesystemTextFileDataSourceModel struct {
	Path          types.String `tfsdk:"path"`
	Content       types.String `tfsdk:"content"`
	AbsolutePath  types.String `tfsdk:"absolute_path"`
	Permissions   types.String `tfsdk:"permissions"`
	SizeBytes     types.Int64  `tfsdk:"size_bytes"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
}

type filesystemTextFileReadClient interface {
	GetEntry(context.Context, string) (*fileman.Entry, error)
	GetTextFile(context.Context, string) (*fileman.TextFile, error)
}

func readFilesystemTextFile(
	ctx context.Context,
	client filesystemTextFileReadClient,
	filePath string,
) (*fileman.TextFile, error) {
	entry, err := client.GetEntry(ctx, filePath)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	if entry.Type != fileman.EntryTypeFile {
		return nil, fmt.Errorf(
			"path %q has type %q; expected %q",
			filePath,
			entry.Type,
			fileman.EntryTypeFile,
		)
	}
	if entry.SizeBytes > filesystemTextFileMaximumContentBytes {
		return nil, fmt.Errorf(
			"text file %q contains %d bytes; maximum supported size is %d bytes",
			filePath,
			entry.SizeBytes,
			filesystemTextFileMaximumContentBytes,
		)
	}

	textFile, err := client.GetTextFile(ctx, filePath)
	if err != nil {
		return nil, err
	}
	if textFile == nil {
		return nil, nil
	}
	actualSize := int64(len([]byte(textFile.Content)))
	if actualSize > filesystemTextFileMaximumContentBytes {
		return nil, fmt.Errorf(
			"text file %q contains %d bytes; maximum supported size is %d bytes",
			filePath,
			actualSize,
			filesystemTextFileMaximumContentBytes,
		)
	}
	if textFile.Entry.SizeBytes != actualSize {
		return nil, fmt.Errorf(
			"text file %q reports %d bytes but returned %d bytes",
			filePath,
			textFile.Entry.SizeBytes,
			actualSize,
		)
	}

	return textFile, nil
}

func applyFilesystemTextFileToResourceModel(
	model *FilesystemTextFileResourceModel,
	textFile fileman.TextFile,
	owned bool,
	contentMatchesOwnershipMarker bool,
) {
	model.Path = types.StringValue(textFile.Entry.Path)
	model.Content = types.StringValue(textFile.Content)
	model.AbsolutePath = types.StringValue(textFile.Entry.AbsolutePath)
	model.Permissions = types.StringValue(textFile.Entry.Permissions)
	model.SizeBytes = types.Int64Value(textFile.Entry.SizeBytes)
	model.ContentSHA256 = types.StringValue(
		filesystemTextFileContentSHA256(textFile.Content),
	)
	model.Owned = types.BoolValue(owned)
	model.ContentMatchesOwnershipMarker = types.BoolValue(
		contentMatchesOwnershipMarker,
	)
}

func filesystemTextFileToDataSourceModel(
	textFile fileman.TextFile,
) *FilesystemTextFileDataSourceModel {
	return &FilesystemTextFileDataSourceModel{
		Path:         types.StringValue(textFile.Entry.Path),
		Content:      types.StringValue(textFile.Content),
		AbsolutePath: types.StringValue(textFile.Entry.AbsolutePath),
		Permissions: types.StringValue(
			textFile.Entry.Permissions,
		),
		SizeBytes: types.Int64Value(textFile.Entry.SizeBytes),
		ContentSHA256: types.StringValue(
			filesystemTextFileContentSHA256(textFile.Content),
		),
	}
}

func filesystemTextFileMayOwn(owned types.Bool) bool {
	return !owned.IsNull() &&
		!owned.IsUnknown() &&
		owned.ValueBool()
}
