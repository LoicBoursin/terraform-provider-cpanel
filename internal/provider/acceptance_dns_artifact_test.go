package provider

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"testing"
)

func testAccRegisterDNSRecord(
	t *testing.T,
	zone string,
	name string,
	recordType string,
	ttl int64,
	data []string,
) {
	t.Helper()

	artifact, err := json.Marshal(struct {
		Version int      `json:"version"`
		Kind    string   `json:"kind"`
		Zone    string   `json:"zone"`
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		TTL     int64    `json:"ttl"`
		Data    []string `json:"data"`
	}{
		Version: 1,
		Kind:    "dns_record",
		Zone:    strings.TrimSuffix(zone, "."),
		Name:    name,
		Type:    strings.ToUpper(recordType),
		TTL:     ttl,
		Data:    append([]string(nil), data...),
	})
	if err != nil {
		t.Fatalf("encode DNS record artifact: %v", err)
	}
	testAccRegisterArtifactLine(string(artifact))
}

func TestTestAccRegisterDNSRecord(t *testing.T) {
	manifestPath := path.Join(t.TempDir(), "artifacts.txt")
	t.Setenv("TF_ACC", "1")
	t.Setenv("CPANEL_TEST_ARTIFACT_MANIFEST", manifestPath)

	testAccRegisterDNSRecord(
		t,
		"example.test.",
		"www",
		"txt",
		300,
		[]string{"first value", "second value"},
	)

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read DNS artifact manifest: %v", err)
	}
	const want = `{"version":1,"kind":"dns_record","zone":"example.test","name":"www","type":"TXT","ttl":300,"data":["first value","second value"]}` + "\n"
	if got := string(content); got != want {
		t.Fatalf("DNS artifact manifest = %q; want %q", got, want)
	}
}
