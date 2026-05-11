package cloudwatch

import (
	"context"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type AlarmAPI interface {
	DescribeAlarms(context.Context, *cloudwatch.DescribeAlarmsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
}

type AlarmClient struct {
	api AlarmAPI
}

func NewAlarmClient(api AlarmAPI) *AlarmClient {
	return &AlarmClient{api: api}
}

func (c *AlarmClient) AlarmNames(ctx context.Context) ([]string, error) {
	var names []string
	var nextToken *string
	for {
		out, err := c.api.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
			StateValue: types.StateValueAlarm,
			NextToken:  nextToken,
		})
		if err != nil {
			return nil, err
		}
		for _, alarm := range out.MetricAlarms {
			if alarm.AlarmName != nil {
				names = append(names, *alarm.AlarmName)
			}
		}
		for _, alarm := range out.CompositeAlarms {
			if alarm.AlarmName != nil {
				names = append(names, *alarm.AlarmName)
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	sort.Strings(names)
	return names, nil
}
