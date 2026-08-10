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
	_ datasource.DataSource              = &calendarDelegateDataSource{}
	_ datasource.DataSourceWithConfigure = &calendarDelegateDataSource{}
)

func NewCalendarDelegateDataSource() datasource.DataSource {
	return &calendarDelegateDataSource{}
}

type calendarDelegateDataSource struct {
	client *cpanelcalendar.Client
}

func (d *calendarDelegateDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_calendar_delegate"
}

func (d *calendarDelegateDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Looks up access from one cPanel mail account to another account's default calendar.",
		MarkdownDescription: "Looks up access from one cPanel mail account to another account's default CalDAV calendar.",
		Attributes: map[string]schema.Attribute{
			"delegator": schema.StringAttribute{
				Required:            true,
				Description:         "The mail account that owns the calendar.",
				MarkdownDescription: "The mail account that owns the calendar.",
				Validators:          emailAddressValidators(),
			},
			"delegatee": schema.StringAttribute{
				Required:            true,
				Description:         "The mail account that receives access.",
				MarkdownDescription: "The mail account that receives access.",
				Validators:          emailAddressValidators(),
			},
			"calendar": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "The cPanel calendar collection identifier.",
				MarkdownDescription: "The cPanel calendar collection identifier. Only the default `calendar` collection is supported.",
				Validators:          calendarNameValidators(),
			},
			"readonly": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether the delegate may only read the calendar.",
				MarkdownDescription: "Whether the delegate may only read the calendar.",
			},
			"calendar_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The CPDAVD display name for the delegated calendar. Legacy CCS returns an empty value.",
				MarkdownDescription: "The `CPDAVD` display name for the delegated calendar. The value is empty when the legacy `CCS` API is used.",
			},
		},
	}
}

func (d *calendarDelegateDataSource) Read(
	ctx context.Context,
	request datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	var config CalendarDelegateModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}
	if config.Calendar.IsNull() || config.Calendar.IsUnknown() {
		config.Calendar = types.StringValue(cpanelcalendar.DefaultCalendar)
	}

	definition := calendarDelegateDefinitionFromModel(config)
	if err := validateCalendarDelegateDefinition(definition); err != nil {
		response.Diagnostics.AddError(
			"Invalid calendar delegate",
			err.Error(),
		)
		return
	}

	unlock := d.client.LockDelegate(
		definition.Delegator,
		definition.Calendar,
		definition.Delegatee,
	)
	defer unlock()

	delegate, err := d.client.GetDelegate(
		ctx,
		definition.Delegator,
		definition.Calendar,
		definition.Delegatee,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read calendar delegate",
			err.Error(),
		)
		return
	}
	if delegate == nil {
		response.Diagnostics.AddError(
			"Calendar delegate not found",
			fmt.Sprintf(
				"No delegation from %q to %q exists for calendar %q.",
				definition.Delegator,
				definition.Delegatee,
				definition.Calendar,
			),
		)
		return
	}

	applyCalendarDelegateToModel(&config, *delegate)
	response.Diagnostics.Append(response.State.Set(ctx, &config)...)
}

func (d *calendarDelegateDataSource) Configure(
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
