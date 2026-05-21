package cloudwatch

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func TestAlarmStateValueSupportsOpsAlarmStates(t *testing.T) {
	cases := []struct {
		input      string
		want       types.StateValue
		wantFilter bool
	}{
		{input: "", want: types.StateValueAlarm, wantFilter: true},
		{input: "ALARM", want: types.StateValueAlarm, wantFilter: true},
		{input: "OK", want: types.StateValueOk, wantFilter: true},
		{input: "INSUFFICIENT_DATA", want: types.StateValueInsufficientData, wantFilter: true},
		{input: "all", want: "", wantFilter: false},
	}
	for _, tt := range cases {
		got, gotFilter := alarmStateValue(tt.input)
		if got != tt.want || gotFilter != tt.wantFilter {
			t.Fatalf("alarmStateValue(%q) = (%q, %v), want (%q, %v)", tt.input, got, gotFilter, tt.want, tt.wantFilter)
		}
	}
}
