package monitor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func TestAlertFingerprintDedupeAndCooldown(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"count": "2", "logType": "API_ERROR", "level": "ERROR", "http.statusCode": "500", "http.path": "/v2/admin/courses/java/assignments/copy", "response.error.code": "49000"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[2:3]
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	if fakeDiscord.sends != 3 {
		t.Fatalf("expected one send per active fingerprint within cooldown, got %d", fakeDiscord.sends)
	}
}

func TestServiceAlertsSkipUnconnectedServices(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"count": "2", "logType": "API_ERROR", "level": "ERROR", "http.statusCode": "500", "http.path": "/v2/report", "response.error.code": "500"},
	}})

	alerts := service.collectAlerts(context.Background())
	for _, alert := range alerts {
		if alert.Service != "report" {
			t.Fatalf("unconnected service should not create alert: %#v", alert)
		}
	}
}

func TestServiceAlertMentionsOperatorRoleAndUsesOpsCommands(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"count": "3", "logType": "API_ERROR", "level": "ERROR", "http.statusCode": "500", "http.path": "/v2/report", "response.error.code": "500", "trace.traceId": "trace-1"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[2:3]
	service.cfg.DiscordAllowedRoleIDs = []string{"1234567890"}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends == 0 {
		t.Fatal("expected service alert message")
	}
	content := fakeDiscord.sentContents[0]
	for _, want := range []string{"<@&1234567890>", "서비스 장애 감지", "서비스: report/web", "/ops logs service:report mode:errors", "/ops trace trace_id:trace-1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("alert missing %q: %s", want, content)
		}
	}
}

func TestResolvedAlertStateTransition(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newTestService(store, &fakeLogs{rows: []map[string]string{
		{"count": "2", "logType": "API_ERROR", "level": "ERROR", "http.statusCode": "500", "http.path": "/v2/report", "response.error.code": "500"},
	}})
	service.cfg.ServiceRegistry = service.cfg.ServiceRegistry[2:3]
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.logs = &fakeLogs{}
	if err := service.PollAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}

	snapshot := store.Snapshot()
	resolved := false
	for _, alert := range snapshot.Alerts {
		if !alert.Active && !alert.ResolvedAt.IsZero() && time.Since(alert.ResolvedAt) < time.Minute {
			resolved = true
		}
	}
	if !resolved {
		t.Fatalf("expected resolved alert state, got %#v", snapshot.Alerts)
	}
}
