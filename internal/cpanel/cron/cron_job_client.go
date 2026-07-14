package cron

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrCronJobNotFound        = errors.New("cron job not found")
	ErrCronJobChanged         = errors.New("cron job changed")
	ErrCronJobCreateAmbiguous = errors.New("cron job creation is ambiguous")
)

func (c *Client) CreateCronJob(
	ctx context.Context,
	input CronJobCreateModel,
) (*CronJobCreateDataSourceModel, error) {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	before, err := c.GetCronJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read cron jobs before creation: %w", err)
	}

	cronJob := CronJobCreateDataSourceModel{}
	mutationErr := c.executeMutation(ctx, OperationAddLine, map[string]string{
		"command": input.Command,
		"minute":  input.Minute,
		"hour":    input.Hour,
		"day":     input.Day,
		"weekday": input.Weekday,
		"month":   input.Month,
	}, &cronJob)
	if mutationErr == nil &&
		len(cronJob.CpanelResult.Data) == 1 &&
		cronJob.CpanelResult.Data[0].Status == 1 &&
		cronJob.CpanelResult.Data[0].LineKey != "" {
		return &cronJob, nil
	}

	after, readErr := c.GetCronJobs(ctx)
	if readErr != nil {
		if mutationErr != nil {
			return nil, fmt.Errorf(
				"create cron job: %w; reconcile creation: %v",
				mutationErr,
				readErr,
			)
		}

		return &cronJob, fmt.Errorf(
			"reconcile cron job creation: %w",
			readErr,
		)
	}

	beforeLineKeys := make(map[CronLineKey]struct{})
	for _, existing := range before.CpanelResult.Data {
		if existing.Type == "command" {
			beforeLineKeys[existing.LineKey] = struct{}{}
		}
	}
	candidates := make([]CronJobDataSourceDataModel, 0, 1)
	for _, existing := range after.CpanelResult.Data {
		if existing.Type != "command" ||
			existing.CronJobDetailsModel != input.CronJobDetailsModel {
			continue
		}
		if _, existed := beforeLineKeys[existing.LineKey]; !existed {
			candidates = append(candidates, existing)
		}
	}
	if len(candidates) == 1 {
		return &CronJobCreateDataSourceModel{
			CpanelResult: CronJobCreateCpanelResultModel{
				Data: []CronJobCreateDataSourceDataModel{{
					LineKey: candidates[0].LineKey,
					CronJobCommonDataSourceDataModel: CronJobCommonDataSourceDataModel{
						StatusMsg: "reconciled",
						Status:    1,
						Result:    1,
					},
				}},
			},
		}, nil
	}
	if len(candidates) > 1 {
		return nil, fmt.Errorf(
			"%w: found %d newly created matching entries",
			ErrCronJobCreateAmbiguous,
			len(candidates),
		)
	}
	if mutationErr != nil {
		return nil, mutationErr
	}

	return &cronJob, nil
}

func cronJobByLineKey(
	cronJobs *CronJobDataSourceModel,
	lineKey string,
) *CronJobDataSourceDataModel {
	if cronJobs == nil {
		return nil
	}
	for index := range cronJobs.CpanelResult.Data {
		job := &cronJobs.CpanelResult.Data[index]
		if job.Type == "command" && string(job.LineKey) == lineKey {
			return job
		}
	}

	return nil
}

func (c *Client) UpdateCronJob(
	ctx context.Context,
	input CronJobUpdateModel,
) (*CronJobCreateDataSourceModel, error) {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	if input.Expected != nil {
		cronJobs, err := c.GetCronJobs(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"read cron job before update: %w",
				err,
			)
		}
		current := cronJobByLineKey(cronJobs, input.LineKey)
		if current == nil {
			return nil, fmt.Errorf(
				"%w: %q",
				ErrCronJobNotFound,
				input.LineKey,
			)
		}
		if current.CronJobDetailsModel != *input.Expected {
			return nil, fmt.Errorf(
				"%w: %q",
				ErrCronJobChanged,
				input.LineKey,
			)
		}
	}

	cronJob := CronJobCreateDataSourceModel{}
	err := c.executeMutation(ctx, OperationEditLine, map[string]string{
		"linekey": input.LineKey,
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

func (c *Client) GetCronJobs(ctx context.Context) (*CronJobDataSourceModel, error) {
	cronJobs := CronJobDataSourceModel{}
	err := c.executeReadOperation(ctx, OperationFetchCron, map[string]string{}, &cronJobs)

	if err != nil {
		return nil, err
	}

	return &cronJobs, nil
}

func (c *Client) DeleteCronJob(
	ctx context.Context,
	input CronJobDeleteModel,
) (*CronJobDeleteDataSourceModel, error) {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	cronJobs, err := c.GetCronJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve cron line before deletion: %w", err)
	}

	current := cronJobByLineKey(cronJobs, input.LineKey)
	if current == nil {
		return nil, nil
	}
	if input.Expected != nil &&
		current.CronJobDetailsModel != *input.Expected {
		return nil, fmt.Errorf(
			"%w: %q",
			ErrCronJobChanged,
			input.LineKey,
		)
	}
	commandNumber := current.CommandNumber
	if commandNumber < 1 {
		return nil, fmt.Errorf(
			"cron job %q has invalid command number %d",
			input.LineKey,
			commandNumber,
		)
	}

	cronJob := CronJobDeleteDataSourceModel{}
	err = c.executeMutation(ctx, OperationRemoveLine, map[string]string{
		"line": strconv.FormatInt(commandNumber, 10),
	}, &cronJob)

	if err != nil {
		return nil, err
	}

	return &cronJob, nil
}
