package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

var sslCSRNoWhitespacePattern = regexp.MustCompile(`^\S+$`)

var sslCSRPrintableTextPattern = regexp.MustCompile(
	`^[^\x00-\x1f\x7f]*$`,
)

type sslCSRDomainsValidator struct{}

func (sslCSRDomainsValidator) Description(context.Context) string {
	return "domains must be a non-empty ordered list of unique names without whitespace or commas"
}

func (v sslCSRDomainsValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (sslCSRDomainsValidator) ValidateList(
	ctx context.Context,
	request validator.ListRequest,
	response *validator.ListResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	hasUnknownDomain := false
	for _, domain := range request.ConfigValue.Elements() {
		if domain.IsUnknown() {
			hasUnknownDomain = true
			continue
		}
		if domain.IsNull() {
			response.Diagnostics.AddAttributeError(
				request.Path,
				"Invalid SSL CSR domains",
				"domains must not contain null values",
			)
			return
		}
	}
	if hasUnknownDomain {
		return
	}

	var domains []string
	response.Diagnostics.Append(
		request.ConfigValue.ElementsAs(ctx, &domains, false)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	if err := sslcsr.ValidateDomains(domains); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid SSL CSR domains",
			err.Error(),
		)
	}
}

func sslCSRIDValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
		stringvalidator.RegexMatches(
			sslCSRNoWhitespacePattern,
			"must not contain whitespace",
		),
	}
}

func sslCSRFriendlyNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 1024),
		stringvalidator.RegexMatches(
			sslCSRPrintableTextPattern,
			"must not contain control characters",
		),
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^\S(?:.*\S)?$`),
			"must not contain surrounding whitespace",
		),
	}
}

func sslCSRRequiredSubjectValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 1024),
		stringvalidator.RegexMatches(
			sslCSRPrintableTextPattern,
			"must not contain control characters",
		),
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^\S(?:.*\S)?$`),
			"must not contain surrounding whitespace",
		),
	}
}

func sslCSROptionalSubjectValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 1024),
		stringvalidator.RegexMatches(
			sslCSRPrintableTextPattern,
			"must not contain control characters",
		),
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^\S(?:.*\S)?$`),
			"must not contain surrounding whitespace",
		),
	}
}

func sslCSRDomainValidators() []validator.List {
	return []validator.List{
		listvalidator.SizeAtLeast(1),
		sslCSRDomainsValidator{},
	}
}

func sslCSRCountryValidators() []validator.String {
	return []validator.String{
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^[A-Z]{2}$`),
			"must be a two-letter uppercase country code",
		),
	}
}

func sslCSREmailValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 320),
		stringvalidator.RegexMatches(
			sslCSRPrintableTextPattern,
			"must not contain control characters",
		),
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^\S+@\S+$`),
			"must be one plain email address without whitespace",
		),
	}
}
