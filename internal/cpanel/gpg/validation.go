package gpg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

const (
	maxArmoredPublicKeyLength = 1 << 20
	publicKeyArmorType        = "PGP PUBLIC KEY BLOCK"
	publicKeyArmorBegin       = "-----BEGIN PGP PUBLIC KEY BLOCK-----"
	publicKeyArmorEnd         = "-----END PGP PUBLIC KEY BLOCK-----"

	openPGPPacketSignature     = 2
	openPGPPacketPrivateKey    = 5
	openPGPPacketPublicKey     = 6
	openPGPPacketPrivateSubkey = 7
	openPGPPacketUserID        = 13
	openPGPPacketPublicSubkey  = 14
	openPGPPacketUserAttribute = 17
)

// ParsePublicKey accepts exactly one public-only RSA v4 OpenPGP entity.
func ParsePublicKey(value string) (*ParsedPublicKey, error) {
	if value == "" {
		return nil, fmt.Errorf("GPG public key must not be empty")
	}
	if len(value) > maxArmoredPublicKeyLength {
		return nil, fmt.Errorf(
			"GPG public key must not exceed %d bytes",
			maxArmoredPublicKeyLength,
		)
	}

	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, publicKeyArmorBegin) ||
		!strings.HasSuffix(trimmed, publicKeyArmorEnd) ||
		strings.Count(trimmed, publicKeyArmorBegin) != 1 ||
		strings.Count(trimmed, publicKeyArmorEnd) != 1 {
		return nil, fmt.Errorf(
			"GPG public key must contain exactly one PUBLIC KEY BLOCK",
		)
	}
	if strings.Contains(trimmed, "PGP PRIVATE KEY BLOCK") {
		return nil, fmt.Errorf("GPG private key material is not allowed")
	}

	block, err := armor.Decode(strings.NewReader(trimmed))
	if err != nil {
		return nil, fmt.Errorf("decode GPG public key armor: %w", err)
	}
	if block.Type != publicKeyArmorType {
		return nil, fmt.Errorf(
			"GPG armor type is %q; expected %q",
			block.Type,
			publicKeyArmorType,
		)
	}

	packetData, err := io.ReadAll(
		io.LimitReader(block.Body, maxArmoredPublicKeyLength+1),
	)
	if err != nil {
		return nil, fmt.Errorf("read GPG public key packets: %w", err)
	}
	if len(packetData) > maxArmoredPublicKeyLength {
		return nil, fmt.Errorf("GPG public key packet data is too large")
	}
	canonicalSHA256, err := canonicalPublicPacketSHA256(packetData)
	if err != nil {
		return nil, err
	}

	entities, err := openpgp.ReadKeyRing(bytes.NewReader(packetData))
	if err != nil {
		return nil, fmt.Errorf("parse GPG public key: %w", err)
	}
	if len(entities) != 1 {
		return nil, fmt.Errorf(
			"GPG public key must contain exactly one entity; found %d",
			len(entities),
		)
	}

	entity := entities[0]
	if entity.PrimaryKey == nil {
		return nil, fmt.Errorf("GPG public key has no primary key")
	}
	if entity.PrivateKey != nil {
		return nil, fmt.Errorf("GPG private key material is not allowed")
	}
	for index, subkey := range entity.Subkeys {
		if subkey.PrivateKey != nil {
			return nil, fmt.Errorf(
				"GPG subkey %d contains private key material",
				index,
			)
		}
		if err := validateRSAPublicKey(
			subkey.PublicKey,
			fmt.Sprintf("subkey %d", index),
		); err != nil {
			return nil, err
		}
	}
	if len(entity.Identities) == 0 {
		return nil, fmt.Errorf(
			"GPG public key must contain at least one user identity",
		)
	}
	if err := validateRSAPublicKey(
		entity.PrimaryKey,
		"primary key",
	); err != nil {
		return nil, err
	}

	fingerprint := strings.ToUpper(
		hex.EncodeToString(entity.PrimaryKey.Fingerprint),
	)
	if len(fingerprint) != 40 {
		return nil, fmt.Errorf(
			"GPG v4 primary fingerprint must contain 40 hexadecimal characters",
		)
	}
	id := strings.ToUpper(entity.PrimaryKey.KeyIdString())
	if err := ValidatePublicID(id); err != nil {
		return nil, fmt.Errorf("invalid GPG primary key id: %w", err)
	}
	if !strings.HasSuffix(fingerprint, id) {
		return nil, fmt.Errorf(
			"GPG key id %q does not match primary fingerprint %q",
			id,
			fingerprint,
		)
	}
	bits, err := entity.PrimaryKey.BitLength()
	if err != nil {
		return nil, fmt.Errorf("read GPG primary key length: %w", err)
	}

	return &ParsedPublicKey{
		Armored:       trimmed,
		ID:            id,
		Fingerprint:   fingerprint,
		ContentSHA256: canonicalSHA256,
		Bits:          int64(bits),
	}, nil
}

func validateRSAPublicKey(key *packet.PublicKey, label string) error {
	if key == nil {
		return fmt.Errorf("GPG %s is missing", label)
	}
	if key.Version != 4 {
		return fmt.Errorf(
			"GPG %s uses version %d; only version 4 is supported",
			label,
			key.Version,
		)
	}
	switch key.PubKeyAlgo {
	case packet.PubKeyAlgoRSA,
		packet.PubKeyAlgoRSAEncryptOnly,
		packet.PubKeyAlgoRSASignOnly:
	default:
		return fmt.Errorf(
			"GPG %s uses unsupported public-key algorithm %d; only RSA is supported",
			label,
			key.PubKeyAlgo,
		)
	}
	bits, err := key.BitLength()
	if err != nil {
		return fmt.Errorf("read GPG %s length: %w", label, err)
	}
	switch bits {
	case 2048, 3072, 4096:
		return nil
	default:
		return fmt.Errorf(
			"GPG %s has %d bits; supported RSA lengths are 2048, 3072, and 4096",
			label,
			bits,
		)
	}
}

func canonicalPublicPacketSHA256(packetData []byte) (string, error) {
	reader := packet.NewOpaqueReader(bytes.NewReader(packetData))
	var opaquePackets []*packet.OpaquePacket
	primaryKeyCount := 0
	for {
		opaque, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse GPG packet framing: %w", err)
		}
		switch opaque.Tag {
		case openPGPPacketPrivateKey, openPGPPacketPrivateSubkey:
			return "", fmt.Errorf("GPG private key material is not allowed")
		case openPGPPacketPublicKey:
			primaryKeyCount++
		case openPGPPacketSignature,
			openPGPPacketUserID,
			openPGPPacketPublicSubkey,
			openPGPPacketUserAttribute:
		default:
			return "", fmt.Errorf(
				"GPG public key contains unsupported packet tag %d",
				opaque.Tag,
			)
		}
		opaquePackets = append(opaquePackets, opaque)
	}
	if primaryKeyCount != 1 {
		return "", fmt.Errorf(
			"GPG public key must contain exactly one primary key packet; found %d",
			primaryKeyCount,
		)
	}

	var packets [][]byte
	for index, opaque := range opaquePackets {
		value, err := opaque.Parse()
		if err != nil {
			return "", fmt.Errorf(
				"parse GPG public packet %d with tag %d: %w",
				index,
				opaque.Tag,
				err,
			)
		}
		serializer, ok := value.(interface {
			Serialize(io.Writer) error
		})
		if !ok {
			return "", fmt.Errorf(
				"GPG public packet %T cannot be canonicalized",
				value,
			)
		}
		var serialized bytes.Buffer
		if err := serializer.Serialize(&serialized); err != nil {
			return "", fmt.Errorf(
				"canonicalize GPG public packet %T: %w",
				value,
				err,
			)
		}
		packets = append(
			packets,
			bytes.Clone(serialized.Bytes()),
		)
	}
	sort.Slice(packets, func(left, right int) bool {
		return bytes.Compare(packets[left], packets[right]) < 0
	})

	hash := sha256.New()
	var length [8]byte
	for _, serialized := range packets {
		binary.BigEndian.PutUint64(length[:], uint64(len(serialized)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(serialized)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// EqualPublicKey reports equality for the canonical set of decoded public packets.
func EqualPublicKey(first, second string) (bool, error) {
	firstKey, err := ParsePublicKey(first)
	if err != nil {
		return false, fmt.Errorf("parse first GPG public key: %w", err)
	}
	secondKey, err := ParsePublicKey(second)
	if err != nil {
		return false, fmt.Errorf("parse second GPG public key: %w", err)
	}

	return firstKey.ContentSHA256 == secondKey.ContentSHA256, nil
}

// ValidatePublicID checks a full 64-bit cPanel public-key identifier.
func ValidatePublicID(id string) error {
	return validateHexID(id, 16, "GPG public key id")
}

func validateSecretID(id string) error {
	if len(id) != 8 && len(id) != 16 {
		return fmt.Errorf(
			"GPG secret key id must contain 8 or 16 hexadecimal characters",
		)
	}

	return validateHexID(id, len(id), "GPG secret key id")
}

func validateHexID(id string, length int, label string) error {
	if len(id) != length {
		return fmt.Errorf(
			"%s must contain exactly %d hexadecimal characters",
			label,
			length,
		)
	}
	if strings.ToUpper(id) != id {
		return fmt.Errorf("%s must use uppercase hexadecimal characters", label)
	}
	if _, err := hex.DecodeString(id); err != nil {
		return fmt.Errorf("%s must contain only hexadecimal characters", label)
	}

	return nil
}

func validateFingerprint(fingerprint string) error {
	return validateHexID(
		fingerprint,
		40,
		"GPG public key fingerprint",
	)
}

func validateContentSHA256(contentSHA256 string) error {
	if len(contentSHA256) != sha256.Size*2 {
		return fmt.Errorf(
			"GPG public key content SHA-256 must contain 64 hexadecimal characters",
		)
	}
	if strings.ToLower(contentSHA256) != contentSHA256 {
		return fmt.Errorf(
			"GPG public key content SHA-256 must use lowercase hexadecimal characters",
		)
	}
	if _, err := hex.DecodeString(contentSHA256); err != nil {
		return fmt.Errorf(
			"GPG public key content SHA-256 must contain only hexadecimal characters",
		)
	}

	return nil
}

func secretMatchesPublicID(secretID, publicID string) bool {
	secretID = strings.ToUpper(secretID)
	publicID = strings.ToUpper(publicID)

	return (len(secretID) == 8 || len(secretID) == 16) &&
		strings.HasSuffix(publicID, secretID)
}
