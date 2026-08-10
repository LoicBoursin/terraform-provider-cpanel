package sslcertificate

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestClientListsInstalledHostsWithSafeMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", request.URL.RawQuery)
		}
		switch request.URL.Path {
		case "/execute/SSL/installed_hosts":
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": []map[string]any{
					installedHostInventoryItem(
						"z.example.test",
						"certificate-z",
					),
					installedHostInventoryItem(
						"a.example.test",
						"certificate-a",
					),
				},
			})
		case "/execute/SSL/installed_host":
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": dedicatedHostInventoryItem(
					"a.example.test",
					"certificate-a",
				),
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	hosts, err := newTestClient(t, server).ListInstalledHosts(t.Context())
	if err != nil {
		t.Fatalf("ListInstalledHosts() error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("len(hosts) = %d, want 2", len(hosts))
	}
	host := hosts[0]
	if host.ServerName != "a.example.test" ||
		!slices.Equal(
			host.Domains,
			[]string{"a.example.test", "www.a.example.test"},
		) ||
		!slices.Equal(
			host.FQDNs,
			[]string{"a.example.test", "mail.a.example.test"},
		) ||
		host.IsPrimaryOnIP == nil ||
		!*host.IsPrimaryOnIP ||
		host.MailSNIStatus == nil ||
		*host.MailSNIStatus ||
		host.NeedsSNI == nil ||
		!*host.NeedsSNI {
		t.Fatalf("host = %#v", host)
	}
	certificate := host.Certificate
	if certificate.ID != "certificate-a" ||
		!slices.Equal(
			certificate.Domains,
			[]string{"a.example.test", "www.a.example.test"},
		) ||
		certificate.AutoSSLProvider != "provider-id" ||
		certificate.AutoSSLProviderDisplayName != "Provider" ||
		certificate.IsAutoSSL == nil ||
		!*certificate.IsAutoSSL ||
		certificate.IsSelfSigned ||
		certificate.NotBefore != 1700000000 ||
		certificate.NotAfter != 1800000000 ||
		certificate.SignatureAlgorithm != "sha256WithRSAEncryption" ||
		certificate.ModulusLength == nil ||
		*certificate.ModulusLength != 2048 ||
		certificate.IssuerCommonName != "Example issuer" ||
		certificate.SubjectCommonName != "a.example.test" ||
		certificate.ValidationType != "dv" {
		t.Fatalf("certificate = %#v", certificate)
	}
}

func TestClientAddsDedicatedIPHostMissingFromPluralInventory(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/SSL/installed_hosts":
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data":   []map[string]any{},
			})
		case "/execute/SSL/installed_host":
			writeJSON(t, response, map[string]any{
				"status": 1,
				"data": dedicatedHostInventoryItem(
					"dedicated.example.test",
					"dedicated-certificate",
				),
			})
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	hosts, err := newTestClient(t, server).ListInstalledHosts(t.Context())
	if err != nil {
		t.Fatalf("ListInstalledHosts() error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts = %#v", hosts)
	}
	host := hosts[0]
	if host.ServerName != "dedicated.example.test" ||
		host.Domains != nil ||
		host.FQDNs != nil ||
		host.IsPrimaryOnIP != nil ||
		host.MailSNIStatus != nil ||
		host.NeedsSNI != nil ||
		host.Certificate.IsAutoSSL != nil {
		t.Fatalf("dedicated host = %#v", host)
	}
	if host.Certificate.ID != "dedicated-certificate" ||
		host.Certificate.IsSelfSigned ||
		len(host.Certificate.Domains) != 2 {
		t.Fatalf("dedicated certificate = %#v", host.Certificate)
	}
}

func TestClientRejectsIncompleteInstalledHostInventories(t *testing.T) {
	t.Parallel()

	valid := installedHostInventoryItem(
		"example.test",
		"certificate-id",
	)
	testCases := map[string]func(map[string]any){
		"missing server name": func(item map[string]any) {
			delete(item, "servername")
		},
		"null domains": func(item map[string]any) {
			item["domains"] = nil
		},
		"empty domains": func(item map[string]any) {
			item["domains"] = []string{}
		},
		"numeric server name": func(item map[string]any) {
			item["servername"] = 123
		},
		"numeric domain": func(item map[string]any) {
			item["domains"] = []any{123}
		},
		"scalar FQDNs": func(item map[string]any) {
			item["fqdns"] = "example.test"
		},
		"duplicate domain": func(item map[string]any) {
			item["domains"] = []string{"example.test", "example.test"}
		},
		"missing host flag": func(item map[string]any) {
			delete(item, "needs_sni")
		},
		"missing certificate": func(item map[string]any) {
			item["certificate"] = nil
		},
		"missing certificate domains": func(item map[string]any) {
			delete(installedCertificateInventory(item), "domains")
		},
		"invalid certificate flag": func(item map[string]any) {
			installedCertificateInventory(item)["is_autossl"] = 2
		},
		"negative validity": func(item map[string]any) {
			installedCertificateInventory(item)["not_after"] = -1
		},
		"reversed validity": func(item map[string]any) {
			installedCertificateInventory(item)["not_after"] = 1600000000
		},
		"zero modulus length": func(item map[string]any) {
			installedCertificateInventory(item)["modulus_length"] = 0
		},
	}
	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			item := cloneInstalledHostInventoryItem(valid)
			mutate(item)
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				_ *http.Request,
			) {
				writeJSON(t, response, map[string]any{
					"status": 1,
					"data":   []map[string]any{item},
				})
			}))
			defer server.Close()

			if _, err := newTestClient(t, server).ListInstalledHosts(
				t.Context(),
			); err == nil {
				t.Fatal("ListInstalledHosts() returned no error")
			}
		})
	}
}

func TestClientRejectsDuplicateInstalledHostIdentities(t *testing.T) {
	t.Parallel()

	item := installedHostInventoryItem(
		"example.test",
		"certificate-id",
	)
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data":   []map[string]any{item, item},
		})
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListInstalledHosts(t.Context())
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("ListInstalledHosts() error = %v", err)
	}
}

func TestClientPreservesCertificateECDSACurveMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeJSON(t, response, map[string]any{
			"status": 1,
			"data": []map[string]any{{
				"id":                   "certificate-id",
				"friendly_name":        "Certificate",
				"domains":              []string{"example.test"},
				"created":              1700000000,
				"not_before":           1700000001,
				"not_after":            1800000000,
				"key_algorithm":        "id-ecPublicKey",
				"ecdsa_curve_name":     "prime256v1",
				"is_self_signed":       0,
				"domain_is_configured": 1,
			}},
		})
	}))
	defer server.Close()

	certificates, err := newTestClient(t, server).List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(certificates) != 1 ||
		certificates[0].ECDSACurveName != "prime256v1" {
		t.Fatalf("certificates = %#v", certificates)
	}
}

func installedHostInventoryItem(
	serverName string,
	certificateID string,
) map[string]any {
	return map[string]any{
		"servername": serverName,
		"domains": []string{
			"www." + serverName,
			serverName,
		},
		"fqdns": []string{
			"mail." + serverName,
			serverName,
		},
		"is_primary_on_ip": 1,
		"mail_sni_status":  "0",
		"needs_sni":        true,
		"certificate_text": "ignored certificate text",
		"docroot":          "/ignored",
		"ip":               "192.0.2.1",
		"certificate": map[string]any{
			"id": certificateID,
			"domains": []string{
				"www." + serverName,
				serverName,
			},
			"auto_ssl_provider":              "provider-id",
			"auto_ssl_provider_display_name": "Provider",
			"is_autossl":                     1,
			"is_self_signed":                 false,
			"not_before":                     "1700000000",
			"not_after":                      1800000000,
			"signature_algorithm":            "sha256WithRSAEncryption",
			"modulus_length":                 "2048",
			"issuer.commonName":              "Example issuer",
			"subject.commonName":             serverName,
			"validation_type":                "dv",
			"certificate_text":               "ignored nested text",
			"modulus":                        "ignored modulus",
		},
	}
}

func dedicatedHostInventoryItem(
	serverName string,
	certificateID string,
) map[string]any {
	certificate := installedCertificateInventory(
		cloneInstalledHostInventoryItem(
			installedHostInventoryItem(serverName, certificateID),
		),
	)
	delete(certificate, "auto_ssl_provider")
	delete(certificate, "auto_ssl_provider_display_name")
	delete(certificate, "is_autossl")

	return map[string]any{
		"host":        serverName,
		"certificate": certificate,
	}
}

func cloneInstalledHostInventoryItem(
	item map[string]any,
) map[string]any {
	clone := make(map[string]any, len(item))
	for key, value := range item {
		clone[key] = value
	}
	certificate := installedCertificateInventory(item)
	certificateClone := make(map[string]any, len(certificate))
	for key, value := range certificate {
		certificateClone[key] = value
	}
	clone["certificate"] = certificateClone

	return clone
}

func installedCertificateInventory(
	item map[string]any,
) map[string]any {
	certificate, ok := item["certificate"].(map[string]any)
	if !ok {
		panic("test installed host certificate must be an object")
	}

	return certificate
}
