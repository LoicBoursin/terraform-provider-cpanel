package ssh

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	cryptossh "golang.org/x/crypto/ssh"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"deploy",
		"deploy-2026",
		"team_key.v2",
	} {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{
		"",
		".hidden",
		"deploy.pub",
		"authorized_keys",
		"CONFIG",
		"with space",
		"../escape",
		strings.Repeat("a", 129),
	} {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) returned no error", name)
		}
	}
}

func TestParsePublicKeyCanonicalizesComments(t *testing.T) {
	t.Parallel()

	first := testPublicKey(t, "first comment")
	second := strings.Fields(first)[0] + " " +
		strings.Fields(first)[1] + " second comment"

	parsed, err := ParsePublicKey(first)
	if err != nil {
		t.Fatalf("ParsePublicKey() error = %v", err)
	}
	if strings.Contains(parsed.PublicKey, "comment") {
		t.Fatalf("canonical public key contains comment: %q", parsed.PublicKey)
	}
	if parsed.KeyType != cryptossh.KeyAlgoED25519 {
		t.Fatalf("KeyType = %q", parsed.KeyType)
	}
	if !strings.HasPrefix(parsed.FingerprintSHA256, "SHA256:") {
		t.Fatalf("FingerprintSHA256 = %q", parsed.FingerprintSHA256)
	}
	if len(parsed.ContentSHA256) != 64 {
		t.Fatalf("ContentSHA256 length = %d", len(parsed.ContentSHA256))
	}

	equal, err := EqualPublicKey(first, second)
	if err != nil {
		t.Fatalf("EqualPublicKey() error = %v", err)
	}
	if !equal {
		t.Fatal("EqualPublicKey() = false for comment-only change")
	}
}

func TestParsePublicKeySupportsModernAlgorithms(t *testing.T) {
	t.Parallel()

	rsaPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	ecdsaPrivate, err := ecdsa.GenerateKey(
		elliptic.P256(),
		rand.Reader,
	)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	tests := map[string]any{
		"rsa":   &rsaPrivate.PublicKey,
		"ecdsa": &ecdsaPrivate.PublicKey,
	}
	for name, publicKey := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			key, err := cryptossh.NewPublicKey(publicKey)
			if err != nil {
				t.Fatalf("convert public key: %v", err)
			}
			if _, err := ParsePublicKey(
				string(cryptossh.MarshalAuthorizedKey(key)),
			); err != nil {
				t.Fatalf("ParsePublicKey() error = %v", err)
			}
		})
	}
}

func TestParsePublicKeyRejectsWeakRSA(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate weak RSA key: %v", err)
	}
	key, err := cryptossh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("convert weak RSA public key: %v", err)
	}
	_, err = ParsePublicKey(
		string(cryptossh.MarshalAuthorizedKey(key)),
	)
	if err == nil || !strings.Contains(err.Error(), "at least 2048") {
		t.Fatalf("ParsePublicKey() error = %v", err)
	}
}

func TestParsePublicKeyRejectsUnsafeInput(t *testing.T) {
	t.Parallel()

	publicKey := testPublicKey(t, "")
	tests := map[string]string{
		"empty":          "",
		"private marker": "-----BEGIN OPENSSH PRIVATE KEY-----",
		"multiple keys":  publicKey + "\n" + testPublicKey(t, ""),
		"options":        `command="false" ` + publicKey,
		"not a key":      "ssh-ed25519 invalid",
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParsePublicKey(value); err == nil {
				t.Fatal("ParsePublicKey() returned no error")
			}
		})
	}
}

func testPublicKey(t *testing.T, comment string) string {
	t.Helper()

	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	key, err := cryptossh.NewPublicKey(public)
	if err != nil {
		t.Fatalf("convert Ed25519 public key: %v", err)
	}
	value := strings.TrimSpace(string(cryptossh.MarshalAuthorizedKey(key)))
	if comment != "" {
		value += " " + comment
	}

	return value
}
