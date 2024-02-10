package cron

import "strconv"

func (c *Client) CreateCronJob(input CronJobCreateModel) (*CronJobCreateDataSourceModel, error) {
	cronJob := CronJobCreateDataSourceModel{}

	err := c.executeOperation(OperationAddLine, map[string]string{
		"command": input.Command,
		"minute":  input.Minute,
		"hour":    input.Hour,
		"day":     input.Day,
		"weekday": input.Weekday,
		"month":   input.Month,
	}, &cronJob)

	if err != nil {
		return nil, err
	}

	return &cronJob, nil
}

func (c *Client) UpdateCronJob(input CronJobUpdateModel) (*CronJobCreateDataSourceModel, error) {
	cronJob := CronJobCreateDataSourceModel{}
	err := c.executeOperation(OperationEditLine, map[string]string{
		"linekey": strconv.FormatInt(input.LineKey, 10),
		"weekday": input.Weekday,
		"command": input.Command,
		"day":     input.Day,
		"hour":    input.Hour,
		"minute":  input.Minute,
		"month":   input.Month,
	}, &cronJob)

	if err != nil {
		return nil, err
	}

	return &cronJob, nil
}

func (c *Client) GetCronJobs() (*CronJobDataSourceModel, error) {
	cronJobs := CronJobDataSourceModel{}
	err := c.executeOperation(OperationFetchCron, map[string]string{}, &cronJobs)

	if err != nil {
		return nil, err
	}

	return &cronJobs, nil
}

func (c *Client) DeleteCronJob(input CronJobDeleteModel) (*CronJobDeleteDataSourceModel, error) {
	cronJob := CronJobDeleteDataSourceModel{}
	err := c.executeOperation(OperationRemoveLine, map[string]string{
		"linekey": strconv.FormatInt(input.LineKey, 10),
	}, &cronJob)

	if err != nil {
		return nil, err
	}

	return &cronJob, nil
}
