package spamassassin

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	scoreRulePattern = regexp.MustCompile(
		`^[A-Za-z0-9_]{1,128} [+-]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)$`,
	)
	emailLocalPartPattern = regexp.MustCompile(
		`^[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+$`,
	)
	domainLabelPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
)

func SupportedPreferenceNames() []string {
	return []string{
		PreferenceBlacklistFrom,
		PreferenceRequiredScore,
		PreferenceScore,
		PreferenceWhitelistFrom,
	}
}

func ValidateName(name string) error {
	if !slices.Contains(SupportedPreferenceNames(), name) {
		return fmt.Errorf(
			"unsupported SpamAssassin preference %q; expected one of %q",
			name,
			strings.Join(SupportedPreferenceNames(), ", "),
		)
	}

	return nil
}

func ValidateDefinition(definition Definition) error {
	if err := ValidateName(definition.Name); err != nil {
		return err
	}
	if !definition.Present {
		if len(definition.Values) != 0 {
			return fmt.Errorf(
				"absent SpamAssassin preference %q must not contain values",
				definition.Name,
			)
		}

		return nil
	}
	if len(definition.Values) == 0 {
		return fmt.Errorf(
			"SpamAssassin preference %q must contain at least one value",
			definition.Name,
		)
	}

	seen := make(map[string]struct{}, len(definition.Values))
	for _, value := range definition.Values {
		if value == "" {
			return fmt.Errorf(
				"SpamAssassin preference %q must not contain an empty value",
				definition.Name,
			)
		}
		if strings.TrimSpace(value) != value ||
			strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf(
				"SpamAssassin preference %q value %q contains invalid whitespace or control characters",
				definition.Name,
				value,
			)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf(
				"SpamAssassin preference %q contains duplicate value %q",
				definition.Name,
				value,
			)
		}
		seen[value] = struct{}{}
	}

	switch definition.Name {
	case PreferenceRequiredScore:
		if len(definition.Values) != 1 {
			return fmt.Errorf(
				"SpamAssassin preference %q requires exactly one value",
				definition.Name,
			)
		}

		return validateScoreValue(definition.Name, definition.Values[0])
	case PreferenceScore:
		for _, value := range definition.Values {
			if !scoreRulePattern.MatchString(value) {
				return fmt.Errorf(
					"SpamAssassin preference %q value %q must contain a rule name, one space, and a numeric score",
					definition.Name,
					value,
				)
			}
			_, rawScore, _ := strings.Cut(value, " ")
			if err := validateScoreValue(definition.Name, rawScore); err != nil {
				return err
			}
		}
	case PreferenceWhitelistFrom, PreferenceBlacklistFrom:
		for _, value := range definition.Values {
			if err := validateEmailAddress(value); err != nil {
				return fmt.Errorf(
					"invalid %s value %q: %w",
					definition.Name,
					value,
					err,
				)
			}
		}
	}

	return nil
}

func validateScoreValue(preference string, value string) error {
	score, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(score, 0) || math.IsNaN(score) {
		return fmt.Errorf(
			"SpamAssassin preference %q value %q must be a finite number",
			preference,
			value,
		)
	}
	if score <= 0 || score >= 1000 {
		return fmt.Errorf(
			"SpamAssassin preference %q value %q must be greater than 0 and less than 1000",
			preference,
			value,
		)
	}

	return nil
}

func validateEmailAddress(address string) error {
	if len(address) > 254 || strings.Count(address, "@") != 1 {
		return fmt.Errorf(
			"must be one complete email address of at most 254 characters",
		)
	}
	if strings.ContainsAny(address, "*?") {
		return fmt.Errorf("must not contain wildcard characters")
	}
	localPart, domain, _ := strings.Cut(address, "@")
	if localPart == "" || len(localPart) > 64 ||
		!emailLocalPartPattern.MatchString(localPart) ||
		strings.HasPrefix(localPart, ".") ||
		strings.HasSuffix(localPart, ".") ||
		strings.Contains(localPart, "..") {
		return fmt.Errorf("contains an invalid local part")
	}
	if domain == "" || domain != strings.ToLower(domain) ||
		len(domain) > 253 {
		return fmt.Errorf("contains an invalid or non-lowercase domain")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain must contain at least two labels")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 ||
			!domainLabelPattern.MatchString(label) ||
			strings.HasPrefix(label, "-") ||
			strings.HasSuffix(label, "-") {
			return fmt.Errorf("contains an invalid domain label")
		}
	}

	return nil
}
