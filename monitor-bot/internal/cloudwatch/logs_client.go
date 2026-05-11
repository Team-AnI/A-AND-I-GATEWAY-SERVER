package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

type LogsAPI interface {
	StartQuery(context.Context, *cloudwatchlogs.StartQueryInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartQueryOutput, error)
	GetQueryResults(context.Context, *cloudwatchlogs.GetQueryResultsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.GetQueryResultsOutput, error)
}

type LogsClient struct {
	api          LogsAPI
	timeout      time.Duration
	pollInterval time.Duration
	limit        int
}

func NewLogsClient(api LogsAPI, timeout, pollInterval time.Duration, limit int) *LogsClient {
	return &LogsClient{api: api, timeout: timeout, pollInterval: pollInterval, limit: limit}
}

func (c *LogsClient) Query(ctx context.Context, logGroups []string, query string, since time.Duration, limit int32) ([]map[string]string, error) {
	if len(logGroups) == 0 {
		return nil, fmt.Errorf("log groups are required")
	}
	if limit <= 0 {
		limit = int32(c.limit)
	}
	start, end := TimeRange(since)
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	started, err := c.api.StartQuery(queryCtx, &cloudwatchlogs.StartQueryInput{
		LogGroupNames: logGroups,
		QueryString:   aws.String(query),
		StartTime:     aws.Int64(start),
		EndTime:       aws.Int64(end),
		Limit:         aws.Int32(limit),
	})
	if err != nil {
		return nil, err
	}
	if started.QueryId == nil || *started.QueryId == "" {
		return nil, fmt.Errorf("cloudwatch returned empty query id")
	}

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-queryCtx.Done():
			return nil, queryCtx.Err()
		case <-ticker.C:
			results, err := c.api.GetQueryResults(queryCtx, &cloudwatchlogs.GetQueryResultsInput{QueryId: started.QueryId})
			if err != nil {
				return nil, err
			}
			switch results.Status {
			case types.QueryStatusComplete:
				return flattenResults(results.Results), nil
			case types.QueryStatusFailed, types.QueryStatusCancelled, types.QueryStatusTimeout, types.QueryStatusUnknown:
				return nil, fmt.Errorf("cloudwatch query ended with status: %s", results.Status)
			}
		}
	}
}

func flattenResults(results [][]types.ResultField) []map[string]string {
	rows := make([]map[string]string, 0, len(results))
	for _, result := range results {
		row := make(map[string]string)
		for _, field := range result {
			if field.Field != nil && field.Value != nil {
				row[*field.Field] = *field.Value
			}
		}
		rows = append(rows, row)
	}
	return rows
}
