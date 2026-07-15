package sslcertificate

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxCertificateIDLength   = 4096
	maxFriendlyNameLength    = 1024
	maxCertificatePEMLength  = 1 << 20
	certificatePEMBlockStart = "-----BEGIN CERTIFICATE-----"
)

// ParsedCertificate contains a parsed X.509 certificate and its canonical
// public PEM representation.
type ParsedCertificate struct {
	Certificate       *x509.Certificate
	NormalizedPEM     string
	SHA256Fingerprint string
}

// ParsePEM accepts exactly one public CERTIFICATE PEM block.
func ParsePEM(value string) (*ParsedCertificate, error) {
	if value == "" {
		return nil, fmt.Errorf("certificate PEM must not be empty")
	}
	if len(value) > maxCertificatePEMLength {
		return nil, fmt.Errorf(
			"certificate PEM must not exceed %d bytes",
			maxCertificatePEMLength,
		)
	}

	trimmedValue := bytes.TrimSpace([]byte(value))
	if !bytes.HasPrefix(trimmedValue, []byte(certificatePEMBlockStart)) {
		return nil, fmt.Errorf(
			"certificate PEM must contain exactly one CERTIFICATE block",
		)
	}

	block, rest := pem.Decode(trimmedValue)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf(
			"certificate PEM must contain exactly one CERTIFICATE block",
		)
	}
	if len(block.Headers) != 0 {
		return nil, fmt.Errorf(
			"certificate PEM block must not contain headers",
		)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf(
			"certificate PEM must not contain additional PEM blocks or data",
		)
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse X.509 certificate: %w", err)
	}

	normalized := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate.Raw,
	})
	fingerprint := sha256.Sum256(certificate.Raw)

	return &ParsedCertificate{
		Certificate:       certificate,
		NormalizedPEM:     strings.TrimSpace(string(normalized)),
		SHA256Fingerprint: hex.EncodeToString(fingerprint[:]),
	}, nil
}

// EqualPEM reports whether two PEM values contain the same X.509 certificate.
func EqualPEM(first, second string) (bool, error) {
	firstCertificate, err := ParsePEM(first)
	if err != nil {
		return false, fmt.Errorf("parse first certificate PEM: %w", err)
	}
	secondCertificate, err := ParsePEM(second)
	if err != nil {
		return false, fmt.Errorf("parse second certificate PEM: %w", err)
	}

	return bytes.Equal(
		firstCertificate.Certificate.Raw,
		secondCertificate.Certificate.Raw,
	), nil
}

// ValidateID checks a cPanel certificate inventory identifier.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("SSL certificate id must not be empty")
	}
	if len(id) > maxCertificateIDLength {
		return fmt.Errorf(
			"SSL certificate id must not exceed %d bytes",
			maxCertificateIDLength,
		)
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf(
			"SSL certificate id must not contain surrounding whitespace",
		)
	}
	for _, character := range id {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf(
				"SSL certificate id must not contain whitespace or control characters",
			)
		}
	}

	return nil
}

// ValidateFriendlyName checks a user-supplied cPanel certificate label.
func ValidateFriendlyName(friendlyName string) error {
	if len(friendlyName) > maxFriendlyNameLength {
		return fmt.Errorf(
			"SSL certificate friendly name must not exceed %d bytes",
			maxFriendlyNameLength,
		)
	}
	if strings.TrimSpace(friendlyName) != friendlyName {
		return fmt.Errorf(
			"SSL certificate friendly name must not contain surrounding whitespace",
		)
	}
	for _, character := range friendlyName {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"SSL certificate friendly name must not contain control characters",
			)
		}
	}

	return nil
}
