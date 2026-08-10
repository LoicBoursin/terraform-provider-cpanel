package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelssh "terraform-provider-cpanel/internal/cpanel/ssh"
)

type sshPublicKeyNameValidator struct{}

func (sshPublicKeyNameValidator) Description(context.Context) string {
	return "value must be a safe cPanel SSH public-key base filename"
}

func (v sshPublicKeyNameValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (sshPublicKeyNameValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() ||
		request.ConfigValue.IsUnknown() {
		return
	}
	if err := cpanelssh.ValidateName(
		request.ConfigValue.ValueString(),
	); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid SSH public-key name",
			err.Error(),
		)
	}
}

func sshPublicKeyNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 128),
		sshPublicKeyNameValidator{},
	}
}
