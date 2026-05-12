package monitor

import (
	"context"
	"net/http"
	"path/filepath"
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
	sends int
	edits int
}

func (f *fakeDiscord) SendChannelMessage(context.Context, *http.Client, string, string, string) (discord.Message, error) {
	f.sends++
	return discord.Message{ID: "created-message"}, nil
}

func (f *fakeDiscord) EditChannelMessage(context.Context, *http.Client, string, string, string, string) error {
	f.edits++
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
}

func TestDashboardRespectsMaxCloudWatchQueriesPerTick(t *testing.T) {
	logs := &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}}}
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, logs)
	service.cfg.Dashboard.MaxCloudWatchQueries = 2

	_ = service.RenderDashboard(context.Background(), "30m", 5*time.Minute)

	if logs.calls != 2 {
		t.Fatalf("expected 2 CloudWatch queries, got %d", logs.calls)
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
