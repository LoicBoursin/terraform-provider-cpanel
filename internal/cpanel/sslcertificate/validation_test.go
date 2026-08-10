package sslcertificate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestParsePEMReturnsNormalizedCertificate(t *testing.T) {
	t.Parallel()

	certificatePEM, _ := testCertificateMaterial(t)
	parsed, err := ParsePEM(
		"\n  " + strings.TrimSpace(certificatePEM) + "  \n",
	)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	if parsed.Certificate == nil {
		t.Fatal("Certificate = nil")
	}
	if !strings.HasPrefix(
		parsed.NormalizedPEM,
		certificatePEMBlockStart,
	) || strings.HasSuffix(parsed.NormalizedPEM, "\n") {
		t.Fatalf(
			"NormalizedPEM is not normalized: %q",
			parsed.NormalizedPEM,
		)
	}

	fingerprint := sha256.Sum256(parsed.Certificate.Raw)
	if parsed.SHA256Fingerprint != hex.EncodeToString(fingerprint[:]) {
		t.Fatalf(
			"SHA256Fingerprint = %q",
			parsed.SHA256Fingerprint,
		)
	}
}

func TestParsePEMRejectsMultipleCertificates(t *testing.T) {
	t.Parallel()

	certificatePEM, _ := testCertificateMaterial(t)
	if _, err := ParsePEM(
		certificatePEM + certificatePEM,
	); err == nil {
		t.Fatal("ParsePEM() returned no error")
	}
}

func TestParsePEMRejectsPrivateKey(t *testing.T) {
	t.Parallel()

	certificatePEM, privateKeyPEM := testCertificateMaterial(t)
	for name, value := range map[string]string{
		"private key only":     privateKeyPEM,
		"certificate plus key": certificatePEM + privateKeyPEM,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParsePEM(value); err == nil {
				t.Fatal("ParsePEM() returned no error")
			}
		})
	}
}

func TestParsePEMRejectsInvalidCertificateDER(t *testing.T) {
	t.Parallel()

	value := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a certificate"),
	})
	if _, err := ParsePEM(string(value)); err == nil {
		t.Fatal("ParsePEM() returned no error")
	}
}

func TestEqualPEMComparesCertificateDER(t *testing.T) {
	t.Parallel()

	firstPEM, _ := testCertificateMaterial(t)
	secondPEM, _ := testCertificateMaterial(t)

	equal, err := EqualPEM(
		"\n"+strings.TrimSpace(firstPEM)+"\n",
		firstPEM,
	)
	if err != nil {
		t.Fatalf("EqualPEM() error: %v", err)
	}
	if !equal {
		t.Fatal("EqualPEM() = false, want true")
	}

	equal, err = EqualPEM(firstPEM, secondPEM)
	if err != nil {
		t.Fatalf("EqualPEM() error: %v", err)
	}
	if equal {
		t.Fatal("EqualPEM() = true, want false")
	}

	if _, err := EqualPEM("not PEM", firstPEM); err == nil {
		t.Fatal("EqualPEM() returned no error for invalid first PEM")
	}
	if _, err := EqualPEM(firstPEM, "not PEM"); err == nil {
		t.Fatal("EqualPEM() returned no error for invalid second PEM")
	}
}

func TestValidateIDAndFriendlyName(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"certificate-id", "123", "a.b_c"} {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) error: %v", id, err)
		}
	}
	for _, id := range []string{"", " certificate-id", "certificate id", "id\n"} {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) returned no error", id)
		}
	}

	for _, friendlyName := range []string{"", "Certificate", "TLS 2026"} {
		if err := ValidateFriendlyName(friendlyName); err != nil {
			t.Errorf(
				"ValidateFriendlyName(%q) error: %v",
				friendlyName,
				err,
			)
		}
	}
	for _, friendlyName := range []string{" Certificate", "Certificate\n"} {
		if err := ValidateFriendlyName(friendlyName); err == nil {
			t.Errorf(
				"ValidateFriendlyName(%q) returned no error",
				friendlyName,
			)
		}
	}
}

func testCertificateMaterial(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	now := time.Unix(1_700_000_000, 0)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "example.test",
		},
		DNSNames:              []string{"example.test"},
		NotBefore:             now,
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		t.Fatalf("CreateCertificate() error: %v", err)
	}

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error: %v", err)
	}

	certificatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificateDER,
	})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	return string(certificatePEM), string(privateKeyPEM)
}
