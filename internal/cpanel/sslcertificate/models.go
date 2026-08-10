package sslcertificate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

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

type dedicatedInstalledHostResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiShowCertificate struct {
	CertificatePEM json.RawMessage `json:"cert"`
	Details        apiCertificate  `json:"details"`
}

type apiDedicatedInstalledHost struct {
	Host        json.RawMessage          `json:"host"`
	Certificate *apiInstalledCertificate `json:"certificate"`
}

type apiInstalledHost struct {
	ServerName    json.RawMessage          `json:"servername"`
	Domains       json.RawMessage          `json:"domains"`
	FQDNs         json.RawMessage          `json:"fqdns"`
	IsPrimaryOnIP json.RawMessage          `json:"is_primary_on_ip"`
	MailSNIStatus json.RawMessage          `json:"mail_sni_status"`
	NeedsSNI      json.RawMessage          `json:"needs_sni"`
	Certificate   *apiInstalledCertificate `json:"certificate"`
}

type apiInstalledCertificate struct {
	ID                         json.RawMessage      `json:"id"`
	Domains                    json.RawMessage      `json:"domains"`
	AutoSSLProvider            json.RawMessage      `json:"auto_ssl_provider"`
	AutoSSLProviderDisplayName json.RawMessage      `json:"auto_ssl_provider_display_name"`
	IsAutoSSL                  json.RawMessage      `json:"is_autossl"`
	IsSelfSigned               json.RawMessage      `json:"is_self_signed"`
	NotBefore                  json.RawMessage      `json:"not_before"`
	NotAfter                   json.RawMessage      `json:"not_after"`
	SignatureAlgorithm         json.RawMessage      `json:"signature_algorithm"`
	ModulusLength              json.RawMessage      `json:"modulus_length"`
	IssuerCommonName           json.RawMessage      `json:"issuer.commonName"`
	SubjectCommonName          json.RawMessage      `json:"subject.commonName"`
	Issuer                     apiDistinguishedName `json:"issuer"`
	Subject                    apiDistinguishedName `json:"subject"`
	ValidationType             json.RawMessage      `json:"validation_type"`
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
	ECDSACurveName     json.RawMessage      `json:"ecdsa_curve_name"`
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
	ECDSACurveName     string
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
	ecdsaCurveName, err := parseScalarString(value.ECDSACurveName)
	if err != nil {
		return Certificate{}, certificateFieldError(
			id,
			"ecdsa_curve_name",
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
		ECDSACurveName:     ecdsaCurveName,
		IsSelfSigned:       isSelfSigned,
		IssuerCommonName:   issuerCommonName,
		SubjectCommonName:  subjectCommonName,
		ValidationType:     validationType,
		DomainIsConfigured: domainIsConfigured,
	}, nil
}

// InstalledHost contains safe metadata for one installed SSL virtual host.
type InstalledHost struct {
	ServerName    string
	Domains       []string
	FQDNs         []string
	IsPrimaryOnIP *bool
	MailSNIStatus *bool
	NeedsSNI      *bool
	Certificate   InstalledCertificate
}

// InstalledCertificate contains safe metadata for an installed certificate.
type InstalledCertificate struct {
	ID                         string
	Domains                    []string
	AutoSSLProvider            string
	AutoSSLProviderDisplayName string
	IsAutoSSL                  *bool
	IsSelfSigned               bool
	NotBefore                  int64
	NotAfter                   int64
	SignatureAlgorithm         string
	ModulusLength              *int64
	IssuerCommonName           string
	SubjectCommonName          string
	ValidationType             string
}

func installedHostFromAPI(
	value apiInstalledHost,
	index int,
) (InstalledHost, error) {
	serverName, err := parseRequiredJSONString(value.ServerName)
	if err != nil {
		return InstalledHost{}, fmt.Errorf(
			"decode installed SSL host %d servername: %w",
			index,
			err,
		)
	}
	if err := validateInventoryName("servername", serverName); err != nil {
		return InstalledHost{}, fmt.Errorf(
			"invalid installed SSL host %d: %w",
			index,
			err,
		)
	}
	domains, err := parseRequiredInventoryStringList(
		value.Domains,
		"domains",
	)
	if err != nil {
		return InstalledHost{}, fmt.Errorf(
			"decode installed SSL host %q domains: %w",
			serverName,
			err,
		)
	}
	fqdns, err := parseRequiredInventoryStringList(value.FQDNs, "fqdns")
	if err != nil {
		return InstalledHost{}, fmt.Errorf(
			"decode installed SSL host %q FQDNs: %w",
			serverName,
			err,
		)
	}
	isPrimaryOnIP, err := parseRequiredScalarBool(value.IsPrimaryOnIP)
	if err != nil {
		return InstalledHost{}, installedHostFieldError(
			serverName,
			"is_primary_on_ip",
			err,
		)
	}
	mailSNIStatus, err := parseRequiredScalarBool(value.MailSNIStatus)
	if err != nil {
		return InstalledHost{}, installedHostFieldError(
			serverName,
			"mail_sni_status",
			err,
		)
	}
	needsSNI, err := parseRequiredScalarBool(value.NeedsSNI)
	if err != nil {
		return InstalledHost{}, installedHostFieldError(
			serverName,
			"needs_sni",
			err,
		)
	}
	if value.Certificate == nil {
		return InstalledHost{}, fmt.Errorf(
			"installed SSL host %q has no certificate object",
			serverName,
		)
	}
	certificate, err := installedCertificateFromAPI(
		*value.Certificate,
		true,
	)
	if err != nil {
		return InstalledHost{}, fmt.Errorf(
			"decode installed SSL host %q certificate: %w",
			serverName,
			err,
		)
	}

	return InstalledHost{
		ServerName:    serverName,
		Domains:       domains,
		FQDNs:         fqdns,
		IsPrimaryOnIP: boolPointer(isPrimaryOnIP),
		MailSNIStatus: boolPointer(mailSNIStatus),
		NeedsSNI:      boolPointer(needsSNI),
		Certificate:   certificate,
	}, nil
}

func dedicatedInstalledHostFromAPI(
	value apiDedicatedInstalledHost,
) (InstalledHost, error) {
	serverName, err := parseRequiredJSONString(value.Host)
	if err != nil {
		return InstalledHost{}, fmt.Errorf(
			"decode dedicated-IP SSL host: %w",
			err,
		)
	}
	if err := validateInventoryName("host", serverName); err != nil {
		return InstalledHost{}, fmt.Errorf(
			"invalid dedicated-IP SSL host: %w",
			err,
		)
	}
	if value.Certificate == nil {
		return InstalledHost{}, fmt.Errorf(
			"dedicated-IP SSL host %q has no certificate object",
			serverName,
		)
	}
	certificate, err := installedCertificateFromAPI(
		*value.Certificate,
		false,
	)
	if err != nil {
		return InstalledHost{}, fmt.Errorf(
			"decode dedicated-IP SSL host %q certificate: %w",
			serverName,
			err,
		)
	}

	return InstalledHost{
		ServerName:  serverName,
		Certificate: certificate,
	}, nil
}

func installedCertificateFromAPI(
	value apiInstalledCertificate,
	requireAutoSSL bool,
) (InstalledCertificate, error) {
	id, err := parseRequiredScalarString(value.ID)
	if err != nil {
		return InstalledCertificate{}, fmt.Errorf(
			"decode certificate id: %w",
			err,
		)
	}
	if err := ValidateID(id); err != nil {
		return InstalledCertificate{}, fmt.Errorf(
			"invalid certificate id: %w",
			err,
		)
	}
	domains, err := parseRequiredInventoryStringList(
		value.Domains,
		"certificate domains",
	)
	if err != nil {
		return InstalledCertificate{}, err
	}
	autoSSLProvider, err := parseScalarString(value.AutoSSLProvider)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"auto_ssl_provider",
			err,
		)
	}
	autoSSLProviderDisplayName, err := parseScalarString(
		value.AutoSSLProviderDisplayName,
	)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"auto_ssl_provider_display_name",
			err,
		)
	}
	var isAutoSSL *bool
	if requireAutoSSL {
		parsed, err := parseRequiredScalarBool(value.IsAutoSSL)
		if err != nil {
			return InstalledCertificate{}, installedCertificateFieldError(
				id,
				"is_autossl",
				err,
			)
		}
		isAutoSSL = boolPointer(parsed)
	} else {
		isAutoSSL, err = parseOptionalScalarBool(value.IsAutoSSL)
		if err != nil {
			return InstalledCertificate{}, installedCertificateFieldError(
				id,
				"is_autossl",
				err,
			)
		}
	}
	isSelfSigned, err := parseRequiredScalarBool(value.IsSelfSigned)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"is_self_signed",
			err,
		)
	}
	notBefore, err := parseRequiredScalarInt64(value.NotBefore)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"not_before",
			err,
		)
	}
	notAfter, err := parseRequiredScalarInt64(value.NotAfter)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"not_after",
			err,
		)
	}
	if notBefore < 0 || notAfter < 0 {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"validity",
			fmt.Errorf("timestamps must not be negative"),
		)
	}
	if notAfter < notBefore {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"validity",
			fmt.Errorf("not_after must not precede not_before"),
		)
	}
	signatureAlgorithm, err := parseScalarString(
		value.SignatureAlgorithm,
	)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"signature_algorithm",
			err,
		)
	}
	modulusLength, err := parseOptionalScalarInt64(value.ModulusLength)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"modulus_length",
			err,
		)
	}
	if modulusLength != nil && *modulusLength <= 0 {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"modulus_length",
			fmt.Errorf("must be positive when present"),
		)
	}
	issuerCommonName, err := parseCommonName(
		value.IssuerCommonName,
		value.Issuer.CommonName,
	)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
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
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"subject common name",
			err,
		)
	}
	validationType, err := parseScalarString(value.ValidationType)
	if err != nil {
		return InstalledCertificate{}, installedCertificateFieldError(
			id,
			"validation_type",
			err,
		)
	}

	return InstalledCertificate{
		ID:                         id,
		Domains:                    domains,
		AutoSSLProvider:            autoSSLProvider,
		AutoSSLProviderDisplayName: autoSSLProviderDisplayName,
		IsAutoSSL:                  isAutoSSL,
		IsSelfSigned:               isSelfSigned,
		NotBefore:                  notBefore,
		NotAfter:                   notAfter,
		SignatureAlgorithm:         signatureAlgorithm,
		ModulusLength:              modulusLength,
		IssuerCommonName:           issuerCommonName,
		SubjectCommonName:          subjectCommonName,
		ValidationType:             validationType,
	}, nil
}

func installedHostFieldError(serverName, field string, err error) error {
	return fmt.Errorf(
		"decode installed SSL host %q %s: %w",
		serverName,
		field,
		err,
	)
}

func installedCertificateFieldError(id, field string, err error) error {
	return fmt.Errorf(
		"decode installed SSL certificate %q %s: %w",
		id,
		field,
		err,
	)
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

func parseRequiredScalarInt64(raw json.RawMessage) (int64, error) {
	if isNullScalar(raw) {
		return 0, fmt.Errorf("required integer value is missing or null")
	}

	return parseScalarInt64(raw)
}

func parseOptionalScalarInt64(raw json.RawMessage) (*int64, error) {
	if isNullScalar(raw) {
		return nil, nil
	}
	value, err := parseScalarInt64(raw)
	if err != nil {
		return nil, err
	}

	return &value, nil
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

func parseOptionalScalarBool(raw json.RawMessage) (*bool, error) {
	if isNullScalar(raw) {
		return nil, nil
	}
	value, err := parseScalarBool(raw)
	if err != nil {
		return nil, err
	}

	return boolPointer(value), nil
}

func boolPointer(value bool) *bool {
	return &value
}

func parseRequiredScalarString(raw json.RawMessage) (string, error) {
	if isNullScalar(raw) {
		return "", fmt.Errorf("required string value is missing or null")
	}
	value, err := parseScalarString(raw)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("required string value is empty")
	}

	return value, nil
}

func parseRequiredJSONString(raw json.RawMessage) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", fmt.Errorf("required string value is missing or null")
	}
	if value[0] != '"' {
		return "", fmt.Errorf("expected a string, got %s", value)
	}
	var output string
	if err := json.Unmarshal(value, &output); err != nil {
		return "", fmt.Errorf("decode string: %w", err)
	}
	if output == "" {
		return "", fmt.Errorf("required string value is empty")
	}

	return output, nil
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

func parseRequiredObject[T any](
	raw json.RawMessage,
	field string,
) (T, error) {
	var output T
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return output, fmt.Errorf("%s is missing or null", field)
	}
	if value[0] != '{' {
		return output, fmt.Errorf("%s must be an object", field)
	}
	if err := json.Unmarshal(value, &output); err != nil {
		return output, fmt.Errorf("decode %s: %w", field, err)
	}

	return output, nil
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

func parseRequiredInventoryStringList(
	raw json.RawMessage,
	field string,
) ([]string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("%s is missing or null", field)
	}
	if value[0] != '[' {
		return nil, fmt.Errorf("%s must be an array", field)
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(value, &rawItems); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if len(rawItems) == 0 {
		return nil, fmt.Errorf("%s must not be empty", field)
	}
	items := make([]string, 0, len(rawItems))
	seen := make(map[string]struct{}, len(rawItems))
	for _, rawItem := range rawItems {
		item, err := parseRequiredJSONString(rawItem)
		if err != nil {
			return nil, fmt.Errorf("decode %s value: %w", field, err)
		}
		if err := validateInventoryName(field, item); err != nil {
			return nil, err
		}
		if _, exists := seen[item]; exists {
			return nil, fmt.Errorf("%s contains duplicate %q", field, item)
		}
		seen[item] = struct{}{}
		items = append(items, item)
	}
	sort.Strings(items)

	return items, nil
}

func validateInventoryName(field, value string) error {
	if len(value) > 4096 {
		return fmt.Errorf("%s must not exceed 4096 bytes", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", field)
	}
	for _, character := range value {
		if unicode.IsControl(character) ||
			unicode.IsSpace(character) ||
			character == ',' {
			return fmt.Errorf(
				"%s must not contain commas, whitespace, or control characters",
				field,
			)
		}
	}

	return nil
}

func isNullScalar(raw json.RawMessage) bool {
	value := bytes.TrimSpace(raw)

	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}
