package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailAutoResponderDataSource{}
	_ datasource.DataSourceWithConfigure = &emailAutoResponderDataSource{}
)

func NewEmailAutoResponderDataSource() datasource.DataSource {
	return &emailAutoResponderDataSource{}
}

type emailAutoResponderDataSource struct {
	client *cpanelmail.Client
}

func (d *emailAutoResponderDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_auto_responder"
}

func (d *emailAutoResponderDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one cPanel email autoresponder and its current schedule.",
		MarkdownDescription: "Looks up one cPanel email autoresponder and its current schedule.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:            true,
				Description:         "The complete responder email address on a mail domain owned by the cPanel account.",
				MarkdownDescription: "The complete responder email address on a mail domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
			},
			"from": schema.StringAttribute{
				Computed:            true,
				Description:         "The display name used in autoresponder messages.",
				MarkdownDescription: "The display name used in autoresponder messages.",
			},
			"subject": schema.StringAttribute{
				Computed:            true,
				Description:         "The autoresponder message subject.",
				MarkdownDescription: "The autoresponder message subject.",
			},
			"body": schema.StringAttribute{
				Computed:            true,
				Description:         "The autoresponder message body with cPanel's trailing newline normalized.",
				MarkdownDescription: "The autoresponder message body with cPanel's trailing newline normalized.",
			},
			"charset": schema.StringAttribute{
				Computed:            true,
				Description:         "The message character set.",
				MarkdownDescription: "The message character set.",
			},
			"interval_hours": schema.Int64Attribute{
				Computed:            true,
				Description:         "Hours cPanel waits before replying again to the same sender.",
				MarkdownDescription: "Hours cPanel waits before replying again to the same sender.",
			},
			"is_html": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel adds an HTML content type to the message.",
				MarkdownDescription: "Whether cPanel adds an HTML content type to the message.",
			},
			"start_unix": schema.Int64Attribute{
				Computed:            true,
				Description:         "Unix activation timestamp, or 0 for immediate activation.",
				MarkdownDescription: "Unix activation timestamp, or `0` for immediate activation.",
			},
			"stop_unix": schema.Int64Attribute{
				Computed:            true,
				Description:         "Unix deactivation timestamp, or 0 when no stop is configured.",
				MarkdownDescription: "Unix deactivation timestamp, or `0` when no stop is configured.",
			},
		},
	}
}

func (d *emailAutoResponderDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config EmailAutoResponderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, domain, err := validateEmailAccountAddress(
		ctx,
		d.client,
		config.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid autoresponder email", err.Error())
		return
	}

	autoResponder, err := d.client.GetAutoResponder(
		ctx,
		config.Email.ValueString(),
		domain,
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email autoresponder", err.Error())
		return
	}
	if autoResponder == nil {
		resp.Diagnostics.AddError(
			"Email autoresponder not found",
			fmt.Sprintf(
				"No email autoresponder exists for %q.",
				config.Email.ValueString(),
			),
		)
		return
	}

	applyAutoResponderToModel(&config, *autoResponder)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func (d *emailAutoResponderDataSource) Configure(
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
