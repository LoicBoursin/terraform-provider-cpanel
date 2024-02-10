package provider

import (
	"crypto/md5"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-cpanel/internal/cpanel/cron"
	"time"
)

type CronJobModel struct {
	LineKey     types.Int64  `tfsdk:"linekey"`
	Weekday     types.String `tfsdk:"weekday"`
	Minute      types.String `tfsdk:"minute"`
	Hour        types.String `tfsdk:"hour"`
	Day         types.String `tfsdk:"day"`
	Month       types.String `tfsdk:"month"`
	Command     types.String `tfsdk:"command"`
	LastUpdated types.String `tfsdk:"last_updated"`
}

func CronJobAPIToModel(cronJobDataSourceModel *cron.CronJobDataSourceModel, internalId string) *CronJobModel {
	for _, data := range cronJobDataSourceModel.CpanelResult.Data {
		if CalculateCronJobDataSourceDataModelInternalId(data) != internalId {
			continue
		}

		return &CronJobModel{
			LineKey:     types.Int64Value(data.LineKey),
			Weekday:     types.StringValue(data.Weekday),
			Minute:      types.StringValue(data.Minute),
			Hour:        types.StringValue(data.Hour),
			Day:         types.StringValue(data.Day),
			Month:       types.StringValue(data.Month),
			Command:     types.StringValue(data.Command),
			LastUpdated: types.StringValue(time.Now().Format(time.RFC3339)),
		}
	}

	return nil
}

func CalculateCronJobDataSourceDataModelInternalId(cronJobDataSourceDataModel cron.CronJobDataSourceDataModel) string {
	return calculateInternalId(
		cronJobDataSourceDataModel.Minute,
		cronJobDataSourceDataModel.Hour,
		cronJobDataSourceDataModel.Day,
		cronJobDataSourceDataModel.Weekday,
		cronJobDataSourceDataModel.Month,
		cronJobDataSourceDataModel.Command,
	)
}

func CalculateCronJobModelInternalId(cronJobModel CronJobModel) string {
	return calculateInternalId(
		cronJobModel.Minute.ValueString(),
		cronJobModel.Hour.ValueString(),
		cronJobModel.Day.ValueString(),
		cronJobModel.Weekday.ValueString(),
		cronJobModel.Month.ValueString(),
		cronJobModel.Command.ValueString(),
	)
}

func calculateInternalId(minute, hour, day, weekday, month, command string) string {
	concatenatedString := fmt.Sprintf("%s-%s-%s-%s-%s-%s",
		minute,
		hour,
		day,
		weekday,
		month,
		command,
	)
	hash := md5.Sum([]byte(concatenatedString))

	return fmt.Sprintf("%x", hash)
}
