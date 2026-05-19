package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

type Data struct {
	Version                       int                           `json:"version,omitempty"`
	DashboardChannelID            string                        `json:"dashboardChannelId,omitempty"`
	DashboardMessageID            string                        `json:"dashboardMessageId,omitempty"`
	DashboardIntervalSec          int                           `json:"dashboardIntervalSeconds,omitempty"`
	LastDashboardUpdatedAt        time.Time                     `json:"lastDashboardUpdatedAt,omitempty"`
	ServiceDashboards             map[string]ServiceDashboard   `json:"serviceDashboards,omitempty"`
	ServiceAlerts                 ServiceAlertsConfig           `json:"serviceAlerts,omitempty"`
	LogFeeds                      map[string]LogFeed            `json:"logFeeds,omitempty"`
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

type ServiceDashboard struct {
	Scope         string    `json:"scope,omitempty"`
	Service       string    `json:"service,omitempty"`
	ChannelID     string    `json:"channelId,omitempty"`
	MessageID     string    `json:"messageId,omitempty"`
	IntervalSec   int       `json:"intervalSeconds,omitempty"`
	LastUpdatedAt time.Time `json:"lastUpdatedAt,omitempty"`
	LastStatus    string    `json:"lastStatus,omitempty"`
	Disabled      bool      `json:"disabled,omitempty"`
	ConfigError   string    `json:"configError,omitempty"`
}

type ServiceAlertsConfig struct {
	Enabled     bool                 `json:"enabled,omitempty"`
	ChannelID   string               `json:"channelId,omitempty"`
	RoleID      string               `json:"roleId,omitempty"`
	CooldownSec int                  `json:"cooldownSeconds,omitempty"`
	LastSent    map[string]time.Time `json:"lastSent,omitempty"`
}

type LogFeed struct {
	Service       string               `json:"service,omitempty"`
	Mode          string               `json:"mode,omitempty"`
	ChannelID     string               `json:"channelId,omitempty"`
	IntervalSec   int                  `json:"intervalSeconds,omitempty"`
	Since         string               `json:"since,omitempty"`
	Limit         int                  `json:"limit,omitempty"`
	LastCheckedAt time.Time            `json:"lastCheckedAt,omitempty"`
	Fingerprints  map[string]time.Time `json:"fingerprints,omitempty"`
	Disabled      bool                 `json:"disabled,omitempty"`
	Status        string               `json:"status,omitempty"`
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
		backup := s.path + ".corrupt." + time.Now().UTC().Format("20060102T150405Z")
		_ = os.WriteFile(backup, data, 0o600)
		s.data = normalize(Data{})
		return nil
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
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func normalize(data Data) Data {
	if data.Version < 2 || (data.ServiceDashboards == nil && (data.DashboardChannelID != "" || data.DashboardMessageID != "")) {
		data = migrateV2(data)
	}
	data.Version = 2
	if data.ServiceDashboards == nil {
		data.ServiceDashboards = make(map[string]ServiceDashboard)
	}
	if data.ServiceAlerts.LastSent == nil {
		data.ServiceAlerts.LastSent = make(map[string]time.Time)
	}
	if data.LogFeeds == nil {
		data.LogFeeds = make(map[string]LogFeed)
	}
	for key, dashboard := range data.ServiceDashboards {
		if dashboard.Scope == "" {
			dashboard.Scope = dashboardScopeFromKey(key)
		}
		if dashboard.Scope == "service" && dashboard.Service == "" {
			dashboard.Service = dashboardServiceFromKey(key)
		}
		data.ServiceDashboards[key] = dashboard
	}
	for key, feed := range data.LogFeeds {
		if feed.Fingerprints == nil {
			feed.Fingerprints = make(map[string]time.Time)
		}
		if feed.IntervalSec <= 0 {
			feed.IntervalSec = 300
		}
		if feed.Since == "" {
			feed.Since = "30m"
		}
		if feed.Limit <= 0 {
			feed.Limit = 10
		}
		pruneTimeMap(feed.Fingerprints, 24*time.Hour, 1000)
		data.LogFeeds[key] = feed
	}
	pruneTimeMap(data.ServiceAlerts.LastSent, 24*time.Hour, 1000)
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

func migrateV2(data Data) Data {
	if data.ServiceDashboards == nil {
		data.ServiceDashboards = make(map[string]ServiceDashboard)
	}
	if data.LogFeeds == nil {
		data.LogFeeds = make(map[string]LogFeed)
	}
	if data.ServiceAlerts.LastSent == nil {
		data.ServiceAlerts.LastSent = make(map[string]time.Time)
	}
	if data.DashboardChannelID != "" || data.DashboardMessageID != "" {
		interval := data.DashboardIntervalSec
		if interval <= 0 {
			interval = 300
		}
		data.ServiceDashboards["all"] = ServiceDashboard{
			Scope:         "all",
			ChannelID:     data.DashboardChannelID,
			MessageID:     data.DashboardMessageID,
			IntervalSec:   interval,
			LastUpdatedAt: data.LastDashboardUpdatedAt,
		}
	}
	return data
}

func dashboardScopeFromKey(key string) string {
	if key == "all" {
		return "all"
	}
	return "service"
}

func dashboardServiceFromKey(key string) string {
	const prefix = "service:"
	if len(key) > len(prefix) && key[:len(prefix)] == prefix {
		return key[len(prefix):]
	}
	return ""
}

func pruneTimeMap(values map[string]time.Time, ttl time.Duration, max int) {
	if len(values) == 0 {
		return
	}
	now := time.Now()
	for key, value := range values {
		if !value.IsZero() && now.Sub(value) > ttl {
			delete(values, key)
		}
	}
	if max <= 0 || len(values) <= max {
		return
	}
	type pair struct {
		key string
		at  time.Time
	}
	pairs := make([]pair, 0, len(values))
	for key, at := range values {
		pairs = append(pairs, pair{key: key, at: at})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].at.Before(pairs[j].at)
	})
	for len(pairs) > max {
		delete(values, pairs[0].key)
		pairs = pairs[1:]
	}
}

func cloneData(data Data) Data {
	data = normalize(data)
	cloned := data
	cloned.ServiceDashboards = make(map[string]ServiceDashboard, len(data.ServiceDashboards))
	for key, value := range data.ServiceDashboards {
		cloned.ServiceDashboards[key] = value
	}
	cloned.ServiceAlerts.LastSent = make(map[string]time.Time, len(data.ServiceAlerts.LastSent))
	for key, value := range data.ServiceAlerts.LastSent {
		cloned.ServiceAlerts.LastSent[key] = value
	}
	cloned.LogFeeds = make(map[string]LogFeed, len(data.LogFeeds))
	for key, value := range data.LogFeeds {
		value.Fingerprints = cloneTimeMap(value.Fingerprints)
		cloned.LogFeeds[key] = value
	}
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

func cloneTimeMap(values map[string]time.Time) map[string]time.Time {
	cloned := make(map[string]time.Time, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func DashboardKey(scope, service string) string {
	if scope == "all" {
		return "all"
	}
	if service == "" {
		return "service"
	}
	return fmt.Sprintf("service:%s", service)
}

func LogFeedKey(service, mode string) string {
	return fmt.Sprintf("%s:%s", service, mode)
}
