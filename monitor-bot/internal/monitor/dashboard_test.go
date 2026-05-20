package monitor

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/discord"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

type fakeLogs struct {
	calls int
	rows  []map[string]string
}

func (f *fakeLogs) Query(context.Context, []string, string, time.Duration, int32) ([]map[string]string, error) {
	f.calls++
	return f.rows, nil
}

type fakeAlarms struct {
	names []string
}

func (f fakeAlarms) AlarmNames(context.Context) ([]string, error) {
	return f.names, nil
}

type fakeDiscord struct {
	sends        int
	roleSends    int
	edits        int
	sentContents []string
	roleContents []string
	editContents []string
	sentChannels []string
	roleChannels []string
	roleIDs      []string
	editChannels []string
}

func (f *fakeDiscord) SendChannelMessage(_ context.Context, _ *http.Client, _ string, channelID string, content string) (discord.Message, error) {
	f.sends++
	f.sentChannels = append(f.sentChannels, channelID)
	f.sentContents = append(f.sentContents, content)
	return discord.Message{ID: "created-message"}, nil
}

func (f *fakeDiscord) SendChannelMessageWithRoleMention(_ context.Context, _ *http.Client, _ string, channelID string, content string, roleID string) (discord.Message, error) {
	f.roleSends++
	f.roleChannels = append(f.roleChannels, channelID)
	f.roleContents = append(f.roleContents, "<@&"+roleID+">\n"+content)
	f.roleIDs = append(f.roleIDs, roleID)
	return discord.Message{ID: "created-message"}, nil
}

func (f *fakeDiscord) EditChannelMessage(_ context.Context, _ *http.Client, _ string, channelID string, _ string, content string) error {
	f.edits++
	f.editChannels = append(f.editChannels, channelID)
	f.editContents = append(f.editContents, content)
	return nil
}

func TestRefreshDashboardEditsExistingMessage(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.DashboardChannelID = "channel-1"
		data.DashboardMessageID = "message-1"
	}); err != nil {
		t.Fatal(err)
	}
	fakeDiscord := &fakeDiscord{}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}}})
	service.discord = fakeDiscord

	if err := service.RefreshDashboard(context.Background()); err != nil {
		t.Fatal(err)
	}

	if fakeDiscord.edits != 1 || fakeDiscord.sends != 0 {
		t.Fatalf("expected edit only, edits=%d sends=%d", fakeDiscord.edits, fakeDiscord.sends)
	}
	if !strings.Contains(fakeDiscord.editContents[0], "A&I 서비스 운영 대시보드") || !strings.Contains(fakeDiscord.editContents[0], "```txt") {
		t.Fatalf("dashboard should use Korean compact table: %s", fakeDiscord.editContents[0])
	}
}

func TestDashboardQueriesOnlyV2ConnectedServices(t *testing.T) {
	logs := &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}}}
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, logs)
	service.cfg.Dashboard.MaxCloudWatchQueries = 4

	_ = service.RenderDashboard(context.Background(), "30m", 5*time.Minute)

	if logs.calls != 4 {
		t.Fatalf("expected gateway/auth/report/blog CloudWatch queries, got %d", logs.calls)
	}
}

func TestRecentServiceAlertsGroupIncidentsWithTraceDrilldown(t *testing.T) {
	base := time.Date(2026, 5, 20, 14, 9, 0, 0, time.FixedZone("KST", 9*60*60))
	events := []state.ServiceAlertEventState{
		{IncidentKey: "gateway\x00critical\x00v2-log\x00critical\x00/v2/reports\x0018801", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", TraceIDs: []string{"trace-a"}, CreatedAt: base},
		{IncidentKey: "gateway\x00critical\x00v2-log\x00critical\x00/v2/reports\x0018801", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", TraceIDs: []string{"trace-b"}, CreatedAt: base.Add(-3 * time.Minute)},
		{IncidentKey: "gateway\x00critical\x00v2-log\x00critical\x00/v2/reports\x0018801", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", TraceIDs: []string{"trace-c"}, CreatedAt: base.Add(-4 * time.Minute)},
		{IncidentKey: "gateway\x00critical\x00v2-log\x00critical\x00/v2/reports\x0018801", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", TraceIDs: []string{"trace-d"}, CreatedAt: base.Add(-5 * time.Minute)},
	}
	got := formatting.FormatDashboardWithMetaAndAlerts("30m", []formatting.DashboardServiceInput{{
		Service:   "gateway",
		Health:    formatting.ServiceStatus{Service: "gateway", State: "UP"},
		LogStatus: "OK",
		Rows:      []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}},
	}}, nil, base, 5*time.Minute, recentServiceAlertLines(events, 5, "30m"))
	for _, want := range []string{"gateway CRITICAL - critical ×4", "latest=14:09 first=14:04", "traces: trace-a, trace-b, trace-c (+1)", "/ops logs service:gateway mode:critical since:30m limit:10", "/ops logs mode:trace query:trace-a"} {
		if !strings.Contains(got, want) {
			t.Fatalf("grouped service dashboard missing %q: %s", want, got)
		}
	}
	if strings.Count(got, "gateway CRITICAL - critical") >= 5 {
		t.Fatalf("dashboard should not render duplicate raw alert rows: %s", got)
	}
	if strings.Contains(got, "/ops trace trace_id:") {
		t.Fatalf("dashboard must use latest trace command: %s", got)
	}
}

func TestRecentServiceAlertsKeepDistinctIncidentKeysAndHandleMissingTrace(t *testing.T) {
	base := time.Date(2026, 5, 20, 14, 9, 0, 0, time.FixedZone("KST", 9*60*60))
	events := []state.ServiceAlertEventState{
		{IncidentKey: "gateway\x00critical\x00v2-log\x00critical\x00/v2/reports\x0018801", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", CreatedAt: base},
		{IncidentKey: "gateway\x00critical\x00v2-log\x00critical\x00/v2/auth\x0021801", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", TraceIDs: []string{"trace-auth"}, CreatedAt: base.Add(-time.Minute)},
	}
	lines := recentServiceAlertLines(events, 5, "30m")
	if len(lines) != 2 {
		t.Fatalf("different incident keys should stay separate: %#v", lines)
	}
	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "trace: none") || !strings.Contains(got, "/ops logs mode:trace query:trace-auth") {
		t.Fatalf("missing trace and trace drilldown should both be represented: %s", got)
	}
}

func TestRecentServiceAlertsFallbackGroupsLegacyState(t *testing.T) {
	base := time.Date(2026, 5, 20, 14, 9, 0, 0, time.FixedZone("KST", 9*60*60))
	events := []state.ServiceAlertEventState{
		{Fingerprint: "fp-trace-1", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", CreatedAt: base},
		{Fingerprint: "fp-trace-2", Service: "gateway", Severity: "CRITICAL", AlertType: "v2-log", Summary: "gateway CRITICAL - critical", CreatedAt: base.Add(-time.Minute)},
	}
	lines := recentServiceAlertLines(events, 5, "30m")
	if len(lines) != 1 || !strings.Contains(lines[0], "×2") {
		t.Fatalf("legacy state without incident key should group by summary: %#v", lines)
	}
}

func TestServiceOpsConnectionRegistry(t *testing.T) {
	for _, service := range []string{"auth", "post"} {
		if !isServiceOpsNameConnected(service) {
			t.Fatalf("%s should be connected", service)
		}
	}
	if isServiceOpsNameConnected("online-judge") {
		t.Fatal("online-judge should remain catalog-only")
	}
}

func TestWatchDashboardScopeStoresServiceWatch(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	fakeDiscord := &fakeDiscord{}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API"}}})
	service.discord = fakeDiscord

	result, err := service.WatchDashboardScope(context.Background(), "channel-1", "service", "auth", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "서비스 대시보드 등록 완료") {
		t.Fatalf("unexpected result: %s", result)
	}
	snapshot := store.Snapshot()
	watch := snapshot.ServiceDashboards["service:auth"]
	if watch.ChannelID != "channel-1" || watch.MessageID == "" || watch.IntervalSec != 300 {
		t.Fatalf("service watch was not stored: %#v", watch)
	}
	if fakeDiscord.sends != 1 {
		t.Fatalf("expected dashboard message creation, got sends=%d", fakeDiscord.sends)
	}
}

func TestWatchDashboardScopeAcceptsBlogAlias(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API"}}})
	service.discord = &fakeDiscord{}

	result, err := service.WatchDashboardScope(context.Background(), "channel-1", "service", "blog", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "service:blog") {
		t.Fatalf("expected blog display in result: %s", result)
	}
	if _, ok := store.Snapshot().ServiceDashboards["service:post"]; !ok {
		t.Fatal("blog dashboard watch should store canonical post key")
	}
}

func TestRenderServiceDashboardForAuthAndBlog(t *testing.T) {
	for _, serviceName := range []string{"auth", "blog"} {
		store := state.NewStore(filepath.Join(t.TempDir(), serviceName+".json"))
		if err := store.Load(); err != nil {
			t.Fatal(err)
		}
		service := newTestService(store, &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API"}}})
		content := service.RenderServiceDashboard(context.Background(), serviceName, "30m", 5*time.Minute)
		if strings.Contains(content, "not connected") || strings.Contains(content, "NOT_CONNECTED") {
			t.Fatalf("%s dashboard should be connected: %s", serviceName, content)
		}
	}
}

func TestRenderServiceDashboardMissingConfigShowsNotConfigured(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	service.cfg.LogGroups["post"] = ""
	service.cfg.HealthURLs["post"] = ""
	service.cfg.ServiceRegistry = config.BuildServiceRegistry(service.cfg.LogGroups, service.cfg.HealthURLs)

	content := service.RenderServiceDashboard(context.Background(), "blog", "30m", 5*time.Minute)
	if !strings.Contains(content, "NOCFG") {
		t.Fatalf("missing blog config should render NOT_CONFIGURED: %s", content)
	}
}

func TestWatchDashboardRejectsUnconnectedService(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	result, err := service.WatchDashboardScope(context.Background(), "channel-1", "service", "online-judge", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "NOT_CONNECTED") {
		t.Fatalf("expected not-connected guidance: %s", result)
	}
	if _, ok := store.Snapshot().ServiceDashboards["service:online-judge"]; ok {
		t.Fatal("unconnected service watch should not be stored")
	}
}

func newTestService(store *state.Store, logs *fakeLogs) *Service {
	logGroups := map[string]string{
		"gateway":      "/gateway",
		"auth":         "/auth",
		"report":       "/report",
		"online-judge": "/oj",
		"post":         "/post",
	}
	healthURLs := map[string]string{}
	cfg := config.Config{
		DiscordBotToken: "token",
		LogGroups:       logGroups,
		HealthURLs:      healthURLs,
		ServiceRegistry: config.BuildServiceRegistry(logGroups, healthURLs),
		Dashboard: config.DashboardConfig{
			ChannelID:            "channel-1",
			RefreshInterval:      5 * time.Minute,
			Since:                "30m",
			MaxCloudWatchQueries: 6,
		},
		Alert: config.AlertConfig{
			ChannelID:                "alert-channel",
			Cooldown:                 15 * time.Minute,
			FiveXXThreshold5m:        1,
			ErrorThreshold5m:         1,
			HealthDownConsecutive:    2,
			CopyAPIFiveXXThreshold5m: 1,
		},
	}
	return NewService(cfg, health.NewClient(healthURLs, 1*time.Millisecond), logs, fakeAlarms{}, store, http.DefaultClient)
}
