package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"testing"

	cpanelapitoken "terraform-provider-cpanel/internal/cpanel/apitoken"
)

func testAccRegisterAPITokenCandidate(name string, notBefore, notAfter int64) {
	if os.Getenv("TF_ACC") == "" {
		return
	}

	artifact, err := json.Marshal(struct {
		Version   int    `json:"version"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		NotBefore int64  `json:"not_before"`
		NotAfter  int64  `json:"not_after"`
	}{
		Version:   1,
		Kind:      "api_token_candidate",
		Name:      name,
		NotBefore: notBefore,
		NotAfter:  notAfter,
	})
	if err != nil {
		panic(fmt.Sprintf("encode API token candidate artifact: %v", err))
	}
	testAccRegisterArtifactLine(string(artifact))
}

func testAccRegisterAPITokenIdentity(token *cpanelapitoken.Token) {
	features := append([]string{}, token.Features...)
	whitelistIPs := append([]string{}, token.WhitelistIPs...)
	slices.Sort(features)
	slices.Sort(whitelistIPs)

	artifact, err := json.Marshal(struct {
		Version       int      `json:"version"`
		Kind          string   `json:"kind"`
		Name          string   `json:"name"`
		CreatedAt     int64    `json:"created_at"`
		ExpiresAt     int64    `json:"expires_at"`
		HasFullAccess int      `json:"has_full_access"`
		Features      []string `json:"features"`
		WhitelistIPs  []string `json:"whitelist_ips"`
	}{
		Version:       1,
		Kind:          "api_token",
		Name:          token.Name,
		CreatedAt:     token.CreateTime,
		ExpiresAt:     token.ExpiresAt.ValueOrZero(),
		HasFullAccess: token.HasFullAccess,
		Features:      features,
		WhitelistIPs:  whitelistIPs,
	})
	if err != nil {
		panic(fmt.Sprintf("encode API token acceptance artifact: %v", err))
	}
	testAccRegisterArtifactLine(string(artifact))
}

func TestTestAccRegisterAPITokenIdentityNormalizesEmptyCollections(
	t *testing.T,
) {
	manifestPath := path.Join(t.TempDir(), "artifacts.txt")
	t.Setenv("TF_ACC", "1")
	t.Setenv("CPANEL_TEST_ARTIFACT_MANIFEST", manifestPath)
	t.Setenv("CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST", "")

	testAccRegisterAPITokenIdentity(&cpanelapitoken.Token{
		Name:          "tfcpaneltokenmanifest",
		HasFullAccess: 1,
		CreateTime:    42,
	})

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read API token artifact manifest: %v", err)
	}
	const want = `{"version":1,"kind":"api_token","name":"tfcpaneltokenmanifest","created_at":42,"expires_at":0,"has_full_access":1,"features":[],"whitelist_ips":[]}` + "\n"
	if got := string(content); got != want {
		t.Fatalf("API token artifact manifest = %q; want %q", got, want)
	}
}

func TestTestAccRegisterAPITokenCandidate(t *testing.T) {
	manifestPath := path.Join(t.TempDir(), "artifacts.txt")
	t.Setenv("TF_ACC", "1")
	t.Setenv("CPANEL_TEST_ARTIFACT_MANIFEST", manifestPath)
	t.Setenv("CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST", "")

	testAccRegisterAPITokenCandidate("tfcpaneltokencandidate", 42, 84)

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read API token candidate manifest: %v", err)
	}
	const want = `{"version":1,"kind":"api_token_candidate","name":"tfcpaneltokencandidate","not_before":42,"not_after":84}` + "\n"
	if got := string(content); got != want {
		t.Fatalf("API token candidate manifest = %q; want %q", got, want)
	}
}
