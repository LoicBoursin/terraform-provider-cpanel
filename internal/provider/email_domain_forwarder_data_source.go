package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailDomainForwarderDataSource{}
	_ datasource.DataSourceWithConfigure = &emailDomainForwarderDataSource{}
)

func NewEmailDomainForwarderDataSource() datasource.DataSource {
	return &emailDomainForwarderDataSource{}
}

type emailDomainForwarderDataSource struct {
	client *cpanelmail.Client
}

func (d *emailDomainForwarderDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_domain_forwarder"
}

func (d *emailDomainForwarderDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up the domain-level email forwarder for one cPanel mail domain.",
		MarkdownDescription: "Looks up the domain-level email forwarder for one cPanel mail domain.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The source mail domain owned by the cPanel account.",
				MarkdownDescription: "The source mail domain owned by the cPanel account.",
				Validators:          domainNameValidators(),
			},
			"destination": schema.StringAttribute{
				Computed:            true,
				Description:         "The external domain that receives mail sent to the source domain.",
				MarkdownDescription: "The external domain that receives mail sent to the source domain.",
			},
		},
	}
}

func (d *emailDomainForwarderDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config EmailDomainForwarderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain := config.Domain.ValueString()
	if err := validateEmailDomain(ctx, d.client, domain); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder domain", err.Error())
		return
	}

	forwarder, err := d.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email domain forwarder", err.Error())
		return
	}
	if forwarder == nil {
		resp.Diagnostics.AddError(
			"Email domain forwarder not found",
			fmt.Sprintf(
				"No domain-level email forwarder exists for %q.",
				domain,
			),
		)
		return
	}

	state := EmailDomainForwarderModel{
		Domain:      types.StringValue(forwarder.Domain),
		Destination: types.StringValue(forwarder.Destination),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *emailDomainForwarderDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected map[string]interface{}, got: %T.", req.ProviderData),
		)
		return
	}

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf("Expected *email.Client, got: %T.", providerData["email"]),
		)
		return
	}

	d.client = client
}
