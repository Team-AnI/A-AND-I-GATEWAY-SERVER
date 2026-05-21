package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsDashboardAndAlertState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *Data) {
		data.DashboardChannelID = "channel"
		data.DashboardMessageID = "message"
		data.Alerts["prod:report:5xx:/v2:49000"] = AlertState{Active: true, LastSentAt: time.Unix(1, 0)}
	}); err != nil {
		t.Fatal(err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.Snapshot()
	if got.DashboardMessageID != "message" || !got.Alerts["prod:report:5xx:/v2:49000"].Active {
		t.Fatalf("state was not persisted: %#v", got)
	}
	if got.ServiceDashboards["all"].MessageID != "message" {
		t.Fatalf("legacy dashboard state was not migrated: %#v", got.ServiceDashboards)
	}
}

func TestStorePersistsServiceOpsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *Data) {
		data.ServiceDashboards["service:report"] = ServiceDashboard{Scope: "service", Service: "report", ChannelID: "channel", MessageID: "message", IntervalSec: 300}
		data.ServiceAlerts.Enabled = true
		data.ServiceAlerts.ChannelID = "alert-channel"
		data.ServiceAlerts.GeneralChannelID = "general-channel"
		data.ServiceAlerts.CriticalChannelID = "critical-channel"
		data.ServiceAlerts.RoleID = "123456"
		data.ServiceAlerts.LastSent["report:5xx"] = time.Now()
		data.LogFeeds["report:errors"] = LogFeed{Service: "report", Mode: "errors", ChannelID: "log-channel", Fingerprints: map[string]time.Time{"fp": time.Now()}}
	}); err != nil {
		t.Fatal(err)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.Snapshot()
	if got.ServiceDashboards["service:report"].MessageID != "message" {
		t.Fatalf("dashboard watch was not persisted: %#v", got.ServiceDashboards)
	}
	if !got.ServiceAlerts.Enabled || got.ServiceAlerts.RoleID != "123456" || got.ServiceAlerts.GeneralChannelID != "general-channel" || got.ServiceAlerts.CriticalChannelID != "critical-channel" {
		t.Fatalf("service alert config was not persisted: %#v", got.ServiceAlerts)
	}
	if got.LogFeeds["report:errors"].Fingerprints["fp"].IsZero() {
		t.Fatalf("log feed fingerprints were not persisted: %#v", got.LogFeeds)
	}
}

func TestStoreLoadsLegacyRecentServiceAlerts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	body := []byte(`{
  "version": 2,
  "recentServiceAlerts": [
    {
      "fingerprint": "legacy-trace-fingerprint",
      "severity": "CRITICAL",
      "service": "gateway",
      "alertType": "v2-log",
      "summary": "gateway CRITICAL - critical",
      "createdAt": "2026-05-20T05:09:00Z"
    }
  ]
}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot().RecentServiceAlerts
	if len(got) != 1 {
		t.Fatalf("legacy recent service alert not loaded: %#v", got)
	}
	if got[0].IncidentKey != "" || len(got[0].TraceIDs) != 0 {
		t.Fatalf("missing new fields should stay zero-value for compatibility: %#v", got[0])
	}
}

func TestStoreCorruptedStateFallsBackGracefully(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("corrupted state should not fail load: %v", err)
	}
	if got := store.Snapshot(); got.Version != 2 {
		t.Fatalf("fallback state should be normalized: %#v", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "state.json.corrupt.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one corrupt backup, got %#v", matches)
	}
}
