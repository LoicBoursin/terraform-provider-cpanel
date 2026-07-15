package sslcertificate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

type listResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type showResponse struct {
	cpanel.UAPIDataSourceModel
	Data apiShowCertificate `json:"data"`
}

type uploadResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type installedHostsResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiShowCertificate struct {
	CertificatePEM json.RawMessage `json:"cert"`
	Details        apiCertificate  `json:"details"`
}

type apiInstalledHost struct {
	Certificate *apiInstalledCertificate `json:"certificate"`
}

type apiInstalledCertificate struct {
	ID json.RawMessage `json:"id"`
}

type apiCertificate struct {
	ID                 json.RawMessage      `json:"id"`
	FriendlyName       json.RawMessage      `json:"friendly_name"`
	Domains            json.RawMessage      `json:"domains"`
	Created            json.RawMessage      `json:"created"`
	NotBefore          json.RawMessage      `json:"not_before"`
	NotAfter           json.RawMessage      `json:"not_after"`
	Serial             json.RawMessage      `json:"serial"`
	SignatureAlgorithm json.RawMessage      `json:"signature_algorithm"`
	KeyAlgorithm       json.RawMessage      `json:"key_algorithm"`
	ModulusLength      json.RawMessage      `json:"modulus_length"`
	IsSelfSigned       json.RawMessage      `json:"is_self_signed"`
	IssuerCommonName   json.RawMessage      `json:"issuer.commonName"`
	SubjectCommonName  json.RawMessage      `json:"subject.commonName"`
	Issuer             apiDistinguishedName `json:"issuer"`
	Subject            apiDistinguishedName `json:"subject"`
	ValidationType     json.RawMessage      `json:"validation_type"`
	DomainIsConfigured json.RawMessage      `json:"domain_is_configured"`
}

type apiDistinguishedName struct {
	CommonName json.RawMessage `json:"commonName"`
}

// Certificate is the public metadata for one certificate stored by cPanel.
type Certificate struct {
	ID                 string
	FriendlyName       string
	CertificatePEM     string
	FingerprintSHA256  string
	Domains            []string
	Created            int64
	NotBefore          int64
	NotAfter           int64
	Serial             string
	SignatureAlgorithm string
	KeyAlgorithm       string
	ModulusLength      int64
	IsSelfSigned       bool
	IssuerCommonName   string
	SubjectCommonName  string
	ValidationType     string
	DomainIsConfigured bool
}

func certificateFromAPI(
	value apiCertificate,
	requireDomainIsConfigured bool,
) (Certificate, error) {
	id, err := parseScalarString(value.ID)
	if err != nil {
		return Certificate{}, fmt.Errorf("decode SSL certificate id: %w", err)
	}
	if err := ValidateID(id); err != nil {
		return Certificate{}, fmt.Errorf("invalid SSL certificate id: %w", err)
	}

	friendlyName, err := parseScalarString(value.FriendlyName)
	if err != nil {
		return Certificate{}, fmt.Errorf(
			"decode SSL certificate %q friendly name: %w",
			id,
			err,
		)
	}
	if err := ValidateFriendlyName(friendlyName); err != nil {
		return Certificate{}, fmt.Errorf(
			"invalid SSL certificate %q friendly name: %w",
			id,
			err,
		)
	}

	domains, err := parseStringList(value.Domains)
	if err != nil {
		return Certificate{}, fmt.Errorf(
			"decode SSL certificate %q domains: %w",
			id,
			err,
		)
	}
	sort.Strings(domains)

	created, err := parseScalarInt64(value.Created)
	if err != nil {
		return Certificate{}, certificateFieldError(id, "created", err)
	}
	notBefore, err := parseScalarInt64(value.NotBefore)
	if err != nil {
		return Certificate{}, certificateFieldError(id, "not_before", err)
	}
	notAfter, err := parseScalarInt64(value.NotAfter)
	if err != nil {
		return Certificate{}, certificateFieldError(id, "not_after", err)
	}
	modulusLength, err := parseScalarInt64(value.ModulusLength)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"modulus_length",
			err,
		)
	}
	isSelfSigned, err := parseScalarBool(value.IsSelfSigned)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"is_self_signed",
			err,
		)
	}
	var domainIsConfigured bool
	if requireDomainIsConfigured {
		domainIsConfigured, err = parseRequiredScalarBool(
			value.DomainIsConfigured,
		)
	} else {
		domainIsConfigured, err = parseScalarBool(value.DomainIsConfigured)
	}
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"domain_is_configured",
			err,
		)
	}

	serial, err := parseScalarString(value.Serial)
	if err != nil {
		return Certificate{}, certificateFieldError(id, "serial", err)
	}
	signatureAlgorithm, err := parseScalarString(value.SignatureAlgorithm)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"signature_algorithm",
			err,
		)
	}
	keyAlgorithm, err := parseScalarString(value.KeyAlgorithm)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"key_algorithm",
			err,
		)
	}
	issuerCommonName, err := parseCommonName(
		value.IssuerCommonName,
		value.Issuer.CommonName,
	)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"issuer common name",
			err,
		)
	}
	subjectCommonName, err := parseCommonName(
		value.SubjectCommonName,
		value.Subject.CommonName,
	)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"subject common name",
			err,
		)
	}
	validationType, err := parseScalarString(value.ValidationType)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"validation_type",
			err,
		)
	}

	return Certificate{
		ID:                 id,
		FriendlyName:       friendlyName,
		Domains:            domains,
		Created:            created,
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		Serial:             serial,
		SignatureAlgorithm: signatureAlgorithm,
		KeyAlgorithm:       keyAlgorithm,
		ModulusLength:      modulusLength,
		IsSelfSigned:       isSelfSigned,
		IssuerCommonName:   issuerCommonName,
		SubjectCommonName:  subjectCommonName,
		ValidationType:     validationType,
		DomainIsConfigured: domainIsConfigured,
	}, nil
}

func certificateFieldError(id, field string, err error) error {
	return fmt.Errorf(
		"decode SSL certificate %q %s: %w",
		id,
		field,
		err,
	)
}

func parseCommonName(
	dottedValue json.RawMessage,
	nestedValue json.RawMessage,
) (string, error) {
	if !isNullScalar(dottedValue) {
		return parseScalarString(dottedValue)
	}

	return parseScalarString(nestedValue)
}

func parseScalarString(raw json.RawMessage) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", nil
	}

	var stringValue string
	if value[0] == '"' {
		if err := json.Unmarshal(value, &stringValue); err != nil {
			return "", fmt.Errorf("decode string: %w", err)
		}

		return stringValue, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var numberValue json.Number
	if err := decoder.Decode(&numberValue); err == nil {
		return numberValue.String(), nil
	}

	return "", fmt.Errorf(
		"expected a string, number, or null, got %s",
		value,
	)
}

func parseScalarInt64(raw json.RawMessage) (int64, error) {
	value, err := parseScalarString(raw)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as an integer: %w", value, err)
	}

	return parsed, nil
}

func parseScalarBool(raw json.RawMessage) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return false, nil
	}

	switch string(value) {
	case "0", `"0"`, "false", `"false"`:
		return false, nil
	case "1", `"1"`, "true", `"true"`:
		return true, nil
	default:
		return false, fmt.Errorf(
			"expected 0, 1, true, false, or null, got %s",
			value,
		)
	}
}

func parseRequiredScalarBool(raw json.RawMessage) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return false, fmt.Errorf("required boolean value is missing or null")
	}

	return parseScalarBool(raw)
}

func parseRequiredArray[T any](
	raw json.RawMessage,
	field string,
) ([]T, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("%s is missing or null", field)
	}
	if value[0] != '[' {
		return nil, fmt.Errorf("%s must be an array", field)
	}

	var items []T
	if err := json.Unmarshal(value, &items); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}

	return items, nil
}

func parseStringList(raw json.RawMessage) ([]string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return []string{}, nil
	}

	if value[0] != '[' {
		item, err := parseScalarString(value)
		if err != nil {
			return nil, err
		}
		if item == "" {
			return []string{}, nil
		}

		return []string{item}, nil
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(value, &rawItems); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}

	items := make([]string, 0, len(rawItems))
	seen := make(map[string]struct{}, len(rawItems))
	for _, rawItem := range rawItems {
		item, err := parseScalarString(rawItem)
		if err != nil {
			return nil, err
		}
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			return nil, fmt.Errorf("duplicate value %q", item)
		}
		seen[item] = struct{}{}
		items = append(items, item)
	}

	return items, nil
}

func isNullScalar(raw json.RawMessage) bool {
	value := bytes.TrimSpace(raw)

	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}
