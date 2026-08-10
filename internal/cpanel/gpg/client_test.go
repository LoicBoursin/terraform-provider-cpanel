package gpg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientImportsLooksUpAndDeletesKeyPair(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{}
	client := newGPGTestClient(t, serverState)

	key, warnings, err := client.Import(t.Context(), armored)
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Import() warnings = %#v", warnings)
	}
	if key.ID != parsed.ID || key.Fingerprint != parsed.Fingerprint {
		t.Fatalf("Import() key = %#v", key)
	}
	if key.ContentSHA256 != parsed.ContentSHA256 {
		t.Fatalf("ContentSHA256 = %q", key.ContentSHA256)
	}

	lookup, _, err := client.Lookup(t.Context(), parsed.ID)
	if err != nil {
		t.Fatalf("Lookup() error: %v", err)
	}
	if lookup.Key == nil || lookup.HasSecretKey {
		t.Fatalf("Lookup() = %#v", lookup)
	}

	warnings, err = client.DeleteKeyPairGuarded(t.Context(), Ownership{
		ID:            key.ID,
		Fingerprint:   key.Fingerprint,
		ContentSHA256: key.ContentSHA256,
	})
	if err != nil {
		t.Fatalf("DeleteGuarded() error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("DeleteGuarded() warnings = %#v", warnings)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.publicKey != "" {
		t.Fatal("public key remains after deletion")
	}
	if serverState.importCalls != 1 || serverState.deleteCalls != 1 {
		t.Fatalf(
			"mutation calls = import %d, delete %d",
			serverState.importCalls,
			serverState.deleteCalls,
		)
	}
}

func TestClientRecoversAmbiguousImportWithoutReplay(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{
		ambiguousImport: true,
	}
	client := newGPGTestClient(t, serverState)

	key, warnings, err := client.Import(t.Context(), armored)
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if key.ID != parsed.ID {
		t.Fatalf("Import() ID = %q, want %q", key.ID, parsed.ID)
	}
	if !warningsContain(warnings, "ambiguous GPG import") {
		t.Fatalf("Import() warnings = %#v", warnings)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.importCalls != 1 {
		t.Fatalf("import calls = %d, want 1", serverState.importCalls)
	}
}

func TestClientRetriesTransientPostImportRead(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{
		publicInventoryFailuresAfterImport: 1,
	}
	client := newGPGTestClient(t, serverState)

	key, _, err := client.Import(t.Context(), armored)
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if key.ID != parsed.ID ||
		key.ContentSHA256 != parsed.ContentSHA256 {
		t.Fatalf("Import() key = %#v", key)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.importCalls != 1 {
		t.Fatalf("import calls = %d, want 1", serverState.importCalls)
	}
}

func TestClientRecoversImportAfterRequestContextCancellation(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	ctx, cancel := context.WithCancel(context.Background())
	serverState := &gpgTestServerState{
		ambiguousImport: true,
		onImport:        cancel,
	}
	client := newGPGTestClient(t, serverState)

	key, _, err := client.Import(ctx, armored)
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if key.ID != parsed.ID {
		t.Fatalf("Import() ID = %q, want %q", key.ID, parsed.ID)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.importCalls != 1 {
		t.Fatalf("import calls = %d, want 1", serverState.importCalls)
	}
}

func TestClientDoesNotRecoverDeterministicImportError(t *testing.T) {
	t.Parallel()

	armored, _ := testPublicKey(t)
	serverState := &gpgTestServerState{
		deterministicImportError: true,
	}
	client := newGPGTestClient(t, serverState)

	if _, _, err := client.Import(t.Context(), armored); err == nil {
		t.Fatal("Import() returned no error")
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.importCalls != 1 {
		t.Fatalf("import calls = %d, want 1", serverState.importCalls)
	}
	if serverState.publicListCalls != 1 ||
		serverState.secretListCalls != 1 {
		t.Fatalf(
			"inventory calls = public %d, secret %d; expected baseline only",
			serverState.publicListCalls,
			serverState.secretListCalls,
		)
	}
}

func TestClientRecoversAmbiguousDeletionWithoutReplay(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{
		publicKey:       armored,
		ambiguousDelete: true,
	}
	client := newGPGTestClient(t, serverState)

	warnings, err := client.DeleteKeyPairGuarded(t.Context(), Ownership{
		ID:            parsed.ID,
		Fingerprint:   parsed.Fingerprint,
		ContentSHA256: parsed.ContentSHA256,
	})
	if err != nil {
		t.Fatalf("DeleteGuarded() error: %v", err)
	}
	if !warningsContain(warnings, "ambiguous GPG deletion") {
		t.Fatalf("DeleteGuarded() warnings = %#v", warnings)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", serverState.deleteCalls)
	}
}

func TestClientRefusesDeletionWhenSecretKeyMatches(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{
		publicKey: armored,
		secretID:  parsed.ID[len(parsed.ID)-8:],
	}
	client := newGPGTestClient(t, serverState)

	_, err := client.DeleteKeyPairGuarded(t.Context(), Ownership{
		ID:            parsed.ID,
		Fingerprint:   parsed.Fingerprint,
		ContentSHA256: parsed.ContentSHA256,
	})
	if err == nil || !strings.Contains(err.Error(), "matching secret key") {
		t.Fatalf("DeleteGuarded() error = %v", err)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", serverState.deleteCalls)
	}
}

func TestClientRefusesPairDeletionWhenSecretAppearsAfterExport(
	t *testing.T,
) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{
		publicKey:         armored,
		secretAfterExport: true,
	}
	client := newGPGTestClient(t, serverState)

	_, err := client.DeleteKeyPairGuarded(t.Context(), Ownership{
		ID:            parsed.ID,
		Fingerprint:   parsed.Fingerprint,
		ContentSHA256: parsed.ContentSHA256,
	})
	if err == nil || !strings.Contains(err.Error(), "matching secret key") {
		t.Fatalf("DeleteKeyPairGuarded() error = %v", err)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", serverState.deleteCalls)
	}
}

func TestClientRefusesDeletionWhenExportChanged(t *testing.T) {
	t.Parallel()

	armored, parsed := testPublicKey(t)
	serverState := &gpgTestServerState{publicKey: armored}
	client := newGPGTestClient(t, serverState)

	_, err := client.DeleteKeyPairGuarded(t.Context(), Ownership{
		ID:            parsed.ID,
		Fingerprint:   parsed.Fingerprint,
		ContentSHA256: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("DeleteGuarded() error = %v", err)
	}

	serverState.mu.Lock()
	defer serverState.mu.Unlock()
	if serverState.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", serverState.deleteCalls)
	}
}

func TestClientRejectsMalformedInventory(t *testing.T) {
	t.Parallel()

	serverState := &gpgTestServerState{malformedPublicInventory: true}
	client := newGPGTestClient(t, serverState)
	if _, _, err := client.ListPublic(t.Context()); err == nil {
		t.Fatal("ListPublic() returned no error")
	}
}

type gpgTestServerState struct {
	mu sync.Mutex

	publicKey                          string
	secretID                           string
	ambiguousImport                    bool
	ambiguousDelete                    bool
	deterministicImportError           bool
	malformedPublicInventory           bool
	publicInventoryFailuresAfterImport int
	secretAfterExport                  bool
	onImport                           func()

	importCalls     int
	deleteCalls     int
	publicListCalls int
	secretListCalls int
}

func newGPGTestClient(
	t *testing.T,
	state *gpgTestServerState,
) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		state.handle(t, writer, request)
	}))
	t.Cleanup(server.Close)

	baseClient, err := cpanel.NewClient(
		server.URL,
		"test-user",
		"test-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func (s *gpgTestServerState) handle(
	t *testing.T,
	writer http.ResponseWriter,
	request *http.Request,
) {
	t.Helper()

	if got := request.Header.Get("Authorization"); got !=
		"cpanel test-user:test-token" {
		t.Fatalf("Authorization = %q", got)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch request.URL.Path {
	case "/execute/GPG/list_public_keys":
		requireMethod(t, request, http.MethodGet)
		s.publicListCalls++
		if s.importCalls > 0 &&
			s.publicInventoryFailuresAfterImport > 0 {
			s.publicInventoryFailuresAfterImport--
			_, _ = writer.Write([]byte(`{"status":1,"data":`))
			return
		}
		if s.malformedPublicInventory {
			writeGPGEnvelope(t, writer, map[string]any{
				"data": map[string]any{"unexpected": true},
			})
			return
		}
		data := []map[string]any{}
		if s.publicKey != "" {
			parsed, err := ParsePublicKey(s.publicKey)
			if err != nil {
				t.Fatalf("parse server public key: %v", err)
			}
			data = append(data, gpgMetadata(
				parsed.ID,
				"pub",
				parsed.Bits,
			))
		}
		writeGPGEnvelope(t, writer, map[string]any{"data": data})
	case "/execute/GPG/list_secret_keys":
		requireMethod(t, request, http.MethodGet)
		s.secretListCalls++
		data := []map[string]any{}
		if s.secretID != "" {
			data = append(data, gpgMetadata(s.secretID, "sec", 2048))
		}
		writeGPGEnvelope(t, writer, map[string]any{"data": data})
	case "/execute/GPG/export_public_key":
		requireMethod(t, request, http.MethodGet)
		parsed := s.requirePublicKey(t)
		requireExactValues(t, request.URL.Query(), url.Values{
			"key_id": []string{parsed.ID},
		})
		if s.secretAfterExport {
			s.secretID = parsed.ID[len(parsed.ID)-8:]
			s.secretAfterExport = false
		}
		writeGPGEnvelope(t, writer, map[string]any{
			"data": map[string]any{"key_data": s.publicKey},
		})
	case "/execute/GPG/import_key":
		requireMethod(t, request, http.MethodPost)
		s.importCalls++
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm(): %v", err)
		}
		requireExactValues(t, request.PostForm, url.Values{
			"key_data": []string{request.PostForm.Get("key_data")},
		})
		parsed, err := ParsePublicKey(request.PostForm.Get("key_data"))
		if err != nil {
			t.Fatalf("parse imported key: %v", err)
		}
		if s.deterministicImportError {
			writeGPGAPIError(
				t,
				writer,
				"deterministic import failure",
			)
			return
		}
		if s.publicKey != "" {
			writeGPGAPIError(t, writer, "key already exists")
			return
		}
		s.publicKey = request.PostForm.Get("key_data")
		if s.onImport != nil {
			s.onImport()
		}
		if s.ambiguousImport {
			_, _ = writer.Write([]byte(`{"status":1,"data":`))
			return
		}
		writeGPGEnvelope(t, writer, map[string]any{
			"data": map[string]any{"key_id": parsed.ID},
		})
	case "/execute/GPG/delete_keypair":
		requireMethod(t, request, http.MethodPost)
		s.deleteCalls++
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm(): %v", err)
		}
		parsed := s.requirePublicKey(t)
		requireExactValues(t, request.PostForm, url.Values{
			"key_id": []string{parsed.ID},
		})
		s.publicKey = ""
		if s.ambiguousDelete {
			_, _ = writer.Write([]byte(`{"status":1,"data":`))
			return
		}
		writeGPGEnvelope(t, writer, map[string]any{"data": nil})
	default:
		t.Fatalf("unexpected request path %q", request.URL.Path)
	}
}

func (s *gpgTestServerState) requirePublicKey(
	t *testing.T,
) *ParsedPublicKey {
	t.Helper()

	if s.publicKey == "" {
		t.Fatal("server has no public key")
	}
	parsed, err := ParsePublicKey(s.publicKey)
	if err != nil {
		t.Fatalf("parse server public key: %v", err)
	}

	return parsed
}

func gpgMetadata(id, keyType string, bits int64) map[string]any {
	return map[string]any{
		"algorithm": "RSA (Encrypt or Sign)",
		"bits":      fmt.Sprintf("%d", bits),
		"created":   "1700000000",
		"expires":   "",
		"id":        id,
		"type":      keyType,
		"user_id":   "Terraform cPanel test <terraform@example.invalid>",
	}
}

func writeGPGEnvelope(
	t *testing.T,
	writer http.ResponseWriter,
	fields map[string]any,
) {
	t.Helper()

	envelope := map[string]any{
		"status":   1,
		"errors":   nil,
		"messages": nil,
		"metadata": map[string]any{"transformed": 1},
		"warnings": nil,
	}
	for key, value := range fields {
		envelope[key] = value
	}
	if err := json.NewEncoder(writer).Encode(envelope); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func writeGPGAPIError(
	t *testing.T,
	writer http.ResponseWriter,
	message string,
) {
	t.Helper()

	if err := json.NewEncoder(writer).Encode(map[string]any{
		"status":   0,
		"data":     nil,
		"errors":   []string{message},
		"messages": nil,
		"metadata": map[string]any{},
		"warnings": nil,
	}); err != nil {
		t.Fatalf("encode API error: %v", err)
	}
}

func requireMethod(
	t *testing.T,
	request *http.Request,
	expected string,
) {
	t.Helper()

	if request.Method != expected {
		t.Fatalf("method = %q, want %q", request.Method, expected)
	}
}

func requireExactValues(
	t *testing.T,
	actual url.Values,
	expected url.Values,
) {
	t.Helper()

	if actual.Encode() != expected.Encode() {
		t.Fatalf(
			"parameters = %q, want %q",
			actual.Encode(),
			expected.Encode(),
		)
	}
}

func warningsContain(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}

	return false
}

func TestMutationVerificationErrorUnwraps(t *testing.T) {
	t.Parallel()

	cause := context.Canceled
	err := &MutationVerificationError{
		Operation: "test",
		Err:       cause,
	}
	if !strings.Contains(err.Error(), "test") {
		t.Fatalf("Error() = %q", err.Error())
	}
	if err.Unwrap() != cause {
		t.Fatal("Unwrap() did not return cause")
	}
}
