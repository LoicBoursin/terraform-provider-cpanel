package boxtrapper

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

func ValidateDefinition(definition Definition) error {
	if definition.QueueDays < 1 {
		return fmt.Errorf("BoxTrapper queue days must be at least 1")
	}
	if err := validateSpamScore(definition.SpamScore); err != nil {
		return err
	}
	if err := validateManagedString(
		"BoxTrapper from_addresses",
		definition.FromAddresses,
		true,
	); err != nil {
		return err
	}

	return nil
}

func SettingsMatchDefinition(
	settings Settings,
	definition Definition,
) bool {
	return settings.Enabled == definition.Enabled &&
		configurationMatchesDefinition(settings.Configuration, definition)
}

func configurationMatchesDefinition(
	configuration Configuration,
	definition Definition,
) bool {
	return configuration.EnableAutoWhitelist ==
		definition.EnableAutoWhitelist &&
		configuration.FromAddresses == definition.FromAddresses &&
		configuration.QueueDays == definition.QueueDays &&
		spamScoresEqual(configuration.SpamScore, definition.SpamScore) &&
		configuration.WhitelistByAssociation ==
			definition.WhitelistByAssociation
}

func verifyConfiguration(
	actual Configuration,
	expected Definition,
) error {
	if actual.EnableAutoWhitelist != expected.EnableAutoWhitelist {
		return fmt.Errorf(
			"cPanel BoxTrapper enable_auto_whitelist is %t after mutation; expected %t",
			actual.EnableAutoWhitelist,
			expected.EnableAutoWhitelist,
		)
	}
	if actual.FromAddresses != expected.FromAddresses {
		return fmt.Errorf(
			"cPanel BoxTrapper from_addresses is %q after mutation; expected %q",
			actual.FromAddresses,
			expected.FromAddresses,
		)
	}
	if actual.QueueDays != expected.QueueDays {
		return fmt.Errorf(
			"cPanel BoxTrapper queue_days is %d after mutation; expected %d",
			actual.QueueDays,
			expected.QueueDays,
		)
	}
	if !spamScoresEqual(actual.SpamScore, expected.SpamScore) {
		return fmt.Errorf(
			"cPanel BoxTrapper spam_score is %s after mutation; expected %s",
			formatSpamScore(actual.SpamScore),
			formatSpamScore(expected.SpamScore),
		)
	}
	if actual.WhitelistByAssociation != expected.WhitelistByAssociation {
		return fmt.Errorf(
			"cPanel BoxTrapper whitelist_by_association is %t after mutation; expected %t",
			actual.WhitelistByAssociation,
			expected.WhitelistByAssociation,
		)
	}

	return nil
}

func validateAccount(account string) error {
	if account == "" {
		return fmt.Errorf("BoxTrapper account must not be empty")
	}
	if strings.TrimSpace(account) != account {
		return fmt.Errorf(
			"BoxTrapper account %q must not contain surrounding whitespace",
			account,
		)
	}
	if containsControlCharacter(account) {
		return fmt.Errorf(
			"BoxTrapper account %q must not contain control characters",
			account,
		)
	}

	return nil
}

func validateConfiguration(configuration Configuration) error {
	definition := Definition{
		EnableAutoWhitelist:    configuration.EnableAutoWhitelist,
		FromAddresses:          configuration.FromAddresses,
		QueueDays:              configuration.QueueDays,
		SpamScore:              configuration.SpamScore,
		WhitelistByAssociation: configuration.WhitelistByAssociation,
	}
	if err := ValidateDefinition(definition); err != nil {
		return err
	}
	if configuration.FromName != nil {
		if err := validateManagedString(
			"BoxTrapper from_name",
			*configuration.FromName,
			false,
		); err != nil {
			return err
		}
	}

	return nil
}

func validateManagedString(
	field string,
	value string,
	rejectSurroundingWhitespace bool,
) error {
	if rejectSurroundingWhitespace &&
		strings.TrimSpace(value) != value {
		return fmt.Errorf(
			"%s %q must not contain surrounding whitespace",
			field,
			value,
		)
	}
	if containsControlCharacter(value) {
		return fmt.Errorf("%s must not contain control characters", field)
	}

	return nil
}

func validateSpamScore(score float64) error {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return fmt.Errorf("BoxTrapper spam score must be finite")
	}

	roundedText := strconv.FormatFloat(score, 'f', 1, 64)
	rounded, err := strconv.ParseFloat(roundedText, 64)
	if err != nil {
		return fmt.Errorf("normalize BoxTrapper spam score: %w", err)
	}
	tolerance := spamScoreComparisonTolerance(score)
	if math.Abs(score-rounded) > tolerance {
		return fmt.Errorf(
			"BoxTrapper spam score must be representable exactly to one decimal place",
		)
	}

	return nil
}

func spamScoresEqual(left float64, right float64) bool {
	tolerance := math.Max(
		spamScoreComparisonTolerance(left),
		spamScoreComparisonTolerance(right),
	)

	return math.Abs(left-right) <= tolerance
}

func spamScoreComparisonTolerance(value float64) float64 {
	next := math.Nextafter(value, math.Inf(1))
	spacing := math.Abs(next - value)
	if math.IsInf(next, 0) {
		spacing = math.Abs(
			value - math.Nextafter(value, math.Inf(-1)),
		)
	}

	return 4 * spacing
}

func containsControlCharacter(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func booleanParameter(value bool) string {
	if value {
		return "1"
	}

	return "0"
}

func formatSpamScore(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
