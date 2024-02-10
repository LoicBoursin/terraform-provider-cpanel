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
	"terraform-provider-cpanel/internal/cpanel/cron"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
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
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				Optional: true,
			},
			"api_token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
			},
			"host": schema.StringAttribute{
				Optional: true,
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
	ctx = tflog.MaskFieldValuesWithFieldKeys(ctx, "cpanel_api_token", apiToken)

	tflog.Info(ctx, "Creating cpanel client")

	// Create a new cpanel client using the configuration values
	client, err := cpanel.NewClient(&host, &username, &apiToken)
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
	cronClient := cron.NewClient(client)
	postgreSQLClient := postgresql.NewClient(client)

	// Make the module clients available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = map[string]interface{}{
		"cron":       cronClient,
		"postgresql": postgreSQLClient,
	}
	resp.ResourceData = map[string]interface{}{
		"cron":       cronClient,
		"postgresql": postgreSQLClient,
	}

	tflog.Error(ctx, "Configured module clients", map[string]any{"success": true})
}

// DataSources defines the data sources implemented in the provider.
func (p *cpanelProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCronJobDataSource,
		NewPostgreSQLDatabaseDataSource,
		NewPostgreSQLUserDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *cpanelProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCronJobResource,
		NewPostgreSQLDatabaseResource,
		NewPostgreSQLUserResource,
	}
}

// cpanelProviderModel maps provider schema data to a Go type.
type cpanelProviderModel struct {
	Username types.String `tfsdk:"username"`
	ApiToken types.String `tfsdk:"api_token"`
	Host     types.String `tfsdk:"host"`
}
