package provider

import (
	"crypto/md5"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/cron"
)

type CronJobModel struct {
	LineKey types.String `tfsdk:"linekey"`
	Weekday types.String `tfsdk:"weekday"`
	Minute  types.String `tfsdk:"minute"`
	Hour    types.String `tfsdk:"hour"`
	Day     types.String `tfsdk:"day"`
	Month   types.String `tfsdk:"month"`
	Command types.String `tfsdk:"command"`
}

type cronJobModelV0 struct {
	LineKey     types.Int64  `tfsdk:"linekey"`
	Weekday     types.String `tfsdk:"weekday"`
	Minute      types.String `tfsdk:"minute"`
	Hour        types.String `tfsdk:"hour"`
	Day         types.String `tfsdk:"day"`
	Month       types.String `tfsdk:"month"`
	Command     types.String `tfsdk:"command"`
	LastUpdated types.String `tfsdk:"last_updated"`
}

func CronJobAPIToModelByLineKey(
	cronJobDataSourceModel *cron.CronJobDataSourceModel,
	lineKey string,
) *CronJobModel {
	for _, data := range cronJobDataSourceModel.CpanelResult.Data {
		if data.Type != "command" || string(data.LineKey) != lineKey {
			continue
		}

		return cronJobDataToModel(data)
	}

	return nil
}

func CronJobAPIToModelsByInternalID(
	cronJobDataSourceModel *cron.CronJobDataSourceModel,
	internalID string,
) []*CronJobModel {
	var matches []*CronJobModel

	for _, data := range cronJobDataSourceModel.CpanelResult.Data {
		if data.Type != "command" || CalculateCronJobDataSourceDataModelInternalID(data) != internalID {
			continue
		}

		matches = append(matches, cronJobDataToModel(data))
	}

	return matches
}

func cronJobDataToModel(data cron.CronJobDataSourceDataModel) *CronJobModel {
	return &CronJobModel{
		LineKey: types.StringValue(string(data.LineKey)),
		Weekday: types.StringValue(data.Weekday),
		Minute:  types.StringValue(data.Minute),
		Hour:    types.StringValue(data.Hour),
		Day:     types.StringValue(data.Day),
		Month:   types.StringValue(data.Month),
		Command: types.StringValue(data.Command),
	}
}

func CalculateCronJobDataSourceDataModelInternalID(cronJobDataSourceDataModel cron.CronJobDataSourceDataModel) string {
	return calculateInternalID(
		cronJobDataSourceDataModel.Minute,
		cronJobDataSourceDataModel.Hour,
		cronJobDataSourceDataModel.Day,
		cronJobDataSourceDataModel.Weekday,
		cronJobDataSourceDataModel.Month,
		cronJobDataSourceDataModel.Command,
	)
}

func CalculateCronJobModelInternalID(cronJobModel CronJobModel) string {
	return calculateInternalID(
		cronJobModel.Minute.ValueString(),
		cronJobModel.Hour.ValueString(),
		cronJobModel.Day.ValueString(),
		cronJobModel.Weekday.ValueString(),
		cronJobModel.Month.ValueString(),
		cronJobModel.Command.ValueString(),
	)
}

func calculateInternalID(minute, hour, day, weekday, month, command string) string {
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
