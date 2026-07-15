package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

type filesystemDirectoryPathValidator struct{}

func (filesystemDirectoryPathValidator) Description(_ context.Context) string {
	return "must be a normalized path strictly below public_html and outside cPanel-controlled directories"
}

func (v filesystemDirectoryPathValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (filesystemDirectoryPathValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	if err := validateFilesystemDirectoryPath(
		request.ConfigValue.ValueString(),
	); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid filesystem directory path",
			err.Error(),
		)
	}
}

func filesystemDirectoryPathValidators() []validator.String {
	return []validator.String{
		filesystemDirectoryPathValidator{},
	}
}

func validateFilesystemDirectoryPath(directoryPath string) error {
	if len(directoryPath) < len("public_html/a") {
		return fmt.Errorf(
			"filesystem directory path must contain at least one non-empty segment below public_html",
		)
	}
	if len(directoryPath) > 4096 {
		return fmt.Errorf("filesystem directory path must not exceed 4096 bytes")
	}
	if err := fileman.ValidateManagedPath(directoryPath); err != nil {
		return fmt.Errorf("invalid filesystem directory path: %w", err)
	}

	return nil
}
