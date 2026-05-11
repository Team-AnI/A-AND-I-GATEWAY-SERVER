package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

type Data struct {
	DashboardChannelID     string                `json:"dashboardChannelId,omitempty"`
	DashboardMessageID     string                `json:"dashboardMessageId,omitempty"`
	DashboardIntervalSec   int                   `json:"dashboardIntervalSeconds,omitempty"`
	LastDashboardUpdatedAt time.Time             `json:"lastDashboardUpdatedAt,omitempty"`
	Alerts                 map[string]AlertState `json:"alertFingerprints,omitempty"`
	HealthDownCounts       map[string]int        `json:"healthDownCounts,omitempty"`
	LastAlertSentAt        time.Time             `json:"lastAlertSentAt,omitempty"`
}

type AlertState struct {
	Active     bool      `json:"active"`
	LastSentAt time.Time `json:"lastSentAt,omitempty"`
	ResolvedAt time.Time `json:"resolvedAt,omitempty"`
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = normalize(Data{})
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.data = normalize(Data{})
		return nil
	}
	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}
	s.data = normalize(s.data)
	return nil
}

func (s *Store) Snapshot() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneData(s.data)
}

func (s *Store) Update(fn func(*Data)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = normalize(s.data)
	fn(&s.data)
	s.data = normalize(s.data)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func normalize(data Data) Data {
	if data.Alerts == nil {
		data.Alerts = make(map[string]AlertState)
	}
	if data.HealthDownCounts == nil {
		data.HealthDownCounts = make(map[string]int)
	}
	return data
}

func cloneData(data Data) Data {
	data = normalize(data)
	cloned := data
	cloned.Alerts = make(map[string]AlertState, len(data.Alerts))
	for key, value := range data.Alerts {
		cloned.Alerts[key] = value
	}
	cloned.HealthDownCounts = make(map[string]int, len(data.HealthDownCounts))
	for key, value := range data.HealthDownCounts {
		cloned.HealthDownCounts[key] = value
	}
	return cloned
}
