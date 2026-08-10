package boxtrapper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type jsonObject map[string]json.RawMessage

func (r *accountListResponse) UnmarshalJSON(data []byte) error {
	topLevel, err := decodeJSONObject(data, "API 2 BoxTrapper response")
	if err != nil {
		return err
	}
	if err := validateExactFields(
		topLevel,
		[]string{"cpanelresult"},
		nil,
		"API 2 BoxTrapper response",
	); err != nil {
		return err
	}

	result, err := decodeJSONObject(
		topLevel["cpanelresult"],
		"API 2 BoxTrapper cpanelresult",
	)
	if err != nil {
		return err
	}
	if err := validateExactFields(
		result,
		[]string{"apiversion", "data", "event", "func", "module"},
		[]string{"postevent", "preevent"},
		"API 2 BoxTrapper cpanelresult",
	); err != nil {
		return err
	}
	for _, field := range []string{"preevent", "postevent"} {
		if value, exists := result[field]; exists {
			if err := validateObjectOrNull(
				value,
				"API 2 BoxTrapper "+field,
			); err != nil {
				return err
			}
		}
	}

	apiVersion, err := parseJSONInteger(
		result["apiversion"],
		"API 2 BoxTrapper apiversion",
	)
	if err != nil {
		return err
	}
	if apiVersion != 2 {
		return fmt.Errorf(
			"API 2 BoxTrapper apiversion is %d; expected 2",
			apiVersion,
		)
	}
	function, err := parseRequiredString(
		result["func"],
		"API 2 BoxTrapper func",
	)
	if err != nil {
		return err
	}
	if function != operationAccountManageList {
		return fmt.Errorf(
			"API 2 BoxTrapper func is %q; expected %q",
			function,
			operationAccountManageList,
		)
	}
	module, err := parseRequiredString(
		result["module"],
		"API 2 BoxTrapper module",
	)
	if err != nil {
		return err
	}
	if module != "BoxTrapper" {
		return fmt.Errorf(
			"API 2 BoxTrapper module is %q; expected BoxTrapper",
			module,
		)
	}

	event, err := decodeJSONObject(
		result["event"],
		"API 2 BoxTrapper event",
	)
	if err != nil {
		return err
	}
	if err := validateExactFields(
		event,
		[]string{"result"},
		nil,
		"API 2 BoxTrapper event",
	); err != nil {
		return err
	}
	eventResult, err := parseJSONInteger(
		event["result"],
		"API 2 BoxTrapper event result",
	)
	if err != nil {
		return err
	}
	if eventResult != 1 {
		return fmt.Errorf(
			"API 2 BoxTrapper event result is %d; expected 1",
			eventResult,
		)
	}

	rawAccounts, err := decodeJSONArray(
		result["data"],
		"API 2 BoxTrapper account inventory",
	)
	if err != nil {
		return err
	}
	accounts := make([]accountInventoryEntry, 0, len(rawAccounts))
	seen := make(map[string]struct{}, len(rawAccounts))
	for index, rawAccount := range rawAccounts {
		account, err := decodeAccountInventoryEntry(rawAccount)
		if err != nil {
			return fmt.Errorf(
				"invalid BoxTrapper account inventory entry at index %d: %w",
				index,
				err,
			)
		}
		if _, duplicate := seen[account.Account]; duplicate {
			return fmt.Errorf(
				"cPanel returned duplicate BoxTrapper account %q",
				account.Account,
			)
		}
		seen[account.Account] = struct{}{}
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(left, right int) bool {
		return accounts[left].Account < accounts[right].Account
	})

	r.Accounts = accounts

	return nil
}

func (r *statusResponse) UnmarshalJSON(data []byte) error {
	envelope, err := decodeUAPIEnvelope(data, operationGetStatus)
	if err != nil {
		return err
	}
	enabled, err := parseFlag(
		envelope.Data,
		"BoxTrapper get_status data",
	)
	if err != nil {
		return err
	}

	r.Enabled = enabled

	return nil
}

func (r *configurationResponse) UnmarshalJSON(data []byte) error {
	envelope, err := decodeUAPIEnvelope(
		data,
		operationGetConfiguration,
	)
	if err != nil {
		return err
	}

	fields, err := decodeJSONObject(
		envelope.Data,
		"BoxTrapper get_configuration data",
	)
	if err != nil {
		return err
	}
	if err := validateExactFields(
		fields,
		[]string{
			"enable_auto_whitelist",
			"from_addresses",
			"from_name",
			"queue_days",
			"spam_score",
			"whitelist_by_association",
		},
		nil,
		"BoxTrapper get_configuration data",
	); err != nil {
		return err
	}

	enableAutoWhitelist, err := parseFlag(
		fields["enable_auto_whitelist"],
		"BoxTrapper enable_auto_whitelist",
	)
	if err != nil {
		return err
	}
	fromAddresses, err := parseRequiredString(
		fields["from_addresses"],
		"BoxTrapper from_addresses",
	)
	if err != nil {
		return err
	}
	fromName, err := parseNullableString(
		fields["from_name"],
		"BoxTrapper from_name",
	)
	if err != nil {
		return err
	}
	queueDays, err := parseFlexibleInteger(
		fields["queue_days"],
		"BoxTrapper queue_days",
	)
	if err != nil {
		return err
	}
	spamScore, err := parseFlexibleFloat(
		fields["spam_score"],
		"BoxTrapper spam_score",
	)
	if err != nil {
		return err
	}
	whitelistByAssociation, err := parseFlag(
		fields["whitelist_by_association"],
		"BoxTrapper whitelist_by_association",
	)
	if err != nil {
		return err
	}

	configuration := Configuration{
		EnableAutoWhitelist:    enableAutoWhitelist,
		FromAddresses:          fromAddresses,
		FromName:               fromName,
		QueueDays:              queueDays,
		SpamScore:              spamScore,
		WhitelistByAssociation: whitelistByAssociation,
	}
	if err := validateConfiguration(configuration); err != nil {
		return fmt.Errorf(
			"invalid BoxTrapper configuration returned by cPanel: %w",
			err,
		)
	}

	r.Configuration = configuration

	return nil
}

func (r *mutationResponse) UnmarshalJSON(data []byte) error {
	envelope, err := decodeUAPIEnvelope(data, "mutation")
	if err != nil {
		return err
	}
	r.Warnings = append([]string(nil), envelope.Warnings...)

	mutationData := bytes.TrimSpace(envelope.Data)
	if !bytes.Equal(mutationData, []byte("null")) {
		if _, err := decodeJSONObject(
			mutationData,
			"BoxTrapper mutation data",
		); err != nil {
			return fmt.Errorf(
				"BoxTrapper mutation data must be an object or null: %w",
				err,
			)
		}
	}

	return nil
}

func decodeAccountInventoryEntry(
	raw json.RawMessage,
) (accountInventoryEntry, error) {
	fields, err := decodeJSONObject(
		raw,
		"BoxTrapper account inventory entry",
	)
	if err != nil {
		return accountInventoryEntry{}, err
	}
	if err := validateExactFields(
		fields,
		[]string{"account", "accounturi", "bg", "enabled", "status"},
		nil,
		"BoxTrapper account inventory entry",
	); err != nil {
		return accountInventoryEntry{}, err
	}

	account, err := parseRequiredString(
		fields["account"],
		"BoxTrapper account",
	)
	if err != nil {
		return accountInventoryEntry{}, err
	}
	if err := validateAccount(account); err != nil {
		return accountInventoryEntry{}, fmt.Errorf(
			"invalid account returned by cPanel: %w",
			err,
		)
	}
	for _, field := range []string{"accounturi", "bg", "status"} {
		if _, err := parseRequiredString(
			fields[field],
			"BoxTrapper "+field,
		); err != nil {
			return accountInventoryEntry{}, err
		}
	}
	enabled, err := parseFlag(
		fields["enabled"],
		"BoxTrapper inventory enabled",
	)
	if err != nil {
		return accountInventoryEntry{}, err
	}

	return accountInventoryEntry{
		Account: account,
		Enabled: enabled,
	}, nil
}

func decodeUAPIEnvelope(
	raw []byte,
	function string,
) (uapiEnvelope, error) {
	label := fmt.Sprintf("UAPI BoxTrapper::%s response", function)
	fields, err := decodeJSONObject(raw, label)
	if err != nil {
		return uapiEnvelope{}, err
	}
	if err := validateExactFields(
		fields,
		[]string{"data", "status"},
		[]string{"errors", "messages", "metadata", "warnings"},
		label,
	); err != nil {
		return uapiEnvelope{}, err
	}

	status, err := parseJSONInteger(fields["status"], label+" status")
	if err != nil {
		return uapiEnvelope{}, err
	}
	if status != 1 {
		return uapiEnvelope{}, fmt.Errorf(
			"%s status is %d; expected 1",
			label,
			status,
		)
	}

	errorsValue, err := parseOptionalStringArray(
		fields["errors"],
		label+" errors",
	)
	if err != nil {
		return uapiEnvelope{}, err
	}
	if len(errorsValue) > 0 {
		return uapiEnvelope{}, fmt.Errorf(
			"%s succeeded with non-empty errors",
			label,
		)
	}
	if _, err := parseOptionalStringArray(
		fields["messages"],
		label+" messages",
	); err != nil {
		return uapiEnvelope{}, err
	}
	warnings, err := parseOptionalStringArray(
		fields["warnings"],
		label+" warnings",
	)
	if err != nil {
		return uapiEnvelope{}, err
	}
	if metadata, exists := fields["metadata"]; exists {
		if err := validateObjectOrNull(
			metadata,
			label+" metadata",
		); err != nil {
			return uapiEnvelope{}, err
		}
	}

	return uapiEnvelope{
		Data:     fields["data"],
		Warnings: warnings,
	}, nil
}

func decodeJSONObject(
	raw []byte,
	label string,
) (jsonObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", label)
	}

	fields := make(jsonObject)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode %s field name: %w", label, err)
		}
		field, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("%s returned a non-string field name", label)
		}
		if _, duplicate := fields[field]; duplicate {
			return nil, fmt.Errorf(
				"%s returned duplicate field %q",
				label,
				field,
			)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf(
				"decode %s field %q: %w",
				label,
				field,
				err,
			)
		}
		fields[field] = value
	}

	closing, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s closing token: %w", label, err)
	}
	if closing != json.Delim('}') {
		return nil, fmt.Errorf("%s did not end with an object", label)
	}
	if err := ensureJSONEOF(decoder, label); err != nil {
		return nil, err
	}

	return fields, nil
}

func decodeJSONArray(
	raw []byte,
	label string,
) ([]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	if opening != json.Delim('[') {
		return nil, fmt.Errorf("%s must be an array", label)
	}

	values := make([]json.RawMessage, 0)
	for decoder.More() {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode %s entry: %w", label, err)
		}
		values = append(values, value)
	}

	closing, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s closing token: %w", label, err)
	}
	if closing != json.Delim(']') {
		return nil, fmt.Errorf("%s did not end with an array", label)
	}
	if err := ensureJSONEOF(decoder, label); err != nil {
		return nil, err
	}

	return values, nil
}

func ensureJSONEOF(
	decoder *json.Decoder,
	label string,
) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing %s data: %w", label, err)
	}

	return fmt.Errorf("%s contains trailing data", label)
}

func validateExactFields(
	fields jsonObject,
	required []string,
	optional []string,
	label string,
) error {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, field := range required {
		allowed[field] = struct{}{}
		if _, exists := fields[field]; !exists {
			return fmt.Errorf(
				"%s is missing required field %q",
				label,
				field,
			)
		}
	}
	for _, field := range optional {
		allowed[field] = struct{}{}
	}

	unexpected := make([]string, 0)
	for field := range fields {
		if _, expected := allowed[field]; !expected {
			unexpected = append(unexpected, field)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)

		return fmt.Errorf(
			"%s returned unexpected field %q",
			label,
			unexpected[0],
		)
	}

	return nil
}

func parseRequiredString(
	raw json.RawMessage,
	label string,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", fmt.Errorf("%s is missing or null", label)
	}
	if value[0] != '"' {
		return "", fmt.Errorf("%s must be a string", label)
	}

	var parsed string
	if err := json.Unmarshal(value, &parsed); err != nil {
		return "", fmt.Errorf("decode %s: %w", label, err)
	}

	return parsed, nil
}

func parseNullableString(
	raw json.RawMessage,
	label string,
) (*string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return nil, fmt.Errorf("%s is missing", label)
	}
	if bytes.Equal(value, []byte("null")) {
		return nil, nil
	}

	parsed, err := parseRequiredString(value, label)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func parseFlag(
	raw json.RawMessage,
	label string,
) (bool, error) {
	switch string(bytes.TrimSpace(raw)) {
	case "0", `"0"`:
		return false, nil
	case "1", `"1"`:
		return true, nil
	default:
		return false, fmt.Errorf(
			"%s must be integer or string 0 or 1",
			label,
		)
	}
}

func parseJSONInteger(
	raw json.RawMessage,
	label string,
) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("%s is missing or null", label)
	}

	var parsed int64
	if err := json.Unmarshal(value, &parsed); err != nil {
		return 0, fmt.Errorf("%s must be a JSON integer", label)
	}

	return parsed, nil
}

func parseFlexibleInteger(
	raw json.RawMessage,
	label string,
) (int64, error) {
	text, err := scalarNumberText(raw, label)
	if err != nil {
		return 0, err
	}
	if !isDecimalInteger(text) {
		return 0, fmt.Errorf(
			"%s must be an integer or decimal integer string",
			label,
		)
	}

	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s value %q: %w", label, text, err)
	}

	return parsed, nil
}

func parseFlexibleFloat(
	raw json.RawMessage,
	label string,
) (float64, error) {
	text, err := scalarNumberText(raw, label)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a number or numeric string",
			label,
		)
	}
	if err := validateSpamScore(parsed); err != nil {
		return 0, err
	}

	return parsed, nil
}

func scalarNumberText(
	raw json.RawMessage,
	label string,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", fmt.Errorf("%s is missing or null", label)
	}
	if value[0] != '"' {
		return string(value), nil
	}

	var parsed string
	if err := json.Unmarshal(value, &parsed); err != nil {
		return "", fmt.Errorf("decode %s: %w", label, err)
	}
	if parsed == "" || strings.TrimSpace(parsed) != parsed {
		return "", fmt.Errorf(
			"%s contains invalid surrounding whitespace",
			label,
		)
	}

	return parsed, nil
}

func isDecimalInteger(value string) bool {
	if value == "" {
		return false
	}
	start := 0
	if value[0] == '-' {
		if len(value) == 1 {
			return false
		}
		start = 1
	}
	for _, character := range value[start:] {
		if character < '0' || character > '9' {
			return false
		}
	}

	return true
}

func parseOptionalStringArray(
	raw json.RawMessage,
	label string,
) ([]string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, nil
	}

	rawValues, err := decodeJSONArray(value, label)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(rawValues))
	for index, rawValue := range rawValues {
		parsed, err := parseRequiredString(
			rawValue,
			fmt.Sprintf("%s entry at index %d", label, index),
		)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
	}

	return values, nil
}

func validateObjectOrNull(
	raw json.RawMessage,
	label string,
) error {
	value := bytes.TrimSpace(raw)
	if bytes.Equal(value, []byte("null")) {
		return nil
	}
	if _, err := decodeJSONObject(value, label); err != nil {
		return err
	}

	return nil
}
