package gpg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	operationListPublicKeys  = "list_public_keys"
	operationListSecretKeys  = "list_secret_keys"
	operationExportPublicKey = "export_public_key"
	operationImportKey       = "import_key"
	operationDeleteKeyPair   = "delete_keypair"
)

// Metadata is the public inventory metadata cPanel reports for an OpenPGP key.
type Metadata struct {
	ID        string
	Algorithm string
	Bits      int64
	Created   int64
	Expires   *int64
	Type      string
	UserID    string
}

// ParsedPublicKey is a validated public-only OpenPGP key.
type ParsedPublicKey struct {
	Armored       string
	ID            string
	Fingerprint   string
	ContentSHA256 string
	Bits          int64
}

// PublicKey contains one exported public key and its cPanel metadata.
type PublicKey struct {
	Metadata
	Armored       string
	Fingerprint   string
	ContentSHA256 string
}

// Lookup combines a public-key lookup with the matching secret-key signal.
type Lookup struct {
	Key          *PublicKey
	HasSecretKey bool
}

// Inventory contains the complete public and secret GPG inventories.
type Inventory struct {
	Public []Metadata
	Secret []Metadata
}

// Ownership is the immutable identity Terraform must prove before deletion.
type Ownership struct {
	ID            string
	Fingerprint   string
	ContentSHA256 string
}

// MutationVerificationError means cPanel accepted a mutation request but the
// provider could not prove the resulting state.
type MutationVerificationError struct {
	Operation string
	Err       error
}

func (e *MutationVerificationError) Error() string {
	return fmt.Sprintf(
		"verify cPanel GPG %s mutation: %v",
		e.Operation,
		e.Err,
	)
}

func (e *MutationVerificationError) Unwrap() error {
	return e.Err
}

type listResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type exportResponse struct {
	cpanel.UAPIDataSourceModel
	Data struct {
		KeyData json.RawMessage `json:"key_data"`
	} `json:"data"`
}

type importResponse struct {
	cpanel.UAPIDataSourceModel
	Data struct {
		KeyID json.RawMessage `json:"key_id"`
	} `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiMetadata struct {
	Algorithm json.RawMessage `json:"algorithm"`
	Bits      json.RawMessage `json:"bits"`
	Created   json.RawMessage `json:"created"`
	Expires   json.RawMessage `json:"expires"`
	ID        json.RawMessage `json:"id"`
	Type      json.RawMessage `json:"type"`
	UserID    json.RawMessage `json:"user_id"`
}

func (m *apiMetadata) UnmarshalJSON(data []byte) error {
	type rawMetadata apiMetadata

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode((*rawMetadata)(m)); err != nil {
		return fmt.Errorf("decode GPG key metadata: %w", err)
	}

	return nil
}

func parseInventory(
	raw json.RawMessage,
	expectedType string,
	idLength int,
) ([]Metadata, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("cPanel GPG inventory data must be an array")
	}

	var values []apiMetadata
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode cPanel GPG inventory: %w", err)
	}

	result := make([]Metadata, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		metadata, err := metadataFromAPI(value, expectedType, idLength)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel GPG inventory item %d: %w",
				index,
				err,
			)
		}
		if _, exists := seen[metadata.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate GPG %s key id %q",
				expectedType,
				metadata.ID,
			)
		}
		seen[metadata.ID] = struct{}{}
		result = append(result, metadata)
	}

	sort.Slice(result, func(left, right int) bool {
		return result[left].ID < result[right].ID
	})

	return result, nil
}

func metadataFromAPI(
	value apiMetadata,
	expectedType string,
	idLength int,
) (Metadata, error) {
	id, err := parseRequiredString(value.ID, "id")
	if err != nil {
		return Metadata{}, err
	}
	id = strings.ToUpper(id)
	if err := validateHexID(id, idLength, "GPG key id"); err != nil {
		return Metadata{}, err
	}

	keyType, err := parseRequiredString(value.Type, "type")
	if err != nil {
		return Metadata{}, err
	}
	if keyType != expectedType {
		return Metadata{}, fmt.Errorf(
			"GPG key %q has type %q; expected %q",
			id,
			keyType,
			expectedType,
		)
	}

	algorithm, err := parseRequiredString(value.Algorithm, "algorithm")
	if err != nil {
		return Metadata{}, err
	}
	bits, err := parseRequiredInt64(value.Bits, "bits")
	if err != nil {
		return Metadata{}, err
	}
	if bits <= 0 {
		return Metadata{}, fmt.Errorf(
			"GPG key %q bits must be positive",
			id,
		)
	}
	created, err := parseRequiredInt64(value.Created, "created")
	if err != nil {
		return Metadata{}, err
	}
	if created < 0 {
		return Metadata{}, fmt.Errorf(
			"GPG key %q created must not be negative",
			id,
		)
	}
	expires, err := parseOptionalInt64(value.Expires, "expires")
	if err != nil {
		return Metadata{}, err
	}
	if expires != nil && *expires < 0 {
		return Metadata{}, fmt.Errorf(
			"GPG key %q expires must not be negative",
			id,
		)
	}
	userID, err := parseRequiredString(value.UserID, "user_id")
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		ID:        id,
		Algorithm: algorithm,
		Bits:      bits,
		Created:   created,
		Expires:   expires,
		Type:      keyType,
		UserID:    userID,
	}, nil
}

func parseRequiredString(
	raw json.RawMessage,
	field string,
) (string, error) {
	value, err := parseString(raw, field)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("GPG %s must not be empty", field)
	}

	return value, nil
}

func parseString(raw json.RawMessage, field string) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("decode GPG %s as string: %w", field, err)
	}

	return value, nil
}

func parseRequiredInt64(
	raw json.RawMessage,
	field string,
) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("GPG %s is required", field)
	}

	value, err := parseFlexibleInt64(raw)
	if err != nil {
		return 0, fmt.Errorf("decode GPG %s: %w", field, err)
	}

	return value, nil
}

func parseOptionalInt64(
	raw json.RawMessage,
	field string,
) (*int64, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return nil, nil
	}

	value, err := parseFlexibleInt64(raw)
	if err != nil {
		return nil, fmt.Errorf("decode GPG %s: %w", field, err)
	}

	return &value, nil
}

func parseFlexibleInt64(raw json.RawMessage) (int64, error) {
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return integer, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf("expected an integer or decimal string")
	}
	if text == "" || strings.TrimSpace(text) != text {
		return 0, fmt.Errorf("expected a non-empty decimal string")
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse decimal string %q: %w", text, err)
	}

	return value, nil
}

func mergeWarnings(groups ...[]string) []string {
	var result []string
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, warning := range group {
			if warning == "" {
				continue
			}
			if _, exists := seen[warning]; exists {
				continue
			}
			seen[warning] = struct{}{}
			result = append(result, warning)
		}
	}

	return result
}
