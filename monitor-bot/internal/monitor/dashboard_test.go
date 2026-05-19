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
	edits        int
	sentContents []string
	editContents []string
	sentChannels []string
	editChannels []string
}

func (f *fakeDiscord) SendChannelMessage(_ context.Context, _ *http.Client, _ string, channelID string, content string) (discord.Message, error) {
	f.sends++
	f.sentChannels = append(f.sentChannels, channelID)
	f.sentContents = append(f.sentContents, content)
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
	service.cfg.Dashboard.MaxCloudWatchQueries = 3

	_ = service.RenderDashboard(context.Background(), "30m", 5*time.Minute)

	if logs.calls != 3 {
		t.Fatalf("expected gateway/auth/report CloudWatch queries, got %d", logs.calls)
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
