package discord

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
)

func TestOpsDashboardCommandUsesFakeCloudWatchAndHealth(t *testing.T) {
	h := newObservabilityHandler(t, resultRows([]map[string]string{{
		"count":           "1",
		"logType":         "API",
		"level":           "INFO",
		"http.statusCode": "200",
	}}))

	got := h.opsCommand(context.Background(), opsInteraction("dashboard",
		stringInteractionOption("since", "30m"),
	))

	if !strings.Contains(got, "A&I 서비스 운영 대시보드") || !strings.Contains(got, "gateway") {
		t.Fatalf("dashboard handler did not render mock dashboard: %s", got)
	}
}

func TestOpsLogsCommandUsesFakeCloudWatchRows(t *testing.T) {
	h := newObservabilityHandler(t, resultRows([]map[string]string{{
		"count":                  "2",
		"service.domain":         "gateway",
		"service.name":           "gateway",
		"http.route":             "/v2/ping",
		"http.statusCode":        "500",
		"response.error.code":    "18801",
		"response.error.message": "internal",
		"trace.traceId":          "trace-observability",
		"logType":                "API_ERROR",
	}}))

	got := h.opsCommand(context.Background(), opsInteraction("logs",
		stringInteractionOption("service", "gateway"),
		stringInteractionOption("mode", "errors"),
		stringInteractionOption("since", "30m"),
		stringInteractionOption("limit", "10"),
	))

	if !strings.Contains(got, "trace-observability") || !strings.Contains(got, "/ops logs mode:trace query:trace-observability") {
		t.Fatalf("logs handler did not render fake CloudWatch rows: %s", got)
	}
}

func TestOpsAlertCommandUsesFakeOpsController(t *testing.T) {
	h := newObservabilityHandler(t, nil)
	ops := &fakeOpsController{}
	h.SetOpsController(ops)

	got := h.opsCommand(context.Background(), opsInteraction("alert",
		stringInteractionOption("action", "channel"),
		stringInteractionOption("target", "critical"),
		stringInteractionOption("channel", "channel-1"),
	))

	if got != "alert configured by fake ops" {
		t.Fatalf("alert handler returned %q", got)
	}
	if ops.alertAction != "channel" || ops.alertTarget != "critical" || ops.alertChannel != "channel-1" {
		t.Fatalf("fake ops did not record alert request: %#v", ops)
	}
}

func TestOpsHelpCommandDocumentsDashboardLogsAlertAndHelp(t *testing.T) {
	h := newObservabilityHandler(t, nil)

	got := h.opsCommand(context.Background(), opsInteraction("help",
		stringInteractionOption("topic", "overview"),
	))

	for _, want := range []string{"/ops dashboard", "/ops logs", "/ops alert", "/ops help"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help handler missing %q: %s", want, got)
		}
	}
}

type fakeAlarmAPI struct{}

func (fakeAlarmAPI) DescribeAlarms(context.Context, *cloudwatch.DescribeAlarmsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error) {
	return &cloudwatch.DescribeAlarmsOutput{}, nil
}

type fakeOpsController struct {
	alertAction  string
	alertTarget  string
	alertChannel string
}

func (f *fakeOpsController) WatchDashboardScope(context.Context, string, string, string, time.Duration) (string, error) {
	return "dashboard watch configured by fake ops", nil
}

func (f *fakeOpsController) UnwatchDashboardScope(context.Context, string, string) (string, error) {
	return "dashboard watch removed by fake ops", nil
}

func (f *fakeOpsController) ListDashboardWatches(context.Context) string {
	return "dashboard watches from fake ops"
}

func (f *fakeOpsController) ConfigureAlert(_ context.Context, channelID, action, _ string, target string) (string, error) {
	f.alertAction = action
	f.alertTarget = target
	f.alertChannel = channelID
	return "alert configured by fake ops", nil
}

func (f *fakeOpsController) WatchLogFeed(context.Context, string, string, string, string, time.Duration, int) (string, error) {
	return "log feed configured by fake ops", nil
}

func (f *fakeOpsController) UnwatchLogFeed(context.Context, string, string) (string, error) {
	return "log feed removed by fake ops", nil
}

func (f *fakeOpsController) ListLogFeeds(context.Context) string {
	return "log feeds from fake ops"
}

func (f *fakeOpsController) DescribeAssignmentDiagnosis(string, reportadmin.Assignment) string {
	return "assignment diagnosis from fake ops"
}

func (f *fakeOpsController) AssignmentIssueStatus(string, string) string {
	return "assignment issue status from fake ops"
}

func (f *fakeOpsController) AssignmentIssueHistory(string, string) string {
	return "assignment issue history from fake ops"
}

func (f *fakeOpsController) AcknowledgeAssignmentIssue(string, string, string, string, string, string) (string, error) {
	return "assignment issue acknowledged by fake ops", nil
}

func (f *fakeOpsController) UnacknowledgeAssignmentIssue(string, string, string) (string, error) {
	return "assignment issue unacknowledged by fake ops", nil
}

func newObservabilityHandler(t *testing.T, rows [][]types.ResultField) *Handler {
	t.Helper()
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	}))
	t.Cleanup(healthServer.Close)

	cfg := config.Config{
		DiscordApplicationID:        "app-id",
		DiscordEphemeralResponses:   true,
		CloudWatchQueryTimeout:      time.Second,
		CloudWatchQueryPollInterval: time.Nanosecond,
		CloudWatchQueryLimit:        20,
		CloudWatchMaxLogGroups:      5,
		LogGroups: map[string]string{
			"gateway": "/gateway",
			"auth":    "/auth",
			"report":  "/report",
			"post":    "/post",
		},
		HealthURLs: map[string]string{
			"gateway": healthServer.URL,
			"auth":    healthServer.URL,
			"report":  healthServer.URL,
			"post":    healthServer.URL,
		},
	}
	cfg.ServiceRegistry = config.BuildServiceRegistry(cfg.LogGroups, cfg.HealthURLs)

	return &Handler{
		cfg:          cfg,
		health:       health.NewClient(cfg.HealthURLs, time.Second),
		logs:         cw.NewLogsClient(&fakeLogsAPI{rows: rows}, cfg.CloudWatchQueryTimeout, cfg.CloudWatchQueryPollInterval, cfg.CloudWatchQueryLimit),
		alarms:       cw.NewAlarmClient(fakeAlarmAPI{}),
		httpClient:   http.DefaultClient,
		replayWindow: time.Hour,
	}
}

func opsInteraction(subcommand string, options ...ApplicationCommandOpt) Interaction {
	return Interaction{
		ChannelID: "channel-default",
		Data: ApplicationCommandData{
			Name: "ops",
			Options: []ApplicationCommandOpt{{
				Name:    subcommand,
				Options: options,
			}},
		},
	}
}
