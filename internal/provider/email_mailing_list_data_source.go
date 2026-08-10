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
	_ datasource.DataSource              = &emailMailingListDataSource{}
	_ datasource.DataSourceWithConfigure = &emailMailingListDataSource{}
)

func NewEmailMailingListDataSource() datasource.DataSource {
	return &emailMailingListDataSource{}
}

type emailMailingListDataSource struct {
	client *cpanelmail.Client
}

func (d *emailMailingListDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_mailing_list"
}

func (d *emailMailingListDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Looks up one cPanel Mailman mailing list.",
		MarkdownDescription: "Looks up one cPanel Mailman mailing list.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Required:            true,
				Description:         "The complete mailing list address on a mail domain owned by the cPanel account.",
				MarkdownDescription: "The complete mailing list address on a mail domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
			},
			"private": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether every Mailman privacy setting is private.",
				MarkdownDescription: "Whether every Mailman privacy setting is private. The value is `true` only when the list is not advertised, its archive is private, and its subscription policy requires administrator approval.",
			},
			"list_id": schema.StringAttribute{
				Computed:            true,
				Description:         "The internal Mailman list identifier returned by cPanel.",
				MarkdownDescription: "The internal Mailman list identifier returned by cPanel.",
			},
			"administrators": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The mailing list administrator addresses reported by cPanel.",
				MarkdownDescription: "The mailing list administrator addresses reported by cPanel.",
			},
			"advertised": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the Mailman directory advertises the list.",
				MarkdownDescription: "Whether the Mailman directory advertises the list.",
			},
			"archive_private": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the mailing list archive is private.",
				MarkdownDescription: "Whether the mailing list archive is private.",
			},
			"subscribe_policy": schema.Int64Attribute{
				Computed:            true,
				Description:         "The Mailman subscription policy code reported by cPanel.",
				MarkdownDescription: "The Mailman subscription policy code reported by cPanel: `1` allows confirmed subscriptions, while `2` and `3` require administrator approval.",
			},
			"human_disk_used": schema.StringAttribute{
				Computed:            true,
				Description:         "The human-readable mailing list disk usage reported by cPanel.",
				MarkdownDescription: "The human-readable mailing list disk usage reported by cPanel.",
			},
		},
	}
}

func (d *emailMailingListDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config EmailMailingListDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := validateEmailMailingListAddress(
		config.Address.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid mailing list address", err.Error())
		return
	}
	if _, _, err := validateEmailAccountAddress(
		ctx,
		d.client,
		config.Address.ValueString(),
	); err != nil {
		resp.Diagnostics.AddError("Invalid mailing list address", err.Error())
		return
	}

	mailingList, err := d.client.GetMailingList(
		ctx,
		config.Address.ValueString(),
		domain,
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mailing list", err.Error())
		return
	}
	if mailingList == nil {
		resp.Diagnostics.AddError(
			"Mailing list not found",
			fmt.Sprintf(
				"No mailing list exists at %q.",
				config.Address.ValueString(),
			),
		)
		return
	}

	state, diagnostics := mailingListDataSourceModel(ctx, *mailingList)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *emailMailingListDataSource) Configure(
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
