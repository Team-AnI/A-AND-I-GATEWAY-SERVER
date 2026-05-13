package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/discord"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

type LogsQueryer interface {
	Query(ctx context.Context, logGroups []string, query string, since time.Duration, limit int32) ([]map[string]string, error)
}

type AlarmLister interface {
	AlarmNames(ctx context.Context) ([]string, error)
}

type Service struct {
	cfg     config.Config
	health  *health.Client
	logs    LogsQueryer
	alarms  AlarmLister
	report  ReportAdminAPI
	store   *state.Store
	client  *http.Client
	discord DiscordMessenger
}

type ReportAdminAPI interface {
	ListCourses(ctx context.Context) ([]reportadmin.Course, error)
	ListAssignments(ctx context.Context, courseSlug string) ([]reportadmin.Assignment, error)
	SubmissionStatuses(ctx context.Context, courseSlug, assignmentID string) (reportadmin.SubmissionSummary, error)
}

type DiscordMessenger interface {
	SendChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, content string) (discord.Message, error)
	EditChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, messageID, content string) error
}

type discordAPI struct{}

func (discordAPI) SendChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, content string) (discord.Message, error) {
	return discord.SendChannelMessage(ctx, client, botToken, channelID, content)
}

func (discordAPI) EditChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, messageID, content string) error {
	return discord.EditChannelMessage(ctx, client, botToken, channelID, messageID, content)
}

func NewService(cfg config.Config, healthClient *health.Client, logs LogsQueryer, alarms AlarmLister, store *state.Store, client *http.Client) *Service {
	return &Service{
		cfg:     cfg,
		health:  healthClient,
		logs:    logs,
		alarms:  alarms,
		report:  reportadmin.NewClientWithRefresh(cfg.ReportServiceURI, cfg.AuthServiceURI, cfg.OpsAdminRefreshToken, cfg.HealthRequestTimeout),
		store:   store,
		client:  client,
		discord: discordAPI{},
	}
}

func (s *Service) Start(ctx context.Context) {
	go s.dashboardLoop(ctx)
	if s.cfg.Alert.Enabled {
		go s.alertLoop(ctx)
	}
	go s.assignmentOpsLoop(ctx)
}

func (s *Service) WatchDashboard(ctx context.Context, channelID string, interval time.Duration) (string, error) {
	if strings.TrimSpace(channelID) == "" {
		return "", fmt.Errorf("channel id is required")
	}
	if interval <= 0 {
		interval = s.cfg.Dashboard.RefreshInterval
	}
	if err := s.store.Update(func(data *state.Data) {
		data.DashboardChannelID = strings.TrimSpace(channelID)
		data.DashboardIntervalSec = int(interval.Seconds())
	}); err != nil {
		return "", err
	}
	if err := s.RefreshDashboard(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("dashboard watch enabled: channel=%s interval=%s", channelID, interval), nil
}

func (s *Service) UnwatchDashboard(ctx context.Context) (string, error) {
	if err := s.store.Update(func(data *state.Data) {
		data.DashboardChannelID = ""
		data.DashboardMessageID = ""
		data.DashboardIntervalSec = 0
	}); err != nil {
		return "", err
	}
	return "dashboard watch disabled", nil
}

func (s *Service) RenderDashboard(ctx context.Context, sinceLabel string, interval time.Duration) string {
	snapshot := s.store.Snapshot()
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		sinceLabel = "30m"
		since, _ = security.ParseSince(sinceLabel)
	}
	alarmNames, err := s.alarms.AlarmNames(ctx)
	if err != nil {
		alarmNames = nil
	}
	inputs := s.dashboardInputs(ctx, since, alarmNames, s.cfg.Dashboard.MaxCloudWatchQueries)
	return formatting.FormatDashboardWithMetaAndAlerts(sinceLabel, inputs, alarmNames, time.Now(), interval, recentServiceAlertLines(snapshot.RecentServiceAlerts, 5))
}

func (s *Service) RefreshDashboard(ctx context.Context) error {
	snapshot := s.store.Snapshot()
	channelID := strings.TrimSpace(firstNonEmpty(snapshot.DashboardChannelID, s.cfg.Dashboard.ChannelID))
	if !s.cfg.Dashboard.Enabled && strings.TrimSpace(snapshot.DashboardChannelID) == "" {
		return nil
	}
	if channelID == "" {
		return nil
	}
	interval := s.dashboardInterval(snapshot)
	content := s.RenderDashboard(ctx, s.cfg.Dashboard.Since, interval)
	messageID := strings.TrimSpace(snapshot.DashboardMessageID)
	if messageID != "" {
		if err := s.discord.EditChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, messageID, content); err == nil {
			return s.store.Update(func(data *state.Data) {
				data.DashboardChannelID = channelID
				data.LastDashboardUpdatedAt = time.Now()
			})
		} else {
			log.Printf("dashboard message edit failed: %v", err)
		}
	}
	msg, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, content)
	if err != nil {
		return err
	}
	return s.store.Update(func(data *state.Data) {
		data.DashboardChannelID = channelID
		data.DashboardMessageID = msg.ID
		data.LastDashboardUpdatedAt = time.Now()
	})
}

func (s *Service) dashboardLoop(ctx context.Context) {
	for {
		if err := s.RefreshDashboard(ctx); err != nil {
			log.Printf("dashboard refresh failed: %v", err)
		}
		interval := s.dashboardInterval(s.store.Snapshot())
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) dashboardInputs(ctx context.Context, since time.Duration, alarmNames []string, maxQueries int) []formatting.DashboardServiceInput {
	registry := s.cfg.ServiceRegistry
	if len(registry) == 0 {
		registry = config.BuildServiceRegistry(s.cfg.LogGroups, s.cfg.HealthURLs)
	}
	queries := newQueryBudget(maxQueries)
	inputs := make([]formatting.DashboardServiceInput, 0, len(registry))
	for _, service := range registry {
		input := formatting.DashboardServiceInput{
			Service:     service.Name,
			DisplayName: service.DisplayName,
			Health:      formatting.ServiceStatus{Service: service.Name, State: "UNKNOWN", Detail: "not connected in service ops phase"},
			Alarm:       serviceHasAlarm(service.Name, alarmNames),
		}
		if !isServiceOpsConnected(service) {
			input.LogStatus = "NO_LOGS"
			input.Alarm = false
			inputs = append(inputs, input)
			continue
		}
		input.Health = s.health.Check(ctx, service.Name)
		if !service.Enabled {
			input.Health = formatting.ServiceStatus{Service: service.Name, State: "NOT_CONFIGURED", Detail: "health URL and log group are not configured"}
			input.LogStatus = "NOT_CONFIGURED"
			inputs = append(inputs, input)
			continue
		}
		if strings.TrimSpace(service.LogGroup) == "" {
			input.LogStatus = "NOT_CONFIGURED"
			inputs = append(inputs, input)
			continue
		}
		if !queries.Allow() {
			input.LogStatus = "LOG_QUERY_FAILED"
			inputs = append(inputs, input)
			continue
		}
		query, err := cw.BuildDashboardSummaryQuery(service.Name)
		if err != nil {
			input.LogStatus = "LOG_QUERY_FAILED"
			inputs = append(inputs, input)
			continue
		}
		rows, err := s.logs.Query(ctx, []string{service.LogGroup}, query, since, 100)
		if err != nil {
			input.LogStatus = logStatusFromQueryError(err)
			inputs = append(inputs, input)
			continue
		}
		input.Rows = rows
		if len(rows) == 0 {
			input.LogStatus = "NO_LOGS"
		} else {
			input.LogStatus = "OK"
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func isServiceOpsConnected(service config.ServiceDefinition) bool {
	return service.Name == "report"
}

func logStatusFromQueryError(err error) string {
	if err == nil {
		return "OK"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "accessdenied"), strings.Contains(message, "unauthorized"), strings.Contains(message, "forbidden"), strings.Contains(message, "unrecognizedclient"), strings.Contains(message, "expiredtoken"):
		return "AUTH"
	case strings.Contains(message, "timeout"), strings.Contains(message, "deadline exceeded"):
		return "TIMEOUT"
	default:
		return "ERR"
	}
}

func recentServiceAlertLines(events []state.ServiceAlertEventState, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	lines := make([]string, 0, limit)
	for _, event := range events {
		if len(lines) >= limit {
			break
		}
		summary := strings.TrimSpace(event.Summary)
		if summary == "" {
			summary = strings.TrimSpace(event.Service + " " + event.AlertType)
		}
		if !event.CreatedAt.IsZero() {
			summary = fmt.Sprintf("%s (%s)", summary, event.CreatedAt.In(time.FixedZone("KST", 9*60*60)).Format("15:04"))
		}
		lines = append(lines, summary)
	}
	return lines
}

func (s *Service) dashboardInterval(snapshot state.Data) time.Duration {
	if snapshot.DashboardIntervalSec > 0 {
		return time.Duration(snapshot.DashboardIntervalSec) * time.Second
	}
	if s.cfg.Dashboard.RefreshInterval > 0 {
		return s.cfg.Dashboard.RefreshInterval
	}
	return 5 * time.Minute
}

type queryBudget struct {
	max  int
	used int
}

func newQueryBudget(max int) *queryBudget {
	if max <= 0 {
		max = 6
	}
	return &queryBudget{max: max}
}

func (b *queryBudget) Allow() bool {
	if b.used >= b.max {
		return false
	}
	b.used++
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func serviceHasAlarm(service string, alarmNames []string) bool {
	for _, name := range alarmNames {
		normalized := strings.ToLower(name)
		if strings.Contains(normalized, service) || (service == "post" && strings.Contains(normalized, "blog")) {
			return true
		}
	}
	return false
}
