// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelapachehandler "terraform-provider-cpanel/internal/cpanel/apachehandler"
	cpanelapitoken "terraform-provider-cpanel/internal/cpanel/apitoken"
	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
	cpanelcapabilities "terraform-provider-cpanel/internal/cpanel/capabilities"
	"terraform-provider-cpanel/internal/cpanel/cron"
	cpanelddns "terraform-provider-cpanel/internal/cpanel/ddns"
	cpaneldirectoryindex "terraform-provider-cpanel/internal/cpanel/directoryindex"
	cpaneldirectoryprivacy "terraform-provider-cpanel/internal/cpanel/directoryprivacy"
	cpaneldns "terraform-provider-cpanel/internal/cpanel/dns"
	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
	cpanelfileman "terraform-provider-cpanel/internal/cpanel/fileman"
	cpanelftp "terraform-provider-cpanel/internal/cpanel/ftp"
	cpanelipblock "terraform-provider-cpanel/internal/cpanel/ipblock"
	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
	cpanelmimetype "terraform-provider-cpanel/internal/cpanel/mimetype"
	"terraform-provider-cpanel/internal/cpanel/mysql"
	cpanelpassenger "terraform-provider-cpanel/internal/cpanel/passenger"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
	cpanelsslcertificate "terraform-provider-cpanel/internal/cpanel/sslcertificate"
	cpanelsslcsr "terraform-provider-cpanel/internal/cpanel/sslcsr"
	cpanelversioncontrol "terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

const providerConfig = ``

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"cpanel": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()

	for _, variable := range []string{"CPANEL_HOST", "CPANEL_USERNAME", "CPANEL_API_TOKEN"} {
		if os.Getenv(variable) == "" {
			t.Fatalf("%s must be set for acceptance tests", variable)
		}
	}

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := cpanelapachehandler.NewClient(client).ListUser(ctx); err != nil {
		t.Fatalf("verify Apache handler API access: %v", err)
	}
	if _, err := cpanelapitoken.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify API token access: %v", err)
	}
	if _, err := cpanelcapabilities.NewClient(client).Get(ctx); err != nil {
		t.Fatalf("verify account capabilities API access: %v", err)
	}
	if _, err := cpanelcalendar.NewClient(client).ListDelegates(ctx); err != nil {
		t.Fatalf("verify calendar delegation API access: %v", err)
	}
	if _, err := cron.NewClient(client).GetCronJobs(ctx); err != nil {
		t.Fatalf("verify Cron API access: %v", err)
	}
	directoryIndex, err := cpaneldirectoryindex.NewClient(client).Get(
		ctx,
		"public_html",
	)
	if err != nil {
		t.Fatalf("verify Directory Indexes API access: %v", err)
	}
	if directoryIndex == nil {
		t.Fatal("verify Directory Indexes API access: public_html not found")
	}
	directoryPrivacy, err := cpaneldirectoryprivacy.NewClient(client).Get(
		ctx,
		"public_html",
	)
	if err != nil {
		t.Fatalf("verify Directory Privacy API access: %v", err)
	}
	if directoryPrivacy == nil {
		t.Fatal("verify Directory Privacy API access: public_html not found")
	}
	if _, err := cpaneldirectoryprivacy.NewClient(client).ListUsers(
		ctx,
		"public_html",
	); err != nil {
		t.Fatalf("verify Directory Privacy user API access: %v", err)
	}
	if _, err := cpaneldomain.NewClient(client).ListSubdomains(ctx); err != nil {
		t.Fatalf("verify Domain API access: %v", err)
	}
	if _, err := cpaneldomain.NewClient(client).ListAddonDomains(ctx); err != nil {
		t.Fatalf("verify AddonDomain API access: %v", err)
	}
	if _, err := cpaneldomain.NewClient(client).ListDomainAliases(ctx); err != nil {
		t.Fatalf("verify domain alias API access: %v", err)
	}
	mainDomain, err := cpaneldomain.NewClient(client).GetMainDomain(ctx)
	if err != nil {
		t.Fatalf("read cPanel main domain: %v", err)
	}
	if _, err := cpaneldns.NewClient(client).ParseZone(ctx, mainDomain); err != nil {
		t.Fatalf("verify DNS API access: %v", err)
	}
	if _, err := cpanelddns.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify Dynamic DNS API access: %v", err)
	}
	if _, err := cpanelmail.NewClient(client).ListForwarders(ctx, mainDomain); err != nil {
		t.Fatalf("verify email forwarder API access: %v", err)
	}
	if _, err := cpanelmail.NewClient(client).ListDomainForwarders(ctx); err != nil {
		t.Fatalf("verify email domain forwarder API access: %v", err)
	}
	if _, err := cpanelmail.NewClient(client).ListAutoResponders(ctx, mainDomain); err != nil {
		t.Fatalf("verify email autoresponder API access: %v", err)
	}
	if _, err := cpanelmail.NewClient(client).ListMailingLists(ctx, mainDomain); err != nil {
		t.Fatalf("verify email mailing list API access: %v", err)
	}
	if _, err := postgresql.NewClient(client).GetDatabases(ctx); err != nil {
		t.Fatalf("verify PostgreSQL API access: %v", err)
	}
	if _, err := mysql.NewClient(client).GetRestrictions(ctx); err != nil {
		t.Fatalf("verify MySQL API access: %v", err)
	}
	if _, err := mysql.NewClient(client).ListRemoteHosts(ctx); err != nil {
		t.Fatalf("verify remote MySQL host API access: %v", err)
	}
	if _, err := cpanelmail.NewClient(client).ListMailDomains(ctx); err != nil {
		t.Fatalf("verify Email API access: %v", err)
	}
	if _, err := cpanelftp.NewClient(client).ListAccounts(ctx); err != nil {
		t.Fatalf("verify FTP API access: %v", err)
	}
	if _, err := cpanelipblock.NewClient(client).ListAddresses(ctx); err != nil {
		t.Fatalf("verify IP blocker API access: %v", err)
	}
	if _, err := cpanellocale.NewClient(client).GetCurrent(ctx); err != nil {
		t.Fatalf("verify Locale API access: %v", err)
	}
	if _, err := cpanellogmanager.NewClient(client).Get(ctx); err != nil {
		t.Fatalf("verify LogManager API access: %v", err)
	}
	if _, err := cpanelmimetype.NewClient(client).ListUser(ctx); err != nil {
		t.Fatalf("verify MIME type API access: %v", err)
	}
	if _, err := cpanelredirect.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify Redirects API access: %v", err)
	}
	if _, err := cpanelversioncontrol.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify Version Control API access: %v", err)
	}
	if _, err := cpanelpassenger.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify Passenger API access: %v", err)
	}
	if _, err := cpanelsslcertificate.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify SSL certificate API access: %v", err)
	}
	if _, err := cpanelsslcsr.NewClient(client).List(ctx); err != nil {
		t.Fatalf("verify SSL CSR API access: %v", err)
	}
}

func testAccClient() (*cpanel.Client, error) {
	return cpanel.NewClient(
		os.Getenv("CPANEL_HOST"),
		os.Getenv("CPANEL_USERNAME"),
		os.Getenv("CPANEL_API_TOKEN"),
	)
}

func testAccPostgreSQLName(kind string) string {
	return fmt.Sprintf(
		"%s_tf%s%s",
		os.Getenv("CPANEL_USERNAME"),
		kind,
		acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum),
	)
}

func testAccMySQLName(kind string) string {
	return fmt.Sprintf(
		"%s_tf%s%s",
		os.Getenv("CPANEL_USERNAME"),
		kind,
		acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum),
	)
}

func testAccAPITokenName(kind string) string {
	name := fmt.Sprintf(
		"tfcpaneltoken%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(12, acctest.CharSetAlphaNum)),
	)

	testAccRegisterAPITokenCandidate(name, time.Now().Add(-5*time.Minute).Unix())

	return name
}

func testAccDynamicDNSDomain(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelddns%s%s.%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccRedirectSource(kind string) string {
	return fmt.Sprintf(
		"/tfcpanelredirect-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccRedirectDestination(kind string) string {
	return fmt.Sprintf(
		"https://example.net/tfcpanelredirect-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccMIMEType(kind string) string {
	return fmt.Sprintf(
		"application/x-tfcpanel-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccMIMEExtension(kind string) string {
	return fmt.Sprintf(
		".tfcpanelmime%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccApacheHandlerExtension(kind string) string {
	return fmt.Sprintf(
		".tfcpanelhandler%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccApacheHandlerName(kind string) string {
	return fmt.Sprintf(
		"tfcpanel-handler-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccDirectoryIndexDirectory(kind string) string {
	return fmt.Sprintf(
		"public_html/tfcpanel-index-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccFilesystemDirectoryPath(kind string) string {
	return fmt.Sprintf(
		"public_html/tfcpanel-fs-dir-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccDirectoryPrivacyDirectory(kind string) string {
	return fmt.Sprintf(
		"public_html/tfcpanel-privacy-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccDirectoryPrivacyUsername(kind string) string {
	return fmt.Sprintf(
		"tfcpanelprivacy%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccGitRepositoryRoot(kind string) string {
	return fmt.Sprintf(
		"tfcpanel-git-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccEmailAddress(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanel%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccEmailForwarderAddress(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelfwd%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccEmailForwarderDestination(kind string) string {
	return fmt.Sprintf(
		"tfcpanel-%s-%s@example.net",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccEmailDomainForwarderDestination(kind string) string {
	return fmt.Sprintf(
		"tfcpaneldomainfwd%s%s.example.net",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccEmailAutoResponderAddress(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelauto%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccEmailFilterAddress(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelfilter%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccEmailFilterName(kind string) string {
	return fmt.Sprintf(
		"tfcpanelfilter%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
	)
}

func testAccEmailMailingListAddress(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanellist%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccFTPUsername(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelftp%s%s@%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccFTPHomeDirectory(kind string) string {
	return fmt.Sprintf(
		"tfcpanel-ftp-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccSubdomain(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelsub%s%s.%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
		testAccMainDomain(t),
	)
}

func testAccDomainDocumentRoot(kind string) string {
	return fmt.Sprintf(
		"public_html/tfcpanel-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccAddonDomain(t *testing.T, kind string) (string, string) {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	internalSubdomain := fmt.Sprintf(
		"tfcpaneladdon%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
	)

	return internalSubdomain + ".example.test", internalSubdomain
}

func testAccDomainAlias(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpanelalias%s%s.example.test",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
	)
}

func testAccDNSRecordName(t *testing.T, kind string) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	return fmt.Sprintf(
		"tfcpaneldns%s%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(6, acctest.CharSetAlphaNum)),
	)
}

func testAccDNSRecordData(kind string) string {
	return fmt.Sprintf(
		"terraform-provider-cpanel-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)),
	)
}

func testAccCheckAPITokenExists(
	name string,
	expectedExpiresAt int64,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		token, err := cpanelapitoken.NewClient(client).Get(ctx, name)
		if err != nil {
			return err
		}
		if token == nil {
			return fmt.Errorf("API token %q was not found", name)
		}
		if token.ExpiresAt.ValueOrZero() != expectedExpiresAt {
			return fmt.Errorf(
				"API token %q expires at %d; want %d",
				name,
				token.ExpiresAt.ValueOrZero(),
				expectedExpiresAt,
			)
		}
		if token.HasFullAccess != 1 {
			return fmt.Errorf("API token %q does not have full access", name)
		}

		return nil
	}
}

func testAccCheckAPITokensDestroyed(
	names ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		apiTokenClient := cpanelapitoken.NewClient(client)
		for _, name := range names {
			token, err := apiTokenClient.Get(ctx, name)
			if err != nil {
				return err
			}
			if token != nil {
				return fmt.Errorf("API token %q still exists", name)
			}
		}

		return nil
	}
}

func testAccRevokeAPIToken(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelapitoken.NewClient(client).Revoke(ctx, name); err != nil {
		t.Fatalf("revoke API token %q: %v", name, err)
	}
}

func testAccCheckDynamicDNSExists(
	domain string,
	expectedDescription string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		dynamicDomain, err := cpanelddns.NewClient(client).Get(ctx, domain)
		if err != nil {
			return err
		}
		if dynamicDomain == nil {
			return fmt.Errorf("Dynamic DNS domain %q was not found", domain)
		}
		if dynamicDomain.Description != expectedDescription {
			return fmt.Errorf(
				"Dynamic DNS domain %q description is %q; want %q",
				domain,
				dynamicDomain.Description,
				expectedDescription,
			)
		}
		if dynamicDomain.ID == "" {
			return fmt.Errorf("Dynamic DNS domain %q has an empty ID", domain)
		}

		return nil
	}
}

func testAccCheckDynamicDNSDestroyed(
	domains ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		dynamicDNSClient := cpanelddns.NewClient(client)
		for _, domain := range domains {
			dynamicDomain, err := dynamicDNSClient.Get(ctx, domain)
			if err != nil {
				return err
			}
			if dynamicDomain != nil {
				return fmt.Errorf(
					"Dynamic DNS domain %q still exists",
					domain,
				)
			}
		}

		return nil
	}
}

func testAccSetDynamicDNSDescription(
	t *testing.T,
	domain string,
	description string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	dynamicDNSClient := cpanelddns.NewClient(client)
	dynamicDomain, err := dynamicDNSClient.Get(ctx, domain)
	if err != nil {
		t.Fatalf("read Dynamic DNS domain %q: %v", domain, err)
	}
	if dynamicDomain == nil {
		t.Fatalf("Dynamic DNS domain %q was not found", domain)
	}
	if err := dynamicDNSClient.SetDescription(
		ctx,
		dynamicDomain.ID,
		description,
	); err != nil {
		t.Fatalf("set Dynamic DNS domain %q description: %v", domain, err)
	}
}

func testAccRecreateDynamicDNS(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	dynamicDNSClient := cpanelddns.NewClient(client)
	dynamicDomain, err := dynamicDNSClient.Get(ctx, domain)
	if err != nil {
		t.Fatalf("read Dynamic DNS domain %q: %v", domain, err)
	}
	if dynamicDomain == nil {
		t.Fatalf("Dynamic DNS domain %q was not found", domain)
	}
	if _, err := dynamicDNSClient.Recreate(ctx, dynamicDomain.ID); err != nil {
		t.Fatalf("recreate Dynamic DNS domain %q webcall URL: %v", domain, err)
	}
}

func testAccDeleteDynamicDNS(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	dynamicDNSClient := cpanelddns.NewClient(client)
	dynamicDomain, err := dynamicDNSClient.Get(ctx, domain)
	if err != nil {
		t.Fatalf("read Dynamic DNS domain %q: %v", domain, err)
	}
	if dynamicDomain == nil {
		return
	}
	if _, err := dynamicDNSClient.Delete(ctx, dynamicDomain.ID); err != nil {
		t.Fatalf("delete Dynamic DNS domain %q: %v", domain, err)
	}
}

func testAccCheckRedirectExists(
	expected cpanelredirect.Definition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		redirect, err := cpanelredirect.NewClient(client).Get(
			ctx,
			expected.Domain,
			expected.Source,
		)
		if err != nil {
			return err
		}
		if redirect == nil {
			return fmt.Errorf(
				"redirect for %s%s was not found",
				expected.Domain,
				expected.Source,
			)
		}

		actual := cpanelredirect.Definition{
			Domain:      redirect.Domain,
			Source:      redirect.Source,
			Destination: redirect.Destination,
			Type:        redirect.Type,
			WWWMode:     cpanelredirect.WWWModeBoth,
			Wildcard:    redirect.Wildcard == 1,
		}
		if redirect.MatchWWW == 0 {
			actual.WWWMode = cpanelredirect.WWWModeWithout
		}
		if actual != expected {
			return fmt.Errorf(
				"redirect for %s%s is %#v; want %#v",
				expected.Domain,
				expected.Source,
				actual,
				expected,
			)
		}

		return nil
	}
}

func testAccCheckRedirectsDestroyed(
	domain string,
	sources ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		redirectClient := cpanelredirect.NewClient(client)
		for _, source := range sources {
			redirect, err := redirectClient.Get(ctx, domain, source)
			if err != nil {
				return err
			}
			if redirect != nil {
				return fmt.Errorf(
					"redirect for %s%s still exists",
					domain,
					source,
				)
			}
		}

		return nil
	}
}

func testAccReplaceRedirect(
	t *testing.T,
	definition cpanelredirect.Definition,
) {
	t.Helper()

	testAccDeleteRedirect(t, definition.Domain, definition.Source)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelredirect.NewClient(client).Add(ctx, definition); err != nil {
		t.Fatalf(
			"create redirect for %s%s: %v",
			definition.Domain,
			definition.Source,
			err,
		)
	}
}

func testAccDeleteRedirect(t *testing.T, domain string, source string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	redirectClient := cpanelredirect.NewClient(client)
	redirect, err := redirectClient.Get(ctx, domain, source)
	if err != nil {
		t.Fatalf("read redirect for %s%s: %v", domain, source, err)
	}
	if redirect == nil {
		return
	}
	if err := redirectClient.Delete(ctx, domain, source); err != nil {
		t.Fatalf("delete redirect for %s%s: %v", domain, source, err)
	}
}

func testAccCheckMIMETypeExists(
	expected cpanelmimetype.Definition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		apiMIMEType, err := cpanelmimetype.NewClient(client).Get(
			ctx,
			expected.Type,
		)
		if err != nil {
			return err
		}
		if apiMIMEType == nil {
			return fmt.Errorf("MIME type %q was not found", expected.Type)
		}

		actual := cpanelmimetype.Definition{
			Type:       apiMIMEType.Type,
			Extensions: apiMIMEType.Extensions(),
		}.Sorted()
		expected = expected.Sorted()
		if actual.Type != expected.Type ||
			!slices.Equal(actual.Extensions, expected.Extensions) {
			return fmt.Errorf(
				"MIME type %q is %#v; want %#v",
				expected.Type,
				actual,
				expected,
			)
		}
		if apiMIMEType.Origin != "user" {
			return fmt.Errorf(
				"MIME type %q origin is %q; want user",
				expected.Type,
				apiMIMEType.Origin,
			)
		}

		return nil
	}
}

func testAccCheckMIMETypesDestroyed(
	mimeTypes ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		mimeTypeClient := cpanelmimetype.NewClient(client)
		for _, mimeTypeName := range mimeTypes {
			apiMIMEType, err := mimeTypeClient.Get(ctx, mimeTypeName)
			if err != nil {
				return err
			}
			if apiMIMEType != nil {
				return fmt.Errorf(
					"MIME type %q still exists",
					mimeTypeName,
				)
			}
		}

		return nil
	}
}

func testAccReplaceMIMEType(
	t *testing.T,
	definition cpanelmimetype.Definition,
) {
	t.Helper()

	testAccDeleteMIMEType(t, definition.Type)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	mimeTypeClient := cpanelmimetype.NewClient(client)
	for _, extension := range definition.Sorted().Extensions {
		if err := mimeTypeClient.AddExtension(
			ctx,
			definition.Type,
			extension,
		); err != nil {
			t.Fatalf(
				"add extension %q to MIME type %q: %v",
				extension,
				definition.Type,
				err,
			)
		}
	}
}

func testAccDeleteMIMEType(t *testing.T, mimeTypeName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	mimeTypeClient := cpanelmimetype.NewClient(client)
	apiMIMEType, err := mimeTypeClient.Get(ctx, mimeTypeName)
	if err != nil {
		t.Fatalf("read MIME type %q: %v", mimeTypeName, err)
	}
	if apiMIMEType == nil {
		return
	}
	if err := mimeTypeClient.Delete(ctx, mimeTypeName); err != nil {
		t.Fatalf("delete MIME type %q: %v", mimeTypeName, err)
	}
}

func testAccCheckApacheHandlerExists(
	expected cpanelapachehandler.Definition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		apiHandler, err := cpanelapachehandler.NewClient(client).Get(
			ctx,
			expected.Extension,
		)
		if err != nil {
			return err
		}
		if apiHandler == nil {
			return fmt.Errorf(
				"Apache handler for extension %q was not found",
				expected.Extension,
			)
		}

		actual := cpanelapachehandler.Definition{
			Extension: apiHandler.Extension,
			Handler:   apiHandler.Handler,
		}
		if actual != expected {
			return fmt.Errorf(
				"Apache handler for extension %q is %#v; want %#v",
				expected.Extension,
				actual,
				expected,
			)
		}
		if apiHandler.Origin != "user" {
			return fmt.Errorf(
				"Apache handler for extension %q origin is %q; want user",
				expected.Extension,
				apiHandler.Origin,
			)
		}

		return nil
	}
}

func testAccCheckApacheHandlersDestroyed(
	extensions ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		handlerClient := cpanelapachehandler.NewClient(client)
		for _, extension := range extensions {
			apiHandler, err := handlerClient.Get(ctx, extension)
			if err != nil {
				return err
			}
			if apiHandler != nil {
				return fmt.Errorf(
					"Apache handler for extension %q still exists",
					extension,
				)
			}
		}

		return nil
	}
}

func testAccReplaceApacheHandler(
	t *testing.T,
	definition cpanelapachehandler.Definition,
) {
	t.Helper()

	testAccDeleteApacheHandler(t, definition.Extension)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelapachehandler.NewClient(client).Add(
		ctx,
		definition,
	); err != nil {
		t.Fatalf(
			"create Apache handler for extension %q: %v",
			definition.Extension,
			err,
		)
	}
}

func testAccDeleteApacheHandler(t *testing.T, extension string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	handlerClient := cpanelapachehandler.NewClient(client)
	apiHandler, err := handlerClient.Get(ctx, extension)
	if err != nil {
		t.Fatalf("read Apache handler for extension %q: %v", extension, err)
	}
	if apiHandler == nil {
		return
	}
	if err := handlerClient.Delete(ctx, extension); err != nil {
		t.Fatalf("delete Apache handler for extension %q: %v", extension, err)
	}
}

func testAccCreateDirectory(t *testing.T, directory string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	indexClient := cpaneldirectoryindex.NewClient(client)
	existing, err := indexClient.Get(ctx, directory)
	if err != nil {
		t.Fatalf("read directory %q: %v", directory, err)
	}
	if existing != nil {
		return
	}

	homeDirectory, err := indexClient.HomeDirectory(ctx)
	if err != nil {
		t.Fatalf("read cPanel account home directory: %v", err)
	}
	parentDirectory := path.Dir(directory)
	if parentDirectory == "." {
		parentDirectory = ""
	}

	response := struct{}{}
	if err := client.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		"mkdir",
		map[string]string{
			"name":        path.Base(directory),
			"path":        path.Join(homeDirectory, parentDirectory),
			"permissions": "0755",
		},
		&response,
	); err != nil {
		t.Fatalf("create directory %q: %v", directory, err)
	}

	created, err := indexClient.Get(ctx, directory)
	if err != nil {
		t.Fatalf("verify directory %q: %v", directory, err)
	}
	if created == nil {
		t.Fatalf("directory %q was not found after creation", directory)
	}
}

func testAccDeleteDirectory(t *testing.T, directory string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := testAccRemoveDirectory(ctx, client, directory); err != nil {
		t.Fatalf("delete directory %q: %v", directory, err)
	}
}

func testAccRemoveDirectory(
	ctx context.Context,
	client *cpanel.Client,
	directory string,
) error {
	indexClient := cpaneldirectoryindex.NewClient(client)
	existing, err := indexClient.Get(ctx, directory)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", directory, err)
	}
	if existing == nil {
		return nil
	}

	response := struct{}{}
	if err := client.ExecuteAPI2Operation(
		ctx,
		http.MethodPost,
		cpanel.ModuleFileman,
		"fileop",
		map[string]string{
			"doubledecode": "0",
			"op":           "unlink",
			"sourcefiles":  directory,
		},
		&response,
	); err != nil {
		return fmt.Errorf("delete directory %q: %w", directory, err)
	}

	remaining, err := indexClient.Get(ctx, directory)
	if err != nil {
		return fmt.Errorf("verify directory %q deletion: %w", directory, err)
	}
	if remaining != nil {
		return fmt.Errorf("directory %q still exists after deletion", directory)
	}

	return nil
}

func testAccSetDirectoryIndex(
	t *testing.T,
	definition cpaneldirectoryindex.Definition,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, err := cpaneldirectoryindex.NewClient(client).Set(
		ctx,
		definition.Directory,
		definition.Type,
	); err != nil {
		t.Fatalf(
			"set directory %q indexing to %q: %v",
			definition.Directory,
			definition.Type,
			err,
		)
	}
}

func testAccCheckDirectoryIndexExists(
	expected cpaneldirectoryindex.Definition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		apiIndex, err := cpaneldirectoryindex.NewClient(client).Get(
			ctx,
			expected.Directory,
		)
		if err != nil {
			return err
		}
		if apiIndex == nil {
			return fmt.Errorf(
				"directory %q was not found",
				expected.Directory,
			)
		}
		if apiIndex.Type != expected.Type {
			return fmt.Errorf(
				"directory %q indexing type is %q; want %q",
				expected.Directory,
				apiIndex.Type,
				expected.Type,
			)
		}

		return nil
	}
}

func testAccCheckDirectoryIndexesDestroyed(
	directories ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		indexClient := cpaneldirectoryindex.NewClient(client)
		var validationErr error
		for _, directory := range directories {
			apiIndex, err := indexClient.Get(ctx, directory)
			if err != nil && validationErr == nil {
				validationErr = err
			}
			if apiIndex != nil &&
				apiIndex.Type != cpaneldirectoryindex.IndexTypeInherit &&
				validationErr == nil {
				validationErr = fmt.Errorf(
					"directory %q indexing type is %q after destroy; want inherit",
					directory,
					apiIndex.Type,
				)
			}
			if err := testAccRemoveDirectory(
				ctx,
				client,
				directory,
			); err != nil && validationErr == nil {
				validationErr = err
			}
		}

		return validationErr
	}
}

func testAccCheckFilesystemDirectory(
	directoryPath string,
	owned bool,
) resource.TestCheckFunc {
	return testAccCheckFilesystemDirectoryMarker(directoryPath, owned)
}

func testAccCheckFilesystemDirectoryMarker(
	directoryPath string,
	markerExpected bool,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		filemanClient := cpanelfileman.NewClient(client)
		directory, err := filemanClient.GetDirectory(ctx, directoryPath)
		if err != nil {
			return err
		}
		if directory == nil {
			return fmt.Errorf(
				"filesystem directory %q was not found",
				directoryPath,
			)
		}

		marker, err := filemanClient.GetTextFile(
			ctx,
			filesystemDirectoryMarkerPath(directoryPath),
		)
		if err != nil {
			return err
		}
		if markerExpected && marker == nil {
			return fmt.Errorf(
				"filesystem directory %q has no ownership marker",
				directoryPath,
			)
		}
		if !markerExpected && marker != nil {
			return fmt.Errorf(
				"filesystem directory %q unexpectedly has an ownership marker",
				directoryPath,
			)
		}

		return nil
	}
}

func testAccWriteFilesystemDirectoryMarker(
	t *testing.T,
	directoryPath string,
) {
	t.Helper()

	const token = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf(
			"encode filesystem directory %q ownership marker: %v",
			directoryPath,
			err,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	filemanClient := cpanelfileman.NewClient(client)
	unlock := filemanClient.LockMutations()
	defer unlock()

	if _, err := filemanClient.SaveTextFile(
		ctx,
		filesystemDirectoryMarkerPath(directoryPath),
		content,
	); err != nil {
		t.Fatalf(
			"write filesystem directory %q ownership marker: %v",
			directoryPath,
			err,
		)
	}
}

func testAccDeleteFilesystemDirectory(
	t *testing.T,
	directoryPath string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	filemanClient := cpanelfileman.NewClient(client)
	unlock := filemanClient.LockMutations()
	defer unlock()

	entries, err := filemanClient.ListDirectory(ctx, directoryPath)
	if err != nil {
		t.Fatalf("list filesystem directory %q: %v", directoryPath, err)
	}
	for _, entry := range entries {
		if err := filemanClient.DeletePath(ctx, entry.Path); err != nil {
			t.Fatalf(
				"delete filesystem entry %q: %v",
				entry.Path,
				err,
			)
		}
	}
	if err := filemanClient.DeletePath(ctx, directoryPath); err != nil {
		t.Fatalf("delete filesystem directory %q: %v", directoryPath, err)
	}
}

func testAccCreateUnmarkedFilesystemDirectory(
	t *testing.T,
	directoryPath string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	filemanClient := cpanelfileman.NewClient(client)
	unlock := filemanClient.LockMutations()
	defer unlock()

	existing, err := filemanClient.GetDirectory(ctx, directoryPath)
	if err != nil {
		t.Fatalf("read filesystem directory %q: %v", directoryPath, err)
	}
	if existing != nil {
		return
	}
	if _, err := filemanClient.CreateDirectory(
		ctx,
		directoryPath,
	); err != nil {
		t.Fatalf("create unmarked filesystem directory %q: %v", directoryPath, err)
	}
}

func testAccCheckFilesystemDirectoriesDestroyed(
	directories ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		filemanClient := cpanelfileman.NewClient(client)
		for _, directoryPath := range directories {
			directory, err := filemanClient.GetDirectory(ctx, directoryPath)
			if err != nil {
				return err
			}
			if directory != nil {
				return fmt.Errorf(
					"filesystem directory %q still exists after destroy",
					directoryPath,
				)
			}
		}

		return nil
	}
}

func testAccSetDirectoryPrivacy(
	t *testing.T,
	definition cpaneldirectoryprivacy.Definition,
	enabled bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, err := cpaneldirectoryprivacy.NewClient(client).Configure(
		ctx,
		definition.Directory,
		definition.AuthName,
		enabled,
	); err != nil {
		t.Fatalf(
			"configure directory privacy for %q: %v",
			definition.Directory,
			err,
		)
	}
}

func testAccCheckDirectoryPrivacyExists(
	expected cpaneldirectoryprivacy.Definition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		apiPrivacy, err := cpaneldirectoryprivacy.NewClient(client).Get(
			ctx,
			expected.Directory,
		)
		if err != nil {
			return err
		}
		if apiPrivacy == nil {
			return fmt.Errorf(
				"directory %q was not found",
				expected.Directory,
			)
		}
		if !apiPrivacy.Protected {
			return fmt.Errorf(
				"directory %q is not protected",
				expected.Directory,
			)
		}
		if apiPrivacy.AuthName != expected.AuthName {
			return fmt.Errorf(
				"directory %q auth name is %q; want %q",
				expected.Directory,
				apiPrivacy.AuthName,
				expected.AuthName,
			)
		}

		return nil
	}
}

func testAccCheckDirectoryPrivacyDisabled(
	directory string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		apiPrivacy, err := cpaneldirectoryprivacy.NewClient(client).Get(
			ctx,
			directory,
		)
		if err != nil {
			return err
		}
		if apiPrivacy != nil && apiPrivacy.Protected {
			return fmt.Errorf(
				"directory %q is still protected",
				directory,
			)
		}

		return nil
	}
}

func testAccCheckDirectoryPrivaciesDestroyed(
	directories ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		privacyClient := cpaneldirectoryprivacy.NewClient(client)
		var validationErr error
		for _, directory := range directories {
			apiPrivacy, err := privacyClient.Get(ctx, directory)
			if err != nil && validationErr == nil {
				validationErr = err
			}
			if apiPrivacy != nil &&
				apiPrivacy.Protected &&
				validationErr == nil {
				validationErr = fmt.Errorf(
					"directory %q is still protected after destroy",
					directory,
				)
			}

			for _, cleanupDirectory := range []string{
				directory,
				path.Join(".htpasswds", directory),
			} {
				if err := testAccRemoveDirectory(
					ctx,
					client,
					cleanupDirectory,
				); err != nil && validationErr == nil {
					validationErr = err
				}
			}
		}

		return validationErr
	}
}

func testAccDeleteDirectoryPrivacyUser(
	t *testing.T,
	directory string,
	username string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	privacyClient := cpaneldirectoryprivacy.NewClient(client)
	apiUser, err := privacyClient.GetUser(ctx, directory, username)
	if err != nil {
		t.Fatalf(
			"read Directory Privacy user %q for %q: %v",
			username,
			directory,
			err,
		)
	}
	if apiUser == nil {
		return
	}
	if err := privacyClient.DeleteUser(ctx, directory, username); err != nil {
		t.Fatalf(
			"delete Directory Privacy user %q for %q: %v",
			username,
			directory,
			err,
		)
	}
}

func testAccCheckDirectoryPrivacyUserExists(
	expected cpaneldirectoryprivacy.UserDefinition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		apiUser, err := cpaneldirectoryprivacy.NewClient(client).GetUser(
			ctx,
			expected.Directory,
			expected.Username,
		)
		if err != nil {
			return err
		}
		if apiUser == nil {
			return fmt.Errorf(
				"Directory Privacy user %q was not found for %q",
				expected.Username,
				expected.Directory,
			)
		}

		return nil
	}
}

func testAccCheckDirectoryPrivacyUserMissing(
	directory string,
	username string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		apiUser, err := cpaneldirectoryprivacy.NewClient(client).GetUser(
			ctx,
			directory,
			username,
		)
		if err != nil {
			return err
		}
		if apiUser != nil {
			return fmt.Errorf(
				"Directory Privacy user %q still exists for %q",
				username,
				directory,
			)
		}

		return nil
	}
}

func testAccCheckDirectoryPrivacyUsersDestroyed(
	definitions ...cpaneldirectoryprivacy.UserDefinition,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		privacyClient := cpaneldirectoryprivacy.NewClient(client)
		directorySet := map[string]struct{}{}
		var validationErr error
		for _, definition := range definitions {
			directorySet[definition.Directory] = struct{}{}
			apiUser, err := privacyClient.GetUser(
				ctx,
				definition.Directory,
				definition.Username,
			)
			if err != nil && validationErr == nil {
				validationErr = err
			}
			if apiUser != nil && validationErr == nil {
				validationErr = fmt.Errorf(
					"Directory Privacy user %q still exists for %q after destroy",
					definition.Username,
					definition.Directory,
				)
			}
		}

		directories := make([]string, 0, len(directorySet))
		for directory := range directorySet {
			directories = append(directories, directory)
		}
		slices.Sort(directories)
		if cleanupErr := testAccCheckDirectoryPrivaciesDestroyed(
			directories...,
		)(state); cleanupErr != nil && validationErr == nil {
			validationErr = cleanupErr
		}

		return validationErr
	}
}

func testAccSetGitRepositoryName(
	t *testing.T,
	repositoryRoot string,
	name string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, err := cpanelversioncontrol.NewClient(client).Update(
		ctx,
		repositoryRoot,
		name,
	); err != nil {
		t.Fatalf(
			"update Git repository %q name to %q: %v",
			repositoryRoot,
			name,
			err,
		)
	}
}

func testAccDeleteGitRepository(
	t *testing.T,
	repositoryRoot string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	versionControlClient := cpanelversioncontrol.NewClient(client)
	repository, err := versionControlClient.Get(ctx, repositoryRoot)
	if err != nil {
		t.Fatalf("read Git repository %q: %v", repositoryRoot, err)
	}
	if repository == nil {
		if err := versionControlClient.DeleteDirectory(
			ctx,
			repositoryRoot,
		); err != nil {
			t.Fatalf(
				"delete Git repository directory %q: %v",
				repositoryRoot,
				err,
			)
		}

		return
	}
	if err := versionControlClient.Delete(ctx, repositoryRoot); err != nil {
		t.Fatalf("unregister Git repository %q: %v", repositoryRoot, err)
	}
	if err := versionControlClient.DeleteDirectory(
		ctx,
		repositoryRoot,
	); err != nil {
		t.Fatalf(
			"delete Git repository directory %q: %v",
			repositoryRoot,
			err,
		)
	}
}

func testAccCheckGitRepositoryExists(
	expected cpanelversioncontrol.Definition,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		repository, err := cpanelversioncontrol.NewClient(client).Get(
			ctx,
			expected.RepositoryRoot,
		)
		if err != nil {
			return err
		}
		if repository == nil {
			return fmt.Errorf(
				"Git repository %q was not found",
				expected.RepositoryRoot,
			)
		}
		if repository.Name != expected.Name {
			return fmt.Errorf(
				"Git repository %q name is %q; want %q",
				expected.RepositoryRoot,
				repository.Name,
				expected.Name,
			)
		}
		if repository.SourceRepositoryURL != expected.SourceRepositoryURL {
			return fmt.Errorf(
				"Git repository %q source URL does not match",
				expected.RepositoryRoot,
			)
		}

		return nil
	}
}

func testAccCheckDirectoryEntryExists(
	directory string,
	entryName string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		response := cpaneldirectoryindex.FileListResponse{}
		if err := client.ExecuteUAPIOperation(
			ctx,
			http.MethodGet,
			cpanel.ModuleFileman,
			"list_files",
			map[string]string{
				"dir":         directory,
				"limit":       "100000",
				"show_hidden": "1",
			},
			&response,
		); err != nil {
			return err
		}
		for _, entry := range response.Data {
			if entry.File == entryName {
				return nil
			}
		}

		return fmt.Errorf(
			"directory %q does not contain %q",
			directory,
			entryName,
		)
	}
}

func testAccCheckGitRepositoryMissing(
	repositoryRoot string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		versionControlClient := cpanelversioncontrol.NewClient(client)
		repository, err := versionControlClient.Get(ctx, repositoryRoot)
		if err != nil {
			return err
		}
		if repository != nil {
			return fmt.Errorf(
				"Git repository %q still exists",
				repositoryRoot,
			)
		}
		rootExists, err := versionControlClient.RootExists(ctx, repositoryRoot)
		if err != nil {
			return err
		}
		if rootExists {
			return fmt.Errorf(
				"Git repository directory %q still exists",
				repositoryRoot,
			)
		}

		return nil
	}
}

func testAccCheckGitRepositoriesDestroyed(
	repositoryRoots ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		versionControlClient := cpanelversioncontrol.NewClient(client)
		var validationErr error
		for _, repositoryRoot := range repositoryRoots {
			repository, err := versionControlClient.Get(ctx, repositoryRoot)
			if err != nil && validationErr == nil {
				validationErr = err
			}
			if repository != nil {
				if validationErr == nil {
					validationErr = fmt.Errorf(
						"Git repository %q still exists after destroy",
						repositoryRoot,
					)
				}
				if err := versionControlClient.Delete(
					ctx,
					repositoryRoot,
				); err != nil && validationErr == nil {
					validationErr = err
				}
			}

			rootExists, err := versionControlClient.RootExists(
				ctx,
				repositoryRoot,
			)
			if err != nil && validationErr == nil {
				validationErr = err
			}
			if rootExists && validationErr == nil {
				validationErr = fmt.Errorf(
					"Git repository directory %q still exists after destroy",
					repositoryRoot,
				)
			}
			if rootExists {
				if err := versionControlClient.DeleteDirectory(
					ctx,
					repositoryRoot,
				); err != nil && validationErr == nil {
					validationErr = err
				}
			}

			topLevelDirectory := strings.Split(repositoryRoot, "/")[0]
			if topLevelDirectory != repositoryRoot &&
				strings.HasPrefix(topLevelDirectory, "tfcpanel-git-") {
				if err := versionControlClient.DeleteDirectory(
					ctx,
					topLevelDirectory,
				); err != nil && validationErr == nil {
					validationErr = err
				}
			}
		}

		return validationErr
	}
}

func testAccCheckIPBlockExists(
	address string,
	expectedStart string,
	expectedEnd string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		blockedAddress, err := cpanelipblock.NewClient(client).GetAddress(ctx, address)
		if err != nil {
			return err
		}
		if blockedAddress == nil {
			return fmt.Errorf("IP block %q was not found", address)
		}

		start, err := cpanelipblock.NormalizeAddress(blockedAddress.Start)
		if err != nil {
			return err
		}
		end, err := cpanelipblock.NormalizeAddress(blockedAddress.End)
		if err != nil {
			return err
		}
		if start != expectedStart || end != expectedEnd {
			return fmt.Errorf(
				"IP block %q spans %q to %q, want %q to %q",
				address,
				start,
				end,
				expectedStart,
				expectedEnd,
			)
		}

		return nil
	}
}

func testAccCheckIPBlocksDestroyed(
	addresses ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		ipBlockClient := cpanelipblock.NewClient(client)
		for _, address := range addresses {
			blockedAddress, err := ipBlockClient.GetAddress(ctx, address)
			if err != nil {
				return err
			}
			if blockedAddress != nil {
				return fmt.Errorf("IP block %q still exists", address)
			}
		}

		return nil
	}
}

func testAccDeleteIPBlock(t *testing.T, address string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelipblock.NewClient(client).RemoveAddress(ctx, address); err != nil {
		t.Fatalf("delete IP block %q: %v", address, err)
	}
}

func testAccMainDomain(t *testing.T) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	var response struct {
		Data struct {
			MainDomain string `json:"main_domain"`
		} `json:"data"`
	}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		"DomainInfo",
		"list_domains",
		map[string]string{},
		&response,
	); err != nil {
		t.Fatalf("read cPanel main domain: %v", err)
	}
	if response.Data.MainDomain == "" {
		t.Fatal("cPanel main domain is empty")
	}

	return response.Data.MainDomain
}

func testAccCronCommand(kind string) string {
	return fmt.Sprintf(
		"/bin/true # terraform-provider-cpanel-%s-%s",
		kind,
		acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum),
	)
}

func testAccImportStateIDFromAttribute(resourceName, attribute string) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}

		value := resourceState.Primary.Attributes[attribute]
		if value == "" {
			return "", fmt.Errorf("attribute %s.%s is empty", resourceName, attribute)
		}

		return value, nil
	}
}

func testAccDNSRecordImportStateID(
	resourceName string,
	zone string,
) resource.ImportStateIdFunc {
	return func(state *terraform.State) (string, error) {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}

		lineIndex := resourceState.Primary.Attributes["line_index"]
		if lineIndex == "" {
			return "", fmt.Errorf("attribute %s.line_index is empty", resourceName)
		}

		return zone + "/" + lineIndex, nil
	}
}

func testAccCheckCronJobExists(command string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		cronJobs, err := cron.NewClient(client).GetCronJobs(ctx)
		if err != nil {
			return err
		}

		for _, cronJob := range cronJobs.CpanelResult.Data {
			if cronJob.Command == command {
				return nil
			}
		}

		return fmt.Errorf("cron job with command %q was not found", command)
	}
}

func testAccCheckCronJobsDestroyed(commands ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		cronJobs, err := cron.NewClient(client).GetCronJobs(ctx)
		if err != nil {
			return err
		}

		for _, cronJob := range cronJobs.CpanelResult.Data {
			if slices.Contains(commands, cronJob.Command) {
				return fmt.Errorf("cron job with command %q still exists", cronJob.Command)
			}
		}

		return nil
	}
}

func testAccDeleteCronJobs(t *testing.T, commands ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	cronClient := cron.NewClient(client)
	cronJobs, err := cronClient.GetCronJobs(ctx)
	if err != nil {
		t.Fatalf("list cron jobs: %v", err)
	}

	for _, cronJob := range cronJobs.CpanelResult.Data {
		if !slices.Contains(commands, cronJob.Command) {
			continue
		}

		if _, err := cronClient.DeleteCronJob(
			ctx,
			cron.CronJobDeleteModel{LineKey: string(cronJob.LineKey)},
		); err != nil {
			t.Fatalf("delete cron job %q: %v", cronJob.LineKey, err)
		}
	}
}

func testAccCheckPostgreSQLUserExists(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		exists, err := postgresql.NewClient(client).UserExists(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("PostgreSQL user %q was not found", name)
		}

		return nil
	}
}

func testAccCheckPostgreSQLUsersDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		users, err := postgresql.NewClient(client).GetUsers(ctx)
		if err != nil {
			return err
		}

		for _, user := range users.Data {
			if slices.Contains(names, user) {
				return fmt.Errorf("PostgreSQL user %q still exists", user)
			}
		}

		return nil
	}
}

func testAccDeletePostgreSQLUser(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if _, err := postgresql.NewClient(client).DeleteUser(
		ctx,
		postgresql.UserDeleteModel{Name: name},
	); err != nil {
		t.Fatalf("delete PostgreSQL user %q: %v", name, err)
	}
}

func testAccCheckPostgreSQLDatabaseExists(name string, expectedUsers ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		databases, err := postgresql.NewClient(client).GetDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if database.Database != name {
				continue
			}
			if len(database.Users) != len(expectedUsers) {
				return fmt.Errorf(
					"PostgreSQL database %q users = %v, want %v",
					name,
					database.Users,
					expectedUsers,
				)
			}
			for _, expectedUser := range expectedUsers {
				if !slices.Contains(database.Users, expectedUser) {
					return fmt.Errorf(
						"PostgreSQL database %q does not grant access to %q",
						name,
						expectedUser,
					)
				}
			}

			return nil
		}

		return fmt.Errorf("PostgreSQL database %q was not found", name)
	}
}

func testAccCheckPostgreSQLDatabasesDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		databases, err := postgresql.NewClient(client).GetDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if slices.Contains(names, database.Database) {
				return fmt.Errorf("PostgreSQL database %q still exists", database.Database)
			}
		}

		return nil
	}
}

func testAccDeletePostgreSQLDatabase(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if _, err := postgresql.NewClient(client).DeleteDatabase(
		ctx,
		postgresql.DatabaseDeleteModel{Name: name},
	); err != nil {
		t.Fatalf("delete PostgreSQL database %q: %v", name, err)
	}
}

func testAccCheckMySQLUserExists(name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		exists, err := mysql.NewClient(client).UserExists(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("MySQL user %q was not found", name)
		}

		return nil
	}
}

func testAccCheckMySQLUsersDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		users, err := mysql.NewClient(client).ListUsers(ctx)
		if err != nil {
			return err
		}

		for _, user := range users.Data {
			if slices.Contains(names, user.User) {
				return fmt.Errorf("MySQL user %q still exists", user.User)
			}
		}

		return nil
	}
}

func testAccDeleteMySQLUser(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := mysql.NewClient(client).DeleteUser(ctx, name); err != nil {
		t.Fatalf("delete MySQL user %q: %v", name, err)
	}
}

func testAccCheckMySQLRemoteHostExists(
	host string,
	expectedNote string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		remoteHost, err := mysql.NewClient(client).GetRemoteHost(ctx, host)
		if err != nil {
			return err
		}
		if remoteHost == nil {
			return fmt.Errorf("remote MySQL host %q was not found", host)
		}
		if remoteHost.Note != expectedNote {
			return fmt.Errorf(
				"remote MySQL host %q note is %q; want %q",
				host,
				remoteHost.Note,
				expectedNote,
			)
		}

		return nil
	}
}

func testAccCheckMySQLRemoteHostsDestroyed(
	hosts ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		remoteHosts, err := mysql.NewClient(client).ListRemoteHosts(ctx)
		if err != nil {
			return err
		}
		for _, remoteHost := range remoteHosts {
			if slices.Contains(hosts, remoteHost.Host) {
				return fmt.Errorf(
					"remote MySQL host %q is still authorized",
					remoteHost.Host,
				)
			}
		}

		return nil
	}
}

func testAccDeleteMySQLRemoteHost(t *testing.T, host string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := mysql.NewClient(client).DeleteRemoteHost(ctx, host); err != nil {
		t.Fatalf("delete remote MySQL host %q: %v", host, err)
	}
}

func testAccCheckMySQLDatabaseExists(
	name string,
	expectedUsers ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		mysqlClient := mysql.NewClient(client)
		databases, err := mysqlClient.ListDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if database.Database != name {
				continue
			}
			if len(database.Users) != len(expectedUsers) {
				return fmt.Errorf(
					"MySQL database %q users = %v, want %v",
					name,
					database.Users,
					expectedUsers,
				)
			}
			for _, expectedUser := range expectedUsers {
				if !slices.Contains(database.Users, expectedUser) {
					return fmt.Errorf(
						"MySQL database %q does not grant access to %q",
						name,
						expectedUser,
					)
				}

				privileges, err := mysqlClient.GetPrivileges(ctx, expectedUser, name)
				if err != nil {
					return err
				}
				if !slices.Contains(privileges, "ALL PRIVILEGES") {
					return fmt.Errorf(
						"MySQL database %q privileges for %q = %v",
						name,
						expectedUser,
						privileges,
					)
				}
			}

			return nil
		}

		return fmt.Errorf("MySQL database %q was not found", name)
	}
}

func testAccCheckMySQLDatabasesDestroyed(names ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		databases, err := mysql.NewClient(client).ListDatabases(ctx)
		if err != nil {
			return err
		}

		for _, database := range databases.Data {
			if slices.Contains(names, database.Database) {
				return fmt.Errorf("MySQL database %q still exists", database.Database)
			}
		}

		return nil
	}
}

func testAccDeleteMySQLDatabase(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := mysql.NewClient(client).DeleteDatabase(ctx, name); err != nil {
		t.Fatalf("delete MySQL database %q: %v", name, err)
	}
}

func testAccCheckEmailAccountExists(
	address string,
	expectedQuotaMiB int64,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		user, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		account, err := cpanelmail.NewClient(client).GetAccount(ctx, user, domain)
		if err != nil {
			return err
		}
		if account == nil {
			return fmt.Errorf("email account %q was not found", address)
		}

		quotaMiB, err := account.QuotaMiB()
		if err != nil {
			return err
		}
		if quotaMiB != expectedQuotaMiB {
			return fmt.Errorf(
				"email account %q quota = %d MiB, want %d MiB",
				address,
				quotaMiB,
				expectedQuotaMiB,
			)
		}

		return nil
	}
}

func testAccCheckEmailAccountsDestroyed(addresses ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		accounts, err := cpanelmail.NewClient(client).ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, account := range accounts.Data {
			if slices.Contains(addresses, account.Email) {
				return fmt.Errorf("email account %q still exists", account.Email)
			}
		}

		return nil
	}
}

func testAccDeleteEmailAccount(t *testing.T, address string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Fatalf("split email account address: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteAccount(ctx, user, domain); err != nil {
		t.Fatalf("delete email account %q: %v", address, err)
	}
}

func testAccCheckEmailForwarderExists(
	address string,
	destination string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		_, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		forwarder, err := cpanelmail.NewClient(client).GetForwarder(
			ctx,
			domain,
			address,
			destination,
		)
		if err != nil {
			return err
		}
		if forwarder == nil {
			return fmt.Errorf(
				"email forwarder %q to %q was not found",
				address,
				destination,
			)
		}

		return nil
	}
}

func testAccCheckEmailForwardersDestroyed(
	address string,
	destinations ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		_, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		forwarders, err := cpanelmail.NewClient(client).ListForwarders(ctx, domain)
		if err != nil {
			return err
		}
		for _, forwarder := range forwarders {
			if forwarder.Address == address &&
				slices.Contains(destinations, forwarder.Destination) {
				return fmt.Errorf(
					"email forwarder %q to %q still exists",
					address,
					forwarder.Destination,
				)
			}
		}

		return nil
	}
}

func testAccDeleteEmailForwarder(
	t *testing.T,
	address string,
	destination string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteForwarder(
		ctx,
		address,
		destination,
	); err != nil {
		t.Fatalf(
			"delete email forwarder %q to %q: %v",
			address,
			destination,
			err,
		)
	}
}

func testAccCheckEmailDomainForwarderExists(
	domain string,
	destination string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		forwarder, err := cpanelmail.NewClient(client).GetDomainForwarder(ctx, domain)
		if err != nil {
			return err
		}
		if forwarder == nil {
			return fmt.Errorf(
				"email domain forwarder for %q was not found",
				domain,
			)
		}
		if forwarder.Destination != destination {
			return fmt.Errorf(
				"email domain forwarder for %q targets %q, want %q",
				domain,
				forwarder.Destination,
				destination,
			)
		}

		return nil
	}
}

func testAccCheckEmailDomainForwarderDestroyed(
	domain string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		forwarder, err := cpanelmail.NewClient(client).GetDomainForwarder(ctx, domain)
		if err != nil {
			return err
		}
		if forwarder != nil &&
			strings.HasPrefix(forwarder.Destination, "tfcpaneldomainfwd") {
			return fmt.Errorf(
				"test email domain forwarder for %q still exists",
				domain,
			)
		}

		return nil
	}
}

func testAccDeleteEmailDomainForwarder(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteDomainForwarder(ctx, domain); err != nil {
		t.Fatalf("delete email domain forwarder for %q: %v", domain, err)
	}
}

func testAccCheckEmailAutoResponderExists(
	expected cpanelmail.AutoResponder,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		_, domain, err := splitEmailAccountAddress(expected.Email)
		if err != nil {
			return err
		}
		autoResponder, err := cpanelmail.NewClient(client).GetAutoResponder(
			ctx,
			expected.Email,
			domain,
		)
		if err != nil {
			return err
		}
		if autoResponder == nil {
			return fmt.Errorf(
				"email autoresponder for %q was not found",
				expected.Email,
			)
		}
		if !autoRespondersEqual(*autoResponder, expected) {
			return fmt.Errorf(
				"email autoresponder for %q does not match %#v: %#v",
				expected.Email,
				expected,
				*autoResponder,
			)
		}

		return nil
	}
}

func testAccCheckEmailAutoRespondersDestroyed(
	addresses ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		emailClient := cpanelmail.NewClient(client)
		for _, address := range addresses {
			_, domain, err := splitEmailAccountAddress(address)
			if err != nil {
				return err
			}
			autoResponder, err := emailClient.GetAutoResponder(ctx, address, domain)
			if err != nil {
				return err
			}
			if autoResponder != nil {
				return fmt.Errorf(
					"email autoresponder for %q still exists",
					address,
				)
			}
		}

		return nil
	}
}

func testAccDeleteEmailAutoResponder(t *testing.T, address string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteAutoResponder(ctx, address); err != nil {
		t.Fatalf("delete email autoresponder for %q: %v", address, err)
	}
}

func testAccCheckEmailMailingList(
	address string,
	expected cpanelmail.MailingListPrivacyOptions,
	expectedID *string,
	capturedID *string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		_, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		mailingList, err := cpanelmail.NewClient(client).GetMailingList(
			ctx,
			address,
			domain,
		)
		if err != nil {
			return err
		}
		if mailingList == nil {
			return fmt.Errorf("email mailing list %q was not found", address)
		}
		actual := cpanelmail.MailingListPrivacyOptions{
			Advertised:      mailingList.Advertised,
			ArchivePrivate:  mailingList.ArchivePrivate,
			SubscribePolicy: mailingList.SubscribePolicy,
		}
		if actual != expected {
			return fmt.Errorf(
				"email mailing list %q privacy is %#v; want %#v",
				address,
				actual,
				expected,
			)
		}
		if expectedID != nil && mailingList.ID != *expectedID {
			return fmt.Errorf(
				"email mailing list %q id is %q; want %q",
				address,
				mailingList.ID,
				*expectedID,
			)
		}
		if capturedID != nil {
			*capturedID = mailingList.ID
		}

		return nil
	}
}

func testAccCheckEmailMailingListPassword(
	address string,
	password string,
	accepted bool,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		_, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		mailingList, err := cpanelmail.NewClient(client).GetMailingList(
			ctx,
			address,
			domain,
		)
		if err != nil {
			return err
		}
		if mailingList == nil {
			return fmt.Errorf("email mailing list %q was not found", address)
		}

		form := url.Values{
			"adminpw":  {password},
			"admlogin": {"Let me in..."},
		}
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf(
				"https://%s/mailman/admin/%s",
				domain,
				url.PathEscape(mailingList.ID),
			),
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			return fmt.Errorf("build Mailman administrator login request: %w", err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return fmt.Errorf("default HTTP transport has type %T; want *http.Transport", http.DefaultTransport)
		}
		transport := defaultTransport.Clone()
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			// #nosec G402 -- the isolated acceptance domain uses cPanel's self-signed certificate.
			InsecureSkipVerify: true,
		}
		httpClient := &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}
		response, err := httpClient.Do(request)
		if err != nil {
			return fmt.Errorf("authenticate to Mailman administrator page: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		closeErr := response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Mailman administrator page: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close Mailman administrator page: %w", closeErr)
		}
		loginAccepted := response.StatusCode == http.StatusOK &&
			!strings.Contains(string(body), `name="adminpw"`)
		if loginAccepted != accepted {
			return fmt.Errorf(
				"Mailman administrator password acceptance for %q is %t; want %t",
				address,
				loginAccepted,
				accepted,
			)
		}

		return nil
	}
}

type testAccMailmanDelegatesResponse struct {
	Data struct {
		Delegates []string `json:"delegates"`
	} `json:"data"`
}

func testAccEnsureEmailMailingListDelegate(
	t *testing.T,
	address string,
	delegate string,
	delegatePassword string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	emailClient := cpanelmail.NewClient(client)

	delegateUser, delegateDomain, err := splitEmailAccountAddress(delegate)
	if err != nil {
		t.Fatalf("split Mailman delegate address %q: %v", delegate, err)
	}
	account, err := emailClient.GetAccount(ctx, delegateUser, delegateDomain)
	if err != nil {
		t.Fatalf("read Mailman delegate account %q: %v", delegate, err)
	}
	if account == nil {
		if err := emailClient.CreateAccount(
			ctx,
			delegateUser,
			delegateDomain,
			delegatePassword,
			10,
		); err != nil {
			t.Fatalf("create Mailman delegate account %q: %v", delegate, err)
		}
	}

	listUser, _, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Fatalf("split email mailing list address %q: %v", address, err)
	}
	delegates, err := testAccGetMailmanDelegates(ctx, client, listUser)
	if err != nil {
		t.Fatalf("read Mailman delegates for %q: %v", address, err)
	}
	if !slices.Contains(delegates, delegate) {
		response := testAccMailmanDelegatesResponse{}
		if err := client.ExecuteUAPIOperation(
			ctx,
			http.MethodPost,
			cpanel.ModuleEmail,
			"add_mailman_delegates",
			map[string]string{
				"list":      listUser,
				"delegates": delegate,
			},
			&response,
		); err != nil {
			t.Fatalf("add Mailman delegate %q to %q: %v", delegate, address, err)
		}
	}

	t.Cleanup(func() {
		testAccRemoveEmailMailingListDelegate(t, address, delegate)
		testAccDeleteEmailAccountIfPresent(t, delegate)
	})
}

func testAccCheckEmailMailingListDelegate(
	address string,
	delegate string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		listUser, _, err := splitEmailAccountAddress(address)
		if err != nil {
			return err
		}
		delegates, err := testAccGetMailmanDelegates(ctx, client, listUser)
		if err != nil {
			return err
		}
		if !slices.Contains(delegates, delegate) {
			return fmt.Errorf(
				"Mailman delegate %q was not preserved on %q",
				delegate,
				address,
			)
		}

		return nil
	}
}

func testAccGetMailmanDelegates(
	ctx context.Context,
	client *cpanel.Client,
	listUser string,
) ([]string, error) {
	response := testAccMailmanDelegatesResponse{}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleEmail,
		"get_mailman_delegates",
		map[string]string{"list": listUser},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Data.Delegates, nil
}

func testAccRemoveEmailMailingListDelegate(
	t *testing.T,
	address string,
	delegate string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Errorf("create cPanel client for Mailman delegate cleanup: %v", err)
		return
	}
	listUser, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Errorf("split email mailing list address %q: %v", address, err)
		return
	}
	mailingList, err := cpanelmail.NewClient(client).GetMailingList(
		ctx,
		address,
		domain,
	)
	if err != nil {
		t.Errorf("read mailing list %q during delegate cleanup: %v", address, err)
		return
	}
	if mailingList == nil {
		return
	}
	delegates, err := testAccGetMailmanDelegates(ctx, client, listUser)
	if err != nil {
		t.Errorf("read Mailman delegates for %q during cleanup: %v", address, err)
		return
	}
	if !slices.Contains(delegates, delegate) {
		return
	}

	response := testAccMailmanDelegatesResponse{}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleEmail,
		"remove_mailman_delegates",
		map[string]string{
			"list":      listUser,
			"delegates": delegate,
		},
		&response,
	); err != nil {
		t.Errorf("remove Mailman delegate %q from %q: %v", delegate, address, err)
	}
}

func testAccDeleteEmailAccountIfPresent(t *testing.T, address string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Errorf("create cPanel client for email account cleanup: %v", err)
		return
	}
	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Errorf("split email account address %q: %v", address, err)
		return
	}
	emailClient := cpanelmail.NewClient(client)
	account, err := emailClient.GetAccount(ctx, user, domain)
	if err != nil {
		t.Errorf("read email account %q during cleanup: %v", address, err)
		return
	}
	if account == nil {
		return
	}
	if err := emailClient.DeleteAccount(ctx, user, domain); err != nil {
		t.Errorf("delete email account %q: %v", address, err)
	}
}

func testAccCheckEmailMailingListsDestroyed(
	addresses ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		emailClient := cpanelmail.NewClient(client)
		for _, address := range addresses {
			_, domain, err := splitEmailAccountAddress(address)
			if err != nil {
				return err
			}
			mailingList, err := emailClient.GetMailingList(ctx, address, domain)
			if err != nil {
				return err
			}
			if mailingList != nil {
				return fmt.Errorf(
					"email mailing list %q still exists",
					address,
				)
			}
		}

		return nil
	}
}

func testAccDeleteEmailMailingList(t *testing.T, address string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Fatalf("split email mailing list address %q: %v", address, err)
	}
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	emailClient := cpanelmail.NewClient(client)
	existing, err := emailClient.GetMailingList(ctx, address, domain)
	if err != nil {
		t.Fatalf("read email mailing list %q: %v", address, err)
	}
	if existing == nil {
		return
	}
	if err := emailClient.DeleteMailingList(ctx, address); err != nil {
		t.Fatalf("delete email mailing list %q: %v", address, err)
	}
}

func testAccCreateEmailMailingList(
	t *testing.T,
	address string,
	password string,
	privacy cpanelmail.MailingListPrivacyOptions,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		t.Fatalf("split email mailing list address %q: %v", address, err)
	}
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	emailClient := cpanelmail.NewClient(client)
	existing, err := emailClient.GetMailingList(ctx, address, domain)
	if err != nil {
		t.Fatalf("read email mailing list %q: %v", address, err)
	}
	if existing != nil {
		t.Fatalf("email mailing list %q already exists", address)
	}
	if err := emailClient.CreateMailingList(
		ctx,
		user,
		domain,
		password,
		true,
	); err != nil {
		t.Fatalf("create email mailing list %q: %v", address, err)
	}
	if err := emailClient.SetMailingListPrivacyOptions(
		ctx,
		address,
		privacy,
	); err != nil {
		t.Fatalf("set email mailing list %q privacy: %v", address, err)
	}
	created, err := emailClient.GetMailingList(ctx, address, domain)
	if err != nil {
		t.Fatalf("verify email mailing list %q: %v", address, err)
	}
	if created == nil {
		t.Fatalf("email mailing list %q was not created", address)
	}
	actual := cpanelmail.MailingListPrivacyOptions{
		Advertised:      created.Advertised,
		ArchivePrivate:  created.ArchivePrivate,
		SubscribePolicy: created.SubscribePolicy,
	}
	if actual != privacy {
		t.Fatalf(
			"email mailing list %q privacy is %#v; want %#v",
			address,
			actual,
			privacy,
		)
	}
}

func testAccSetEmailMailingListPrivacy(
	t *testing.T,
	address string,
	privacy cpanelmail.MailingListPrivacyOptions,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).SetMailingListPrivacyOptions(
		ctx,
		address,
		privacy,
	); err != nil {
		t.Fatalf("set email mailing list %q privacy: %v", address, err)
	}
}

func testAccCheckFTPAccountExists(
	username string,
	expectedHomeDirectory string,
	expectedQuotaMiB int64,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		account, err := cpanelftp.NewClient(client).GetAccount(ctx, username)
		if err != nil {
			return err
		}
		if account == nil {
			return fmt.Errorf("FTP account %q was not found", username)
		}
		if account.RelativeDirectory != expectedHomeDirectory {
			return fmt.Errorf(
				"FTP account %q home directory = %q, want %q",
				username,
				account.RelativeDirectory,
				expectedHomeDirectory,
			)
		}

		quotaMiB, err := account.QuotaMiB()
		if err != nil {
			return err
		}
		if quotaMiB != expectedQuotaMiB {
			return fmt.Errorf(
				"FTP account %q quota = %d MiB, want %d MiB",
				username,
				quotaMiB,
				expectedQuotaMiB,
			)
		}

		return nil
	}
}

func testAccCheckFTPAccountsDestroyed(usernames ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		accounts, err := cpanelftp.NewClient(client).ListAccounts(ctx)
		if err != nil {
			return err
		}
		for _, account := range accounts.Data {
			if slices.Contains(usernames, account.Login) {
				return fmt.Errorf("FTP account %q still exists", account.Login)
			}
		}

		return nil
	}
}

func testAccDeleteFTPAccount(t *testing.T, username string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	user, domain, err := splitFTPAccountUsername(username)
	if err != nil {
		t.Fatalf("split FTP account username: %v", err)
	}
	if err := cpanelftp.NewClient(client).DeleteAccount(ctx, user, domain, true); err != nil {
		t.Fatalf("delete FTP account %q: %v", username, err)
	}
}

func testAccCheckSubdomainExists(
	domain string,
	expectedDocumentRoot string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		subdomain, err := cpaneldomain.NewClient(client).GetSubdomain(ctx, domain)
		if err != nil {
			return err
		}
		if subdomain == nil {
			return fmt.Errorf("subdomain %q was not found", domain)
		}
		if subdomain.BaseDirectory != expectedDocumentRoot {
			return fmt.Errorf(
				"subdomain %q document root = %q, want %q",
				domain,
				subdomain.BaseDirectory,
				expectedDocumentRoot,
			)
		}

		return nil
	}
}

func testAccCheckSubdomainsDestroyed(domains ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		response, err := cpaneldomain.NewClient(client).ListSubdomains(ctx)
		if err != nil {
			return err
		}
		for _, subdomain := range response.CpanelResult.Data {
			if slices.Contains(domains, subdomain.Domain) {
				return fmt.Errorf("subdomain %q still exists", subdomain.Domain)
			}
		}

		return nil
	}
}

func testAccDeleteSubdomain(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	if err := cpaneldomain.NewClient(client).DeleteSubdomain(ctx, domain); err != nil {
		t.Fatalf("delete subdomain %q: %v", domain, err)
	}
}

func testAccCheckAddonDomainExists(
	domain string,
	expectedInternalSubdomain string,
	expectedDocumentRoot string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		addonDomain, err := cpaneldomain.NewClient(client).GetAddonDomain(ctx, domain)
		if err != nil {
			return err
		}
		if addonDomain == nil {
			return fmt.Errorf("addon domain %q was not found", domain)
		}
		if addonDomain.InternalSubdomain != expectedInternalSubdomain {
			return fmt.Errorf(
				"addon domain %q internal subdomain = %q, want %q",
				domain,
				addonDomain.InternalSubdomain,
				expectedInternalSubdomain,
			)
		}
		if addonDomain.BaseDirectory != expectedDocumentRoot {
			return fmt.Errorf(
				"addon domain %q document root = %q, want %q",
				domain,
				addonDomain.BaseDirectory,
				expectedDocumentRoot,
			)
		}

		return nil
	}
}

func testAccCheckAddonDomainsDestroyed(domains ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		response, err := cpaneldomain.NewClient(client).ListAddonDomains(ctx)
		if err != nil {
			return err
		}
		for _, addonDomain := range response.CpanelResult.Data {
			if slices.Contains(domains, addonDomain.Domain) {
				return fmt.Errorf("addon domain %q still exists", addonDomain.Domain)
			}
		}

		return nil
	}
}

func testAccDeleteAddonDomain(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	domainClient := cpaneldomain.NewClient(client)
	addonDomain, err := domainClient.GetAddonDomain(ctx, domain)
	if err != nil {
		t.Fatalf("read addon domain %q: %v", domain, err)
	}
	if addonDomain == nil {
		return
	}
	if err := domainClient.DeleteAddonDomain(
		ctx,
		addonDomain.Domain,
		addonDomain.DomainKey,
	); err != nil {
		t.Fatalf("delete addon domain %q: %v", domain, err)
	}
}

func testAccCheckDomainAliasExists(
	domain string,
	expectedTargetDomain string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		const expectedDocumentRoot = "public_html"

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		domainClient := cpaneldomain.NewClient(client)
		domainAlias, err := domainClient.GetDomainAlias(ctx, domain)
		if err != nil {
			return err
		}
		if domainAlias == nil {
			return fmt.Errorf("domain alias %q was not found", domain)
		}
		if domainAlias.BaseDirectory != expectedDocumentRoot {
			return fmt.Errorf(
				"domain alias %q document root = %q, want %q",
				domain,
				domainAlias.BaseDirectory,
				expectedDocumentRoot,
			)
		}

		targetDomain, err := domainClient.GetMainDomain(ctx)
		if err != nil {
			return err
		}
		if targetDomain != expectedTargetDomain {
			return fmt.Errorf(
				"domain alias %q target domain = %q, want %q",
				domain,
				targetDomain,
				expectedTargetDomain,
			)
		}

		return nil
	}
}

func testAccCheckDomainAliasesDestroyed(domains ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		response, err := cpaneldomain.NewClient(client).ListDomainAliases(ctx)
		if err != nil {
			return err
		}
		for _, domainAlias := range response.CpanelResult.Data {
			if slices.Contains(domains, domainAlias.Domain) {
				return fmt.Errorf("domain alias %q still exists", domainAlias.Domain)
			}
		}

		return nil
	}
}

func testAccDeleteDomainAlias(t *testing.T, domain string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	domainClient := cpaneldomain.NewClient(client)
	domainAlias, err := domainClient.GetDomainAlias(ctx, domain)
	if err != nil {
		t.Fatalf("read domain alias %q: %v", domain, err)
	}
	if domainAlias == nil {
		return
	}
	if err := domainClient.DeleteDomainAlias(ctx, domain); err != nil {
		t.Fatalf("delete domain alias %q: %v", domain, err)
	}
}

func testAccCheckDNSRecordExists(
	zoneName string,
	name string,
	ttl int64,
	data []string,
) resource.TestCheckFunc {
	return testAccCheckTypedDNSRecordExists(
		zoneName,
		name,
		"TXT",
		ttl,
		data,
	)
}

func testAccCheckTypedDNSRecordExists(
	zoneName string,
	name string,
	recordType string,
	ttl int64,
	data []string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		zone, err := cpaneldns.NewClient(client).ParseZone(ctx, zoneName)
		if err != nil {
			return err
		}
		for _, record := range zone.RecordsByIdentity(name, recordType) {
			if record.TTL == ttl && slices.Equal(record.Data, data) {
				return nil
			}
		}

		return fmt.Errorf(
			"%s record %q with TTL %d and data %v was not found in zone %q",
			recordType,
			name,
			ttl,
			data,
			zoneName,
		)
	}
}

func testAccCheckDNSRecordsDestroyed(
	zoneName string,
	names ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}

		zone, err := cpaneldns.NewClient(client).ParseZone(ctx, zoneName)
		if err != nil {
			return err
		}
		for _, record := range zone.Records {
			if slices.Contains(names, record.Name) {
				return fmt.Errorf(
					"DNS record %q still exists in zone %q",
					record.Name,
					zoneName,
				)
			}
		}

		return nil
	}
}

func testAccUpdateDNSRecord(
	t *testing.T,
	zoneName string,
	name string,
	ttl int64,
	data []string,
) {
	t.Helper()

	const recordType = "TXT"

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	dnsClient := cpaneldns.NewClient(client)
	zone, err := dnsClient.ParseZone(ctx, zoneName)
	if err != nil {
		t.Fatalf("read DNS zone %q: %v", zoneName, err)
	}
	records := zone.RecordsByIdentity(name, recordType)
	if len(records) != 1 {
		t.Fatalf(
			"found %d %s records named %q in zone %q, want 1",
			len(records),
			recordType,
			name,
			zoneName,
		)
	}

	record := records[0]
	desired := record
	desired.TTL = ttl
	desired.Data = data
	if _, err := dnsClient.UpdateRecord(ctx, zoneName, record, desired); err != nil {
		t.Fatalf("update DNS record %q: %v", name, err)
	}
}

func testAccDeleteDNSRecord(
	t *testing.T,
	zoneName string,
	name string,
) {
	t.Helper()

	const recordType = "TXT"

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	dnsClient := cpaneldns.NewClient(client)
	zone, err := dnsClient.ParseZone(ctx, zoneName)
	if err != nil {
		t.Fatalf("read DNS zone %q: %v", zoneName, err)
	}
	records := zone.RecordsByIdentity(name, recordType)
	if len(records) == 0 {
		return
	}
	if len(records) != 1 {
		t.Fatalf(
			"found %d %s records named %q in zone %q, want at most 1",
			len(records),
			recordType,
			name,
			zoneName,
		)
	}

	if err := dnsClient.DeleteRecord(ctx, zoneName, records[0]); err != nil {
		t.Fatalf("delete DNS record %q: %v", name, err)
	}
}
