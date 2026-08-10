package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailForwarderDataSource{}
	_ datasource.DataSourceWithConfigure = &emailForwarderDataSource{}
)

func NewEmailForwarderDataSource() datasource.DataSource {
	return &emailForwarderDataSource{}
}

type emailForwarderDataSource struct {
	client *cpanelmail.Client
}

func (d *emailForwarderDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_forwarder"
}

func (d *emailForwarderDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one direct email-address forwarder.",
		MarkdownDescription: "Looks up one direct email-address forwarder.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Required:            true,
				Description:         "The complete source address on a mail domain owned by the cPanel account.",
				MarkdownDescription: "The complete source address on a mail domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
			},
			"destination": schema.StringAttribute{
				Required:            true,
				Description:         "The single email address that receives forwarded messages.",
				MarkdownDescription: "The single email address that receives forwarded messages.",
				Validators:          emailAddressValidators(),
			},
		},
	}
}

func (d *emailForwarderDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config EmailForwarderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address := config.Address.ValueString()
	destination := config.Destination.ValueString()
	domain, err := validateEmailForwarderSource(ctx, d.client, address)
	if err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder address", err.Error())
		return
	}
	if err := validateEmailForwarderDestination(destination); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder destination", err.Error())
		return
	}

	forwarder, err := d.client.GetForwarder(ctx, domain, address, destination)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email forwarder", err.Error())
		return
	}
	if forwarder == nil {
		resp.Diagnostics.AddError(
			"Email forwarder not found",
			fmt.Sprintf(
				"No email forwarder from %q to %q exists.",
				address,
				destination,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *emailForwarderDataSource) Configure(
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
