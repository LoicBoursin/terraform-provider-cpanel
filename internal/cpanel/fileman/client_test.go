package fileman

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

const testHomeDirectory = "/home/example"

func TestClientGetsEntryAndDirectory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				writeFilemanData(t, response, []map[string]any{
					testEntry(
						"public_html/assets",
						EntryTypeDirectory,
						"0755",
						4096,
					),
				})
			case "public_html/assets":
				writeFilemanData(t, response, []map[string]any{
					testEntry(
						"public_html/assets/app.js",
						EntryTypeFile,
						"0644",
						"22",
					),
				})
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	client := newFilemanTestClient(t, server)
	entry, err := client.GetEntry(t.Context(), "public_html/assets/app.js")
	if err != nil {
		t.Fatalf("GetEntry() error: %v", err)
	}
	if entry == nil {
		t.Fatal("GetEntry() returned nil")
	}
	if *entry != (Entry{
		Path:         "public_html/assets/app.js",
		AbsolutePath: "/home/example/public_html/assets/app.js",
		Type:         EntryTypeFile,
		Permissions:  "0644",
		SizeBytes:    22,
		CreatedAt:    1_784_138_372,
		ModifiedAt:   1_784_138_373,
	}) {
		t.Fatalf("entry = %#v", entry)
	}

	directory, err := client.GetDirectory(
		t.Context(),
		"public_html/assets",
	)
	if err != nil {
		t.Fatalf("GetDirectory() error: %v", err)
	}
	if directory == nil ||
		directory.Entry.Path != "public_html/assets" ||
		directory.Entry.Type != EntryTypeDirectory {
		t.Fatalf("directory = %#v", directory)
	}
}

func TestClientListsDirectory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				writeFilemanData(t, response, []map[string]any{
					testEntry(
						"public_html/assets",
						EntryTypeDirectory,
						"0755",
						"4096",
					),
				})
			case "public_html/assets":
				writeFilemanData(t, response, []map[string]any{
					testEntry(
						"public_html/assets/empty.txt",
						EntryTypeFile,
						"0644",
						"",
					),
					testEntry(
						"public_html/assets/images",
						EntryTypeDirectory,
						"0755",
						4096,
					),
				})
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	entries, err := newFilemanTestClient(t, server).ListDirectory(
		t.Context(),
		"public_html/assets",
	)
	if err != nil {
		t.Fatalf("ListDirectory() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].Path != "public_html/assets/empty.txt" ||
		entries[0].SizeBytes != 0 ||
		entries[1].Path != "public_html/assets/images" ||
		entries[1].SizeBytes != 4096 {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestClientReturnsNilForAbsentTextFileWithoutReadingContent(t *testing.T) {
	t.Parallel()

	contentRequested := false
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				writeFilemanData(t, response, []map[string]any{})
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/execute/Fileman/get_file_content":
			contentRequested = true
			t.Fatal("get_file_content called for an absent file")
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	file, err := newFilemanTestClient(t, server).GetTextFile(
		t.Context(),
		"public_html/missing.txt",
	)
	if err != nil {
		t.Fatalf("GetTextFile() error: %v", err)
	}
	if file != nil {
		t.Fatalf("GetTextFile() = %#v, want nil", file)
	}
	if contentRequested {
		t.Fatal("get_file_content was requested")
	}
}

func TestClientDeletePathRejectsCommaSeparatedPath(t *testing.T) {
	t.Parallel()

	err := (&Client{}).DeletePath(
		t.Context(),
		"public_html/assets,index.html",
	)
	if err == nil || !strings.Contains(err.Error(), "commas") {
		t.Fatalf("DeletePath() error = %v", err)
	}
}

func TestClientRejectsDuplicateEntry(t *testing.T) {
	t.Parallel()

	server := resolutionTestServer(t, func(
		response http.ResponseWriter,
		request *http.Request,
		directory string,
	) {
		switch directory {
		case "":
			writeFilemanData(t, response, []map[string]any{
				testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
			})
		case "public_html":
			entry := testEntry(
				"public_html/site",
				EntryTypeDirectory,
				"0755",
				"4096",
			)
			writeFilemanData(t, response, []map[string]any{entry, entry})
		default:
			t.Fatalf("unexpected list directory %q", directory)
		}
	})
	defer server.Close()

	_, err := newFilemanTestClient(t, server).GetEntry(
		t.Context(),
		"public_html/site",
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate entry") {
		t.Fatalf("GetEntry() error = %v", err)
	}
}

func TestClientRejectsParentTypeConflict(t *testing.T) {
	t.Parallel()

	server := resolutionTestServer(t, func(
		response http.ResponseWriter,
		request *http.Request,
		directory string,
	) {
		switch directory {
		case "":
			writeFilemanData(t, response, []map[string]any{
				testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
			})
		case "public_html":
			writeFilemanData(t, response, []map[string]any{
				testEntry(
					"public_html/assets",
					EntryTypeFile,
					"0644",
					"10",
				),
			})
		default:
			t.Fatalf("unexpected list directory %q", directory)
		}
	})
	defer server.Close()

	_, err := newFilemanTestClient(t, server).GetEntry(
		t.Context(),
		"public_html/assets/app.js",
	)
	if err == nil || !strings.Contains(err.Error(), "path component") {
		t.Fatalf("GetEntry() error = %v", err)
	}
}

func TestClientRejectsRequestedTypeConflicts(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		entryType string
		call      func(*Client) error
	}{
		"directory requested for file": {
			entryType: EntryTypeFile,
			call: func(client *Client) error {
				_, err := client.GetDirectory(t.Context(), "public_html/target")

				return err
			},
		},
		"text file requested for directory": {
			entryType: EntryTypeDirectory,
			call: func(client *Client) error {
				_, err := client.GetTextFile(t.Context(), "public_html/target")

				return err
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := resolutionTestServer(t, func(
				response http.ResponseWriter,
				request *http.Request,
				directory string,
			) {
				switch directory {
				case "":
					writeFilemanData(t, response, []map[string]any{
						testEntry(
							"public_html",
							EntryTypeDirectory,
							"0750",
							"4096",
						),
					})
				case "public_html":
					permissions := "0644"
					if testCase.entryType == EntryTypeDirectory {
						permissions = "0755"
					}
					writeFilemanData(t, response, []map[string]any{
						testEntry(
							"public_html/target",
							testCase.entryType,
							permissions,
							"10",
						),
					})
				default:
					t.Fatalf("unexpected list directory %q", directory)
				}
			})
			defer server.Close()

			err := testCase.call(newFilemanTestClient(t, server))
			if err == nil || !strings.Contains(err.Error(), "expected") {
				t.Fatalf("operation error = %v", err)
			}
		})
	}
}

func TestClientRejectsMalformedInventoryResponses(t *testing.T) {
	t.Parallel()

	validPublicHTML := testEntry(
		"public_html",
		EntryTypeDirectory,
		"0750",
		"4096",
	)
	testCases := map[string]struct {
		home any
		data any
	}{
		"relative home": {
			home: "home/example",
			data: []map[string]any{},
		},
		"home with control character": {
			home: "/home/exam\nple",
			data: []map[string]any{},
		},
		"null list": {
			home: testHomeDirectory,
			data: nil,
		},
		"object list": {
			home: testHomeDirectory,
			data: map[string]any{},
		},
		"unexpected fullpath": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "fullpath", "/home/other/public_html"),
		},
		"unsupported type": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "type", "link"),
		},
		"invalid permissions": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "nicemode", "755"),
		},
		"separator entry name": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "file", "/"),
		},
		"invalid size": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "size", "large"),
		},
		"missing created time": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "ctime", nil),
		},
		"fractional modified time": {
			home: testHomeDirectory,
			data: mutateEntry(validPublicHTML, "mtime", 1.5),
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Variables/get_user_information":
					home, ok := testCase.home.(string)
					if !ok {
						t.Fatalf("home fixture has type %T", testCase.home)
					}
					writeHomeDirectory(t, response, home)
				case "/execute/Fileman/list_files":
					writeFilemanData(t, response, testCase.data)
				default:
					t.Fatalf("unexpected request: %s", request.URL)
				}
			}))
			defer server.Close()

			_, err := newFilemanTestClient(t, server).GetEntry(
				t.Context(),
				"public_html/site",
			)
			if err == nil {
				t.Fatal("GetEntry() returned no error")
			}
		})
	}
}

func TestClientGetsTextFileAfterProvingExistence(t *testing.T) {
	t.Parallel()

	const content = "line one\nfrançais + % & =\n"

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				writeFilemanData(t, response, []map[string]any{
					testEntry(
						"public_html/index.txt",
						EntryTypeFile,
						"0644",
						"30",
					),
				})
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/execute/Fileman/get_file_content":
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Fileman/get_file_content",
			)
			assertQuery(t, request.URL.Query(), url.Values{
				"dir":                           {"public_html"},
				"file":                          {"index.txt"},
				"from_charset":                  {"UTF-8"},
				"to_charset":                    {"UTF-8"},
				"update_html_document_encoding": {"0"},
			})
			writeFilemanData(t, response, map[string]any{
				"path":         "/home/example/public_html/index.txt",
				"dir":          "/home/example/public_html",
				"filename":     "index.txt",
				"content":      content,
				"from_charset": "utf-8",
				"to_charset":   "utf-8",
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	file, err := newFilemanTestClient(t, server).GetTextFile(
		t.Context(),
		"public_html/index.txt",
	)
	if err != nil {
		t.Fatalf("GetTextFile() error: %v", err)
	}
	if file == nil ||
		file.Content != content ||
		file.Entry.AbsolutePath != "/home/example/public_html/index.txt" {
		t.Fatalf("file = %#v", file)
	}
}

func TestClientRejectsMalformedFileContentResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				writeFilemanData(t, response, []map[string]any{
					testEntry(
						"public_html/index.txt",
						EntryTypeFile,
						"0644",
						"5",
					),
				})
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/execute/Fileman/get_file_content":
			writeFilemanData(t, response, map[string]any{
				"path":         "/home/example/public_html/other.txt",
				"dir":          "/home/example/public_html",
				"filename":     "index.txt",
				"content":      "value",
				"from_charset": "utf-8",
				"to_charset":   "utf-8",
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	_, err := newFilemanTestClient(t, server).GetTextFile(
		t.Context(),
		"public_html/index.txt",
	)
	if err == nil || !strings.Contains(err.Error(), "returned path") {
		t.Fatalf("GetTextFile() error = %v", err)
	}
}

func TestClientCreatesDirectoryWithStrictAPI2Response(t *testing.T) {
	t.Parallel()

	created := false
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				entries := []map[string]any{}
				if created {
					entries = append(entries, testEntry(
						"public_html/assets",
						EntryTypeDirectory,
						"0755",
						"4096",
					))
				}
				writeFilemanData(t, response, entries)
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/json-api/cpanel":
			assertAPI2Form(t, request, operationMakeDirectory)
			if request.Form.Get("name") != "assets" ||
				request.Form.Get("path") != "/home/example/public_html" ||
				request.Form.Get("permissions") != "0755" {
				t.Errorf("mkdir form = %v", request.Form)
			}
			created = true
			writeAPI2Response(t, response, operationMakeDirectory, []map[string]any{{
				"permissions": "0755",
				"name":        "assets",
				"path":        "/home/example/public_html",
			}})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	directory, err := newFilemanTestClient(t, server).CreateDirectory(
		t.Context(),
		"public_html/assets",
	)
	if err != nil {
		t.Fatalf("CreateDirectory() error: %v", err)
	}
	if directory == nil ||
		directory.Entry.Path != "public_html/assets" ||
		directory.Entry.Permissions != "0755" {
		t.Fatalf("directory = %#v", directory)
	}
}

func TestClientRejectsMkdirItemLevelFailures(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		data any
	}{
		"failed result": {
			data: []map[string]any{{
				"result": 0,
				"reason": "directory already exists",
			}},
		},
		"no item": {
			data: []map[string]any{},
		},
		"wrong permissions": {
			data: []map[string]any{{
				"permissions": "0700",
				"name":        "assets",
				"path":        "/home/example/public_html",
			}},
		},
		"wrong item path": {
			data: []map[string]any{{
				"permissions": "0755",
				"name":        "assets",
				"path":        "/home/other/public_html",
			}},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := mutationPreparationServer(
				t,
				false,
				func(response http.ResponseWriter, request *http.Request) {
					assertAPI2Form(t, request, operationMakeDirectory)
					writeAPI2Response(
						t,
						response,
						operationMakeDirectory,
						testCase.data,
					)
				},
			)
			defer server.Close()

			_, err := newFilemanTestClient(t, server).CreateDirectory(
				t.Context(),
				"public_html/assets",
			)
			if err == nil {
				t.Fatal("CreateDirectory() returned no error")
			}
		})
	}
}

func TestClientCreatesEmptyTextFileWithStrictRequestAndVerification(t *testing.T) {
	t.Parallel()

	var mutationCalls atomic.Int32
	server := emptyTextFileLifecycleServer(
		t,
		"0644",
		0,
		"",
		&mutationCalls,
	)
	defer server.Close()

	file, err := newFilemanTestClient(t, server).CreateEmptyTextFile(
		t.Context(),
		"public_html/index.txt",
	)
	if err != nil {
		t.Fatalf("CreateEmptyTextFile() error: %v", err)
	}
	if file == nil ||
		file.Entry.Path != "public_html/index.txt" ||
		file.Entry.AbsolutePath != "/home/example/public_html/index.txt" ||
		file.Entry.Type != EntryTypeFile ||
		file.Entry.Permissions != "0644" ||
		file.Entry.SizeBytes != 0 ||
		file.Content != "" {
		t.Fatalf("file = %#v", file)
	}
	if mutationCalls.Load() != 1 {
		t.Fatalf("mutation calls = %d, want 1", mutationCalls.Load())
	}
}

func TestClientCreateEmptyTextFileRejectsPreexistingTargetWithoutMutation(
	t *testing.T,
) {
	t.Parallel()

	var mutationCalls atomic.Int32
	server := emptyTextFilePreparationServer(
		t,
		true,
		func(_ http.ResponseWriter, _ *http.Request) {
			mutationCalls.Add(1)
			t.Error("mkfile called for a preexisting target")
		},
	)
	defer server.Close()

	_, err := newFilemanTestClient(t, server).CreateEmptyTextFile(
		t.Context(),
		"public_html/index.txt",
	)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateEmptyTextFile() error = %v", err)
	}
	if mutationCalls.Load() != 0 {
		t.Fatalf("mutation calls = %d, want 0", mutationCalls.Load())
	}
}

func TestClientCreateEmptyTextFileRequiresExistingDirectoryParent(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		managedPath string
		writeList   func(*testing.T, http.ResponseWriter, string)
		wantError   string
	}{
		"missing parent": {
			managedPath: "public_html/missing/index.txt",
			writeList: func(
				t *testing.T,
				response http.ResponseWriter,
				directory string,
			) {
				switch directory {
				case "":
					writeFilemanData(t, response, []map[string]any{
						testEntry(
							"public_html",
							EntryTypeDirectory,
							"0750",
							"4096",
						),
					})
				case "public_html":
					writeFilemanData(t, response, []map[string]any{})
				default:
					t.Fatalf("unexpected list directory %q", directory)
				}
			},
			wantError: "parent directory",
		},
		"file parent": {
			managedPath: "public_html/assets/index.txt",
			writeList: func(
				t *testing.T,
				response http.ResponseWriter,
				directory string,
			) {
				switch directory {
				case "":
					writeFilemanData(t, response, []map[string]any{
						testEntry(
							"public_html",
							EntryTypeDirectory,
							"0750",
							"4096",
						),
					})
				case "public_html":
					writeFilemanData(t, response, []map[string]any{
						testEntry(
							"public_html/assets",
							EntryTypeFile,
							"0644",
							0,
						),
					})
				default:
					t.Fatalf("unexpected list directory %q", directory)
				}
			},
			wantError: "path component",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := resolutionTestServer(t, func(
				response http.ResponseWriter,
				_ *http.Request,
				directory string,
			) {
				testCase.writeList(t, response, directory)
			})
			defer server.Close()

			_, err := newFilemanTestClient(t, server).CreateEmptyTextFile(
				t.Context(),
				testCase.managedPath,
			)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("CreateEmptyTextFile() error = %v", err)
			}
		})
	}
}

func TestClientCreateEmptyTextFileRejectsRacingCollision(t *testing.T) {
	t.Parallel()

	var mutationCalls atomic.Int32
	server := emptyTextFilePreparationServer(
		t,
		false,
		func(response http.ResponseWriter, request *http.Request) {
			mutationCalls.Add(1)
			assertAPI2Form(t, request, operationMakeFile)
			writeJSON(t, response, map[string]any{
				"cpanelresult": map[string]any{
					"apiversion": 2,
					"error":      "The system failed to create the file: File exists",
					"event":      map[string]any{"result": 1},
					"func":       operationMakeFile,
					"module":     "Fileman",
					"data":       []map[string]any{},
				},
			})
		},
	)
	defer server.Close()

	_, err := newFilemanTestClient(t, server).CreateEmptyTextFile(
		t.Context(),
		"public_html/index.txt",
	)
	if err == nil || !strings.Contains(err.Error(), "File exists") {
		t.Fatalf("CreateEmptyTextFile() error = %v", err)
	}
	if mutationCalls.Load() != 1 {
		t.Fatalf("mutation calls = %d, want 1", mutationCalls.Load())
	}
}

func TestClientCreateEmptyTextFileRejectsMalformedOrUnexpectedResponse(
	t *testing.T,
) {
	t.Parallel()

	validItem := map[string]any{
		"name":        "index.txt",
		"path":        "/home/example/public_html",
		"permissions": "0644",
	}
	result := func(data any) map[string]any {
		return map[string]any{
			"apiversion": 2,
			"event":      map[string]any{"result": 1},
			"func":       operationMakeFile,
			"module":     "Fileman",
			"data":       data,
		}
	}

	testCases := map[string]struct {
		rawResponse  string
		cpanelResult map[string]any
		wantError    string
	}{
		"invalid JSON": {
			rawResponse: `{"cpanelresult":`,
			wantError:   "decode API 2 response envelope",
		},
		"malformed top-level error": {
			cpanelResult: func() map[string]any {
				value := result([]map[string]any{validItem})
				value["error"] = []string{"unexpected"}

				return value
			}(),
			wantError: `field "error" must be a string`,
		},
		"non-array data": {
			cpanelResult: result(validItem),
			wantError:    "decode API 2 response envelope",
		},
		"no item": {
			cpanelResult: result([]map[string]any{}),
			wantError:    "returned 0 item results",
		},
		"multiple items": {
			cpanelResult: result([]map[string]any{validItem, validItem}),
			wantError:    "returned 2 item results",
		},
		"unexpected item field": {
			cpanelResult: result([]map[string]any{{
				"name":        "index.txt",
				"path":        "/home/example/public_html",
				"permissions": "0644",
				"result":      1,
			}}),
			wantError: "expected exactly 3",
		},
		"non-string item path": {
			cpanelResult: result([]map[string]any{{
				"name":        "index.txt",
				"path":        42,
				"permissions": "0644",
			}}),
			wantError: `field "path" must be a string`,
		},
		"wrong item name": {
			cpanelResult: result([]map[string]any{{
				"name":        "other.txt",
				"path":        "/home/example/public_html",
				"permissions": "0644",
			}}),
			wantError: "returned name",
		},
		"wrong item path": {
			cpanelResult: result([]map[string]any{{
				"name":        "index.txt",
				"path":        "/home/other/public_html",
				"permissions": "0644",
			}}),
			wantError: "returned path",
		},
		"wrong item permissions": {
			cpanelResult: result([]map[string]any{{
				"name":        "index.txt",
				"path":        "/home/example/public_html",
				"permissions": "0600",
			}}),
			wantError: "returned permissions",
		},
		"wrong API version": {
			cpanelResult: func() map[string]any {
				value := result([]map[string]any{validItem})
				value["apiversion"] = 3

				return value
			}(),
			wantError: "unexpected API 2 response identity",
		},
		"wrong API module": {
			cpanelResult: func() map[string]any {
				value := result([]map[string]any{validItem})
				value["module"] = "Other"

				return value
			}(),
			wantError: "unexpected API 2 response identity",
		},
		"wrong API function": {
			cpanelResult: func() map[string]any {
				value := result([]map[string]any{validItem})
				value["func"] = operationMakeDirectory

				return value
			}(),
			wantError: "unexpected API 2 response identity",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := emptyTextFilePreparationServer(
				t,
				false,
				func(response http.ResponseWriter, request *http.Request) {
					assertAPI2Form(t, request, operationMakeFile)
					if testCase.rawResponse != "" {
						response.Header().Set("Content-Type", "application/json")
						_, _ = response.Write([]byte(testCase.rawResponse))

						return
					}
					writeJSON(t, response, map[string]any{
						"cpanelresult": testCase.cpanelResult,
					})
				},
			)
			defer server.Close()

			_, err := newFilemanTestClient(t, server).CreateEmptyTextFile(
				t.Context(),
				"public_html/index.txt",
			)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("CreateEmptyTextFile() error = %v", err)
			}
		})
	}
}

func TestClientCreateEmptyTextFileRejectsNonEmptyVerification(t *testing.T) {
	t.Parallel()

	const content = "unexpected content"

	var mutationCalls atomic.Int32
	server := emptyTextFileLifecycleServer(
		t,
		"0644",
		len(content),
		content,
		&mutationCalls,
	)
	defer server.Close()

	_, err := newFilemanTestClient(t, server).CreateEmptyTextFile(
		t.Context(),
		"public_html/index.txt",
	)
	if err == nil || !strings.Contains(err.Error(), "was not empty") {
		t.Fatalf("CreateEmptyTextFile() error = %v", err)
	}
	if mutationCalls.Load() != 1 {
		t.Fatalf("mutation calls = %d, want 1", mutationCalls.Load())
	}
}

func TestClientSavesAndVerifiesUTF8TextFile(t *testing.T) {
	t.Parallel()

	const content = "line one\nfrançais + % & =\n"

	fileExists := false
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				entries := []map[string]any{}
				if fileExists {
					entries = append(entries, testEntry(
						"public_html/index.txt",
						EntryTypeFile,
						"0644",
						len(content),
					))
				}
				writeFilemanData(t, response, entries)
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/execute/Fileman/save_file_content":
			assertRequest(
				t,
				request,
				http.MethodPost,
				"/execute/Fileman/save_file_content",
			)
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			assertQuery(t, request.Form, url.Values{
				"content":      {content},
				"dir":          {"public_html"},
				"fallback":     {"0"},
				"file":         {"index.txt"},
				"from_charset": {"UTF-8"},
				"to_charset":   {"UTF-8"},
			})
			fileExists = true
			writeFilemanData(t, response, map[string]any{
				"path":         "/home/example/public_html/index.txt",
				"from_charset": "utf-8",
				"to_charset":   "utf-8",
			})
		case "/execute/Fileman/get_file_content":
			writeFilemanData(t, response, map[string]any{
				"path":         "/home/example/public_html/index.txt",
				"dir":          "/home/example/public_html",
				"filename":     "index.txt",
				"content":      content,
				"from_charset": "utf-8",
				"to_charset":   "utf-8",
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	file, err := newFilemanTestClient(t, server).SaveTextFile(
		t.Context(),
		"public_html/index.txt",
		content,
	)
	if err != nil {
		t.Fatalf("SaveTextFile() error: %v", err)
	}
	if file == nil ||
		file.Content != content ||
		file.Entry.Type != EntryTypeFile ||
		file.Entry.Permissions != "0644" {
		t.Fatalf("file = %#v", file)
	}
}

func TestClientRejectsMalformedSaveResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				writeFilemanData(t, response, []map[string]any{})
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/execute/Fileman/save_file_content":
			writeFilemanData(t, response, map[string]any{
				"path":         "/home/example/public_html/other.txt",
				"from_charset": "utf-8",
				"to_charset":   "utf-8",
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
	defer server.Close()

	_, err := newFilemanTestClient(t, server).SaveTextFile(
		t.Context(),
		"public_html/index.txt",
		"content",
	)
	if err == nil || !strings.Contains(err.Error(), "returned path") {
		t.Fatalf("SaveTextFile() error = %v", err)
	}
}

func TestClientRejectsInvalidTextContentBeforeMutation(t *testing.T) {
	t.Parallel()

	baseClient, err := cpanel.NewClient(
		"https://example.test",
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	client := NewClient(baseClient)

	testCases := map[string]string{
		"invalid UTF-8": string([]byte{0xff}),
		"null byte":     "before\x00after",
	}
	for name, content := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.SaveTextFile(
				t.Context(),
				"public_html/index.txt",
				content,
			); err == nil {
				t.Fatal("SaveTextFile() returned no error")
			}
		})
	}
}

func TestClientDeletesPathWithStrictItemValidation(t *testing.T) {
	t.Parallel()

	server := mutationPreparationServer(
		t,
		true,
		func(response http.ResponseWriter, request *http.Request) {
			assertAPI2Form(t, request, operationFileOperation)
			if request.Form.Get("op") != "unlink" ||
				request.Form.Get("sourcefiles") != "public_html/assets" ||
				request.Form.Get("doubledecode") != "0" {
				t.Errorf("fileop form = %v", request.Form)
			}
			writeAPI2Response(t, response, operationFileOperation, []map[string]any{{
				"dest":   nil,
				"result": 1,
				"src":    "/home/example/public_html/assets",
			}})
		},
	)
	defer server.Close()

	if err := newFilemanTestClient(t, server).DeletePath(
		t.Context(),
		"public_html/assets",
	); err != nil {
		t.Fatalf("DeletePath() error: %v", err)
	}
}

func TestClientDoesNotMutateWhenDeleteTargetIsAbsent(t *testing.T) {
	t.Parallel()

	server := mutationPreparationServer(
		t,
		false,
		func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("API 2 mutation called for absent path")
		},
	)
	defer server.Close()

	if err := newFilemanTestClient(t, server).DeletePath(
		t.Context(),
		"public_html/assets",
	); err != nil {
		t.Fatalf("DeletePath() error: %v", err)
	}
}

func TestClientRejectsFileopItemLevelFailures(t *testing.T) {
	t.Parallel()

	testCases := map[string]any{
		"failed result": []map[string]any{{
			"result": 0,
			"reason": "permission denied",
		}},
		"wrong source": []map[string]any{{
			"dest":   nil,
			"result": 1,
			"src":    "/home/example/public_html/other",
		}},
		"non-null destination": []map[string]any{{
			"dest":   "/home/example/.trash/assets",
			"result": 1,
			"src":    "/home/example/public_html/assets",
		}},
		"missing result": []map[string]any{{
			"dest": nil,
			"src":  "/home/example/public_html/assets",
		}},
		"multiple items": []map[string]any{
			{
				"dest":   nil,
				"result": 1,
				"src":    "/home/example/public_html/assets",
			},
			{
				"dest":   nil,
				"result": 1,
				"src":    "/home/example/public_html/other",
			},
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := mutationPreparationServer(
				t,
				true,
				func(response http.ResponseWriter, request *http.Request) {
					assertAPI2Form(t, request, operationFileOperation)
					writeAPI2Response(
						t,
						response,
						operationFileOperation,
						data,
					)
				},
			)
			defer server.Close()

			err := newFilemanTestClient(t, server).DeletePath(
				t.Context(),
				"public_html/assets",
			)
			if err == nil {
				t.Fatal("DeletePath() returned no error")
			}
		})
	}
}

func resolutionTestServer(
	t *testing.T,
	writeList func(http.ResponseWriter, *http.Request, string),
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			writeList(response, request, request.URL.Query().Get("dir"))
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
}

func mutationPreparationServer(
	t *testing.T,
	targetExists bool,
	mutate func(http.ResponseWriter, *http.Request),
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				entries := []map[string]any{}
				if targetExists {
					entries = append(entries, testEntry(
						"public_html/assets",
						EntryTypeDirectory,
						"0755",
						"4096",
					))
				}
				writeFilemanData(t, response, entries)
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/json-api/cpanel":
			mutate(response, request)
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
}

func emptyTextFilePreparationServer(
	t *testing.T,
	targetExists bool,
	mutate func(http.ResponseWriter, *http.Request),
) *httptest.Server {
	t.Helper()

	var mutationSeen atomic.Bool

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if mutationSeen.Load() && request.URL.Path != "/json-api/cpanel" {
			t.Errorf(
				"unexpected request after rejected mkfile response: %s",
				request.URL,
			)
			response.WriteHeader(http.StatusInternalServerError)

			return
		}

		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				entries := []map[string]any{}
				if targetExists {
					entries = append(entries, testEntry(
						"public_html/index.txt",
						EntryTypeFile,
						"0644",
						0,
					))
				}
				writeFilemanData(t, response, entries)
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/json-api/cpanel":
			if !mutationSeen.CompareAndSwap(false, true) {
				t.Errorf("unexpected additional mutation: %v", request.Form)
				response.WriteHeader(http.StatusInternalServerError)

				return
			}
			mutate(response, request)
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
}

func emptyTextFileLifecycleServer(
	t *testing.T,
	permissions string,
	size any,
	content string,
	mutationCalls *atomic.Int32,
) *httptest.Server {
	t.Helper()

	var created atomic.Bool

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Variables/get_user_information":
			writeHomeDirectory(t, response, testHomeDirectory)
		case "/execute/Fileman/list_files":
			assertListFilesRequest(t, request)
			switch request.URL.Query().Get("dir") {
			case "":
				writeFilemanData(t, response, []map[string]any{
					testEntry("public_html", EntryTypeDirectory, "0750", "4096"),
				})
			case "public_html":
				entries := []map[string]any{}
				if created.Load() {
					entries = append(entries, testEntry(
						"public_html/index.txt",
						EntryTypeFile,
						permissions,
						size,
					))
				}
				writeFilemanData(t, response, entries)
			default:
				t.Fatalf("unexpected list directory %q", request.URL.Query().Get("dir"))
			}
		case "/json-api/cpanel":
			if mutationCalls.Add(1) != 1 {
				t.Errorf("unexpected additional mutation: %v", request.Form)
				response.WriteHeader(http.StatusInternalServerError)

				return
			}
			assertAPI2Form(t, request, operationMakeFile)
			assertExactQuery(t, request.Form, url.Values{
				"cpanel_jsonapi_apiversion": {"2"},
				"cpanel_jsonapi_func":       {operationMakeFile},
				"cpanel_jsonapi_module":     {"Fileman"},
				"cpanel_jsonapi_user":       {"username"},
				"name":                      {"index.txt"},
				"path":                      {"/home/example/public_html"},
				"permissions":               {"0644"},
			})
			created.Store(true)
			writeAPI2Response(t, response, operationMakeFile, []map[string]any{{
				"name":        "index.txt",
				"path":        "/home/example/public_html",
				"permissions": "0644",
			}})
		case "/execute/Fileman/get_file_content":
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/Fileman/get_file_content",
			)
			assertExactQuery(t, request.URL.Query(), url.Values{
				"dir":                           {"public_html"},
				"file":                          {"index.txt"},
				"from_charset":                  {"UTF-8"},
				"to_charset":                    {"UTF-8"},
				"update_html_document_encoding": {"0"},
			})
			writeFilemanData(t, response, map[string]any{
				"path":         "/home/example/public_html/index.txt",
				"dir":          "/home/example/public_html",
				"filename":     "index.txt",
				"content":      content,
				"from_charset": "utf-8",
				"to_charset":   "utf-8",
			})
		default:
			t.Fatalf("unexpected request: %s", request.URL)
		}
	}))
}

func newFilemanTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}

func testEntry(
	managedPath string,
	entryType string,
	permissions string,
	size any,
) map[string]any {
	return map[string]any{
		"file":     managedPath[strings.LastIndex(managedPath, "/")+1:],
		"fullpath": testHomeDirectory + "/" + managedPath,
		"type":     entryType,
		"nicemode": permissions,
		"size":     size,
		"ctime":    1_784_138_372,
		"mtime":    1_784_138_373,
	}
}

func mutateEntry(
	entry map[string]any,
	field string,
	value any,
) []map[string]any {
	clonedEntry := make(map[string]any, len(entry))
	for key, current := range entry {
		clonedEntry[key] = current
	}
	if value == nil {
		delete(clonedEntry, field)
	} else {
		clonedEntry[field] = value
	}

	return []map[string]any{clonedEntry}
}

func writeHomeDirectory(
	t *testing.T,
	response http.ResponseWriter,
	home string,
) {
	t.Helper()

	writeJSON(t, response, map[string]any{
		"status": 1,
		"data":   map[string]any{"home": home},
	})
}

func writeFilemanData(
	t *testing.T,
	response http.ResponseWriter,
	data any,
) {
	t.Helper()

	writeJSON(t, response, map[string]any{
		"status": 1,
		"data":   data,
	})
}

func writeAPI2Response(
	t *testing.T,
	response http.ResponseWriter,
	function string,
	data any,
) {
	t.Helper()

	writeJSON(t, response, map[string]any{
		"cpanelresult": map[string]any{
			"apiversion": 2,
			"event":      map[string]any{"result": 1},
			"module":     "Fileman",
			"func":       function,
			"data":       data,
		},
	})
}

func writeJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func assertListFilesRequest(t *testing.T, request *http.Request) {
	t.Helper()

	assertRequest(
		t,
		request,
		http.MethodGet,
		"/execute/Fileman/list_files",
	)
	query := request.URL.Query()
	if query.Get("include_permissions") != "1" ||
		query.Get("limit") != "100000" ||
		query.Get("show_hidden") != "1" {
		t.Errorf("list_files query = %v", query)
	}
}

func assertAPI2Form(
	t *testing.T,
	request *http.Request,
	function string,
) {
	t.Helper()

	assertRequest(t, request, http.MethodPost, "/json-api/cpanel")
	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}
	if request.Form.Get("cpanel_jsonapi_apiversion") != "2" ||
		request.Form.Get("cpanel_jsonapi_user") != "username" ||
		request.Form.Get("cpanel_jsonapi_module") != "Fileman" ||
		request.Form.Get("cpanel_jsonapi_func") != function {
		t.Errorf("API 2 form = %v", request.Form)
	}
}

func assertRequest(
	t *testing.T,
	request *http.Request,
	method string,
	requestPath string,
) {
	t.Helper()

	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != requestPath {
		t.Errorf("path = %s, want %s", request.URL.Path, requestPath)
	}
}

func assertQuery(t *testing.T, actual url.Values, expected url.Values) {
	t.Helper()

	for key, expectedValues := range expected {
		actualValues := actual[key]
		if len(actualValues) != len(expectedValues) {
			t.Errorf("%s = %#v, want %#v", key, actualValues, expectedValues)

			continue
		}
		for index := range expectedValues {
			if actualValues[index] != expectedValues[index] {
				t.Errorf("%s = %#v, want %#v", key, actualValues, expectedValues)

				break
			}
		}
	}
}

func assertExactQuery(t *testing.T, actual url.Values, expected url.Values) {
	t.Helper()

	assertQuery(t, actual, expected)
	if len(actual) != len(expected) {
		t.Errorf("query keys = %v, want exactly %v", actual, expected)
	}
}
