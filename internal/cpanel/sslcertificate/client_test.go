package sslcertificate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsCertificatesWithFlexibleScalars(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/SSL/list_certs",
		)
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{
					"id":                   "cert-b",
					"friendly_name":        "Beta certificate",
					"domains":              []string{"www.example.test", "example.test"},
					"created":              "1700000000",
					"not_before":           1700000001,
					"not_after":            nil,
					"serial":               42,
					"signature_algorithm":  nil,
					"key_algorithm":        "rsaEncryption",
					"modulus_length":       "2048",
					"is_self_signed":       "1",
					"issuer.commonName":    "Example issuer",
					"subject.commonName":   nil,
					"validation_type":      nil,
					"domain_is_configured": 0,
				},
				{
					"id":                  123,
					"friendly_name":       nil,
					"domains":             nil,
					"created":             nil,
					"not_before":          "1700000010",
					"not_after":           1700000020,
					"serial":              "abc",
					"signature_algorithm": "ecdsa-with-SHA256",
					"key_algorithm":       "id-ecPublicKey",
					"modulus_length":      nil,
					"is_self_signed":      false,
					"issuer": map[string]any{
						"commonName": 7,
					},
					"subject": map[string]any{
						"commonName": "example.test",
					},
					"validation_type":      "dv",
					"domain_is_configured": "1",
				},
			},
		})
	}))
	defer server.Close()

	certificates, err := newTestClient(t, server).List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(certificates) != 2 {
		t.Fatalf("len(certificates) = %d, want 2", len(certificates))
	}

	numericID := certificates[0]
	if numericID.ID != "123" ||
		numericID.FriendlyName != "" ||
		numericID.Created != 0 ||
		numericID.NotBefore != 1700000010 ||
		numericID.NotAfter != 1700000020 ||
		numericID.ModulusLength != 0 ||
		numericID.IsSelfSigned ||
		!numericID.DomainIsConfigured ||
		numericID.IssuerCommonName != "7" ||
		numericID.SubjectCommonName != "example.test" {
		t.Fatalf("numericID = %#v", numericID)
	}

	certificate := certificates[1]
	if certificate.ID != "cert-b" ||
		certificate.Created != 1700000000 ||
		certificate.NotBefore != 1700000001 ||
		certificate.NotAfter != 0 ||
		certificate.Serial != "42" ||
		certificate.SignatureAlgorithm != "" ||
		certificate.ModulusLength != 2048 ||
		!certificate.IsSelfSigned ||
		certificate.DomainIsConfigured ||
		certificate.IssuerCommonName != "Example issuer" ||
		certificate.SubjectCommonName != "" ||
		certificate.ValidationType != "" ||
		!slices.Equal(
			certificate.Domains,
			[]string{"example.test", "www.example.test"},
		) {
		t.Fatalf("certificate = %#v", certificate)
	}
}

func TestClientGetsCertificateAfterCanonicalListLookup(t *testing.T) {
	t.Parallel()

	certificatePEM, _ := testCertificateMaterial(t)
	parsed, err := ParsePEM(certificatePEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		switch requestCount {
		case 1:
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/SSL/list_certs",
			)
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"id":                   "cert-id",
					"friendly_name":        "List name",
					"domains":              []string{"old.example.test"},
					"created":              "1700000000",
					"not_before":           "1700000001",
					"not_after":            "1700000002",
					"serial":               "list-serial",
					"signature_algorithm":  "list-signature",
					"key_algorithm":        "list-key",
					"modulus_length":       "2048",
					"is_self_signed":       "0",
					"issuer.commonName":    "List issuer",
					"subject.commonName":   "old.example.test",
					"validation_type":      "dv",
					"domain_is_configured": "1",
				}},
			})
		case 2:
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/execute/SSL/show_cert",
			)
			if request.URL.Query().Get("id") != "cert-id" ||
				len(request.URL.Query()) != 1 {
				t.Errorf("query = %v", request.URL.Query())
			}
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": map[string]any{
					"cert": certificatePEM,
					"text": "human-readable certificate details",
					"details": map[string]any{
						"id":             "cert-id",
						"friendly_name":  "Show name",
						"domains":        []string{"example.test"},
						"not_before":     1700000101,
						"not_after":      "1700000102",
						"key_algorithm":  "show-key",
						"modulus_length": 4096,
						"is_self_signed": 1,
						"issuer": map[string]any{
							"commonName": "Show issuer",
						},
						"subject": map[string]any{
							"commonName": "example.test",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}
	}))
	defer server.Close()

	certificate, err := newTestClient(t, server).Get(
		t.Context(),
		"cert-id",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if certificate == nil {
		t.Fatal("Get() = nil")
	}
	if certificate.ID != "cert-id" ||
		certificate.FriendlyName != "Show name" ||
		certificate.CertificatePEM != parsed.NormalizedPEM ||
		certificate.FingerprintSHA256 != parsed.SHA256Fingerprint ||
		certificate.Created != 1700000000 ||
		certificate.NotBefore != 1700000101 ||
		certificate.NotAfter != 1700000102 ||
		certificate.Serial != "list-serial" ||
		certificate.SignatureAlgorithm != "list-signature" ||
		certificate.KeyAlgorithm != "show-key" ||
		certificate.ModulusLength != 4096 ||
		!certificate.IsSelfSigned ||
		certificate.IssuerCommonName != "Show issuer" ||
		certificate.SubjectCommonName != "example.test" ||
		certificate.ValidationType != "dv" ||
		!certificate.DomainIsConfigured {
		t.Fatalf("certificate = %#v", certificate)
	}
}

func TestClientGetReturnsNilWhenCertificateIsAbsent(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		assertRequest(
			t,
			request,
			http.MethodGet,
			"/execute/SSL/list_certs",
		)
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{{
				"id":                   "other-id",
				"friendly_name":        "Other",
				"domain_is_configured": 0,
			}},
		})
	}))
	defer server.Close()

	certificate, err := newTestClient(t, server).Get(
		t.Context(),
		"missing-id",
	)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if certificate != nil {
		t.Fatalf("Get() = %#v, want nil", certificate)
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
}

func TestClientUploadsCertificateWithExactPOSTForm(t *testing.T) {
	t.Parallel()

	certificatePEM, _ := testCertificateMaterial(t)
	parsed, err := ParsePEM(certificatePEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertRequest(
			t,
			request,
			http.MethodPost,
			"/execute/SSL/upload_cert",
		)
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}
		if request.Header.Get("Content-Type") !=
			"application/x-www-form-urlencoded" {
			t.Errorf(
				"Content-Type = %q",
				request.Header.Get("Content-Type"),
			)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if len(request.Form) != 2 ||
			request.Form.Get("crt") != parsed.NormalizedPEM ||
			request.Form.Get("friendly_name") != "Terraform certificate" {
			t.Errorf("form = %v", request.Form)
		}
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{{
				"id":                  "uploaded-id",
				"friendly_name":       "Terraform certificate",
				"domains":             []string{"example.test"},
				"created":             "1700000000",
				"not_before":          1700000001,
				"not_after":           "1700000002",
				"serial":              "abc",
				"signature_algorithm": "ecdsa-with-SHA256",
				"key_algorithm":       "id-ecPublicKey",
				"modulus_length":      nil,
				"is_self_signed":      "1",
				"issuer.commonName":   "example.test",
				"subject.commonName":  "example.test",
				"validation_type":     nil,
			}},
		})
	}))
	defer server.Close()

	certificate, err := newTestClient(t, server).Upload(
		t.Context(),
		certificatePEM,
		"Terraform certificate",
	)
	if err != nil {
		t.Fatalf("Upload() error: %v", err)
	}
	if certificate.ID != "uploaded-id" ||
		certificate.CertificatePEM != parsed.NormalizedPEM ||
		certificate.FingerprintSHA256 != parsed.SHA256Fingerprint {
		t.Fatalf("certificate = %#v", certificate)
	}
}

func TestClientRejectsInvalidUploadResponseCardinality(t *testing.T) {
	t.Parallel()

	certificatePEM, _ := testCertificateMaterial(t)
	testCases := map[string][]map[string]any{
		"empty": {},
		"multiple": {
			{"id": "first"},
			{"id": "second"},
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertRequest(
					t,
					request,
					http.MethodPost,
					"/execute/SSL/upload_cert",
				)
				writeJSON(t, response, map[string]any{
					"status": 1,
					"data":   data,
				})
			}))
			defer server.Close()

			if _, err := newTestClient(t, server).Upload(
				t.Context(),
				certificatePEM,
				"Terraform certificate",
			); err == nil {
				t.Fatal("Upload() returned no error")
			}
		})
	}
}

func TestClientRenamesAndDeletesCertificateWithExactPOSTParameters(
	t *testing.T,
) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requestCount++
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}

		switch requestCount {
		case 1:
			assertRequest(
				t,
				request,
				http.MethodPost,
				"/execute/SSL/set_cert_friendly_name",
			)
			if len(request.Form) != 2 ||
				request.Form.Get("id") != "cert-id" ||
				request.Form.Get("new_friendly_name") != "Renamed" {
				t.Errorf("rename form = %v", request.Form)
			}
		case 2:
			assertRequest(
				t,
				request,
				http.MethodPost,
				"/execute/SSL/delete_cert",
			)
			if len(request.Form) != 1 ||
				request.Form.Get("id") != "cert-id" {
				t.Errorf("delete form = %v", request.Form)
			}
		default:
			t.Fatalf("unexpected request %d: %s", requestCount, request.URL)
		}

		writeJSON(t, response, map[string]any{
			"status": 1,
			"data":   nil,
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.Rename(
		t.Context(),
		"cert-id",
		"Renamed",
	); err != nil {
		t.Fatalf("Rename() error: %v", err)
	}
	if err := client.Delete(t.Context(), "cert-id"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestClientDetectsInstalledCertificateWithFlexibleIDs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		id        string
		installed bool
	}{
		{name: "string", id: "installed-id", installed: true},
		{name: "number", id: "123", installed: true},
		{name: "dedicated", id: "dedicated-id", installed: true},
		{name: "absent", id: "missing-id", installed: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if request.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", request.Method)
				}
				if request.URL.RawQuery != "" {
					t.Errorf(
						"query = %q, want empty",
						request.URL.RawQuery,
					)
				}
				switch request.URL.Path {
				case "/execute/SSL/installed_hosts":
					writeJSON(t, response, map[string]any{
						"status": 1,
						"data": []map[string]any{
							{
								"certificate": map[string]any{
									"id": "installed-id",
								},
							},
							{"certificate": map[string]any{"id": 123}},
						},
					})
				case "/execute/SSL/installed_host":
					writeJSON(t, response, map[string]any{
						"status": 1,
						"data": dedicatedHostInventoryItem(
							"dedicated.example.test",
							"dedicated-id",
						),
					})
				default:
					t.Errorf("unexpected path %s", request.URL.Path)
				}
			}))
			defer server.Close()

			installed, err := newTestClient(t, server).IsInstalled(
				t.Context(),
				testCase.id,
			)
			if err != nil {
				t.Fatalf("IsInstalled() error: %v", err)
			}
			if installed != testCase.installed {
				t.Fatalf(
					"IsInstalled() = %t, want %t",
					installed,
					testCase.installed,
				)
			}
		})
	}
}

func TestClientRejectsDuplicateCertificateIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{
				{"id": "duplicate", "domain_is_configured": 0},
				{"id": "duplicate", "domain_is_configured": 0},
			},
		})
	}))
	defer server.Close()

	if _, err := newTestClient(t, server).List(t.Context()); err == nil {
		t.Fatal("List() returned no error")
	}
}

func TestClientRejectsIncompleteSafetyInventories(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		response map[string]any
		run      func(*Client) error
	}{
		{
			name:     "missing certificate inventory data",
			path:     "/execute/SSL/list_certs",
			response: map[string]any{"status": 1},
			run: func(client *Client) error {
				_, err := client.List(t.Context())

				return err
			},
		},
		{
			name: "missing configured-domain status",
			path: "/execute/SSL/list_certs",
			response: map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"id":            "certificate-id",
					"friendly_name": "Certificate",
				}},
			},
			run: func(client *Client) error {
				_, err := client.List(t.Context())

				return err
			},
		},
		{
			name:     "null installed-host inventory data",
			path:     "/execute/SSL/installed_hosts",
			response: map[string]any{"status": 1, "data": nil},
			run: func(client *Client) error {
				_, err := client.IsInstalled(
					t.Context(),
					"certificate-id",
				)

				return err
			},
		},
		{
			name: "missing installed certificate object",
			path: "/execute/SSL/installed_hosts",
			response: map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"certificate": nil,
				}},
			},
			run: func(client *Client) error {
				_, err := client.IsInstalled(
					t.Context(),
					"certificate-id",
				)

				return err
			},
		},
		{
			name: "missing installed certificate id",
			path: "/execute/SSL/installed_hosts",
			response: map[string]any{
				"status": 1,
				"data": []map[string]any{{
					"certificate": map[string]any{"id": nil},
				}},
			},
			run: func(client *Client) error {
				_, err := client.IsInstalled(
					t.Context(),
					"certificate-id",
				)

				return err
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				assertRequest(
					t,
					request,
					http.MethodGet,
					testCase.path,
				)
				writeJSON(t, response, testCase.response)
			}))
			defer server.Close()

			if err := testCase.run(newTestClient(t, server)); err == nil {
				t.Fatal("client call returned no error")
			}
		})
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
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

func writeJSON(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}
