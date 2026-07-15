package provider

import (
	"context"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

const filesystemTextFileMaximumContentBytes = 1 << 20

type filesystemTextFilePathValidator struct{}

func (filesystemTextFilePathValidator) Description(_ context.Context) string {
	return "must be a normalized non-reserved file path strictly below public_html"
}

func (v filesystemTextFilePathValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (filesystemTextFilePathValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	if err := validateFilesystemTextFilePath(
		request.ConfigValue.ValueString(),
	); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid filesystem text file path",
			err.Error(),
		)
	}
}

type filesystemTextFileContentValidator struct{}

func (filesystemTextFileContentValidator) Description(_ context.Context) string {
	return fmt.Sprintf(
		"must be valid UTF-8 without null bytes and contain at most %d bytes",
		filesystemTextFileMaximumContentBytes,
	)
}

func (v filesystemTextFileContentValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (filesystemTextFileContentValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	if err := validateFilesystemTextFileContent(
		request.ConfigValue.ValueString(),
	); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid filesystem text file content",
			err.Error(),
		)
	}
}

func filesystemTextFilePathValidators() []validator.String {
	return []validator.String{filesystemTextFilePathValidator{}}
}

func filesystemTextFileContentValidators() []validator.String {
	return []validator.String{filesystemTextFileContentValidator{}}
}

func validateFilesystemTextFilePath(filePath string) error {
	if len(filePath) < len("public_html/a") {
		return fmt.Errorf(
			"filesystem text file path must contain a non-empty file name below public_html",
		)
	}
	if len(filePath) > 4096 {
		return fmt.Errorf(
			"filesystem text file path must not exceed 4096 bytes",
		)
	}
	if err := fileman.ValidateManagedPath(filePath); err != nil {
		return fmt.Errorf("invalid filesystem text file path: %w", err)
	}

	baseName := path.Base(filePath)
	if baseName == filesystemDirectoryMarkerName ||
		strings.HasPrefix(baseName, filesystemTextFileMarkerPrefix) {
		return fmt.Errorf(
			"filesystem text file name %q is reserved for provider ownership markers",
			baseName,
		)
	}
	for _, segment := range strings.Split(filePath, "/") {
		if len(segment) > 255 {
			return fmt.Errorf(
				"filesystem text file path segment %q exceeds 255 bytes",
				segment,
			)
		}
	}

	return nil
}

func validateFilesystemTextFileContent(content string) error {
	if !utf8.ValidString(content) {
		return fmt.Errorf("filesystem text file content must be valid UTF-8")
	}
	if strings.ContainsRune(content, '\x00') {
		return fmt.Errorf(
			"filesystem text file content must not contain a null byte",
		)
	}
	if len([]byte(content)) > filesystemTextFileMaximumContentBytes {
		return fmt.Errorf(
			"filesystem text file content must not exceed %d bytes",
			filesystemTextFileMaximumContentBytes,
		)
	}

	return nil
}
