package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"os"
	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/apachehandler"
	"terraform-provider-cpanel/internal/cpanel/apitoken"
	cpanelboxtrapper "terraform-provider-cpanel/internal/cpanel/boxtrapper"
	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
	"terraform-provider-cpanel/internal/cpanel/capabilities"
	"terraform-provider-cpanel/internal/cpanel/cron"
	"terraform-provider-cpanel/internal/cpanel/ddns"
	"terraform-provider-cpanel/internal/cpanel/directoryindex"
	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
	cpaneldns "terraform-provider-cpanel/internal/cpanel/dns"
	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
	"terraform-provider-cpanel/internal/cpanel/fileman"
	"terraform-provider-cpanel/internal/cpanel/ftp"
	"terraform-provider-cpanel/internal/cpanel/gpg"
	"terraform-provider-cpanel/internal/cpanel/ipblock"
	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
	"terraform-provider-cpanel/internal/cpanel/mimetype"
	"terraform-provider-cpanel/internal/cpanel/modsecurity"
	"terraform-provider-cpanel/internal/cpanel/mysql"
	"terraform-provider-cpanel/internal/cpanel/passenger"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
	"terraform-provider-cpanel/internal/cpanel/sslcsr"
	"terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ provider.Provider = &cpanelProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cpanelProvider{
			version: version,
		}
	}
}

// cpanelProvider is the provider implementation.
type cpanelProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// Metadata returns the provider type name.
func (p *cpanelProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cpanel"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *cpanelProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Inspect cPanel account capabilities and manage API tokens, locale, raw access log settings, cron jobs, DNS records, Dynamic DNS domains, web domains, stored SSL certificates and certificate signing requests, public-only OpenPGP keys, Passenger applications, ModSecurity settings, HTTP redirects, filesystem directories and text files, directory indexes and privacy, Git repositories, custom MIME types and Apache handlers, email accounts and suspensions, BoxTrapper settings, calendar delegations, filters, forwarders, autoresponders, Mailman mailing lists, FTP accounts, website IP blocks, remote database hosts, and MySQL, MariaDB, and PostgreSQL users and databases.",
		MarkdownDescription: "Inspect cPanel account capabilities and manage API tokens, locale, raw access log settings, cron jobs, DNS records, Dynamic DNS domains, web domains, stored SSL certificates and certificate signing requests, public-only OpenPGP keys, Passenger applications, ModSecurity settings, HTTP redirects, filesystem directories and text files, directory indexes and privacy, Git repositories, custom MIME types and Apache handlers, email accounts and suspensions, BoxTrapper settings, calendar delegations, filters, forwarders, autoresponders, Mailman mailing lists, FTP accounts, website IP blocks, remote database hosts, and MySQL, MariaDB, and PostgreSQL users and databases.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Optional:            true,
				Description:         "The cPanel account username. May also be set with CPANEL_USERNAME.",
				MarkdownDescription: "The cPanel account username. May also be set with `CPANEL_USERNAME`.",
			},
			"api_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				Description:         "The cPanel API token. May also be set with CPANEL_API_TOKEN.",
				MarkdownDescription: "The cPanel API token. May also be set with `CPANEL_API_TOKEN`.",
			},
			"host": schema.StringAttribute{
				Optional:            true,
				Description:         "The HTTPS cPanel account API endpoint, usually including port 2083. May also be set with CPANEL_HOST.",
				MarkdownDescription: "The HTTPS cPanel account API endpoint, usually including port `2083`. May also be set with `CPANEL_HOST`.",
			},
		},
	}
}

func (p *cpanelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Retrieve provider data from configuration
	var config cpanelProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.

	if config.Username.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Unknown cpanel API Username",
			"The provider cannot create the cpanel API client as there is an unknown configuration value for the cpanel API username. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CPANEL_USERNAME environment variable.",
		)
	}

	if config.ApiToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Unknown cpanel API Token",
			"The provider cannot create the cpanel API client as there is an unknown configuration value for the cpanel API token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CPANEL_API_TOKEN environment variable.",
		)
	}

	if config.Host.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Unknown cpanel API Host",
			"The provider cannot create the cpanel API client as there is an unknown configuration value for the cpanel API host. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CPANEL_HOST environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	username := os.Getenv("CPANEL_USERNAME")
	apiToken := os.Getenv("CPANEL_API_TOKEN")
	host := os.Getenv("CPANEL_HOST")

	if !config.Username.IsNull() {
		username = config.Username.ValueString()
	}

	if !config.ApiToken.IsNull() {
		apiToken = config.ApiToken.ValueString()
	}

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if username == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Missing cpanel API Username",
			"The provider cannot create the cpanel API client as there is a missing or empty value for the cpanel API username. "+
				"Set the username value in the configuration or use the CPANEL_USERNAME environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if apiToken == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing cpanel API Token",
			"The provider cannot create the cpanel API client as there is a missing or empty value for the cpanel API token. "+
				"Set the cpanel token value in the configuration or use the CPANEL_API_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if host == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Missing cpanel API Host",
			"The provider cannot create the cpanel API client as there is a missing or empty value for the cpanel API host. "+
				"Set the host value in the configuration or use the CPANEL_HOST environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "cpanel_host", host)
	ctx = tflog.SetField(ctx, "cpanel_username", username)
	tflog.Info(ctx, "Creating cpanel client")

	// Create a new cpanel client using the configuration values
	client, err := cpanel.NewClient(host, username, apiToken)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create cpanel API Client",
			"An unexpected error occurred when creating the cpanel API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"cpanel Client Error: "+err.Error(),
		)
		return
	}

	// Initialize module clients
	apacheHandlerClient := apachehandler.NewClient(client)
	apiTokenClient := apitoken.NewClient(client)
	boxTrapperClient := cpanelboxtrapper.NewClient(client)
	calendarClient := cpanelcalendar.NewClient(client)
	capabilitiesClient := capabilities.NewClient(client)
	cronClient := cron.NewClient(client)
	directoryIndexClient := directoryindex.NewClient(client)
	directoryPrivacyClient := directoryprivacy.NewClient(client)
	dnsClient := cpaneldns.NewClient(client)
	domainClient := cpaneldomain.NewClient(client)
	dynamicDNSClient := ddns.NewClient(client)
	emailClient := cpanelmail.NewClient(client)
	filemanClient := fileman.NewClient(client)
	ftpClient := ftp.NewClient(client)
	gpgClient := gpg.NewClient(client)
	ipBlockClient := ipblock.NewClient(client)
	localeClient := cpanellocale.NewClient(client)
	logManagerClient := cpanellogmanager.NewClient(client)
	mimeTypeClient := mimetype.NewClient(client)
	modSecurityClient := modsecurity.NewClient(client)
	mySQLClient := mysql.NewClient(client)
	passengerClient := passenger.NewClient(client)
	postgreSQLClient := postgresql.NewClient(client)
	redirectClient := cpanelredirect.NewClient(client)
	sslCertificateClient := sslcertificate.NewClient(client)
	sslCSRClient := sslcsr.NewClient(client)
	versionControlClient := versioncontrol.NewClient(client)

	// Make the module clients available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = map[string]interface{}{
		"apachehandler":    apacheHandlerClient,
		"apitoken":         apiTokenClient,
		"boxtrapper":       boxTrapperClient,
		"calendar":         calendarClient,
		"capabilities":     capabilitiesClient,
		"cron":             cronClient,
		"directoryindex":   directoryIndexClient,
		"directoryprivacy": directoryPrivacyClient,
		"dns":              dnsClient,
		"domain":           domainClient,
		"ddns":             dynamicDNSClient,
		"email":            emailClient,
		"fileman":          filemanClient,
		"ftp":              ftpClient,
		"gpg":              gpgClient,
		"ipblock":          ipBlockClient,
		"locale":           localeClient,
		"logmanager":       logManagerClient,
		"mimetype":         mimeTypeClient,
		"modsecurity":      modSecurityClient,
		"mysql":            mySQLClient,
		"passenger":        passengerClient,
		"postgresql":       postgreSQLClient,
		"redirect":         redirectClient,
		"sslcertificate":   sslCertificateClient,
		"sslcsr":           sslCSRClient,
		"versioncontrol":   versionControlClient,
	}
	resp.ResourceData = map[string]interface{}{
		"apachehandler":    apacheHandlerClient,
		"apitoken":         apiTokenClient,
		"boxtrapper":       boxTrapperClient,
		"calendar":         calendarClient,
		"cron":             cronClient,
		"directoryindex":   directoryIndexClient,
		"directoryprivacy": directoryPrivacyClient,
		"dns":              dnsClient,
		"domain":           domainClient,
		"ddns":             dynamicDNSClient,
		"email":            emailClient,
		"fileman":          filemanClient,
		"ftp":              ftpClient,
		"gpg":              gpgClient,
		"ipblock":          ipBlockClient,
		"locale":           localeClient,
		"logmanager":       logManagerClient,
		"mimetype":         mimeTypeClient,
		"modsecurity":      modSecurityClient,
		"mysql":            mySQLClient,
		"passenger":        passengerClient,
		"postgresql":       postgreSQLClient,
		"redirect":         redirectClient,
		"sslcertificate":   sslCertificateClient,
		"sslcsr":           sslCSRClient,
		"versioncontrol":   versionControlClient,
	}

	tflog.Info(ctx, "Configured module clients", map[string]any{"success": true})
}

// DataSources defines the data sources implemented in the provider.
func (p *cpanelProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAccountCapabilitiesDataSource,
		NewAddonDomainDataSource,
		NewApacheHandlerDataSource,
		NewAPITokenDataSource,
		NewBoxTrapperSettingsDataSource,
		NewCalendarDelegateDataSource,
		NewCronJobDataSource,
		NewDirectoryIndexDataSource,
		NewDirectoryPrivacyDataSource,
		NewDirectoryPrivacyUserDataSource,
		NewDNSRecordDataSource,
		NewDomainAliasDataSource,
		NewDynamicDNSDataSource,
		NewEmailAccountDataSource,
		NewEmailAccountSuspensionDataSource,
		NewEmailAutoResponderDataSource,
		NewEmailDomainForwarderDataSource,
		NewEmailFilterDataSource,
		NewEmailForwarderDataSource,
		NewEmailMailingListDataSource,
		NewEmailRoutingDataSource,
		NewFilesystemDirectoryDataSource,
		NewFilesystemTextFileDataSource,
		NewFTPAccountDataSource,
		NewGPGPublicKeyDataSource,
		NewGitRepositoryDataSource,
		NewIPBlockDataSource,
		NewLocaleDataSource,
		NewLogSettingsDataSource,
		NewMIMETypeDataSource,
		NewModSecurityDomainDataSource,
		NewMySQLDatabaseDataSource,
		NewMySQLRemoteHostDataSource,
		NewMySQLUserDataSource,
		NewPassengerApplicationDataSource,
		NewPostgreSQLDatabaseDataSource,
		NewPostgreSQLUserDataSource,
		NewRedirectDataSource,
		NewSSLCertificateDataSource,
		NewSSLCSRDataSource,
		NewSubdomainDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *cpanelProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAddonDomainResource,
		NewApacheHandlerResource,
		NewAPITokenResource,
		NewBoxTrapperSettingsResource,
		NewCalendarDelegateResource,
		NewCronJobResource,
		NewDirectoryIndexResource,
		NewDirectoryPrivacyResource,
		NewDirectoryPrivacyUserResource,
		NewDNSRecordResource,
		NewDomainAliasResource,
		NewDynamicDNSResource,
		NewEmailAccountResource,
		NewEmailAccountSuspensionResource,
		NewEmailAutoResponderResource,
		NewEmailDomainForwarderResource,
		NewEmailFilterResource,
		NewEmailForwarderResource,
		NewEmailMailingListResource,
		NewEmailRoutingResource,
		NewFilesystemDirectoryResource,
		NewFilesystemTextFileResource,
		NewFTPAccountResource,
		NewGPGPublicKeyResource,
		NewGitRepositoryResource,
		NewIPBlockResource,
		NewLocaleResource,
		NewLogSettingsResource,
		NewMIMETypeResource,
		NewModSecurityDomainResource,
		NewMySQLDatabaseResource,
		NewMySQLRemoteHostResource,
		NewMySQLUserResource,
		NewPassengerApplicationResource,
		NewPostgreSQLDatabaseResource,
		NewPostgreSQLUserResource,
		NewRedirectResource,
		NewSSLCertificateResource,
		NewSSLCSRResource,
		NewSubdomainResource,
	}
}

// cpanelProviderModel maps provider schema data to a Go type.
type cpanelProviderModel struct {
	Username types.String `tfsdk:"username"`
	ApiToken types.String `tfsdk:"api_token"`
	Host     types.String `tfsdk:"host"`
}
