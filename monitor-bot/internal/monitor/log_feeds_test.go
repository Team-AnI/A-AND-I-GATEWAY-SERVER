package monitor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func TestWatchLogFeedStoresBaselineWithoutSendingOldLogs(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	logs := &fakeLogs{rows: []map[string]string{
		{"@timestamp": "2026-05-13T17:00:00+09:00", "trace.traceId": "trace-1", "http.path": "/v2/admin/courses", "http.statusCode": "500"},
	}}
	service := newTestService(store, logs)
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	result, err := service.WatchLogFeed(context.Background(), "channel-1", "auth", "errors", "30m", 5*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "baseline") {
		t.Fatalf("expected baseline guidance: %s", result)
	}
	feed := store.Snapshot().LogFeeds["auth:errors"]
	if feed.ChannelID != "channel-1" || len(feed.Fingerprints) != 1 {
		t.Fatalf("log feed baseline was not stored: %#v", feed)
	}
	if fakeDiscord.sends != 0 {
		t.Fatalf("first registration should not send historical logs, sends=%d", fakeDiscord.sends)
	}
}

func TestWatchLogFeedRejectsUnconnectedService(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{})
	result, err := service.WatchLogFeed(context.Background(), "channel-1", "online-judge", "errors", "30m", 5*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "NO_V2_LOG") {
		t.Fatalf("expected NO_V2_LOG guidance: %s", result)
	}
	if _, ok := store.Snapshot().LogFeeds["online-judge:errors"]; ok {
		t.Fatal("unconnected service log feed should not be stored")
	}
}

func TestLogFeedFingerprintPrefersTraceID(t *testing.T) {
	row := map[string]string{"trace.traceId": "trace-1", "message": "Authorization: Bearer secret"}
	got := logFeedFingerprint("report", "errors", row)
	if got != "report:errors:trace:trace-1" {
		t.Fatalf("unexpected fingerprint: %s", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatal("fingerprint should not include token-like text")
	}
}
