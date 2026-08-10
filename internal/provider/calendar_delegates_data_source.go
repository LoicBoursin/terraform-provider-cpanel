package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

var (
	_ datasource.DataSource              = &calendarDelegatesDataSource{}
	_ datasource.DataSourceWithConfigure = &calendarDelegatesDataSource{}
)

func NewCalendarDelegatesDataSource() datasource.DataSource {
	return &calendarDelegatesDataSource{}
}

type calendarDelegatesDataSource struct {
	client *cpanelcalendar.Client
}

type CalendarDelegatesDataSourceModel struct {
	Delegates []CalendarDelegateInventoryModel `tfsdk:"delegates"`
}

type CalendarDelegateInventoryModel struct {
	Delegator    types.String `tfsdk:"delegator"`
	Delegatee    types.String `tfsdk:"delegatee"`
	Calendar     types.String `tfsdk:"calendar"`
	CalendarName types.String `tfsdk:"calendar_name"`
	ReadOnly     types.Bool   `tfsdk:"readonly"`
}

func (d *calendarDelegatesDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_calendar_delegates"
}

func (d *calendarDelegatesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel calendar delegation inventory.",
		MarkdownDescription: "Reads the complete cPanel calendar delegation inventory through `CPDAVD::list_delegates`, with a legacy `CCS` fallback only when cPanel reports that `CPDAVD` is unavailable.",
		Attributes: map[string]schema.Attribute{
			"delegates": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The calendar delegations in stable identity order.",
				MarkdownDescription: "The calendar delegations sorted by delegator, delegatee, and calendar identifier.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"delegator": schema.StringAttribute{
							Computed:            true,
							Description:         "The DAV user that owns the calendar.",
							MarkdownDescription: "The DAV user that owns the calendar.",
						},
						"delegatee": schema.StringAttribute{
							Computed:            true,
							Description:         "The DAV user that receives access.",
							MarkdownDescription: "The DAV user that receives access.",
						},
						"calendar": schema.StringAttribute{
							Computed:            true,
							Description:         "The delegated calendar collection identifier.",
							MarkdownDescription: "The delegated calendar collection identifier.",
						},
						"calendar_name": schema.StringAttribute{
							Computed:            true,
							Description:         "The delegated calendar display name, or an empty string when unavailable.",
							MarkdownDescription: "The delegated calendar display name, or an empty string when unavailable.",
						},
						"readonly": schema.BoolAttribute{
							Computed:            true,
							Description:         "Whether the delegate has read-only access.",
							MarkdownDescription: "Whether the delegate has read-only access.",
						},
					},
				},
			},
		},
	}
}

func (d *calendarDelegatesDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	delegates, err := d.client.ListDelegates(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel calendar delegation inventory",
			err.Error(),
		)
		return
	}

	model := CalendarDelegatesDataSourceModel{
		Delegates: make(
			[]CalendarDelegateInventoryModel,
			0,
			len(delegates),
		),
	}
	for _, delegate := range delegates {
		model.Delegates = append(
			model.Delegates,
			CalendarDelegateInventoryModel{
				Delegator:    types.StringValue(delegate.Delegator),
				Delegatee:    types.StringValue(delegate.Delegatee),
				Calendar:     types.StringValue(delegate.Calendar),
				CalendarName: types.StringValue(delegate.CalendarName),
				ReadOnly:     types.BoolValue(delegate.ReadOnly),
			},
		)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *calendarDelegatesDataSource) Configure(
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
	client, ok := providerData["calendar"].(*cpanelcalendar.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Calendar Client Type",
			fmt.Sprintf(
				"Expected *calendar.Client, got: %T.",
				providerData["calendar"],
			),
		)
		return
	}
	d.client = client
}
