package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-cpanel/internal/cpanel/cron"
	"time"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &cronJobResource{}
	_ resource.ResourceWithConfigure = &cronJobResource{}
)

// NewCronJobResource is a helper function to simplify the provider implementation.
func NewCronJobResource() resource.Resource {
	return &cronJobResource{}
}

// cronJobResource is the resource implementation.
type cronJobResource struct {
	client *cron.Client
}

// Metadata returns the resource type name.
func (r *cronJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cron_job"
}

// Schema defines the schema for the resource.
func (r *cronJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
func (r *cronJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Retrieve values from plan
	var plan CronJobModel

	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read crons
	cronJobDataSource, err := r.client.GetCronJobs()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting crons",
			"Could not get crons, unexpected error: "+err.Error(),
		)
		return
	}

	state := CronJobAPIToModel(cronJobDataSource, CalculateCronJobModelInternalId(plan))

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *cronJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan CronJobModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request parameters from plan
	var cronJob cron.CronJobCreateModel
	cronJob.Command = plan.Command.ValueString()
	cronJob.Minute = plan.Minute.ValueString()
	cronJob.Hour = plan.Hour.ValueString()
	cronJob.Day = plan.Day.ValueString()
	cronJob.Weekday = plan.Weekday.ValueString()
	cronJob.Month = plan.Month.ValueString()

	// Create new cron job
	cronJobDataSourceModel, err := r.client.CreateCronJob(cronJob)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating cron job",
			"Could not create cron job, unexpected error: "+err.Error(),
		)
		return
	}
	if len(cronJobDataSourceModel.CpanelResult.Data) != 1 {
		resp.Diagnostics.AddError(
			"Error creating cron job",
			fmt.Sprintf("Could not create cron job, got unexpected errors: %+v", cronJobDataSourceModel),
		)
		return
	}

	cronJobData := cronJobDataSourceModel.CpanelResult.Data[0]

	if cronJobData.Status != 1 {
		resp.Diagnostics.AddError(
			"Error creating cron",
			fmt.Sprintf("Could not create cron, got error: %+v", cronJobData.StatusMsg),
		)
		return
	}

	plan.LineKey = types.Int64Value(cronJobData.LineKey)
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC3339))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *cronJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan CronJobModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state CronJobModel

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request parameters from plan
	var cronJob cron.CronJobUpdateModel
	cronJob.LineKey = state.LineKey.ValueInt64()
	cronJob.Command = plan.Command.ValueString()
	cronJob.Minute = plan.Minute.ValueString()
	cronJob.Hour = plan.Hour.ValueString()
	cronJob.Day = plan.Day.ValueString()
	cronJob.Weekday = plan.Weekday.ValueString()
	cronJob.Month = plan.Month.ValueString()

	cronJobDataSourceModel, err := r.client.UpdateCronJob(cronJob)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating cron job",
			"Could not updating cron job, unexpected error: "+err.Error(),
		)
		return
	}

	cronJobData := cronJobDataSourceModel.CpanelResult.Data[0]

	if cronJobData.Status != 1 {
		resp.Diagnostics.AddError(
			"Error updating cron job",
			fmt.Sprintf("Could not update cron job, got error: %+v", cronJobData.StatusMsg),
		)
		return
	}

	plan.LineKey = types.Int64Value(cronJobData.LineKey)
	plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC3339))

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *cronJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state CronJobModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cronJob cron.CronJobDeleteModel
	cronJob.LineKey = state.LineKey.ValueInt64()

	// Delete existing user
	cronJobDataSourceModel, err := r.client.DeleteCronJob(cronJob)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting cron job",
			"Could not deleting cron job, unexpected error: "+err.Error(),
		)
		return
	}

	cronJobData := cronJobDataSourceModel.CpanelResult.Data[0]

	if cronJobData.Status != 1 {
		resp.Diagnostics.AddError(
			"Error deleting user",
			fmt.Sprintf("Could not create cron, got error: %+v", cronJobData.StatusMsg),
		)
		return
	}
}

// Configure adds the provider configured client to the resource.
func (r *cronJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected map[string]interface{}, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	cronClient, ok := providerData["cron"].(*cron.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Cron Client Type",
			fmt.Sprintf("Expected *cron.Client, got: %T. Please report this issue to the provider developers.", providerData["cron"]),
		)
		return
	}

	r.client = cronClient
}
