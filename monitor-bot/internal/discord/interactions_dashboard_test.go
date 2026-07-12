package discord

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
)

type dashboardOpsRecorder struct {
	OpsController
	watchCall     *dashboardWatchCall
	watchResult   string
	unwatchCall   *dashboardUnwatchCall
	unwatchResult string
	listCalls     int
	listResult    string
}

type dashboardWatchCall struct {
	channelID string
	scope     string
	service   string
	interval  time.Duration
}

type dashboardUnwatchCall struct {
	scope   string
	service string
}

type dashboardAlarmAPI struct{}

func (dashboardAlarmAPI) DescribeAlarms(
	context.Context,
	*cloudwatch.DescribeAlarmsInput,
	...func(*cloudwatch.Options),
) (*cloudwatch.DescribeAlarmsOutput, error) {
	return &cloudwatch.DescribeAlarmsOutput{}, nil
}

func (r *dashboardOpsRecorder) WatchDashboardScope(
	_ context.Context,
	channelID, scope, service string,
	interval time.Duration,
) (string, error) {
	r.watchCall = &dashboardWatchCall{channelID: channelID, scope: scope, service: service, interval: interval}
	return r.watchResult, nil
}

func (r *dashboardOpsRecorder) UnwatchDashboardScope(
	_ context.Context,
	scope, service string,
) (string, error) {
	r.unwatchCall = &dashboardUnwatchCall{scope: scope, service: service}
	return r.unwatchResult, nil
}

func (r *dashboardOpsRecorder) ListDashboardWatches(context.Context) string {
	r.listCalls++
	return r.listResult
}

func TestOpsDashboardWatchUsesInteractionChannelAndDefaults(t *testing.T) {
	ops := &dashboardOpsRecorder{watchResult: "watching"}
	h := &Handler{ops: ops}

	got := h.opsDashboardCommand(
		context.Background(),
		Interaction{ChannelID: "interaction-channel"},
		dashboardSubcommand("watch"),
	)

	if got != "watching" {
		t.Fatalf("unexpected result: %s", got)
	}
	want := dashboardWatchCall{
		channelID: "interaction-channel",
		scope:     "all",
		interval:  5 * time.Minute,
	}
	if ops.watchCall == nil || *ops.watchCall != want {
		t.Fatalf("unexpected watch call: %#v", ops.watchCall)
	}
}

func TestOpsDashboardDefaultViewUsesThirtyMinuteWindow(t *testing.T) {
	h := newDashboardViewTestHandler()

	got := h.opsDashboardCommand(
		context.Background(),
		Interaction{},
		dashboardSubcommand(""),
	)

	if !strings.Contains(got, "조회 범위: 최근 30m") {
		t.Fatalf("default dashboard window was not preserved: %s", got)
	}
}

func TestOpsDashboardServiceViewUsesThirtyMinuteWindow(t *testing.T) {
	h := newDashboardViewTestHandler()

	got := h.opsDashboardCommand(
		context.Background(),
		Interaction{},
		dashboardSubcommand("view", stringInteractionOption("service", "auth")),
	)

	if !strings.Contains(got, "auth detail - last 30m") {
		t.Fatalf("service dashboard defaults were not preserved: %s", got)
	}
}

func TestOpsDashboardWatchUsesServiceScopeAndExplicitOptions(t *testing.T) {
	ops := &dashboardOpsRecorder{watchResult: "service watch"}
	h := &Handler{ops: ops}

	got := h.opsDashboardCommand(
		context.Background(),
		Interaction{ChannelID: "interaction-channel"},
		dashboardSubcommand(
			"watch",
			stringInteractionOption("service", "auth"),
			stringInteractionOption("channel", "configured-channel"),
			stringInteractionOption("interval", "10m"),
		),
	)

	if got != "service watch" {
		t.Fatalf("unexpected result: %s", got)
	}
	want := dashboardWatchCall{
		channelID: "configured-channel",
		scope:     "service",
		service:   "auth",
		interval:  10 * time.Minute,
	}
	if ops.watchCall == nil || *ops.watchCall != want {
		t.Fatalf("unexpected watch call: %#v", ops.watchCall)
	}
}

func TestOpsDashboardUnwatchAndStatusDelegate(t *testing.T) {
	ops := &dashboardOpsRecorder{
		unwatchResult: "unwatched",
		listResult:    "watch list",
	}
	h := &Handler{ops: ops}

	unwatch := h.opsDashboardCommand(
		context.Background(),
		Interaction{},
		dashboardSubcommand("unwatch", stringInteractionOption("service", "report")),
	)
	if unwatch != "unwatched" {
		t.Fatalf("unexpected unwatch result: %s", unwatch)
	}
	if ops.unwatchCall == nil || *ops.unwatchCall != (dashboardUnwatchCall{scope: "service", service: "report"}) {
		t.Fatalf("unexpected unwatch call: %#v", ops.unwatchCall)
	}

	status := h.opsDashboardCommand(context.Background(), Interaction{}, dashboardSubcommand("status"))
	if status != "watch list" || ops.listCalls != 1 {
		t.Fatalf("unexpected status result=%q calls=%d", status, ops.listCalls)
	}
}

func TestOpsCommandDispatchesDashboardStatus(t *testing.T) {
	ops := &dashboardOpsRecorder{listResult: "dashboard status"}
	h := &Handler{ops: ops}
	interaction := Interaction{
		Data: ApplicationCommandData{
			Name:    "ops",
			Options: []ApplicationCommandOpt{dashboardSubcommand("status")},
		},
	}

	got := h.opsCommand(context.Background(), interaction)

	if got != "dashboard status" || ops.listCalls != 1 {
		t.Fatalf("unexpected dispatch result=%q calls=%d", got, ops.listCalls)
	}
}

func TestOpsDashboardRejectsInvalidInputBeforeControllerCall(t *testing.T) {
	ops := &dashboardOpsRecorder{}
	h := &Handler{ops: ops}

	invalidAction := h.opsDashboardCommand(
		context.Background(),
		Interaction{},
		dashboardSubcommand("delete"),
	)
	if invalidAction != "지원하지 않는 dashboard action입니다. view, watch, unwatch, status 중 하나를 사용하세요." {
		t.Fatalf("unexpected invalid action result: %s", invalidAction)
	}

	invalidInterval := h.opsDashboardCommand(
		context.Background(),
		Interaction{},
		dashboardSubcommand("watch", stringInteractionOption("interval", "2m")),
	)
	if invalidInterval != "지원하지 않는 interval입니다. 1m, 3m, 5m, 10m, 15m 중 하나를 사용하세요." {
		t.Fatalf("unexpected invalid interval result: %s", invalidInterval)
	}
	if ops.watchCall != nil {
		t.Fatalf("controller must not be called for invalid input: %#v", ops.watchCall)
	}
}

func TestOpsDashboardControllerActionsRequireController(t *testing.T) {
	for _, action := range []string{"watch", "unwatch", "status"} {
		t.Run(action, func(t *testing.T) {
			h := &Handler{}

			got := h.opsDashboardCommand(
				context.Background(),
				Interaction{},
				dashboardSubcommand(action),
			)

			if got != "Service Ops controller is not ready." {
				t.Fatalf("unexpected missing controller result: %s", got)
			}
		})
	}
}

func dashboardSubcommand(action string, options ...ApplicationCommandOpt) ApplicationCommandOpt {
	return ApplicationCommandOpt{
		Name: "dashboard",
		Options: append(
			[]ApplicationCommandOpt{stringInteractionOption("action", action)},
			options...,
		),
	}
}

func newDashboardViewTestHandler() *Handler {
	h := newComponentTestHandler(resultRows([]map[string]string{{
		"@timestamp": "2026-07-12T12:00:00+09:00",
		"count":      "1",
		"logType":    "API",
	}}))
	h.alarms = cw.NewAlarmClient(dashboardAlarmAPI{})
	h.health = health.NewClient(nil, time.Second)
	return h
}
