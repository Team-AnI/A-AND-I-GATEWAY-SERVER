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
	SendChannelMessageWithRoleMention(ctx context.Context, client *http.Client, botToken, channelID, content, roleID string) (discord.Message, error)
	EditChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, messageID, content string) error
}

type discordAPI struct{}

func (discordAPI) SendChannelMessage(ctx context.Context, client *http.Client, botToken, channelID, content string) (discord.Message, error) {
	return discord.SendChannelMessage(ctx, client, botToken, channelID, content)
}

func (discordAPI) SendChannelMessageWithRoleMention(ctx context.Context, client *http.Client, botToken, channelID, content, roleID string) (discord.Message, error) {
	return discord.SendChannelMessageWithRoleMention(ctx, client, botToken, channelID, content, roleID)
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
	go s.alertLoop(ctx)
	go s.logFeedLoop(ctx)
	go s.assignmentOpsLoop(ctx)
}

func (s *Service) WatchDashboard(ctx context.Context, channelID string, interval time.Duration) (string, error) {
	return s.WatchDashboardScope(ctx, channelID, "all", "", interval)
}

func (s *Service) WatchDashboardScope(ctx context.Context, channelID, scope, service string, interval time.Duration) (string, error) {
	if strings.TrimSpace(channelID) == "" {
		return "", fmt.Errorf("channel id is required")
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "service" {
		return "", fmt.Errorf("지원하지 않는 scope입니다")
	}
	if scope == "service" {
		normalized, ok := security.NormalizeService(service)
		if !ok {
			return "", fmt.Errorf("지원하지 않는 service입니다")
		}
		service = normalized
		if !isServiceOpsNameConnected(service) {
			return fmt.Sprintf("⚠️ 아직 연동되지 않은 서비스입니다\n\nservice: %s\n상태: NOT_CONNECTED\ncatalog에는 표시되지만 자동 dashboard 조회 대상은 아닙니다.", displayServiceName(service)), nil
		}
	}
	if interval <= 0 {
		interval = s.cfg.Dashboard.RefreshInterval
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	key := state.DashboardKey(scope, service)
	if err := s.store.Update(func(data *state.Data) {
		data.ServiceDashboards[key] = state.ServiceDashboard{
			Scope:       scope,
			Service:     service,
			ChannelID:   strings.TrimSpace(channelID),
			IntervalSec: int(interval.Seconds()),
		}
		if scope == "all" {
			data.DashboardChannelID = strings.TrimSpace(channelID)
			data.DashboardIntervalSec = int(interval.Seconds())
		}
	}); err != nil {
		return "", err
	}
	if err := s.RefreshDashboardWatch(ctx, key); err != nil {
		return "", err
	}
	scopeLabel := "all"
	if scope == "service" {
		scopeLabel = "service:" + displayServiceName(service)
	}
	return fmt.Sprintf("✅ 서비스 대시보드 등록 완료\n\nscope: %s\n채널: 현재 채널\n업데이트 주기: %s\n방식: 기존 메시지 자동 갱신", scopeLabel, formatKoreanDuration(interval)), nil
}

func (s *Service) UnwatchDashboard(ctx context.Context) (string, error) {
	return s.UnwatchDashboardScope(ctx, "all", "")
}

func (s *Service) UnwatchDashboardScope(ctx context.Context, scope, service string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "all"
	}
	if scope == "service" {
		normalized, ok := security.NormalizeService(service)
		if !ok {
			return "", fmt.Errorf("지원하지 않는 service입니다")
		}
		service = normalized
	}
	key := state.DashboardKey(scope, service)
	existed := false
	if err := s.store.Update(func(data *state.Data) {
		_, existed = data.ServiceDashboards[key]
		delete(data.ServiceDashboards, key)
		if key == "all" {
			data.DashboardChannelID = ""
			data.DashboardMessageID = ""
			data.DashboardIntervalSec = 0
		}
	}); err != nil {
		return "", err
	}
	if !existed {
		return "NO_DATA: 해당 dashboard watch가 이미 비활성 상태입니다.", nil
	}
	return "✅ 자동 갱신이 중지되었습니다. 기존 Discord 메시지는 삭제하지 않습니다.", nil
}

func (s *Service) ListDashboardWatches(ctx context.Context) string {
	snapshot := s.store.Snapshot()
	if len(snapshot.ServiceDashboards) == 0 {
		return "등록된 서비스 대시보드 watch가 없습니다.\n\nNext:\n- `/ops dashboard action:watch interval:5m`"
	}
	var b strings.Builder
	b.WriteString("📌 Service Dashboard Watches\n\n")
	for key, watch := range snapshot.ServiceDashboards {
		status := "ACTIVE"
		if watch.Disabled {
			status = "DISABLED"
		}
		if strings.TrimSpace(watch.ConfigError) != "" {
			status = "CONFIG_ERROR"
		}
		fmt.Fprintf(&b, "- %s channel=<#%s> interval=%s message=%t status=%s\n",
			key,
			watch.ChannelID,
			formatKoreanDuration(time.Duration(watch.IntervalSec)*time.Second),
			strings.TrimSpace(watch.MessageID) != "",
			status,
		)
	}
	return formatting.TruncateDiscordMessage(b.String())
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
	return s.RefreshDashboardWatch(ctx, "all")
}

func (s *Service) RefreshDashboardWatch(ctx context.Context, key string) error {
	snapshot := s.store.Snapshot()
	watch, ok := snapshot.ServiceDashboards[key]
	if !ok && key == "all" {
		channelID := strings.TrimSpace(firstNonEmpty(snapshot.DashboardChannelID, s.cfg.Dashboard.ChannelID))
		if channelID == "" || !s.cfg.Dashboard.Enabled && strings.TrimSpace(snapshot.DashboardChannelID) == "" {
			return nil
		}
		watch = state.ServiceDashboard{
			Scope:       "all",
			ChannelID:   channelID,
			MessageID:   snapshot.DashboardMessageID,
			IntervalSec: int(s.dashboardInterval(snapshot).Seconds()),
		}
	} else if !ok {
		return nil
	}
	if watch.Disabled {
		return nil
	}
	channelID := strings.TrimSpace(watch.ChannelID)
	if channelID == "" {
		return nil
	}
	interval := dashboardWatchInterval(watch, s.dashboardInterval(snapshot))
	content := s.RenderDashboardForWatch(ctx, watch, interval)
	messageID := strings.TrimSpace(watch.MessageID)
	if messageID != "" {
		if err := s.discord.EditChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, messageID, content); err == nil {
			return s.store.Update(func(data *state.Data) {
				current := data.ServiceDashboards[key]
				current.ChannelID = channelID
				current.LastUpdatedAt = time.Now()
				current.LastStatus = "OK"
				current.ConfigError = ""
				data.ServiceDashboards[key] = current
				if key == "all" {
					data.DashboardChannelID = channelID
					data.LastDashboardUpdatedAt = time.Now()
				}
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
		current := data.ServiceDashboards[key]
		current.ChannelID = channelID
		current.MessageID = msg.ID
		current.LastUpdatedAt = time.Now()
		current.LastStatus = "OK"
		current.ConfigError = ""
		data.ServiceDashboards[key] = current
		if key == "all" {
			data.DashboardChannelID = channelID
			data.DashboardMessageID = msg.ID
			data.LastDashboardUpdatedAt = time.Now()
		}
	})
}

func (s *Service) dashboardLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.refreshDueDashboards(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) refreshDueDashboards(ctx context.Context) {
	snapshot := s.store.Snapshot()
	if len(snapshot.ServiceDashboards) == 0 && s.cfg.Dashboard.Enabled && strings.TrimSpace(s.cfg.Dashboard.ChannelID) != "" {
		if err := s.RefreshDashboardWatch(ctx, "all"); err != nil {
			log.Printf("dashboard refresh failed: %v", err)
		}
		return
	}
	now := time.Now()
	for key, watch := range snapshot.ServiceDashboards {
		interval := dashboardWatchInterval(watch, s.dashboardInterval(snapshot))
		if watch.LastUpdatedAt.IsZero() || now.Sub(watch.LastUpdatedAt) >= interval {
			if err := s.RefreshDashboardWatch(ctx, key); err != nil {
				log.Printf("dashboard refresh failed for %s: %v", key, err)
			}
		}
	}
}

func (s *Service) RenderDashboardForWatch(ctx context.Context, watch state.ServiceDashboard, interval time.Duration) string {
	if watch.Scope == "service" {
		return s.RenderServiceDashboard(ctx, watch.Service, s.cfg.Dashboard.Since, interval)
	}
	return s.RenderDashboard(ctx, s.cfg.Dashboard.Since, interval)
}

func (s *Service) RenderServiceDashboard(ctx context.Context, service, sinceLabel string, interval time.Duration) string {
	normalized, ok := security.NormalizeService(service)
	if !ok {
		return "지원하지 않는 service입니다."
	}
	since, ok := security.ParseSince(sinceLabel)
	if !ok {
		sinceLabel = "30m"
		since, _ = security.ParseSince(sinceLabel)
	}
	registry := s.cfg.ServiceRegistry
	if len(registry) == 0 {
		registry = config.BuildServiceRegistry(s.cfg.LogGroups, s.cfg.HealthURLs)
	}
	for _, item := range registry {
		if item.Name != normalized {
			continue
		}
		inputs := s.dashboardInputsForRegistry(ctx, since, []string{}, s.cfg.Dashboard.MaxCloudWatchQueries, []config.ServiceDefinition{item})
		return formatting.FormatDashboardWithMetaAndAlerts(sinceLabel, inputs, nil, time.Now(), interval, recentServiceAlertLines(s.store.Snapshot().RecentServiceAlerts, 5))
	}
	return "service registry에 없는 서비스입니다."
}

func (s *Service) dashboardInputs(ctx context.Context, since time.Duration, alarmNames []string, maxQueries int) []formatting.DashboardServiceInput {
	registry := s.cfg.ServiceRegistry
	if len(registry) == 0 {
		registry = config.BuildServiceRegistry(s.cfg.LogGroups, s.cfg.HealthURLs)
	}
	return s.dashboardInputsForRegistry(ctx, since, alarmNames, maxQueries, registry)
}

func (s *Service) dashboardInputsForRegistry(ctx context.Context, since time.Duration, alarmNames []string, maxQueries int, registry []config.ServiceDefinition) []formatting.DashboardServiceInput {
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
			input.LogStatus = "NO_V2_LOG"
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
			input.LogStatus = "NO_V2_LOG"
		} else {
			input.LogStatus = "OK"
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func isServiceOpsConnected(service config.ServiceDefinition) bool {
	return isServiceOpsNameConnected(service.Name)
}

func isServiceOpsNameConnected(service string) bool {
	return service == "gateway" || service == "auth" || service == "report" || service == "post"
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

func dashboardWatchInterval(watch state.ServiceDashboard, fallback time.Duration) time.Duration {
	if watch.IntervalSec > 0 {
		return time.Duration(watch.IntervalSec) * time.Second
	}
	if fallback > 0 {
		return fallback
	}
	return 5 * time.Minute
}

func formatKoreanDuration(duration time.Duration) string {
	if duration <= 0 {
		return "-"
	}
	if duration%time.Hour == 0 {
		return fmt.Sprintf("%d시간", int(duration/time.Hour))
	}
	if duration%time.Minute == 0 {
		return fmt.Sprintf("%d분", int(duration/time.Minute))
	}
	return duration.String()
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
