package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

var sslCertificatePrintableNamePattern = regexp.MustCompile(
	`^[^\x00-\x1f\x7f]+$`,
)

var sslCertificateNoWhitespacePattern = regexp.MustCompile(`^\S+$`)

type sslCertificatePEMValidator struct{}

func (sslCertificatePEMValidator) Description(context.Context) string {
	return "value must contain exactly one valid PEM-encoded X.509 certificate and no private key or additional PEM block"
}

func (v sslCertificatePEMValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (v sslCertificatePEMValidator) ValidateString(
	ctx context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	if _, err := sslcertificate.ParsePEM(
		request.ConfigValue.ValueString(),
	); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid SSL certificate",
			err.Error(),
		)
	}
}

type sslCertificatePEMSemanticEqualityPlanModifier struct{}

func (sslCertificatePEMSemanticEqualityPlanModifier) Description(
	context.Context,
) string {
	return "preserves the prior state when the configured PEM encodes the same X.509 certificate"
}

func (m sslCertificatePEMSemanticEqualityPlanModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (sslCertificatePEMSemanticEqualityPlanModifier) PlanModifyString(
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

	equal, err := sslcertificate.EqualPEM(
		request.PlanValue.ValueString(),
		request.StateValue.ValueString(),
	)
	if err == nil && equal {
		response.PlanValue = request.StateValue
	}
}

func sslCertificateIDValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
		stringvalidator.RegexMatches(
			sslCertificateNoWhitespacePattern,
			"must not contain whitespace",
		),
	}
}

func sslCertificateFriendlyNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 255),
		stringvalidator.RegexMatches(
			sslCertificatePrintableNamePattern,
			"must not contain control characters",
		),
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^\S(?:.*\S)?$`),
			"must not contain surrounding whitespace",
		),
	}
}

func sslCertificatePEMValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 1<<20),
		sslCertificatePEMValidator{},
	}
}

func validateSSLCertificateID(id string) error {
	return sslcertificate.ValidateID(id)
}

func validateSSLCertificateDefinition(
	friendlyName string,
	certificatePEM string,
) (*sslcertificate.ParsedCertificate, error) {
	if len(friendlyName) == 0 || len(friendlyName) > 255 {
		return nil, fmt.Errorf(
			"SSL certificate friendly name must contain between 1 and 255 characters",
		)
	}
	if !sslCertificatePrintableNamePattern.MatchString(friendlyName) {
		return nil, fmt.Errorf(
			"SSL certificate friendly name must not contain control characters",
		)
	}
	if err := sslcertificate.ValidateFriendlyName(friendlyName); err != nil {
		return nil, err
	}

	parsed, err := sslcertificate.ParsePEM(certificatePEM)
	if err != nil {
		return nil, err
	}

	return parsed, nil
}
