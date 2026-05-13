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
}

func (f *fakeDiscord) SendChannelMessage(_ context.Context, _ *http.Client, _ string, _ string, content string) (discord.Message, error) {
	f.sends++
	f.sentContents = append(f.sentContents, content)
	return discord.Message{ID: "created-message"}, nil
}

func (f *fakeDiscord) EditChannelMessage(_ context.Context, _ *http.Client, _ string, _ string, _ string, content string) error {
	f.edits++
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

func TestDashboardQueriesOnlyConnectedServiceInPhaseOne(t *testing.T) {
	logs := &fakeLogs{rows: []map[string]string{{"count": "1", "logType": "API", "level": "INFO", "http.statusCode": "200"}}}
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, logs)
	service.cfg.Dashboard.MaxCloudWatchQueries = 2

	_ = service.RenderDashboard(context.Background(), "30m", 5*time.Minute)

	if logs.calls != 1 {
		t.Fatalf("expected only report CloudWatch query, got %d", logs.calls)
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
