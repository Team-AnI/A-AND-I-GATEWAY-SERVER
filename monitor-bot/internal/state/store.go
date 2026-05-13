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
	DashboardChannelID            string                        `json:"dashboardChannelId,omitempty"`
	DashboardMessageID            string                        `json:"dashboardMessageId,omitempty"`
	DashboardIntervalSec          int                           `json:"dashboardIntervalSeconds,omitempty"`
	LastDashboardUpdatedAt        time.Time                     `json:"lastDashboardUpdatedAt,omitempty"`
	AssignmentOpsMessageID        string                        `json:"assignmentOpsMessageId,omitempty"`
	LastAssignmentOpsUpdatedAt    time.Time                     `json:"lastAssignmentOpsUpdatedAt,omitempty"`
	AssignmentBaselineInitialized bool                          `json:"assignmentBaselineInitialized,omitempty"`
	AssignmentSnapshots           map[string]AssignmentSnapshot `json:"assignmentSnapshots,omitempty"`
	AssignmentEventFingerprints   map[string]AlertState         `json:"assignmentEventFingerprints,omitempty"`
	RecentAssignmentEvents        []AssignmentEventState        `json:"recentAssignmentEvents,omitempty"`
	Alerts                        map[string]AlertState         `json:"alertFingerprints,omitempty"`
	RecentServiceAlerts           []ServiceAlertEventState      `json:"recentServiceAlerts,omitempty"`
	HealthDownCounts              map[string]int                `json:"healthDownCounts,omitempty"`
	LastAlertSentAt               time.Time                     `json:"lastAlertSentAt,omitempty"`
}

type AssignmentSnapshot struct {
	CourseSlug   string    `json:"courseSlug,omitempty"`
	CourseClass  string    `json:"courseClass,omitempty"`
	AssignmentID string    `json:"assignmentId,omitempty"`
	Title        string    `json:"title,omitempty"`
	Status       string    `json:"status,omitempty"`
	PublishedAt  string    `json:"publishedAt,omitempty"`
	StartAt      string    `json:"startAt,omitempty"`
	EndAt        string    `json:"endAt,omitempty"`
	ProblemID    string    `json:"problemId,omitempty"`
	UpdatedAt    string    `json:"updatedAt,omitempty"`
	Submitted    int       `json:"submitted,omitempty"`
	Graded       int       `json:"graded,omitempty"`
	Pending      int       `json:"pending,omitempty"`
	Failed       int       `json:"failed,omitempty"`
	AverageScore string    `json:"averageScore,omitempty"`
	LastSeenAt   time.Time `json:"lastSeenAt,omitempty"`
}

type AssignmentEventState struct {
	Fingerprint  string    `json:"fingerprint,omitempty"`
	EventType    string    `json:"eventType,omitempty"`
	Severity     string    `json:"severity,omitempty"`
	CourseSlug   string    `json:"courseSlug,omitempty"`
	AssignmentID string    `json:"assignmentId,omitempty"`
	Title        string    `json:"title,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	CreatedAt    time.Time `json:"createdAt,omitempty"`
}

type AlertState struct {
	Active     bool      `json:"active"`
	LastSentAt time.Time `json:"lastSentAt,omitempty"`
	ResolvedAt time.Time `json:"resolvedAt,omitempty"`
}

type ServiceAlertEventState struct {
	Fingerprint string    `json:"fingerprint,omitempty"`
	Severity    string    `json:"severity,omitempty"`
	Service     string    `json:"service,omitempty"`
	AlertType   string    `json:"alertType,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
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
	if data.AssignmentSnapshots == nil {
		data.AssignmentSnapshots = make(map[string]AssignmentSnapshot)
	}
	if data.AssignmentEventFingerprints == nil {
		data.AssignmentEventFingerprints = make(map[string]AlertState)
	}
	if data.HealthDownCounts == nil {
		data.HealthDownCounts = make(map[string]int)
	}
	if len(data.RecentAssignmentEvents) > 20 {
		data.RecentAssignmentEvents = data.RecentAssignmentEvents[:20]
	}
	if len(data.RecentServiceAlerts) > 20 {
		data.RecentServiceAlerts = data.RecentServiceAlerts[:20]
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
	cloned.AssignmentSnapshots = make(map[string]AssignmentSnapshot, len(data.AssignmentSnapshots))
	for key, value := range data.AssignmentSnapshots {
		cloned.AssignmentSnapshots[key] = value
	}
	cloned.AssignmentEventFingerprints = make(map[string]AlertState, len(data.AssignmentEventFingerprints))
	for key, value := range data.AssignmentEventFingerprints {
		cloned.AssignmentEventFingerprints[key] = value
	}
	cloned.RecentAssignmentEvents = append([]AssignmentEventState(nil), data.RecentAssignmentEvents...)
	cloned.RecentServiceAlerts = append([]ServiceAlertEventState(nil), data.RecentServiceAlerts...)
	cloned.HealthDownCounts = make(map[string]int, len(data.HealthDownCounts))
	for key, value := range data.HealthDownCounts {
		cloned.HealthDownCounts[key] = value
	}
	return cloned
}
