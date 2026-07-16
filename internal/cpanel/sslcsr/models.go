package sslcsr

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	maxCSRIdentifierLength = 4096
	maxFriendlyNameLength  = 1024
	maxSubjectFieldLength  = 1024
	maxEmailAddressLength  = 320
	maxDomainLength        = 255
	maxCSRPEMLength        = 1 << 20
	csrPEMBlockStart       = "-----BEGIN CERTIFICATE REQUEST-----"
)

var emailAddressOID = []int{1, 2, 840, 113549, 1, 9, 1}

var extensionRequestOID = []int{1, 2, 840, 113549, 1, 9, 14}

var subjectAltNameOID = []int{2, 5, 29, 17}

var allowedSubjectOIDs = [][]int{
	{2, 5, 4, 3},
	{2, 5, 4, 6},
	{2, 5, 4, 7},
	{2, 5, 4, 8},
	{2, 5, 4, 10},
	{2, 5, 4, 11},
	emailAddressOID,
}

type keyListResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type listResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type showResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiShowCSR struct {
	CSR     json.RawMessage `json:"csr"`
	Details json.RawMessage `json:"details"`
}

type apiCSR struct {
	ID                     json.RawMessage `json:"id"`
	FriendlyName           json.RawMessage `json:"friendly_name"`
	CommonName             json.RawMessage `json:"commonName"`
	CountryName            json.RawMessage `json:"countryName"`
	StateOrProvinceName    json.RawMessage `json:"stateOrProvinceName"`
	LocalityName           json.RawMessage `json:"localityName"`
	OrganizationName       json.RawMessage `json:"organizationName"`
	OrganizationalUnitName json.RawMessage `json:"organizationalUnitName"`
	EmailAddress           json.RawMessage `json:"emailAddress"`
	Created                json.RawMessage `json:"created"`
	Domains                json.RawMessage `json:"domains"`
	KeyAlgorithm           json.RawMessage `json:"key_algorithm"`
	Modulus                json.RawMessage `json:"modulus"`
	ECDSACurveName         json.RawMessage `json:"ecdsa_curve_name"`
	ECDSAPublic            json.RawMessage `json:"ecdsa_public"`
	Text                   json.RawMessage `json:"text"`
}

type apiPublicKey struct {
	ID             json.RawMessage `json:"id"`
	FriendlyName   json.RawMessage `json:"friendly_name"`
	Created        json.RawMessage `json:"created"`
	KeyAlgorithm   json.RawMessage `json:"key_algorithm"`
	Modulus        json.RawMessage `json:"modulus"`
	ModulusLength  json.RawMessage `json:"modulus_length"`
	ECDSACurveName json.RawMessage `json:"ecdsa_curve_name"`
	ECDSAPublic    json.RawMessage `json:"ecdsa_public"`
}

type publicKeyMetadata struct {
	ID             string
	KeyAlgorithm   string
	Modulus        string
	ECDSACurveName string
	ECDSAPublic    string
}

// CSRMetadata contains safe list-only metadata for one stored CSR.
type CSRMetadata struct {
	ID             string
	FriendlyName   string
	CommonName     string
	Domains        []string
	Created        int64
	KeyAlgorithm   string
	ModulusLength  *int64
	ECDSACurveName string
}

// KeyMetadata contains safe public metadata for one stored SSL key.
type KeyMetadata struct {
	ID             string
	FriendlyName   string
	Created        int64
	KeyAlgorithm   string
	ModulusLength  *int64
	ECDSACurveName string
}

type rawTBSCertificateRequest struct {
	Raw           asn1.RawContent
	Version       int
	Subject       asn1.RawValue
	PublicKey     asn1.RawValue
	RawAttributes []asn1.RawValue `asn1:"tag:0"`
}

type rawPKCS10Attribute struct {
	ID     asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

// Identity is the stable identity of one cPanel certificate signing request.
type Identity struct {
	ID                string
	FingerprintSHA256 string
}

// CSR contains one canonical cPanel certificate signing request and metadata.
type CSR struct {
	ID                     string
	FriendlyName           string
	CSRPEM                 string
	FingerprintSHA256      string
	CommonName             string
	CountryName            string
	StateOrProvinceName    string
	LocalityName           string
	OrganizationName       string
	OrganizationalUnitName string
	EmailAddress           string
	Created                int64
	Domains                []string
	KeyAlgorithm           string
	Modulus                string
	ECDSACurveName         string
	ECDSAPublic            string
}

// Identity returns the cPanel ID and DER-derived fingerprint for the CSR.
func (csr CSR) Identity() Identity {
	return Identity{
		ID:                csr.ID,
		FingerprintSHA256: csr.FingerprintSHA256,
	}
}

// Definition describes a CSR generated from an existing cPanel key.
type Definition struct {
	KeyID                  string
	FriendlyName           string
	Domains                []string
	CountryName            string
	StateOrProvinceName    string
	LocalityName           string
	OrganizationName       string
	OrganizationalUnitName string
	EmailAddress           string
}

// ParsedCSR contains a parsed PKCS#10 request and its canonical public PEM.
type ParsedCSR struct {
	Request           *x509.CertificateRequest
	NormalizedPEM     string
	SHA256Fingerprint string
}

// ParsePEM accepts exactly one signed CERTIFICATE REQUEST PEM block.
func ParsePEM(value string) (*ParsedCSR, error) {
	if value == "" {
		return nil, fmt.Errorf("CSR PEM must not be empty")
	}
	if len(value) > maxCSRPEMLength {
		return nil, fmt.Errorf(
			"CSR PEM must not exceed %d bytes",
			maxCSRPEMLength,
		)
	}

	trimmedValue := bytes.TrimSpace([]byte(value))
	if !bytes.HasPrefix(trimmedValue, []byte(csrPEMBlockStart)) {
		return nil, fmt.Errorf(
			"CSR PEM must contain exactly one CERTIFICATE REQUEST block",
		)
	}

	block, rest := pem.Decode(trimmedValue)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf(
			"CSR PEM must contain exactly one CERTIFICATE REQUEST block",
		)
	}
	if len(block.Headers) != 0 {
		return nil, fmt.Errorf("CSR PEM block must not contain headers")
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf(
			"CSR PEM must not contain additional PEM blocks or data",
		)
	}

	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#10 certificate request: %w", err)
	}
	if err := request.CheckSignature(); err != nil {
		return nil, fmt.Errorf(
			"verify PKCS#10 certificate request signature: %w",
			err,
		)
	}

	normalized := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: request.Raw,
	})
	fingerprint := sha256.Sum256(request.Raw)

	return &ParsedCSR{
		Request:           request,
		NormalizedPEM:     strings.TrimSpace(string(normalized)),
		SHA256Fingerprint: hex.EncodeToString(fingerprint[:]),
	}, nil
}

// EqualPEM reports whether two PEM values contain the same PKCS#10 request.
func EqualPEM(first, second string) (bool, error) {
	firstCSR, err := ParsePEM(first)
	if err != nil {
		return false, fmt.Errorf("parse first CSR PEM: %w", err)
	}
	secondCSR, err := ParsePEM(second)
	if err != nil {
		return false, fmt.Errorf("parse second CSR PEM: %w", err)
	}

	return bytes.Equal(firstCSR.Request.Raw, secondCSR.Request.Raw), nil
}

// ValidateID checks a cPanel CSR or key inventory identifier.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("SSL CSR id must not be empty")
	}
	if len(id) > maxCSRIdentifierLength {
		return fmt.Errorf(
			"SSL CSR id must not exceed %d bytes",
			maxCSRIdentifierLength,
		)
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf(
			"SSL CSR id must not contain surrounding whitespace",
		)
	}
	for _, character := range id {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf(
				"SSL CSR id must not contain whitespace or control characters",
			)
		}
	}

	return nil
}

// ValidateFingerprintSHA256 checks the canonical lowercase SHA-256 identity.
func ValidateFingerprintSHA256(fingerprint string) error {
	if len(fingerprint) != sha256.Size*2 {
		return fmt.Errorf(
			"SSL CSR SHA-256 fingerprint must contain %d lowercase hexadecimal characters",
			sha256.Size*2,
		)
	}
	if strings.ToLower(fingerprint) != fingerprint {
		return fmt.Errorf(
			"SSL CSR SHA-256 fingerprint must use lowercase hexadecimal characters",
		)
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return fmt.Errorf(
			"SSL CSR SHA-256 fingerprint must be hexadecimal: %w",
			err,
		)
	}

	return nil
}

// ValidateIdentity checks both stable CSR identity components.
func ValidateIdentity(identity Identity) error {
	if err := ValidateID(identity.ID); err != nil {
		return err
	}

	return ValidateFingerprintSHA256(identity.FingerprintSHA256)
}

// ValidateFriendlyName checks a user-supplied cPanel CSR label.
func ValidateFriendlyName(friendlyName string) error {
	return validateText(
		"SSL CSR friendly name",
		friendlyName,
		maxFriendlyNameLength,
		false,
	)
}

// ValidateDefinition checks all fields sent to SSL::generate_csr.
func ValidateDefinition(definition Definition) error {
	if err := ValidateID(definition.KeyID); err != nil {
		return fmt.Errorf("invalid SSL key id: %w", err)
	}
	if definition.FriendlyName == "" {
		return fmt.Errorf("SSL CSR friendly name must not be empty")
	}
	if err := ValidateFriendlyName(definition.FriendlyName); err != nil {
		return err
	}
	if len(definition.Domains) == 0 {
		return fmt.Errorf("SSL CSR domains must not be empty")
	}
	if err := ValidateDomains(definition.Domains); err != nil {
		return err
	}

	if len(definition.CountryName) != 2 ||
		definition.CountryName[0] < 'A' ||
		definition.CountryName[0] > 'Z' ||
		definition.CountryName[1] < 'A' ||
		definition.CountryName[1] > 'Z' {
		return fmt.Errorf(
			"SSL CSR country name must be a two-letter uppercase country code",
		)
	}
	for name, value := range map[string]string{
		"locality name":          definition.LocalityName,
		"organization name":      definition.OrganizationName,
		"state or province name": definition.StateOrProvinceName,
	} {
		if err := validateText(
			"SSL CSR "+name,
			value,
			maxSubjectFieldLength,
			true,
		); err != nil {
			return err
		}
	}
	if err := validateText(
		"SSL CSR organizational unit name",
		definition.OrganizationalUnitName,
		maxSubjectFieldLength,
		false,
	); err != nil {
		return err
	}
	if err := validateText(
		"SSL CSR email address",
		definition.EmailAddress,
		maxEmailAddressLength,
		false,
	); err != nil {
		return err
	}
	if definition.EmailAddress != "" {
		address, err := mail.ParseAddress(definition.EmailAddress)
		if err != nil || address.Address != definition.EmailAddress {
			return fmt.Errorf(
				"SSL CSR email address must be one plain email address",
			)
		}
	}

	return nil
}

// ValidateDomains checks the ordered common-name and SAN domain list.
func ValidateDomains(domains []string) error {
	if len(domains) == 0 {
		return fmt.Errorf("SSL CSR domains must not be empty")
	}

	seenDomains := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		if err := validateDomain(domain); err != nil {
			return err
		}
		if _, exists := seenDomains[domain]; exists {
			return fmt.Errorf("SSL CSR domains contain duplicate %q", domain)
		}
		seenDomains[domain] = struct{}{}
	}

	return nil
}

func validateText(
	name string,
	value string,
	maxLength int,
	required bool,
) error {
	if required && value == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%s must not exceed %d bytes", name, maxLength)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s must not contain control characters", name)
		}
	}

	return nil
}

func validateDomain(domain string) error {
	if err := validateText(
		"SSL CSR domain",
		domain,
		maxDomainLength,
		true,
	); err != nil {
		return err
	}
	for _, character := range domain {
		if unicode.IsSpace(character) || character == ',' {
			return fmt.Errorf(
				"SSL CSR domain %q must not contain whitespace or commas",
				domain,
			)
		}
	}

	return nil
}

func csrFromAPI(value apiCSR) (CSR, error) {
	id, err := parseRequiredScalarIdentifier(value.ID)
	if err != nil {
		return CSR{}, fmt.Errorf("decode SSL CSR id: %w", err)
	}
	if err := ValidateID(id); err != nil {
		return CSR{}, fmt.Errorf("invalid SSL CSR id: %w", err)
	}

	friendlyName, err := parseRequiredScalarString(value.FriendlyName)
	if err != nil {
		return CSR{}, csrFieldError(id, "friendly_name", err)
	}
	if err := ValidateFriendlyName(friendlyName); err != nil {
		return CSR{}, fmt.Errorf(
			"invalid SSL CSR %q friendly name: %w",
			id,
			err,
		)
	}

	commonName, err := parseRequiredScalarString(value.CommonName)
	if err != nil {
		return CSR{}, csrFieldError(id, "commonName", err)
	}
	if commonName == "" {
		return CSR{}, csrFieldError(
			id,
			"commonName",
			fmt.Errorf("required string value is empty"),
		)
	}
	created, err := parseRequiredScalarInt64(value.Created)
	if err != nil {
		return CSR{}, csrFieldError(id, "created", err)
	}
	if created < 0 {
		return CSR{}, csrFieldError(
			id,
			"created",
			fmt.Errorf("must not be negative"),
		)
	}
	domains, err := parseRequiredStringList(value.Domains)
	if err != nil {
		return CSR{}, csrFieldError(id, "domains", err)
	}
	keyAlgorithm, err := parseRequiredScalarString(value.KeyAlgorithm)
	if err != nil {
		return CSR{}, csrFieldError(id, "key_algorithm", err)
	}
	if keyAlgorithm == "" {
		return CSR{}, csrFieldError(
			id,
			"key_algorithm",
			fmt.Errorf("required string value is empty"),
		)
	}

	countryName, err := parseScalarString(value.CountryName)
	if err != nil {
		return CSR{}, csrFieldError(id, "countryName", err)
	}
	stateOrProvinceName, err := parseScalarString(
		value.StateOrProvinceName,
	)
	if err != nil {
		return CSR{}, csrFieldError(
			id,
			"stateOrProvinceName",
			err,
		)
	}
	localityName, err := parseScalarString(value.LocalityName)
	if err != nil {
		return CSR{}, csrFieldError(id, "localityName", err)
	}
	organizationName, err := parseScalarString(value.OrganizationName)
	if err != nil {
		return CSR{}, csrFieldError(id, "organizationName", err)
	}
	organizationalUnitName, err := parseScalarString(
		value.OrganizationalUnitName,
	)
	if err != nil {
		return CSR{}, csrFieldError(
			id,
			"organizationalUnitName",
			err,
		)
	}
	emailAddress, err := parseScalarString(value.EmailAddress)
	if err != nil {
		return CSR{}, csrFieldError(id, "emailAddress", err)
	}
	modulus, err := parseScalarString(value.Modulus)
	if err != nil {
		return CSR{}, csrFieldError(id, "modulus", err)
	}
	ecdsaCurveName, err := parseScalarString(value.ECDSACurveName)
	if err != nil {
		return CSR{}, csrFieldError(id, "ecdsa_curve_name", err)
	}
	ecdsaPublic, err := parseScalarString(value.ECDSAPublic)
	if err != nil {
		return CSR{}, csrFieldError(id, "ecdsa_public", err)
	}

	return CSR{
		ID:                     id,
		FriendlyName:           friendlyName,
		CommonName:             commonName,
		CountryName:            countryName,
		StateOrProvinceName:    stateOrProvinceName,
		LocalityName:           localityName,
		OrganizationName:       organizationName,
		OrganizationalUnitName: organizationalUnitName,
		EmailAddress:           emailAddress,
		Created:                created,
		Domains:                domains,
		KeyAlgorithm:           keyAlgorithm,
		Modulus:                modulus,
		ECDSACurveName:         ecdsaCurveName,
		ECDSAPublic:            ecdsaPublic,
	}, nil
}

func publicKeyFromAPI(value apiPublicKey) (publicKeyMetadata, error) {
	id, err := parseRequiredScalarIdentifier(value.ID)
	if err != nil {
		return publicKeyMetadata{}, fmt.Errorf(
			"decode SSL public key id: %w",
			err,
		)
	}
	if err := ValidateID(id); err != nil {
		return publicKeyMetadata{}, fmt.Errorf(
			"invalid SSL public key id: %w",
			err,
		)
	}
	keyAlgorithm, err := parseRequiredScalarString(value.KeyAlgorithm)
	if err != nil {
		return publicKeyMetadata{}, fmt.Errorf(
			"decode SSL public key %q key_algorithm: %w",
			id,
			err,
		)
	}
	modulus, err := parseScalarString(value.Modulus)
	if err != nil {
		return publicKeyMetadata{}, fmt.Errorf(
			"decode SSL public key %q modulus: %w",
			id,
			err,
		)
	}
	ecdsaCurveName, err := parseScalarString(value.ECDSACurveName)
	if err != nil {
		return publicKeyMetadata{}, fmt.Errorf(
			"decode SSL public key %q ecdsa_curve_name: %w",
			id,
			err,
		)
	}
	ecdsaPublic, err := parseScalarString(value.ECDSAPublic)
	if err != nil {
		return publicKeyMetadata{}, fmt.Errorf(
			"decode SSL public key %q ecdsa_public: %w",
			id,
			err,
		)
	}

	key := publicKeyMetadata{
		ID:             id,
		KeyAlgorithm:   keyAlgorithm,
		Modulus:        modulus,
		ECDSACurveName: ecdsaCurveName,
		ECDSAPublic:    ecdsaPublic,
	}
	switch key.KeyAlgorithm {
	case "rsaEncryption":
		if key.Modulus == "" {
			return publicKeyMetadata{}, fmt.Errorf(
				"SSL public key %q has no RSA modulus",
				id,
			)
		}
		if key.ECDSACurveName != "" || key.ECDSAPublic != "" {
			return publicKeyMetadata{}, fmt.Errorf(
				"SSL public key %q mixes RSA and ECDSA metadata",
				id,
			)
		}
		if _, err := parseHexInteger(
			key.Modulus,
			fmt.Sprintf("SSL public key %q RSA modulus", id),
		); err != nil {
			return publicKeyMetadata{}, err
		}
	case "id-ecPublicKey":
		if key.Modulus != "" {
			return publicKeyMetadata{}, fmt.Errorf(
				"SSL public key %q mixes ECDSA and RSA metadata",
				id,
			)
		}
		if key.ECDSACurveName == "" || key.ECDSAPublic == "" {
			return publicKeyMetadata{}, fmt.Errorf(
				"SSL public key %q has incomplete ECDSA metadata",
				id,
			)
		}
		curve, err := curveByCPanelName(key.ECDSACurveName)
		if err != nil {
			return publicKeyMetadata{}, fmt.Errorf(
				"SSL public key %q: %w",
				id,
				err,
			)
		}
		if _, err := parseECDSAPublic(
			curve,
			key.ECDSAPublic,
			fmt.Sprintf("SSL public key %q ECDSA", id),
		); err != nil {
			return publicKeyMetadata{}, err
		}
	default:
		return publicKeyMetadata{}, fmt.Errorf(
			"SSL public key %q uses unsupported algorithm %q",
			id,
			key.KeyAlgorithm,
		)
	}

	return key, nil
}

func csrMetadataFromAPI(value apiCSR) (CSRMetadata, error) {
	csr, err := csrFromAPI(value)
	if err != nil {
		return CSRMetadata{}, err
	}
	key, err := publicKeyFromAPI(apiPublicKey{
		ID:             value.ID,
		KeyAlgorithm:   value.KeyAlgorithm,
		Modulus:        value.Modulus,
		ECDSACurveName: value.ECDSACurveName,
		ECDSAPublic:    value.ECDSAPublic,
	})
	if err != nil {
		return CSRMetadata{}, fmt.Errorf(
			"validate SSL CSR %q public-key metadata: %w",
			csr.ID,
			err,
		)
	}
	modulusLength, err := publicKeyModulusLength(key)
	if err != nil {
		return CSRMetadata{}, fmt.Errorf(
			"derive SSL CSR %q modulus length: %w",
			csr.ID,
			err,
		)
	}

	return CSRMetadata{
		ID:             csr.ID,
		FriendlyName:   csr.FriendlyName,
		CommonName:     csr.CommonName,
		Domains:        csr.Domains,
		Created:        csr.Created,
		KeyAlgorithm:   csr.KeyAlgorithm,
		ModulusLength:  modulusLength,
		ECDSACurveName: csr.ECDSACurveName,
	}, nil
}

func keyMetadataFromAPI(value apiPublicKey) (KeyMetadata, error) {
	key, err := publicKeyFromAPI(value)
	if err != nil {
		return KeyMetadata{}, err
	}
	friendlyName, err := parseRequiredScalarString(value.FriendlyName)
	if err != nil {
		return KeyMetadata{}, fmt.Errorf(
			"decode SSL key %q friendly_name: %w",
			key.ID,
			err,
		)
	}
	if err := ValidateFriendlyName(friendlyName); err != nil {
		return KeyMetadata{}, fmt.Errorf(
			"invalid SSL key %q friendly name: %w",
			key.ID,
			err,
		)
	}
	created, err := parseRequiredScalarInt64(value.Created)
	if err != nil {
		return KeyMetadata{}, fmt.Errorf(
			"decode SSL key %q created: %w",
			key.ID,
			err,
		)
	}
	if created < 0 {
		return KeyMetadata{}, fmt.Errorf(
			"decode SSL key %q created: must not be negative",
			key.ID,
		)
	}
	reportedModulusLength, err := parseOptionalScalarInt64(
		value.ModulusLength,
	)
	if err != nil {
		return KeyMetadata{}, fmt.Errorf(
			"decode SSL key %q modulus_length: %w",
			key.ID,
			err,
		)
	}
	derivedModulusLength, err := publicKeyModulusLength(key)
	if err != nil {
		return KeyMetadata{}, err
	}
	switch key.KeyAlgorithm {
	case "rsaEncryption":
		if reportedModulusLength == nil {
			return KeyMetadata{}, fmt.Errorf(
				"SSL key %q has no modulus_length",
				key.ID,
			)
		}
		if *reportedModulusLength <= 0 {
			return KeyMetadata{}, fmt.Errorf(
				"SSL key %q modulus_length must be positive",
				key.ID,
			)
		}
		if *reportedModulusLength != *derivedModulusLength {
			return KeyMetadata{}, fmt.Errorf(
				"SSL key %q modulus_length is %d; derived %d",
				key.ID,
				*reportedModulusLength,
				*derivedModulusLength,
			)
		}
	case "id-ecPublicKey":
		if reportedModulusLength != nil {
			return KeyMetadata{}, fmt.Errorf(
				"SSL key %q reports an RSA modulus_length for ECDSA",
				key.ID,
			)
		}
	}

	return KeyMetadata{
		ID:             key.ID,
		FriendlyName:   friendlyName,
		Created:        created,
		KeyAlgorithm:   key.KeyAlgorithm,
		ModulusLength:  derivedModulusLength,
		ECDSACurveName: key.ECDSACurveName,
	}, nil
}

func publicKeyModulusLength(
	key publicKeyMetadata,
) (*int64, error) {
	if key.KeyAlgorithm != "rsaEncryption" {
		return nil, nil
	}
	modulus, err := parseHexInteger(
		key.Modulus,
		fmt.Sprintf("SSL public key %q RSA modulus", key.ID),
	)
	if err != nil {
		return nil, err
	}
	length := int64(modulus.BitLen())

	return &length, nil
}

func csrFieldError(id, field string, err error) error {
	return fmt.Errorf("decode SSL CSR %q %s: %w", id, field, err)
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

	return "", fmt.Errorf(
		"expected a string or null, got %s",
		value,
	)
}

func parseRequiredScalarString(raw json.RawMessage) (string, error) {
	if isNullScalar(raw) {
		return "", fmt.Errorf("required string value is missing or null")
	}

	return parseScalarString(raw)
}

func parseScalarIdentifier(raw json.RawMessage) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", nil
	}
	if value[0] == '"' {
		var stringValue string
		if err := json.Unmarshal(value, &stringValue); err != nil {
			return "", fmt.Errorf("decode string identifier: %w", err)
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
		"expected a string, number, or null identifier, got %s",
		value,
	)
}

func parseRequiredScalarIdentifier(
	raw json.RawMessage,
) (string, error) {
	if isNullScalar(raw) {
		return "", fmt.Errorf("required identifier is missing or null")
	}

	return parseScalarIdentifier(raw)
}

func parseRequiredScalarInt64(raw json.RawMessage) (int64, error) {
	value, err := parseRequiredScalarIdentifier(raw)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, fmt.Errorf("required integer value is empty")
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as an integer: %w", value, err)
	}

	return parsed, nil
}

func parseOptionalScalarInt64(raw json.RawMessage) (*int64, error) {
	if isNullScalar(raw) {
		return nil, nil
	}
	value, err := parseRequiredScalarInt64(raw)
	if err != nil {
		return nil, err
	}

	return &value, nil
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

func parseRequiredStringList(raw json.RawMessage) ([]string, error) {
	rawItems, err := parseRequiredArray[json.RawMessage](
		raw,
		"string list",
	)
	if err != nil {
		return nil, err
	}
	if len(rawItems) == 0 {
		return nil, fmt.Errorf("string list must not be empty")
	}

	items := make([]string, 0, len(rawItems))
	seen := make(map[string]struct{}, len(rawItems))
	for _, rawItem := range rawItems {
		item, err := parseRequiredScalarString(rawItem)
		if err != nil {
			return nil, err
		}
		if item == "" {
			return nil, fmt.Errorf("string list contains an empty value")
		}
		if err := validateDomain(item); err != nil {
			return nil, err
		}
		if _, exists := seen[item]; exists {
			return nil, fmt.Errorf("duplicate value %q", item)
		}
		seen[item] = struct{}{}
		items = append(items, item)
	}
	sort.Strings(items)

	return items, nil
}

func isNullScalar(raw json.RawMessage) bool {
	value := bytes.TrimSpace(raw)

	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}

func attachPEM(metadata CSR, value string) (CSR, error) {
	parsed, err := ParsePEM(value)
	if err != nil {
		return CSR{}, err
	}
	if err := verifyPEMMetadata(metadata, parsed.Request); err != nil {
		return CSR{}, err
	}
	if err := verifyCSRPublicKeyMetadata(
		metadata,
		parsed.Request,
	); err != nil {
		return CSR{}, err
	}

	metadata.CSRPEM = parsed.NormalizedPEM
	metadata.FingerprintSHA256 = parsed.SHA256Fingerprint

	return metadata, nil
}

func verifyPEMMetadata(
	metadata CSR,
	request *x509.CertificateRequest,
) error {
	if err := verifyRequestShape(request); err != nil {
		return fmt.Errorf(
			"SSL CSR %q contains unsupported PKCS#10 content: %w",
			metadata.ID,
			err,
		)
	}
	if request.Subject.CommonName != metadata.CommonName {
		return fmt.Errorf(
			"SSL CSR %q PEM common name is %q; inventory reports %q",
			metadata.ID,
			request.Subject.CommonName,
			metadata.CommonName,
		)
	}

	pemDomains, err := requestDomains(request)
	if err != nil {
		return fmt.Errorf(
			"decode SSL CSR %q PEM domains: %w",
			metadata.ID,
			err,
		)
	}
	if !equalStrings(pemDomains, metadata.Domains) {
		return fmt.Errorf(
			"SSL CSR %q PEM domains %v do not match inventory domains %v",
			metadata.ID,
			pemDomains,
			metadata.Domains,
		)
	}

	countryName, err := singleValue(
		"countryName",
		request.Subject.Country,
	)
	if err != nil {
		return err
	}
	stateOrProvinceName, err := singleValue(
		"stateOrProvinceName",
		request.Subject.Province,
	)
	if err != nil {
		return err
	}
	localityName, err := singleValue(
		"localityName",
		request.Subject.Locality,
	)
	if err != nil {
		return err
	}
	organizationName, err := singleValue(
		"organizationName",
		request.Subject.Organization,
	)
	if err != nil {
		return err
	}
	organizationalUnitName, err := singleValue(
		"organizationalUnitName",
		request.Subject.OrganizationalUnit,
	)
	if err != nil {
		return err
	}

	subjectChecks := []struct {
		field    string
		actual   string
		expected string
	}{
		{"countryName", countryName, metadata.CountryName},
		{
			"stateOrProvinceName",
			stateOrProvinceName,
			metadata.StateOrProvinceName,
		},
		{"localityName", localityName, metadata.LocalityName},
		{
			"organizationName",
			organizationName,
			metadata.OrganizationName,
		},
		{
			"organizationalUnitName",
			organizationalUnitName,
			metadata.OrganizationalUnitName,
		},
	}
	for _, check := range subjectChecks {
		if check.actual != check.expected {
			return fmt.Errorf(
				"SSL CSR %q PEM %s is %q; inventory reports %q",
				metadata.ID,
				check.field,
				check.actual,
				check.expected,
			)
		}
	}

	emailAddress, err := requestEmailAddress(request)
	if err != nil {
		return fmt.Errorf("decode SSL CSR %q PEM email address: %w", metadata.ID, err)
	}
	if emailAddress != metadata.EmailAddress {
		return fmt.Errorf(
			"SSL CSR %q PEM emailAddress is %q; inventory reports %q",
			metadata.ID,
			emailAddress,
			metadata.EmailAddress,
		)
	}

	return nil
}

func verifyRequestShape(request *x509.CertificateRequest) error {
	if err := verifyRawCSRShape(request); err != nil {
		return err
	}
	if len(request.IPAddresses) != 0 {
		return fmt.Errorf("IP address SANs are not supported")
	}
	if len(request.EmailAddresses) != 0 {
		return fmt.Errorf("email SANs are not supported")
	}
	if len(request.URIs) != 0 {
		return fmt.Errorf("URI SANs are not supported")
	}
	subjectAltNameCount := 0
	for _, extension := range request.Extensions {
		if !objectIdentifierEqual(extension.Id, subjectAltNameOID) {
			return fmt.Errorf(
				"extension %s is not supported",
				extension.Id.String(),
			)
		}
		subjectAltNameCount++
		if subjectAltNameCount > 1 {
			return fmt.Errorf(
				"subject alternative name extension appears more than once",
			)
		}
	}
	subjectCounts := make(map[string]int)
	for _, name := range request.Subject.Names {
		if !allowedSubjectOID(name.Type) {
			return fmt.Errorf(
				"subject attribute %s is not supported",
				name.Type.String(),
			)
		}
		subjectCounts[name.Type.String()]++
		if subjectCounts[name.Type.String()] > 1 {
			return fmt.Errorf(
				"subject attribute %s appears more than once",
				name.Type.String(),
			)
		}
	}

	return nil
}

func verifyRawCSRShape(request *x509.CertificateRequest) error {
	var rawRequest rawTBSCertificateRequest
	rest, err := asn1.Unmarshal(
		request.RawTBSCertificateRequest,
		&rawRequest,
	)
	if err != nil {
		return fmt.Errorf(
			"decode raw certification request info: %w",
			err,
		)
	}
	if len(rest) != 0 {
		return fmt.Errorf(
			"raw certification request info contains trailing data",
		)
	}
	if rawRequest.Version != 0 {
		return fmt.Errorf(
			"certification request version is %d; expected 0",
			rawRequest.Version,
		)
	}

	extensionRequestCount := 0
	var rawDNSNames []string
	for _, rawAttribute := range rawRequest.RawAttributes {
		var attribute rawPKCS10Attribute
		rest, err := asn1.Unmarshal(
			rawAttribute.FullBytes,
			&attribute,
		)
		if err != nil {
			return fmt.Errorf("decode raw PKCS#10 attribute: %w", err)
		}
		if len(rest) != 0 {
			return fmt.Errorf(
				"raw PKCS#10 attribute contains trailing data",
			)
		}
		if !objectIdentifierEqual(
			attribute.ID,
			extensionRequestOID,
		) {
			return fmt.Errorf(
				"attribute %s is not supported",
				attribute.ID.String(),
			)
		}
		extensionRequestCount++
		if extensionRequestCount > 1 {
			return fmt.Errorf(
				"extension request attribute appears more than once",
			)
		}
		if len(attribute.Values) != 1 {
			return fmt.Errorf(
				"extension request attribute must contain exactly one value",
			)
		}

		var extensions []pkix.Extension
		rest, err = asn1.Unmarshal(
			attribute.Values[0].FullBytes,
			&extensions,
		)
		if err != nil {
			return fmt.Errorf(
				"decode requested PKCS#10 extensions: %w",
				err,
			)
		}
		if len(rest) != 0 {
			return fmt.Errorf(
				"requested PKCS#10 extensions contain trailing data",
			)
		}

		subjectAltNameCount := 0
		for _, extension := range extensions {
			if !objectIdentifierEqual(
				extension.Id,
				subjectAltNameOID,
			) {
				return fmt.Errorf(
					"extension %s is not supported",
					extension.Id.String(),
				)
			}
			subjectAltNameCount++
			if subjectAltNameCount > 1 {
				return fmt.Errorf(
					"subject alternative name extension appears more than once",
				)
			}
			names, err := strictDNSNamesFromSAN(extension.Value)
			if err != nil {
				return err
			}
			rawDNSNames = append(rawDNSNames, names...)
		}
	}
	if !equalStrings(rawDNSNames, request.DNSNames) {
		return fmt.Errorf(
			"raw DNS SANs %v do not match parsed DNS SANs %v",
			rawDNSNames,
			request.DNSNames,
		)
	}

	return nil
}

func strictDNSNamesFromSAN(value []byte) ([]string, error) {
	var sequence asn1.RawValue
	rest, err := asn1.Unmarshal(value, &sequence)
	if err != nil {
		return nil, fmt.Errorf(
			"decode subject alternative name extension: %w",
			err,
		)
	}
	if len(rest) != 0 ||
		sequence.Class != asn1.ClassUniversal ||
		sequence.Tag != asn1.TagSequence ||
		!sequence.IsCompound {
		return nil, fmt.Errorf(
			"subject alternative name extension is not one DER sequence",
		)
	}

	var names []string
	remaining := sequence.Bytes
	for len(remaining) != 0 {
		var name asn1.RawValue
		remaining, err = asn1.Unmarshal(remaining, &name)
		if err != nil {
			return nil, fmt.Errorf(
				"decode subject alternative name: %w",
				err,
			)
		}
		if name.Class != asn1.ClassContextSpecific ||
			name.Tag != 2 ||
			name.IsCompound {
			return nil, fmt.Errorf(
				"subject alternative name class %d tag %d is not a DNS name",
				name.Class,
				name.Tag,
			)
		}
		if len(name.Bytes) == 0 {
			return nil, fmt.Errorf(
				"subject alternative name contains an empty DNS name",
			)
		}
		for _, character := range name.Bytes {
			if character > unicode.MaxASCII {
				return nil, fmt.Errorf(
					"subject alternative name DNS value is not IA5",
				)
			}
		}
		names = append(names, string(name.Bytes))
	}

	return names, nil
}

func allowedSubjectOID(value []int) bool {
	for _, allowed := range allowedSubjectOIDs {
		if objectIdentifierEqual(value, allowed) {
			return true
		}
	}

	return false
}

func requestDomains(
	request *x509.CertificateRequest,
) ([]string, error) {
	domains := make([]string, 0, len(request.DNSNames))
	seenSANs := make(map[string]struct{}, len(request.DNSNames))
	for _, domain := range request.DNSNames {
		if domain == "" {
			return nil, fmt.Errorf("DNS SAN contains an empty name")
		}
		if _, exists := seenSANs[domain]; exists {
			return nil, fmt.Errorf(
				"DNS SAN contains duplicate %q",
				domain,
			)
		}
		seenSANs[domain] = struct{}{}
		domains = append(domains, domain)
	}
	if _, exists := seenSANs[request.Subject.CommonName]; !exists {
		return nil, fmt.Errorf(
			"common name %q is not present in the DNS SAN extension",
			request.Subject.CommonName,
		)
	}
	sort.Strings(domains)

	return domains, nil
}

func requestEmailAddress(
	request *x509.CertificateRequest,
) (string, error) {
	var emailAddress string
	for _, name := range request.Subject.Names {
		if !objectIdentifierEqual(name.Type, emailAddressOID) {
			continue
		}
		value, ok := name.Value.(string)
		if !ok {
			return "", fmt.Errorf("emailAddress value is not a string")
		}
		if emailAddress != "" {
			return "", fmt.Errorf("multiple emailAddress values are present")
		}
		emailAddress = value
	}

	return emailAddress, nil
}

func objectIdentifierEqual(first []int, second []int) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}

	return true
}

func singleValue(name string, values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if len(values) != 1 {
		return "", fmt.Errorf(
			"SSL CSR PEM %s appears %d times",
			name,
			len(values),
		)
	}

	return values[0], nil
}

func verifyCSRUsesPublicKey(
	csr CSR,
	key publicKeyMetadata,
) error {
	parsed, err := ParsePEM(csr.CSRPEM)
	if err != nil {
		return err
	}
	if err := verifyCSRPublicKeyMetadata(csr, parsed.Request); err != nil {
		return err
	}
	if csr.KeyAlgorithm != key.KeyAlgorithm {
		return fmt.Errorf(
			"CSR key algorithm is %q; selected key %q uses %q",
			csr.KeyAlgorithm,
			key.ID,
			key.KeyAlgorithm,
		)
	}

	switch key.KeyAlgorithm {
	case "rsaEncryption":
		publicKey, ok := parsed.Request.PublicKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf(
				"CSR public key is %T; expected RSA",
				parsed.Request.PublicKey,
			)
		}
		keyModulus, err := parseHexInteger(
			key.Modulus,
			"selected RSA key modulus",
		)
		if err != nil {
			return err
		}
		csrModulus, err := parseHexInteger(
			csr.Modulus,
			"CSR inventory RSA modulus",
		)
		if err != nil {
			return err
		}
		if keyModulus.Cmp(csrModulus) != 0 ||
			keyModulus.Cmp(publicKey.N) != 0 {
			return fmt.Errorf(
				"CSR public modulus does not match selected key %q",
				key.ID,
			)
		}
	case "id-ecPublicKey":
		publicKey, ok := parsed.Request.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf(
				"CSR public key is %T; expected ECDSA",
				parsed.Request.PublicKey,
			)
		}
		curve, err := curveByCPanelName(key.ECDSACurveName)
		if err != nil {
			return err
		}
		if publicKey.Curve.Params().Name != curve.Params().Name ||
			csr.ECDSACurveName != key.ECDSACurveName {
			return fmt.Errorf(
				"CSR ECDSA curve does not match selected key %q",
				key.ID,
			)
		}
		keyPublic, err := parseECDSAPublic(
			curve,
			key.ECDSAPublic,
			"selected ECDSA key",
		)
		if err != nil {
			return err
		}
		csrPublic, err := parseECDSAPublic(
			curve,
			csr.ECDSAPublic,
			"CSR inventory ECDSA key",
		)
		if err != nil {
			return err
		}
		if !keyPublic.Equal(csrPublic) ||
			!keyPublic.Equal(publicKey) {
			return fmt.Errorf(
				"CSR ECDSA public point does not match selected key %q",
				key.ID,
			)
		}
	default:
		return fmt.Errorf(
			"selected key %q uses unsupported algorithm %q",
			key.ID,
			key.KeyAlgorithm,
		)
	}

	return nil
}

func verifyCSRPublicKeyMetadata(
	csr CSR,
	request *x509.CertificateRequest,
) error {
	switch csr.KeyAlgorithm {
	case "rsaEncryption":
		if csr.ECDSACurveName != "" || csr.ECDSAPublic != "" {
			return fmt.Errorf(
				"SSL CSR %q mixes RSA and ECDSA metadata",
				csr.ID,
			)
		}
		publicKey, ok := request.PublicKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf(
				"SSL CSR %q public key is %T; inventory reports RSA",
				csr.ID,
				request.PublicKey,
			)
		}
		modulus, err := parseHexInteger(
			csr.Modulus,
			fmt.Sprintf("SSL CSR %q RSA modulus", csr.ID),
		)
		if err != nil {
			return err
		}
		if modulus.Cmp(publicKey.N) != 0 {
			return fmt.Errorf(
				"SSL CSR %q RSA modulus does not match its PKCS#10 public key",
				csr.ID,
			)
		}
	case "id-ecPublicKey":
		if csr.Modulus != "" {
			return fmt.Errorf(
				"SSL CSR %q mixes ECDSA and RSA metadata",
				csr.ID,
			)
		}
		publicKey, ok := request.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf(
				"SSL CSR %q public key is %T; inventory reports ECDSA",
				csr.ID,
				request.PublicKey,
			)
		}
		curve, err := curveByCPanelName(csr.ECDSACurveName)
		if err != nil {
			return fmt.Errorf("SSL CSR %q: %w", csr.ID, err)
		}
		if publicKey.Curve.Params().Name != curve.Params().Name {
			return fmt.Errorf(
				"SSL CSR %q ECDSA curve does not match its PKCS#10 public key",
				csr.ID,
			)
		}
		metadataPublicKey, err := parseECDSAPublic(
			curve,
			csr.ECDSAPublic,
			fmt.Sprintf("SSL CSR %q ECDSA", csr.ID),
		)
		if err != nil {
			return err
		}
		if !metadataPublicKey.Equal(publicKey) {
			return fmt.Errorf(
				"SSL CSR %q ECDSA point does not match its PKCS#10 public key",
				csr.ID,
			)
		}
	default:
		return fmt.Errorf(
			"SSL CSR %q uses unsupported key algorithm %q",
			csr.ID,
			csr.KeyAlgorithm,
		)
	}

	return nil
}

func parseHexInteger(value, name string) (*big.Int, error) {
	if value == "" {
		return nil, fmt.Errorf("%s is empty", name)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s is not hexadecimal: %w", name, err)
	}
	number := new(big.Int).SetBytes(decoded)
	if number.Sign() <= 0 {
		return nil, fmt.Errorf("%s is not positive", name)
	}

	return number, nil
}

func curveByCPanelName(name string) (elliptic.Curve, error) {
	switch name {
	case "prime256v1", "secp256r1":
		return elliptic.P256(), nil
	case "secp384r1":
		return elliptic.P384(), nil
	case "secp521r1":
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf(
			"unsupported cPanel ECDSA curve %q",
			name,
		)
	}
}

func parseECDSAPublic(
	curve elliptic.Curve,
	value string,
	name string,
) (*ecdsa.PublicKey, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf(
			"%s point is not hexadecimal: %w",
			name,
			err,
		)
	}
	if len(decoded) > 0 && (decoded[0] == 2 || decoded[0] == 3) {
		decoded, err = decompressECDSAPublic(curve, decoded)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	publicKey, err := ecdsa.ParseUncompressedPublicKey(curve, decoded)
	if err != nil {
		return nil, fmt.Errorf(
			"%s point is not valid on %s",
			name,
			curve.Params().Name,
		)
	}

	return publicKey, nil
}

func decompressECDSAPublic(
	curve elliptic.Curve,
	compressed []byte,
) ([]byte, error) {
	byteLength := (curve.Params().BitSize + 7) / 8
	if len(compressed) != byteLength+1 ||
		(compressed[0] != 2 && compressed[0] != 3) {
		return nil, fmt.Errorf("invalid compressed SEC1 point")
	}

	x := new(big.Int).SetBytes(compressed[1:])
	if x.Cmp(curve.Params().P) >= 0 {
		return nil, fmt.Errorf("compressed SEC1 x-coordinate is out of range")
	}

	ySquared := new(big.Int).Mul(x, x)
	ySquared.Mul(ySquared, x)
	threeX := new(big.Int).Lsh(new(big.Int).Set(x), 1)
	threeX.Add(threeX, x)
	ySquared.Sub(ySquared, threeX)
	ySquared.Add(ySquared, curve.Params().B)
	ySquared.Mod(ySquared, curve.Params().P)
	y := new(big.Int).ModSqrt(ySquared, curve.Params().P)
	if y == nil {
		return nil, fmt.Errorf(
			"compressed SEC1 point is not on the curve",
		)
	}
	if y.Bit(0) != uint(compressed[0]&1) {
		y.Sub(curve.Params().P, y)
	}

	uncompressed := make([]byte, 1+2*byteLength)
	uncompressed[0] = 4
	x.FillBytes(uncompressed[1 : 1+byteLength])
	y.FillBytes(uncompressed[1+byteLength:])

	return uncompressed, nil
}

func equalStrings(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}

	return true
}
