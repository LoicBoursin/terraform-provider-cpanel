package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

const testAccRemoteArtifactManifestFileName = ".terraform-provider-cpanel-acceptance-artifacts"

func init() {
	testAccSyncArtifactManifest = testAccSyncRemoteArtifactManifest
}

func testAccSyncRemoteArtifactManifest(fileName string, content string) error {
	if fileName != testAccRemoteArtifactManifestFileName {
		return errors.New("invalid remote acceptance artifact manifest name")
	}

	client, err := testAccClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var saveResponse struct {
		Data struct {
			Path string `json:"path"`
		} `json:"data"`
	}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		"save_file_content",
		map[string]string{
			"content":      content,
			"dir":          "",
			"fallback":     "0",
			"file":         fileName,
			"from_charset": "UTF-8",
			"to_charset":   "UTF-8",
		},
		&saveResponse,
	); err != nil {
		return err
	}
	if path.Base(saveResponse.Data.Path) != fileName {
		return fmt.Errorf(
			"save_file_content returned path %q; expected file %q",
			saveResponse.Data.Path,
			fileName,
		)
	}

	var readResponse struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleFileman,
		"get_file_content",
		map[string]string{
			"dir":                           "",
			"file":                          fileName,
			"from_charset":                  "UTF-8",
			"to_charset":                    "UTF-8",
			"update_html_document_encoding": "0",
		},
		&readResponse,
	); err != nil {
		return err
	}
	if readResponse.Data.Content != content {
		return errors.New("remote acceptance artifact manifest content differs after save")
	}

	return nil
}

func TestTestAccRegisterArtifactSyncsRemoteManifest(t *testing.T) {
	var remoteContent string

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if got, want := request.Header.Get("Authorization"), "cpanel test-user:test-token"; got != want {
			t.Fatalf("Authorization = %q; want %q", got, want)
		}

		switch {
		case request.Method == http.MethodPost &&
			request.URL.Path == "/execute/Fileman/save_file_content":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if got := request.PostForm.Get("dir"); got != "" {
				t.Fatalf("save dir = %q; want empty account home", got)
			}
			if got := request.PostForm.Get("file"); got != testAccRemoteArtifactManifestFileName {
				t.Fatalf(
					"save file = %q; want %q",
					got,
					testAccRemoteArtifactManifestFileName,
				)
			}
			remoteContent = request.PostForm.Get("content")
			if _, err := fmt.Fprintf(
				response,
				`{"status":1,"errors":[],"messages":[],"data":{"path":"/home/test-user/%s"}}`,
				testAccRemoteArtifactManifestFileName,
			); err != nil {
				t.Fatalf("write save response: %v", err)
			}
		case request.Method == http.MethodGet &&
			request.URL.Path == "/execute/Fileman/get_file_content":
			if got := request.URL.Query().Get("dir"); got != "" {
				t.Fatalf("read dir = %q; want empty account home", got)
			}
			if got := request.URL.Query().Get("file"); got != testAccRemoteArtifactManifestFileName {
				t.Fatalf(
					"read file = %q; want %q",
					got,
					testAccRemoteArtifactManifestFileName,
				)
			}
			content, err := json.Marshal(remoteContent)
			if err != nil {
				t.Fatalf("marshal remote manifest content: %v", err)
			}
			if _, err := fmt.Fprintf(
				response,
				`{"status":1,"errors":[],"messages":[],"data":{"content":%s}}`,
				content,
			); err != nil {
				t.Fatalf("write read response: %v", err)
			}
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()

	manifestPath := path.Join(t.TempDir(), "artifacts.txt")
	t.Setenv("TF_ACC", "1")
	t.Setenv("CPANEL_TEST_ARTIFACT_MANIFEST", manifestPath)
	t.Setenv(
		"CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST",
		testAccRemoteArtifactManifestFileName,
	)
	t.Setenv("CPANEL_HOST", server.URL)
	t.Setenv("CPANEL_USERNAME", "test-user")
	t.Setenv("CPANEL_API_TOKEN", "test-token")

	const artifact = "tfcpanel-remote-artifact"
	testAccRegisterArtifact(artifact)

	want := `{"version":1,"kind":"value","value":"` + artifact + `"}` + "\n"
	if got := remoteContent; got != want {
		t.Fatalf("remote manifest = %q; want %q", got, want)
	}
}
