package versioncontrol

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsGitRepositories(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case 2:
			if request.Method != http.MethodGet ||
				request.URL.Path != "/execute/VersionControl/retrieve" {
				t.Errorf("request = %s %s", request.Method, request.URL.Path)
			}
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"name":               "Website",
					"repository_root":    "/home/example/repositories/site",
					"type":               "git",
					"branch":             "main",
					"available_branches": []string{"release", "main"},
					"clone_urls": map[string]any{
						"read_only":  []string{},
						"read_write": []string{"ssh://example/site"},
					},
					"source_repository": map[string]any{
						"remote_name": "origin",
						"url":         "https://example.com/site.git",
					},
					"deployable": 1,
				}},
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	repositories, err := newVersionControlTestClient(t, server).List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(repositories) != 1 {
		t.Fatalf("repositories = %#v", repositories)
	}
	repository := repositories[0]
	if repository.RepositoryRoot != "repositories/site" ||
		repository.Name != "Website" ||
		repository.Branch != "main" ||
		repository.SourceRepositoryURL != "https://example.com/site.git" ||
		!repository.Deployable {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestClientCreatesGitRepositoryWithPOST(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case 2:
			if request.Method != http.MethodPost ||
				request.URL.Path != "/execute/VersionControl/create" {
				t.Errorf("request = %s %s", request.Method, request.URL.Path)
			}
			if request.URL.RawQuery != "" {
				t.Errorf("query contains parameters: %q", request.URL.RawQuery)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("repository_root") != "/home/example/repositories/site" ||
				request.Form.Get("name") != "Website" ||
				request.Form.Get("type") != "git" {
				t.Errorf("form = %v", request.Form)
			}
			sourceRepository := SourceRepository{}
			if err := json.Unmarshal(
				[]byte(request.Form.Get("source_repository")),
				&sourceRepository,
			); err != nil {
				t.Fatalf("decode source_repository: %v", err)
			}
			if sourceRepository.RemoteName != "origin" ||
				sourceRepository.URL != "https://example.com/site.git" {
				t.Errorf("source repository = %#v", sourceRepository)
			}
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"name":               "Website",
					"repository_root":    "/home/example/repositories/site",
					"type":               "git",
					"branch":             nil,
					"available_branches": []string{},
					"clone_urls": map[string]any{
						"read_only":  []string{},
						"read_write": []string{"ssh://example/site"},
					},
					"source_repository": map[string]any{
						"remote_name": "origin",
						"url":         "https://example.com/site.git",
					},
					"deployable": 0,
				},
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	repository, err := newVersionControlTestClient(t, server).Create(
		t.Context(),
		Definition{
			Name:                "Website",
			RepositoryRoot:      "repositories/site",
			SourceRepositoryURL: "https://example.com/site.git",
		},
	)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if repository.RepositoryRoot != "repositories/site" ||
		repository.Name != "Website" {
		t.Fatalf("repository = %#v", repository)
	}
}

func TestClientDeletesGitRepositoryDirectoryWithAPI2POST(t *testing.T) {
	t.Parallel()

	const repositoryRoot = "repositories/tfcpanel-git-site"

	stagingRoot := repositoryDeletionStagingRoot(repositoryRoot)
	stagingName := path.Base(stagingRoot)
	originalExists := true
	stagingExists := false
	trashExists := false

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data":   map[string]any{"home": "/home/example"},
			})
		case "/execute/Fileman/list_files":
			if request.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", request.Method)
			}
			entries := []map[string]any{}
			switch directory := request.URL.Query().Get("dir"); directory {
			case "":
				entries = append(
					entries,
					map[string]any{
						"file":     "repositories",
						"fullpath": "/home/example/repositories",
						"type":     "dir",
					},
					map[string]any{
						"file":     ".trash",
						"fullpath": "/home/example/.trash",
						"type":     "dir",
					},
				)
			case "repositories":
				if originalExists {
					entries = append(entries, map[string]any{
						"file":     path.Base(repositoryRoot),
						"fullpath": "/home/example/" + repositoryRoot,
						"type":     "dir",
					})
				}
				if stagingExists {
					entries = append(entries, map[string]any{
						"file":     stagingName,
						"fullpath": "/home/example/" + stagingRoot,
						"type":     "dir",
					})
				}
			case ".trash":
				if trashExists {
					entries = append(entries, map[string]any{
						"file":     stagingName,
						"fullpath": "/home/example/.trash/" + stagingName,
						"type":     "dir",
					})
				}
			default:
				t.Fatalf("unexpected Fileman list directory %q", directory)
			}
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data":   entries,
			})
		case "/json-api/cpanel":
			if request.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("cpanel_jsonapi_module") != "Fileman" ||
				request.Form.Get("cpanel_jsonapi_func") != "fileop" ||
				request.Form.Get("doubledecode") != "0" {
				t.Errorf("form = %v", request.Form)
			}
			switch request.Form.Get("op") {
			case "rename":
				if request.Form.Get("sourcefiles") != repositoryRoot ||
					request.Form.Get("destfiles") != stagingName {
					t.Errorf("rename form = %v", request.Form)
				}
				originalExists = false
				stagingExists = true
			case "trash":
				if request.Form.Get("sourcefiles") != stagingRoot ||
					request.Form.Get("destfiles") != "" {
					t.Errorf("trash form = %v", request.Form)
				}
				stagingExists = false
				trashExists = true
			default:
				t.Fatalf("unexpected file operation %q", request.Form.Get("op"))
			}
			writeVersionControlJSON(t, response, map[string]any{
				"cpanelresult": map[string]any{
					"event": map[string]any{"result": 1},
					"data": []map[string]any{{
						"result": 1,
						"src":    request.Form.Get("sourcefiles"),
						"dest":   request.Form.Get("destfiles"),
					}},
				},
			})
		case "/execute/Fileman/empty_trash":
			if request.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("only_these_files") != stagingName {
				t.Errorf("form = %v", request.Form)
			}
			trashExists = false
			writeVersionControlJSON(t, response, map[string]any{
				"status": 1,
				"data":   nil,
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
		}
	}))
	defer server.Close()

	if err := newVersionControlTestClient(t, server).DeleteDirectory(
		t.Context(),
		repositoryRoot,
	); err != nil {
		t.Fatalf("DeleteDirectory() error: %v", err)
	}
	if originalExists || stagingExists || trashExists {
		t.Fatalf(
			"delete state = original:%t staging:%t trash:%t",
			originalExists,
			stagingExists,
			trashExists,
		)
	}
}

func TestValidateRepositoryRoot(t *testing.T) {
	t.Parallel()

	for _, repositoryRoot := range []string{
		"repositories/site",
		"public_html/application",
	} {
		if err := ValidateRepositoryRoot(repositoryRoot); err != nil {
			t.Fatalf(
				"ValidateRepositoryRoot(%q) error: %v",
				repositoryRoot,
				err,
			)
		}
	}

	for _, repositoryRoot := range []string{
		"",
		"/home/example/site",
		"../site",
		"repositories/my site",
		"repositories/site#copy",
		".ssh/site",
		"public_html/cgi-bin/site",
	} {
		if err := ValidateRepositoryRoot(repositoryRoot); err == nil {
			t.Fatalf(
				"ValidateRepositoryRoot(%q) returned no error",
				repositoryRoot,
			)
		}
	}
}

func newVersionControlTestClient(
	t *testing.T,
	server *httptest.Server,
) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(server.URL, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func writeVersionControlJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
