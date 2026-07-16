package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

func TestSSLCertificateRestoreFriendlyName(t *testing.T) {
	certificatePEM, _, _ := testAccSSLCertificateMaterial(t, "restore")
	parsed, err := sslcertificate.ParsePEM(certificatePEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}

	currentName := "Renamed certificate"
	renameCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/SSL/list_certs":
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"id":                   "certificate-id",
					"friendly_name":        currentName,
					"domain_is_configured": 0,
				}},
			})
		case "/execute/SSL/show_cert":
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"cert": parsed.NormalizedPEM,
					"details": map[string]any{
						"id":                   "certificate-id",
						"friendly_name":        currentName,
						"domain_is_configured": 0,
					},
				},
			})
		case "/execute/SSL/installed_hosts":
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   []any{},
			})
		case "/execute/SSL/installed_host":
			writeSSLCertificateTestJSON(
				t,
				response,
				sslCertificateDedicatedHostTestResponse(),
			)
		case "/execute/SSL/set_cert_friendly_name":
			if request.Method != http.MethodPost {
				t.Errorf("rename method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error: %v", err)
			}
			if request.Form.Get("id") != "certificate-id" ||
				request.Form.Get("new_friendly_name") !=
					"Original certificate" {
				t.Errorf("rename form = %v", request.Form)
			}
			renameCalls++
			currentName = request.Form.Get("new_friendly_name")
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   nil,
			})
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
	resource := sslCertificateResource{
		client: sslcertificate.NewClient(baseClient),
	}

	err = resource.restoreFriendlyName(
		t.Context(),
		sslcertificate.Certificate{
			ID:             "certificate-id",
			FriendlyName:   "Original certificate",
			CertificatePEM: parsed.NormalizedPEM,
		},
		"Renamed certificate",
	)
	if err != nil {
		t.Fatalf("restoreFriendlyName() error: %v", err)
	}
	if currentName != "Original certificate" {
		t.Fatalf("currentName = %q", currentName)
	}
	if renameCalls != 1 {
		t.Fatalf("renameCalls = %d, want 1", renameCalls)
	}

	currentName = "Concurrent certificate"
	err = resource.restoreFriendlyName(
		t.Context(),
		sslcertificate.Certificate{
			ID:             "certificate-id",
			FriendlyName:   "Original certificate",
			CertificatePEM: parsed.NormalizedPEM,
		},
		"Renamed certificate",
	)
	if err == nil {
		t.Fatal("restoreFriendlyName() error = nil, want concurrent rename rejection")
	}
	if currentName != "Concurrent certificate" {
		t.Fatalf("currentName = %q, want concurrent value preserved", currentName)
	}
	if renameCalls != 1 {
		t.Fatalf("renameCalls = %d, want no concurrent rollback", renameCalls)
	}
}

func TestSSLCertificateUpdateRefusesRemoteDrift(t *testing.T) {
	certificatePEM, _, _ := testAccSSLCertificateMaterial(t, "update-drift")
	parsed, err := sslcertificate.ParsePEM(certificatePEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}

	renameCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/SSL/list_certs":
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"id":                   "certificate-id",
					"friendly_name":        "External certificate",
					"domain_is_configured": 0,
				}},
			})
		case "/execute/SSL/show_cert":
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"cert": parsed.NormalizedPEM,
					"details": map[string]any{
						"id":                   "certificate-id",
						"friendly_name":        "External certificate",
						"domain_is_configured": 0,
					},
				},
			})
		case "/execute/SSL/installed_hosts":
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   []any{},
			})
		case "/execute/SSL/installed_host":
			writeSSLCertificateTestJSON(
				t,
				response,
				sslCertificateDedicatedHostTestResponse(),
			)
		case "/execute/SSL/set_cert_friendly_name":
			renameCalls++
			writeSSLCertificateTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   nil,
			})
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
	resource := sslCertificateResource{
		client: sslcertificate.NewClient(baseClient),
	}
	state := SSLCertificateResourceModel{
		Certificate: types.StringValue(certificatePEM),
	}
	diagnostics := applySSLCertificateToResourceModel(
		t.Context(),
		&state,
		sslcertificate.Certificate{
			ID:             "certificate-id",
			FriendlyName:   "Terraform certificate",
			CertificatePEM: certificatePEM,
		},
		false,
	)
	if diagnostics.HasError() {
		t.Fatalf(
			"applySSLCertificateToResourceModel() diagnostics: %v",
			diagnostics,
		)
	}
	plan := state
	plan.FriendlyName = types.StringValue("Renamed certificate")

	response := runSingletonUpdate(
		t,
		NewSSLCertificateResource(),
		state,
		plan,
		resource.Update,
	)
	assertUpdateDriftRefused(t, response.Diagnostics)
	if renameCalls != 0 {
		t.Fatalf("renameCalls = %d, want 0", renameCalls)
	}
}

func TestSSLCertificateRollbackCreatedRequiresOwnership(t *testing.T) {
	certificatePEM, _, _ := testAccSSLCertificateMaterial(t, "rollback")
	otherPEM, _, _ := testAccSSLCertificateMaterial(t, "rollback-other")
	parsed, err := sslcertificate.ParsePEM(certificatePEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}

	testCases := []struct {
		name                 string
		currentFriendlyName  string
		currentCertificate   string
		wantError            bool
		wantCertificateCount int
		wantDeleteCalls      int
	}{
		{
			name:                 "matching certificate",
			currentFriendlyName:  "Terraform certificate",
			currentCertificate:   parsed.NormalizedPEM,
			wantCertificateCount: 0,
			wantDeleteCalls:      1,
		},
		{
			name:                 "friendly name changed",
			currentFriendlyName:  "External certificate",
			currentCertificate:   parsed.NormalizedPEM,
			wantError:            true,
			wantCertificateCount: 1,
		},
		{
			name:                 "certificate changed",
			currentFriendlyName:  "Terraform certificate",
			currentCertificate:   otherPEM,
			wantError:            true,
			wantCertificateCount: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			certificateExists := true
			deleteCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/SSL/list_certs":
					data := []map[string]any{}
					if certificateExists {
						data = append(data, map[string]any{
							"id":                   "certificate-id",
							"friendly_name":        testCase.currentFriendlyName,
							"domain_is_configured": 0,
						})
					}
					writeSSLCertificateTestJSON(
						t,
						response,
						map[string]any{"status": 1, "data": data},
					)
				case "/execute/SSL/show_cert":
					writeSSLCertificateTestJSON(
						t,
						response,
						map[string]any{
							"status": 1,
							"data": map[string]any{
								"cert": testCase.currentCertificate,
								"details": map[string]any{
									"id":                   "certificate-id",
									"friendly_name":        testCase.currentFriendlyName,
									"domain_is_configured": 0,
								},
							},
						},
					)
				case "/execute/SSL/installed_hosts":
					writeSSLCertificateTestJSON(
						t,
						response,
						map[string]any{"status": 1, "data": []any{}},
					)
				case "/execute/SSL/installed_host":
					writeSSLCertificateTestJSON(
						t,
						response,
						sslCertificateDedicatedHostTestResponse(),
					)
				case "/execute/SSL/delete_cert":
					if request.Method != http.MethodPost {
						t.Errorf(
							"delete method = %s, want POST",
							request.Method,
						)
					}
					if err := request.ParseForm(); err != nil {
						t.Fatalf("ParseForm() error: %v", err)
					}
					if request.Form.Get("id") != "certificate-id" {
						t.Errorf("delete form = %v", request.Form)
					}
					deleteCalls++
					certificateExists = false
					writeSSLCertificateTestJSON(
						t,
						response,
						map[string]any{"status": 1, "data": nil},
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
			resource := sslCertificateResource{
				client: sslcertificate.NewClient(baseClient),
			}

			err = resource.rollbackCreated(
				t.Context(),
				"certificate-id",
				"Terraform certificate",
				parsed.NormalizedPEM,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"rollbackCreated() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if deleteCalls != testCase.wantDeleteCalls {
				t.Fatalf(
					"deleteCalls = %d, want %d",
					deleteCalls,
					testCase.wantDeleteCalls,
				)
			}
			certificateCount := 0
			if certificateExists {
				certificateCount = 1
			}
			if certificateCount != testCase.wantCertificateCount {
				t.Fatalf(
					"certificateCount = %d, want %d",
					certificateCount,
					testCase.wantCertificateCount,
				)
			}
		})
	}
}

func TestSSLCertificateEnsureManageableRefusesUnsafeCertificates(
	t *testing.T,
) {
	t.Run("configured domain", func(t *testing.T) {
		resource := sslCertificateResource{}
		err := resource.ensureManageable(
			t.Context(),
			sslcertificate.Certificate{
				ID:                 "certificate-id",
				DomainIsConfigured: true,
			},
		)
		if err == nil {
			t.Fatal("ensureManageable() returned no error")
		}
	})

	t.Run("installed virtual host", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(
			response http.ResponseWriter,
			request *http.Request,
		) {
			if request.URL.Path != "/execute/SSL/installed_hosts" {
				t.Fatalf("unexpected path: %s", request.URL.Path)
			}
			writeSSLCertificateTestJSON(
				t,
				response,
				map[string]any{
					"status": 1,
					"data": []map[string]any{{
						"certificate": map[string]any{
							"id": "certificate-id",
						},
					}},
				},
			)
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
		resource := sslCertificateResource{
			client: sslcertificate.NewClient(baseClient),
		}
		err = resource.ensureManageable(
			t.Context(),
			sslcertificate.Certificate{ID: "certificate-id"},
		)
		if err == nil {
			t.Fatal("ensureManageable() returned no error")
		}
	})
}

func writeSSLCertificateTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func sslCertificateDedicatedHostTestResponse() map[string]any {
	return map[string]any{
		"status": 1,
		"data": map[string]any{
			"host": "dedicated.example.test",
			"certificate": map[string]any{
				"id":             "other-certificate",
				"domains":        []string{"dedicated.example.test"},
				"is_self_signed": 0,
				"not_before":     1700000000,
				"not_after":      1800000000,
				"modulus_length": 2048,
			},
		},
	}
}
