package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestDirectoryPrivacyDisableReconcilesAmbiguousMutation(t *testing.T) {
	t.Parallel()

	for name, mutationApplied := range map[string]bool{
		"applied":   true,
		"unchanged": false,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			protected := true
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/Variables/get_user_information":
					writeDirectoryPrivacyResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data":   map[string]any{"home": "/home/example"},
						},
					)
				case "/execute/Fileman/list_files":
					writeDirectoryPrivacyResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data": []map[string]any{{
								"file":     "public_html",
								"fullpath": "/home/example/public_html",
								"type":     "dir",
							}},
						},
					)
				case "/execute/DirectoryPrivacy/configure_directory_protection":
					if request.Method != http.MethodPost {
						t.Errorf("method = %s, want POST", request.Method)
					}
					if err := request.ParseForm(); err != nil {
						t.Fatalf("ParseForm() error: %v", err)
					}
					if request.Form.Get("enabled") != "0" {
						t.Errorf("form = %v", request.Form)
					}
					if mutationApplied {
						protected = false
					}
					writeDirectoryPrivacyResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 0,
							"errors": []string{"connection closed"},
							"data":   nil,
						},
					)
				case "/execute/DirectoryPrivacy/is_directory_protected":
					protectedValue := 0
					authName := ""
					if protected {
						protectedValue = 1
						authName = "Private files"
					}
					writeDirectoryPrivacyResourceTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data": map[string]any{
								"auth_name":   authName,
								"auth_type":   "Basic",
								"passwd_file": "/home/example/.htpasswds/public_html/passwd",
								"protected":   protectedValue,
							},
						},
					)
				default:
					t.Fatalf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			baseClient, err := cpanel.NewClient(
				server.URL,
				"username",
				"api-token",
			)
			if err != nil {
				t.Fatalf("cpanel.NewClient() error: %v", err)
			}
			resource := directoryPrivacyResource{
				client: directoryprivacy.NewClient(baseClient),
			}

			err = resource.disableDirectoryPrivacy(
				t.Context(),
				"public_html",
			)
			if mutationApplied && err != nil {
				t.Fatalf("disableDirectoryPrivacy() error: %v", err)
			}
			if !mutationApplied && err == nil {
				t.Fatal("disableDirectoryPrivacy() returned no error")
			}
		})
	}
}

func writeDirectoryPrivacyResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
