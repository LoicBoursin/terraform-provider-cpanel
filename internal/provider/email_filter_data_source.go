package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailFilterDataSource{}
	_ datasource.DataSourceWithConfigure = &emailFilterDataSource{}
)

func NewEmailFilterDataSource() datasource.DataSource {
	return &emailFilterDataSource{}
}

type emailFilterDataSource struct {
	client *cpanelmail.Client
}

func (d *emailFilterDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_filter"
}

func (d *emailFilterDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up one user-level cPanel email filter for an existing mailbox.",
		MarkdownDescription: "Looks up one user-level cPanel email filter for an existing mailbox, including its ordered rules, ordered actions, and enabled state. The data source can observe external `save` and `pipe` actions even though the resource deliberately does not manage them.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Required:            true,
				Description:         "The mailbox that owns the user-level filter.",
				MarkdownDescription: "The mailbox that owns the user-level filter.",
				Validators:          emailAddressValidators(),
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel filter name.",
				MarkdownDescription: "The cPanel filter name.",
				Validators:          emailFilterLookupNameValidators(),
			},
			"enabled": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel evaluates the filter.",
				MarkdownDescription: "Whether cPanel evaluates the filter.",
			},
			"rules": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The ordered conditions evaluated by cPanel.",
				MarkdownDescription: "The ordered conditions evaluated by cPanel.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"part": schema.StringAttribute{
							Computed:            true,
							Description:         "The email section inspected by the rule.",
							MarkdownDescription: "The email section inspected by the rule.",
						},
						"match": schema.StringAttribute{
							Computed:            true,
							Description:         "The comparison applied by the rule.",
							MarkdownDescription: "The comparison applied by the rule.",
						},
						"value": schema.StringAttribute{
							Computed:            true,
							Description:         "The value matched by the rule.",
							MarkdownDescription: "The value matched by the rule.",
						},
						"operator": schema.StringAttribute{
							Computed:            true,
							Description:         "The connection to the next rule.",
							MarkdownDescription: "The connection to the next rule, or `none` for the final rule.",
						},
					},
				},
			},
			"actions": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The ordered actions performed when the rules match.",
				MarkdownDescription: "The ordered actions performed when the rules match.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							Computed:            true,
							Description:         "The action type.",
							MarkdownDescription: "The action type.",
						},
						"destination": schema.StringAttribute{
							Computed:            true,
							Description:         "The action destination, when applicable.",
							MarkdownDescription: "The action destination, when applicable.",
						},
					},
				},
			},
		},
	}
}

func (d *emailFilterDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config EmailFilterModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}

	account := config.Account.ValueString()
	name := config.Name.ValueString()
	if _, _, err := splitEmailAccountAddress(account); err != nil {
		response.Diagnostics.AddError("Invalid email filter account", err.Error())
		return
	}
	if name == "" {
		response.Diagnostics.AddError(
			"Invalid email filter name",
			"Email filter names must not be empty.",
		)
		return
	}

	unlock := d.client.LockFilterAccount(account)
	defer unlock()

	filter, err := d.client.GetFilter(ctx, account, name)
	if err != nil {
		response.Diagnostics.AddError("Unable to read email filter", err.Error())
		return
	}
	if filter == nil {
		response.Diagnostics.AddError(
			"Email filter not found",
			fmt.Sprintf(
				"No user-level email filter named %q exists for %q.",
				name,
				account,
			),
		)
		return
	}
	if err := validateReadableEmailFilter(*filter); err != nil {
		response.Diagnostics.AddError(
			"Email filter is not readable",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(applyEmailFilterToModel(ctx, &config, *filter)...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &config)...)
}

func (d *emailFilterDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		response.Diagnostics.AddError(
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
