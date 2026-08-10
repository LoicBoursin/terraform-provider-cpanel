package gpg

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

var (
	testKeyOnce    sync.Once
	testKeyArmored string
	testKeyParsed  *ParsedPublicKey
	testKeyEntity  *openpgp.Entity
	testKeyErr     error
)

func TestParsePublicKey(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	actual, err := ParsePublicKey("\n" + armored + "\n")
	if err != nil {
		t.Fatalf("ParsePublicKey() error: %v", err)
	}
	if actual.Armored != strings.TrimSpace(armored) {
		t.Fatalf("Armored was not normalized")
	}
	if actual.ID != parsed.ID {
		t.Fatalf("ID = %q, want %q", actual.ID, parsed.ID)
	}
	if actual.Fingerprint != parsed.Fingerprint {
		t.Fatalf(
			"Fingerprint = %q, want %q",
			actual.Fingerprint,
			parsed.Fingerprint,
		)
	}
	if actual.ContentSHA256 == "" ||
		actual.ContentSHA256 != parsed.ContentSHA256 {
		t.Fatalf(
			"ContentSHA256 = %q, want %q",
			actual.ContentSHA256,
			parsed.ContentSHA256,
		)
	}
	if actual.Bits != 2048 {
		t.Fatalf("Bits = %d, want 2048", actual.Bits)
	}
}

func TestParsePublicKeyAcceptsArmorHeaders(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	withHeaders := testPublicKeyWithHeaders(t)
	actual, err := ParsePublicKey(withHeaders)
	if err != nil {
		t.Fatalf("ParsePublicKey() error: %v", err)
	}
	if actual.ContentSHA256 != parsed.ContentSHA256 {
		t.Fatalf(
			"header packet SHA-256 = %q, want %q",
			actual.ContentSHA256,
			parsed.ContentSHA256,
		)
	}
	equal, err := EqualPublicKey(armored, withHeaders)
	if err != nil {
		t.Fatalf("EqualPublicKey() error: %v", err)
	}
	if !equal {
		t.Fatal("armor headers changed public packet identity")
	}
}

func TestParsePublicKeyRejectsUnsafeInput(t *testing.T) {
	t.Parallel()

	armored, _ := testPublicKey(t)
	secondArmored := generateTestPublicKey(t, 2048, false)
	privateArmored := testPrivateKeyArmored(t)
	weakArmored := generateTestPublicKey(t, 1024, false)
	v6Armored := generateTestPublicKey(t, 2048, true)
	multipleEntities := mergePublicKeyArmors(t, armored, secondArmored)
	malformedSecondPrimary := appendOpaquePacket(
		t,
		armored,
		&packet.OpaquePacket{
			Tag:      openPGPPacketPublicKey,
			Contents: []byte{4},
		},
	)
	unknownPacket := appendOpaquePacket(
		t,
		armored,
		&packet.OpaquePacket{
			Tag:      60,
			Contents: []byte("terraform-cpanel-unknown-packet"),
		},
	)

	testCases := map[string]string{
		"empty":                    "",
		"plain text":               "not a public key",
		"multiple armor blocks":    armored + "\n" + armored,
		"multiple packet entities": multipleEntities,
		"malformed second primary": malformedSecondPrimary,
		"unknown packet":           unknownPacket,
		"private packet":           privateArmored,
		"weak RSA":                 weakArmored,
		"version 6":                v6Armored,
		"trailing data":            armored + "\ntrailing",
	}
	for name, value := range testCases {
		name := name
		value := value
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParsePublicKey(value); err == nil {
				t.Fatalf("ParsePublicKey() accepted %s", name)
			}
		})
	}
}

func TestEqualPublicKeyUsesCompletePacketStream(t *testing.T) {
	t.Parallel()

	armored, _ := testPublicKey(t)
	equal, err := EqualPublicKey(armored, "\n"+armored+"\n")
	if err != nil {
		t.Fatalf("EqualPublicKey() error: %v", err)
	}
	if !equal {
		t.Fatal("EqualPublicKey() = false for the same key")
	}

	other := generateTestPublicKey(t, 2048, false)
	equal, err = EqualPublicKey(armored, other)
	if err != nil {
		t.Fatalf("EqualPublicKey() second error: %v", err)
	}
	if equal {
		t.Fatal("EqualPublicKey() = true for different keys")
	}

	config := testPacketConfig(2048, false)
	entity, err := openpgp.NewEntity(
		"Terraform cPanel identity test",
		"",
		"identity@example.invalid",
		config,
	)
	if err != nil {
		t.Fatalf("openpgp.NewEntity(): %v", err)
	}
	before, err := serializePublicEntity(entity)
	if err != nil {
		t.Fatalf("serialize initial public entity: %v", err)
	}
	if err := entity.AddUserId(
		"Terraform cPanel second identity",
		"",
		"second@example.invalid",
		config,
	); err != nil {
		t.Fatalf("AddUserId(): %v", err)
	}
	after, err := serializePublicEntity(entity)
	if err != nil {
		t.Fatalf("serialize changed public entity: %v", err)
	}
	beforeParsed, err := ParsePublicKey(before)
	if err != nil {
		t.Fatalf("parse initial entity: %v", err)
	}
	afterParsed, err := ParsePublicKey(after)
	if err != nil {
		t.Fatalf("parse changed entity: %v", err)
	}
	if beforeParsed.Fingerprint != afterParsed.Fingerprint {
		t.Fatal("adding an identity unexpectedly changed the primary fingerprint")
	}
	equal, err = EqualPublicKey(before, after)
	if err != nil {
		t.Fatalf("EqualPublicKey() identity drift error: %v", err)
	}
	if equal {
		t.Fatal("EqualPublicKey() ignored an added public identity")
	}
}

func TestValidatePublicID(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		value   string
		wantErr bool
	}{
		"valid":     {value: "0123456789ABCDEF"},
		"lowercase": {value: "0123456789abcdef", wantErr: true},
		"short":     {value: "89ABCDEF", wantErr: true},
		"non-hex":   {value: "0123456789ABCDEG", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePublicID(testCase.value)
			if testCase.wantErr && err == nil {
				t.Fatalf("ValidatePublicID(%q) returned no error", testCase.value)
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf(
					"ValidatePublicID(%q) error: %v",
					testCase.value,
					err,
				)
			}
		})
	}
}

func TestSecretMatchesPublicID(t *testing.T) {
	t.Parallel()

	if !secretMatchesPublicID("89ABCDEF", "0123456789ABCDEF") {
		t.Fatal("short secret ID did not match public ID suffix")
	}
	if !secretMatchesPublicID(
		"0123456789ABCDEF",
		"0123456789ABCDEF",
	) {
		t.Fatal("full secret ID did not match public ID")
	}
	if secretMatchesPublicID("01234567", "0123456789ABCDEF") {
		t.Fatal("unrelated secret ID matched public ID")
	}
}

func testPublicKey(t *testing.T) (string, *ParsedPublicKey) {
	t.Helper()

	testKeyOnce.Do(func() {
		config := testPacketConfig(2048, false)
		testKeyEntity, testKeyErr = openpgp.NewEntity(
			"Terraform cPanel test",
			"",
			"terraform-cpanel-test@example.invalid",
			config,
		)
		if testKeyErr != nil {
			return
		}
		testKeyArmored, testKeyErr = serializePublicEntity(testKeyEntity)
		if testKeyErr != nil {
			return
		}
		testKeyParsed, testKeyErr = ParsePublicKey(testKeyArmored)
	})
	if testKeyErr != nil {
		t.Fatalf("generate test GPG public key: %v", testKeyErr)
	}

	return testKeyArmored, testKeyParsed
}

func generateTestPublicKey(
	t *testing.T,
	bits int,
	v6 bool,
) string {
	t.Helper()

	entity, err := openpgp.NewEntity(
		"Terraform cPanel generated test",
		"",
		"terraform-cpanel-generated@example.invalid",
		testPacketConfig(bits, v6),
	)
	if err != nil {
		t.Fatalf("openpgp.NewEntity(): %v", err)
	}
	armored, err := serializePublicEntity(entity)
	if err != nil {
		t.Fatalf("serialize public entity: %v", err)
	}

	return armored
}

func testPrivateKeyArmored(t *testing.T) string {
	t.Helper()

	testPublicKey(t)
	var output bytes.Buffer
	writer, err := armor.Encode(&output, publicKeyArmorType, nil)
	if err != nil {
		t.Fatalf("armor.Encode(): %v", err)
	}
	if err := testKeyEntity.SerializePrivate(
		writer,
		testPacketConfig(2048, false),
	); err != nil {
		t.Fatalf("SerializePrivate(): %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close private armor: %v", err)
	}

	return output.String()
}

func testPublicKeyWithHeaders(t *testing.T) string {
	t.Helper()

	armored, _ := testPublicKey(t)
	block, err := armor.Decode(strings.NewReader(armored))
	if err != nil {
		t.Fatalf("decode public key armor: %v", err)
	}
	var output bytes.Buffer
	writer, err := armor.Encode(
		&output,
		publicKeyArmorType,
		map[string]string{"Version": "test"},
	)
	if err != nil {
		t.Fatalf("create public key armor with headers: %v", err)
	}
	if _, err := io.Copy(writer, block.Body); err != nil {
		t.Fatalf("copy public key packet stream: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close public key armor with headers: %v", err)
	}

	return output.String()
}

func mergePublicKeyArmors(
	t *testing.T,
	first string,
	second string,
) string {
	t.Helper()

	firstPackets := decodePublicKeyPackets(t, first)
	secondPackets := decodePublicKeyPackets(t, second)

	return armorPublicKeyPackets(
		t,
		append(firstPackets, secondPackets...),
	)
}

func appendOpaquePacket(
	t *testing.T,
	armored string,
	opaque *packet.OpaquePacket,
) string {
	t.Helper()

	packetData := bytes.NewBuffer(decodePublicKeyPackets(t, armored))
	if err := opaque.Serialize(packetData); err != nil {
		t.Fatalf("serialize opaque GPG packet: %v", err)
	}

	return armorPublicKeyPackets(t, packetData.Bytes())
}

func decodePublicKeyPackets(t *testing.T, armored string) []byte {
	t.Helper()

	block, err := armor.Decode(strings.NewReader(armored))
	if err != nil {
		t.Fatalf("decode GPG public key armor: %v", err)
	}
	packetData, err := io.ReadAll(block.Body)
	if err != nil {
		t.Fatalf("read GPG public key packets: %v", err)
	}

	return packetData
}

func armorPublicKeyPackets(t *testing.T, packetData []byte) string {
	t.Helper()

	var output bytes.Buffer
	writer, err := armor.Encode(&output, publicKeyArmorType, nil)
	if err != nil {
		t.Fatalf("create GPG public key armor: %v", err)
	}
	if _, err := writer.Write(packetData); err != nil {
		t.Fatalf("write GPG public key packets: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close GPG public key armor: %v", err)
	}

	return output.String()
}

func serializePublicEntity(entity *openpgp.Entity) (string, error) {
	var output bytes.Buffer
	writer, err := armor.Encode(&output, publicKeyArmorType, nil)
	if err != nil {
		return "", err
	}
	if err := entity.Serialize(writer); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	return output.String(), nil
}

func testPacketConfig(bits int, v6 bool) *packet.Config {
	return &packet.Config{
		RSABits:    bits,
		MinRSABits: 1024,
		V6Keys:     v6,
		Time: func() time.Time {
			return time.Unix(1_700_000_000, 0)
		},
	}
}
