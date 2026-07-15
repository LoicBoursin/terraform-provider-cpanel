package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/gpg"
)

var gpgPublicIDPattern = regexp.MustCompile(`^[0-9A-F]{16}$`)

type gpgPublicKeyValidator struct{}

func (gpgPublicKeyValidator) Description(context.Context) string {
	return "value must contain exactly one public-only RSA v4 OpenPGP entity with a 2048, 3072, or 4096-bit key"
}

func (v gpgPublicKeyValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (gpgPublicKeyValidator) ValidateString(
	_ context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}
	if _, err := gpg.ParsePublicKey(
		request.ConfigValue.ValueString(),
	); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid GPG public key",
			err.Error(),
		)
	}
}

type gpgPublicKeySemanticEqualityPlanModifier struct{}

func (gpgPublicKeySemanticEqualityPlanModifier) Description(
	context.Context,
) string {
	return "preserves the prior state when both armored values contain the same canonical set of decoded OpenPGP public packets"
}

func (m gpgPublicKeySemanticEqualityPlanModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (gpgPublicKeySemanticEqualityPlanModifier) PlanModifyString(
	_ context.Context,
	request planmodifier.StringRequest,
	response *planmodifier.StringResponse,
) {
	if request.PlanValue.IsNull() ||
		request.PlanValue.IsUnknown() ||
		request.StateValue.IsNull() ||
		request.StateValue.IsUnknown() {
		return
	}
	equal, err := gpg.EqualPublicKey(
		request.PlanValue.ValueString(),
		request.StateValue.ValueString(),
	)
	if err == nil && equal {
		response.PlanValue = request.StateValue
	}
}

func gpgPublicIDValidators() []validator.String {
	return []validator.String{
		stringvalidator.RegexMatches(
			gpgPublicIDPattern,
			"must contain exactly 16 uppercase hexadecimal characters",
		),
	}
}

func gpgPublicKeyValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 1<<20),
		gpgPublicKeyValidator{},
	}
}
