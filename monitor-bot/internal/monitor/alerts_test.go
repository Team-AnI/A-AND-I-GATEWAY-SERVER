package monitor

import (
	"context"
	"path/filepath"
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
