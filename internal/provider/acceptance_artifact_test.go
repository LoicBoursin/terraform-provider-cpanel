package provider

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
)

var (
	testAccArtifactManifestMutex sync.Mutex
	testAccSyncArtifactManifest  = func(string, string) error { return nil }
)

func testAccRegisterArtifact(value string) string {
	if os.Getenv("TF_ACC") == "" {
		return value
	}
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		panic("acceptance test artifact must be a non-empty single line")
	}

	artifact, err := json.Marshal(struct {
		Version int    `json:"version"`
		Kind    string `json:"kind"`
		Value   string `json:"value"`
	}{
		Version: 1,
		Kind:    "value",
		Value:   value,
	})
	if err != nil {
		panic(fmt.Sprintf("encode acceptance artifact: %v", err))
	}
	testAccRegisterArtifactLine(string(artifact))

	return value
}

func testAccPasswordVersion(password string) int64 {
	return int64(crc32.ChecksumIEEE([]byte(password))) + 1
}

func testAccRegisterArtifactLine(artifact string) {
	if artifact == "" || strings.ContainsAny(artifact, "\x00\r\n") {
		panic("acceptance test artifact record must be a non-empty single line")
	}

	manifestPath := os.Getenv("CPANEL_TEST_ARTIFACT_MANIFEST")
	if manifestPath == "" {
		panic(
			"CPANEL_TEST_ARTIFACT_MANIFEST must be set for acceptance tests",
		)
	}

	testAccArtifactManifestMutex.Lock()
	defer testAccArtifactManifestMutex.Unlock()

	file, err := os.OpenFile(
		manifestPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		panic(fmt.Sprintf("open acceptance artifact manifest: %v", err))
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		panic(fmt.Sprintf("secure acceptance artifact manifest: %v", err))
	}
	if _, err := fmt.Fprintln(file, artifact); err != nil {
		_ = file.Close()
		panic(fmt.Sprintf("append acceptance artifact manifest: %v", err))
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		panic(fmt.Sprintf("sync acceptance artifact manifest: %v", err))
	}
	if err := file.Close(); err != nil {
		panic(fmt.Sprintf("close acceptance artifact manifest: %v", err))
	}

	remoteManifest := os.Getenv("CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST")
	if remoteManifest == "" {
		return
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		panic(fmt.Sprintf("read acceptance artifact manifest: %v", err))
	}
	if err := testAccSyncArtifactManifest(
		remoteManifest,
		string(content),
	); err != nil {
		panic(fmt.Sprintf("sync remote acceptance artifact manifest: %v", err))
	}
}

func TestTestAccRegisterArtifact(t *testing.T) {
	manifestPath := path.Join(t.TempDir(), "artifacts.txt")
	t.Setenv("TF_ACC", "1")
	t.Setenv("CPANEL_TEST_ARTIFACT_MANIFEST", manifestPath)
	t.Setenv("CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST", "")

	const artifact = "tfcpanel-test-artifact"
	if got := testAccRegisterArtifact(artifact); got != artifact {
		t.Fatalf("testAccRegisterArtifact() = %q; want %q", got, artifact)
	}

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read artifact manifest: %v", err)
	}
	want := `{"version":1,"kind":"value","value":"` + artifact + `"}` + "\n"
	if got := string(content); got != want {
		t.Fatalf("artifact manifest = %q; want %q", got, want)
	}

	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat artifact manifest: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("artifact manifest permissions = %o; want %o", got, want)
	}
}
