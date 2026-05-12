package state

import (
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
}
