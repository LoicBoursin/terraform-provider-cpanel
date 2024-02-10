package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"terraform-provider-cpanel/internal/cpanel/cron"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &cronJobDataSource{}
	_ datasource.DataSourceWithConfigure = &cronJobDataSource{}
)

// NewCronJobDataSource is a helper function to simplify the provider implementation.
func NewCronJobDataSource() datasource.DataSource {
	return &cronJobDataSource{}
}

// cronJobDataSource is the data source implementation.
type cronJobDataSource struct {
	client *cron.Client
}

// Configure adds the provider configured client to the data source.
func (d *cronJobDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected map[string]interface{}, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	cronClient, ok := providerData["cron"].(*cron.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Cron Client Type",
			fmt.Sprintf("Expected *postgresql.Client, got: %T. Please report this issue to the provider developers.", providerData["postgresql"]),
		)
		return
	}

	d.client = cronClient
}

// Metadata returns the data source type name.
func (d *cronJobDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cron_job"
}

// Schema defines the schema for the data source.
func (d *cronJobDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"command": schema.StringAttribute{
				Required:            true,
				Description:         "The command to run.",
				MarkdownDescription: "The command to run.",
			},
			"minute": schema.StringAttribute{
				Required:            true,
				Description:         "The minute of the hour to run the cron job. Expressions such as */5 or 0,30 are allowed.",
				MarkdownDescription: "The minute of the hour to run the cron job. Expressions such as */5 or 0,30 are allowed.",
			},
			"hour": schema.StringAttribute{
				Required:            true,
				Description:         "The hour of the day to run the cron job. Expressions such as */2 or 0,12 are allowed.",
				MarkdownDescription: "The hour of the day to run the cron job. Expressions such as */2 or 0,12 are allowed.",
			},
			"day": schema.StringAttribute{
				Required:            true,
				Description:         "The day of the month to run the cron job. Expressions such as */15 are allowed.",
				MarkdownDescription: "The day of the month to run the cron job. Expressions such as */15 are allowed.",
			},
			"weekday": schema.StringAttribute{
				Required:            true,
				Description:         "The day of the week to run the cron job.",
				MarkdownDescription: "The day of the week to run the cron job.",
				Validators: []validator.String{
					stringvalidator.OneOf("0", "1", "2", "3", "4", "5", "6", "7", "*"),
				},
			},
			"month": schema.StringAttribute{
				Required:            true,
				Description:         "The month of the year to run the cron job. Expressions such as */3 or 1,4,7 are allowed.",
				MarkdownDescription: "The month of the year to run the cron job. Expressions such as */3 or 1,4,7 are allowed.",
			},
			"linekey": schema.Int64Attribute{
				Computed:            true,
				Description:         "The cron job ID.",
				MarkdownDescription: "The cron job ID.",
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *cronJobDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CronJobModel

	// Read Terraform configuration data into the state
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	cronJobs, err := d.client.GetCronJobs()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to Read Cron jobs: %s", err),
			err.Error(),
		)
		return
	}

	state := CronJobAPIToModel(cronJobs, CalculateCronJobModelInternalId(config))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
