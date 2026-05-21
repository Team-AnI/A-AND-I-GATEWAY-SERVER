package cloudwatch

import (
	"context"
	"sort"
	"strings"

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
	return c.AlarmNamesByState(ctx, "ALARM")
}

func (c *AlarmClient) AlarmNamesByState(ctx context.Context, state string) ([]string, error) {
	var names []string
	var nextToken *string
	inputState, filterByState := alarmStateValue(state)
	for {
		input := &cloudwatch.DescribeAlarmsInput{NextToken: nextToken}
		if filterByState {
			input.StateValue = inputState
		}
		out, err := c.api.DescribeAlarms(ctx, input)
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

func alarmStateValue(state string) (types.StateValue, bool) {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "", "ALARM":
		return types.StateValueAlarm, true
	case "OK":
		return types.StateValueOk, true
	case "INSUFFICIENT_DATA":
		return types.StateValueInsufficientData, true
	case "ALL":
		return "", false
	default:
		return types.StateValueAlarm, true
	}
}
