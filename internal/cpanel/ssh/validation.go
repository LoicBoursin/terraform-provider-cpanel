package ssh

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	cryptossh "golang.org/x/crypto/ssh"
)

const maxPublicKeyLength = 64 << 10

var (
	namePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	reservedNames = map[string]struct{}{
		"authorized_keys": {},
		"config":          {},
		"environment":     {},
		"identity":        {},
		"known_hosts":     {},
		"rc":              {},
	}
	supportedKeyTypes = map[string]struct{}{
		cryptossh.KeyAlgoECDSA256: {},
		cryptossh.KeyAlgoECDSA384: {},
		cryptossh.KeyAlgoECDSA521: {},
		cryptossh.KeyAlgoED25519:  {},
		cryptossh.KeyAlgoRSA:      {},
	}
)

// ValidateName enforces a portable cPanel SSH-key base filename.
func ValidateName(value string) error {
	if !namePattern.MatchString(value) {
		return fmt.Errorf(
			"SSH public-key name must contain 1 to 128 ASCII letters, digits, dots, underscores, or hyphens and must start with a letter or digit",
		)
	}
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, ".pub") {
		return fmt.Errorf(
			"SSH public-key name %q must omit the .pub suffix",
			value,
		)
	}
	if _, reserved := reservedNames[lower]; reserved {
		return fmt.Errorf(
			"SSH public-key name %q is reserved by OpenSSH",
			value,
		)
	}

	return nil
}

// ParsePublicKey accepts exactly one modern OpenSSH public key and returns its
// comment-independent canonical identity.
func ParsePublicKey(value string) (*ParsedPublicKey, error) {
	if value == "" {
		return nil, fmt.Errorf("SSH public key must not be empty")
	}
	if len(value) > maxPublicKeyLength {
		return nil, fmt.Errorf(
			"SSH public key must not exceed %d bytes",
			maxPublicKeyLength,
		)
	}

	trimmed := strings.TrimSpace(value)
	if strings.Contains(strings.ToUpper(trimmed), "PRIVATE KEY") {
		return nil, fmt.Errorf("SSH private key material is not allowed")
	}

	key, _, options, rest, err := cryptossh.ParseAuthorizedKey(
		[]byte(trimmed),
	)
	if err != nil {
		return nil, fmt.Errorf("parse OpenSSH public key: %w", err)
	}
	if len(options) != 0 {
		return nil, fmt.Errorf(
			"SSH public key must not contain authorized_keys options",
		)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf(
			"SSH public key must contain exactly one key",
		)
	}
	if _, supported := supportedKeyTypes[key.Type()]; !supported {
		return nil, fmt.Errorf(
			"SSH public key type %q is not supported; use RSA, Ed25519, or ECDSA",
			key.Type(),
		)
	}
	if _, certificate := key.(*cryptossh.Certificate); certificate {
		return nil, fmt.Errorf("OpenSSH certificates are not supported")
	}
	if err := validateCryptoPublicKey(key); err != nil {
		return nil, err
	}

	canonical := strings.TrimSpace(
		string(cryptossh.MarshalAuthorizedKey(key)),
	)
	contentDigest := sha256.Sum256(key.Marshal())

	return &ParsedPublicKey{
		PublicKey:         canonical,
		FingerprintSHA256: cryptossh.FingerprintSHA256(key),
		ContentSHA256:     hex.EncodeToString(contentDigest[:]),
		KeyType:           key.Type(),
	}, nil
}

// EqualPublicKey compares only cryptographic public-key material.
func EqualPublicKey(left, right string) (bool, error) {
	leftParsed, err := ParsePublicKey(left)
	if err != nil {
		return false, fmt.Errorf("parse first SSH public key: %w", err)
	}
	rightParsed, err := ParsePublicKey(right)
	if err != nil {
		return false, fmt.Errorf("parse second SSH public key: %w", err)
	}

	return leftParsed.ContentSHA256 == rightParsed.ContentSHA256, nil
}

func validateCryptoPublicKey(key cryptossh.PublicKey) error {
	cryptoPublicKey, ok := key.(cryptossh.CryptoPublicKey)
	if !ok {
		return fmt.Errorf(
			"SSH public key type %q cannot be validated cryptographically",
			key.Type(),
		)
	}

	switch publicKey := cryptoPublicKey.CryptoPublicKey().(type) {
	case *rsa.PublicKey:
		if publicKey.N.BitLen() < 2048 {
			return fmt.Errorf(
				"SSH RSA public key has %d bits; at least 2048 bits are required",
				publicKey.N.BitLen(),
			)
		}
	case ed25519.PublicKey:
		if len(publicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("SSH Ed25519 public key has an invalid length")
		}
	case *ecdsa.PublicKey:
		switch publicKey.Curve.Params().Name {
		case "P-256", "P-384", "P-521":
		default:
			return fmt.Errorf(
				"SSH ECDSA public key uses unsupported curve %q",
				publicKey.Curve.Params().Name,
			)
		}
	default:
		return fmt.Errorf(
			"SSH public key type %q is not supported",
			key.Type(),
		)
	}

	return nil
}
