package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailAccountSuspensionDataSource{}
	_ datasource.DataSourceWithConfigure = &emailAccountSuspensionDataSource{}
)

func NewEmailAccountSuspensionDataSource() datasource.DataSource {
	return &emailAccountSuspensionDataSource{}
}

type emailAccountSuspensionDataSource struct {
	client *cpanelmail.Client
}

func (d *emailAccountSuspensionDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_account_suspension"
}

func (d *emailAccountSuspensionDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up login, incoming-mail, and outgoing-mail suspension status for a cPanel email account.",
		MarkdownDescription: "Looks up login, incoming-mail, and outgoing-mail suspension status for a cPanel email account.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:            true,
				Description:         "The complete existing email account address.",
				MarkdownDescription: "The complete existing email account address.",
				Validators:          emailAddressValidators(),
			},
			"login_suspended": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel has suspended mailbox login.",
				MarkdownDescription: "Whether cPanel has suspended mailbox login.",
			},
			"incoming_suspended": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel rejects incoming mail for the account.",
				MarkdownDescription: "Whether cPanel rejects incoming mail for the account.",
			},
			"outgoing_suspended": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel rejects outgoing mail for the account.",
				MarkdownDescription: "Whether cPanel rejects outgoing mail for the account.",
			},
			"outgoing_held": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether outgoing mail is held in the Exim queue. This provider reports but does not manage held mail.",
				MarkdownDescription: "Whether outgoing mail is held in the Exim queue. This provider reports but does not manage held mail.",
			},
			"has_suspended": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports any suspension or outgoing-mail hold for the account.",
				MarkdownDescription: "Whether cPanel reports any suspension or outgoing-mail hold for the account.",
			},
		},
	}
}

func (d *emailAccountSuspensionDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config EmailAccountSuspensionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		d.client,
		config.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid email account address",
			err.Error(),
		)
		return
	}

	account, err := d.client.GetAccount(ctx, user, domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read email account suspension",
			err.Error(),
		)
		return
	}
	if account == nil {
		resp.Diagnostics.AddError(
			"Email account not found",
			fmt.Sprintf(
				"No email account named %q exists.",
				config.Email.ValueString(),
			),
		)
		return
	}

	model, err := emailAccountSuspensionsToDataSourceModel(*account)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode email account suspension",
			err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (d *emailAccountSuspensionDataSource) Configure(
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
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf(
				"Expected *email.Client, got: %T.",
				providerData["email"],
			),
		)
		return
	}

	d.client = client
}
