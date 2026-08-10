package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/cron"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                 = &cronJobResource{}
	_ resource.ResourceWithConfigure    = &cronJobResource{}
	_ resource.ResourceWithImportState  = &cronJobResource{}
	_ resource.ResourceWithUpgradeState = &cronJobResource{}
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
		Description:         "Manages a cPanel cron command identified by its stable line key.",
		MarkdownDescription: "Manages a cPanel cron command identified by its stable line key.",
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
				Validators:          cronMinuteValidators(),
			},
			"hour": schema.StringAttribute{
				Required:            true,
				Description:         "The hour of the day to run the cron job. Expressions such as */2 or 0,12 are allowed.",
				MarkdownDescription: "The hour of the day to run the cron job. Expressions such as */2 or 0,12 are allowed.",
				Validators:          cronHourValidators(),
			},
			"day": schema.StringAttribute{
				Required:            true,
				Description:         "The day of the month to run the cron job. Expressions such as */15 are allowed.",
				MarkdownDescription: "The day of the month to run the cron job. Expressions such as */15 are allowed.",
				Validators:          cronDayValidators(),
			},
			"weekday": schema.StringAttribute{
				Required:            true,
				Description:         "The day of the week to run the cron job.",
				MarkdownDescription: "The day of the week to run the cron job.",
				Validators:          cronWeekdayValidators(),
			},
			"month": schema.StringAttribute{
				Required:            true,
				Description:         "The month of the year to run the cron job. Expressions such as */3 are allowed.",
				MarkdownDescription: "The month of the year to run the cron job. Expressions such as */3 are allowed.",
				Validators:          cronMonthValidators(),
			},
			"linekey": schema.StringAttribute{
				Computed:            true,
				Description:         "The stable cPanel line key used as the cron job identity and import ID.",
				MarkdownDescription: "The stable cPanel line key used as the cron job identity and import ID.",
			},
		},
		Version: 1,
	}
}

func (r *cronJobResource) UpgradeState(
	_ context.Context,
) map[int64]resource.StateUpgrader {
	priorSchema := schema.Schema{
		Attributes: map[string]schema.Attribute{
			"command": schema.StringAttribute{Required: true},
			"minute":  schema.StringAttribute{Required: true},
			"hour":    schema.StringAttribute{Required: true},
			"day":     schema.StringAttribute{Required: true},
			"weekday": schema.StringAttribute{Required: true},
			"month":   schema.StringAttribute{Required: true},
			"linekey": schema.Int64Attribute{Computed: true},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}

	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &priorSchema,
			StateUpgrader: func(
				ctx context.Context,
				req resource.UpgradeStateRequest,
				resp *resource.UpgradeStateResponse,
			) {
				var prior cronJobModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				lineKey := types.StringNull()
				switch {
				case prior.LineKey.IsUnknown():
					lineKey = types.StringUnknown()
				case !prior.LineKey.IsNull():
					lineKey = types.StringValue(strconv.FormatInt(
						prior.LineKey.ValueInt64(),
						10,
					))
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, &CronJobModel{
					LineKey: lineKey,
					Weekday: prior.Weekday,
					Minute:  prior.Minute,
					Hour:    prior.Hour,
					Day:     prior.Day,
					Month:   prior.Month,
					Command: prior.Command,
				})...)
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

	cronJobs, err := r.client.GetCronJobs(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cron jobs",
			"Could not read cron jobs: "+err.Error(),
		)
		return
	}

	state := CronJobAPIToModelByLineKey(cronJobs, plan.LineKey.ValueString())
	if state == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, state)
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
	cronJobDataSourceModel, err := r.client.CreateCronJob(ctx, cronJob)

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cron job",
			"Could not create cron job: "+err.Error(),
		)
		return
	}
	if len(cronJobDataSourceModel.CpanelResult.Data) != 1 {
		resp.Diagnostics.AddError(
			"Unexpected cron creation response",
			fmt.Sprintf(
				"cPanel returned %d result records while creating the cron job; expected exactly one.",
				len(cronJobDataSourceModel.CpanelResult.Data),
			),
		)
		return
	}

	cronJobData := cronJobDataSourceModel.CpanelResult.Data[0]

	if cronJobData.Status != 1 {
		resp.Diagnostics.AddError(
			"Unable to create cron job",
			cronMutationErrorMessage(cronJobData.StatusMsg, cronJobData.Reason),
		)
		return
	}
	if cronJobData.LineKey == "" {
		resp.Diagnostics.AddError(
			"Unexpected cron creation response",
			"cPanel did not return a valid line key for the new cron job.",
		)
		return
	}

	plan.LineKey = types.StringValue(string(cronJobData.LineKey))

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
	cronJob.LineKey = state.LineKey.ValueString()
	cronJob.Command = plan.Command.ValueString()
	cronJob.Minute = plan.Minute.ValueString()
	cronJob.Hour = plan.Hour.ValueString()
	cronJob.Day = plan.Day.ValueString()
	cronJob.Weekday = plan.Weekday.ValueString()
	cronJob.Month = plan.Month.ValueString()
	cronJob.Expected = &cron.CronJobDetailsModel{
		Command: state.Command.ValueString(),
		Minute:  state.Minute.ValueString(),
		Hour:    state.Hour.ValueString(),
		Day:     state.Day.ValueString(),
		Weekday: state.Weekday.ValueString(),
		Month:   state.Month.ValueString(),
	}

	cronJobDataSourceModel, err := r.client.UpdateCronJob(ctx, cronJob)

	if err != nil {
		switch {
		case errors.Is(err, cron.ErrCronJobNotFound):
			resp.Diagnostics.AddError(
				"Cron job no longer exists",
				"Refresh the Terraform state before updating the cron job.",
			)
		case errors.Is(err, cron.ErrCronJobChanged):
			resp.Diagnostics.AddError(
				"Cron job changed during update",
				"The remote cron schedule or command no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to update cron job",
				"Could not update cron job: "+err.Error(),
			)
		}
		return
	}
	if len(cronJobDataSourceModel.CpanelResult.Data) != 1 {
		resp.Diagnostics.AddError(
			"Unexpected cron update response",
			fmt.Sprintf(
				"cPanel returned %d result records while updating the cron job; expected exactly one.",
				len(cronJobDataSourceModel.CpanelResult.Data),
			),
		)
		return
	}

	cronJobData := cronJobDataSourceModel.CpanelResult.Data[0]

	if cronJobData.Status != 1 {
		resp.Diagnostics.AddError(
			"Unable to update cron job",
			cronMutationErrorMessage(cronJobData.StatusMsg, cronJobData.Reason),
		)
		return
	}

	plan.LineKey = state.LineKey
	if cronJobData.LineKey != "" {
		plan.LineKey = types.StringValue(string(cronJobData.LineKey))
	}

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
	cronJob.LineKey = state.LineKey.ValueString()
	cronJob.Expected = &cron.CronJobDetailsModel{
		Command: state.Command.ValueString(),
		Minute:  state.Minute.ValueString(),
		Hour:    state.Hour.ValueString(),
		Day:     state.Day.ValueString(),
		Weekday: state.Weekday.ValueString(),
		Month:   state.Month.ValueString(),
	}

	cronJobs, err := r.client.GetCronJobs(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cron job",
			err.Error(),
		)
		return
	}
	existing := CronJobAPIToModelByLineKey(cronJobs, cronJob.LineKey)
	if existing == nil {
		return
	}
	if !cronJobModelsEqual(*existing, state) {
		resp.Diagnostics.AddError(
			"Refusing to delete changed cron job",
			fmt.Sprintf(
				"Cron job %q no longer matches the schedule and command stored in Terraform state. Refresh and review the replacement before retrying.",
				cronJob.LineKey,
			),
		)
		return
	}

	cronJobDataSourceModel, deleteErr := r.client.DeleteCronJob(ctx, cronJob)
	if errors.Is(deleteErr, cron.ErrCronJobChanged) {
		resp.Diagnostics.AddError(
			"Refusing to delete changed cron job",
			fmt.Sprintf(
				"Cron job %q changed immediately before deletion. Refresh and review the replacement before retrying.",
				cronJob.LineKey,
			),
		)
		return
	}
	remainingCronJobs, verifyErr := r.client.GetCronJobs(ctx)
	if verifyErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify cron job deletion",
			fmt.Sprintf(
				"Delete response: %v. Follow-up read failed: %v",
				deleteErr,
				verifyErr,
			),
		)
		return
	}
	remaining := CronJobAPIToModelByLineKey(
		remainingCronJobs,
		cronJob.LineKey,
	)
	if remaining == nil {
		return
	}
	if !cronJobModelsEqual(*remaining, state) {
		resp.Diagnostics.AddError(
			"Cron job replacement preserved",
			fmt.Sprintf(
				"Cron job %q exists after the deletion attempt but no longer matches the cron job stored in Terraform state. Terraform will not delete the replacement.",
				cronJob.LineKey,
			),
		)
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete cron job",
			"Could not delete cron job: "+deleteErr.Error(),
		)
		return
	}
	if cronJobDataSourceModel != nil &&
		len(cronJobDataSourceModel.CpanelResult.Data) == 1 {
		cronJobData := cronJobDataSourceModel.CpanelResult.Data[0]
		if cronJobData.Status != 1 {
			resp.Diagnostics.AddError(
				"Unable to delete cron job",
				cronMutationErrorMessage(
					cronJobData.StatusMsg,
					cronJobData.Reason,
				),
			)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Unable to verify cron job deletion",
		fmt.Sprintf(
			"Cron job %q still exists after cPanel reported a successful deletion.",
			cronJob.LineKey,
		),
	)
}

func cronJobModelsEqual(left, right CronJobModel) bool {
	return left.LineKey.Equal(right.LineKey) &&
		left.Weekday.Equal(right.Weekday) &&
		left.Minute.Equal(right.Minute) &&
		left.Hour.Equal(right.Hour) &&
		left.Day.Equal(right.Day) &&
		left.Month.Equal(right.Month) &&
		left.Command.Equal(right.Command)
}

func (r *cronJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	lineKey := strings.TrimSpace(req.ID)
	if lineKey == "" || lineKey != req.ID {
		resp.Diagnostics.AddError(
			"Invalid cron job import identifier",
			fmt.Sprintf("Expected a non-empty cPanel line key without surrounding whitespace, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("linekey"), lineKey)...)
}

func cronMutationErrorMessage(statusMessage, reason string) string {
	switch {
	case statusMessage != "" && reason != "":
		return statusMessage + ": " + reason
	case statusMessage != "":
		return statusMessage
	case reason != "":
		return reason
	default:
		return "cPanel reported an unsuccessful cron operation without an error message."
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
