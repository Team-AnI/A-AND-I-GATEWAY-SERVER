package monitor

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

type Alert struct {
	Fingerprint string
	Service     string
	AlertType   string
	Severity    string
	Reason      string
	Path        string
	ErrorCode   string
	FiveXX      int
	Error       int
	Traces      []string
	Resolved    bool
}

func (s *Service) alertLoop(ctx context.Context) {
	for {
		if err := s.PollAlerts(ctx); err != nil {
			log.Printf("alert poll failed: %v", err)
		}
		interval := s.cfg.Alert.PollInterval
		if interval <= 0 {
			interval = 3 * time.Minute
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) PollAlerts(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.Alert.ChannelID) == "" {
		return nil
	}
	active := make(map[string]Alert)
	alerts := s.collectAlerts(ctx)
	for _, alert := range alerts {
		active[alert.Fingerprint] = alert
		if s.shouldSendAlert(alert.Fingerprint, time.Now()) {
			if err := s.sendAlert(ctx, alert); err != nil {
				log.Printf("send alert failed: %v", err)
				continue
			}
			_ = s.markAlertSent(alert, true)
		}
	}
	s.sendResolvedAlerts(ctx, active)
	return nil
}

func (s *Service) collectAlerts(ctx context.Context) []Alert {
	var alerts []Alert
	if names, err := s.alarms.AlarmNames(ctx); err == nil {
		for _, name := range names {
			alerts = append(alerts, Alert{
				Fingerprint: fmt.Sprintf("prod:cloudwatch:alarm:%s:-", name),
				Service:     "cloudwatch",
				AlertType:   "alarm",
				Severity:    "P0",
				Reason:      "CloudWatch alarm is ALARM",
				Path:        name,
			})
		}
	}
	registry := s.cfg.ServiceRegistry
	if len(registry) == 0 {
		registry = config.BuildServiceRegistry(s.cfg.LogGroups, s.cfg.HealthURLs)
	}
	queries := newQueryBudget(s.cfg.Dashboard.MaxCloudWatchQueries)
	for _, service := range registry {
		if !isServiceOpsConnected(service) {
			continue
		}
		healthStatus := s.health.Check(ctx, service.Name)
		s.updateHealthDownCount(service.Name, strings.EqualFold(healthStatus.State, "DOWN"))
		if s.healthDownCount(service.Name) >= s.cfg.Alert.HealthDownConsecutive && s.cfg.Alert.HealthDownConsecutive > 0 {
			alerts = append(alerts, Alert{
				Fingerprint: fmt.Sprintf("prod:%s:health-down:-:-", service.Name),
				Service:     service.Name,
				AlertType:   "health-down",
				Severity:    "P0",
				Reason:      fmt.Sprintf("health check %d회 연속 실패", s.cfg.Alert.HealthDownConsecutive),
			})
		}
		if strings.TrimSpace(service.LogGroup) == "" || !queries.Allow() {
			continue
		}
		query, err := cw.BuildDashboardSummaryQuery(service.Name)
		if err != nil {
			continue
		}
		rows, err := s.logs.Query(ctx, []string{service.LogGroup}, query, 5*time.Minute, 100)
		if err != nil {
			status := logStatusFromQueryError(err)
			alerts = append(alerts, Alert{
				Fingerprint: fmt.Sprintf("prod:%s:log-query-%s:-:-", service.Name, strings.ToLower(status)),
				Service:     service.Name,
				AlertType:   "log-query-failed",
				Severity:    "P1",
				Reason:      "CloudWatch 로그 조회 실패: " + status,
			})
			continue
		}
		if len(rows) == 0 && s.cfg.Alert.NoLogsMinutes > 0 {
			noLogs := s.cfg.Alert.NoLogsMinutes <= 5
			if !noLogs && queries.Allow() {
				lastQuery, err := cw.BuildLastLogQuery(service.Name)
				if err == nil {
					lastRows, err := s.logs.Query(ctx, []string{service.LogGroup}, lastQuery, time.Duration(s.cfg.Alert.NoLogsMinutes)*time.Minute, 1)
					noLogs = err == nil && len(lastRows) == 0
				}
			}
			if noLogs {
				alerts = append(alerts, Alert{
					Fingerprint: fmt.Sprintf("prod:%s:no-logs:-:-", service.Name),
					Service:     service.Name,
					AlertType:   "no-logs",
					Severity:    "P1",
					Reason:      fmt.Sprintf("최근 %d분 로그 없음", s.cfg.Alert.NoLogsMinutes),
				})
				continue
			}
		}
		summary := formatting.SummarizeRows(rows)
		if dbConnectionErrors(rows) > 0 {
			alerts = append(alerts, makeAlert(service.Name, "db-connection", rows, summary))
		}
		if summary.FiveXX >= s.cfg.Alert.FiveXXThreshold5m && s.cfg.Alert.FiveXXThreshold5m > 0 {
			alerts = append(alerts, makeAlert(service.Name, "5xx", rows, summary))
		}
		if summary.Error >= s.cfg.Alert.ErrorThreshold5m && s.cfg.Alert.ErrorThreshold5m > 0 {
			alerts = append(alerts, makeAlert(service.Name, "error", rows, summary))
		}
		if service.Name == "report" && copyAPIFiveXX(rows) >= s.cfg.Alert.CopyAPIFiveXXThreshold5m && s.cfg.Alert.CopyAPIFiveXXThreshold5m > 0 {
			alerts = append(alerts, makeAlert(service.Name, "copy-api-5xx", rows, summary))
		}
	}
	return alerts
}

func makeAlert(service, alertType string, rows []map[string]string, summary formatting.LogSummary) Alert {
	path, code := topPathAndCode(rows)
	if path == "" {
		path = "-"
	}
	return Alert{
		Fingerprint: fmt.Sprintf("prod:%s:%s:%s:%s", service, alertType, path, code),
		Service:     service,
		AlertType:   alertType,
		Severity:    alertSeverity(alertType),
		Reason:      alertReason(alertType),
		Path:        path,
		ErrorCode:   code,
		FiveXX:      summary.FiveXX,
		Error:       summary.Error,
		Traces:      traces(rows, 3),
	}
}

func (s *Service) shouldSendAlert(fingerprint string, now time.Time) bool {
	snapshot := s.store.Snapshot()
	existing := snapshot.Alerts[fingerprint]
	cooldown := s.cfg.Alert.Cooldown
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	return existing.LastSentAt.IsZero() || now.Sub(existing.LastSentAt) >= cooldown || !existing.Active
}

func (s *Service) markAlertSent(alert Alert, active bool) error {
	return s.store.Update(func(data *state.Data) {
		stateAlert := data.Alerts[alert.Fingerprint]
		stateAlert.Active = active
		stateAlert.LastSentAt = time.Now()
		if !active {
			stateAlert.ResolvedAt = time.Now()
		} else {
			data.RecentServiceAlerts = append([]state.ServiceAlertEventState{{
				Fingerprint: alert.Fingerprint,
				Severity:    alert.Severity,
				Service:     alert.Service,
				AlertType:   alert.AlertType,
				Summary:     serviceAlertSummary(alert),
				CreatedAt:   time.Now(),
			}}, data.RecentServiceAlerts...)
		}
		data.Alerts[alert.Fingerprint] = stateAlert
		data.LastAlertSentAt = time.Now()
	})
}

func (s *Service) sendResolvedAlerts(ctx context.Context, active map[string]Alert) {
	snapshot := s.store.Snapshot()
	for fingerprint, existing := range snapshot.Alerts {
		if !existing.Active {
			continue
		}
		if _, ok := active[fingerprint]; ok {
			continue
		}
		content := fmt.Sprintf("🟢 [PROD] alert resolved\n\nFingerprint: `%s`", fingerprint)
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, s.cfg.Alert.ChannelID, content); err != nil {
			log.Printf("send resolved alert failed: %v", err)
			continue
		}
		_ = s.markAlertSent(Alert{Fingerprint: fingerprint}, false)
	}
}

func (s *Service) sendAlert(ctx context.Context, alert Alert) error {
	content := formatAlert(alert, s.alertRoleMention())
	_, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, s.cfg.Alert.ChannelID, content)
	return err
}

func (s *Service) alertRoleMention() string {
	if len(s.cfg.DiscordAllowedRoleIDs) == 0 {
		return ""
	}
	roleID := strings.TrimSpace(s.cfg.DiscordAllowedRoleIDs[0])
	if roleID == "" {
		return ""
	}
	return "<@&" + roleID + ">\n"
}

func formatAlert(alert Alert, mention string) string {
	var b strings.Builder
	b.WriteString(mention)
	icon := "⚠️"
	title := "서비스 에러 증가 감지"
	if alert.Severity == "P0" {
		icon = "🚨"
		title = "서비스 장애 감지"
	}
	fmt.Fprintf(&b, "%s %s\n\n", icon, title)
	fmt.Fprintf(&b, "서비스: %s\n", displayServiceName(alert.Service))
	fmt.Fprintf(&b, "상태: %s\n", alertStatus(alert))
	fmt.Fprintf(&b, "원인: %s\n", security.SanitizeText(alertReasonText(alert)))
	fmt.Fprintf(&b, "감지 시각: %s KST\n\n", time.Now().In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04"))
	b.WriteString("최근 5분\n")
	fmt.Fprintf(&b, "- 5xx: %d건\n", alert.FiveXX)
	fmt.Fprintf(&b, "- ERROR 로그: %d건\n", alert.Error)
	if strings.TrimSpace(alert.Path) != "" && alert.Path != "-" {
		fmt.Fprintf(&b, "- 주요 경로: %s\n", security.SanitizeText(alert.Path))
	}
	b.WriteString("\n최근 trace\n")
	if len(alert.Traces) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, trace := range alert.Traces {
			fmt.Fprintf(&b, "- %s\n", trace)
		}
	}
	b.WriteString("\n상세 확인:\n")
	if alert.Service == "cloudwatch" {
		b.WriteString("/ops alarms state:ALARM\n")
	} else {
		fmt.Fprintf(&b, "/ops logs service:%s mode:errors since:15m limit:10\n", alert.Service)
		fmt.Fprintf(&b, "/ops logs service:%s mode:slow since:15m limit:10\n", alert.Service)
	}
	if len(alert.Traces) > 0 {
		fmt.Fprintf(&b, "/ops trace trace_id:%s", alert.Traces[0])
	}
	return formatting.TruncateDiscordMessage(b.String())
}

func alertSeverity(alertType string) string {
	switch alertType {
	case "5xx", "health-down", "db-connection", "copy-api-5xx", "alarm":
		return "P0"
	default:
		return "P1"
	}
}

func alertReason(alertType string) string {
	switch alertType {
	case "5xx":
		return "최근 5분 5xx 임계치 초과"
	case "error":
		return "최근 5분 ERROR 로그 임계치 초과"
	case "db-connection":
		return "DB connection error 패턴 감지"
	case "copy-api-5xx":
		return "중요 API 5xx 감지"
	case "no-logs":
		return "최근 로그 없음"
	case "log-query-failed":
		return "CloudWatch 로그 조회 실패"
	case "health-down":
		return "health check 연속 실패"
	default:
		return alertType
	}
}

func alertReasonText(alert Alert) string {
	if strings.TrimSpace(alert.Reason) != "" {
		return alert.Reason
	}
	return alertReason(alert.AlertType)
}

func alertStatus(alert Alert) string {
	if alert.AlertType == "health-down" {
		return "DOWN"
	}
	if alert.Severity == "P0" {
		return "장애"
	}
	return "주의"
}

func displayServiceName(service string) string {
	if service == "report" {
		return "report/web"
	}
	return service
}

func serviceAlertSummary(alert Alert) string {
	return fmt.Sprintf("%s %s - %s", displayServiceName(alert.Service), alertStatus(alert), alertReasonText(alert))
}

func topPathAndCode(rows []map[string]string) (string, string) {
	counts := map[string]int{}
	codes := map[string]string{}
	best := ""
	for _, row := range rows {
		path := row["http.path"]
		if path == "" {
			continue
		}
		key := path
		counts[key] += countValue(row)
		codes[key] = row["response.error.code"]
		if best == "" || counts[key] > counts[best] {
			best = key
		}
	}
	return best, codes[best]
}

func traces(rows []map[string]string, limit int) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, row := range rows {
		trace := strings.TrimSpace(row["trace.traceId"])
		if trace == "" {
			continue
		}
		if _, ok := seen[trace]; ok {
			continue
		}
		seen[trace] = struct{}{}
		result = append(result, trace)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func dbConnectionErrors(rows []map[string]string) int {
	count := 0
	for _, row := range rows {
		text := strings.ToLower(strings.Join([]string{
			row["message"],
			row["response.error.message"],
			row["response.error.value"],
			row["response.error.code"],
		}, " "))
		if strings.Contains(text, "db connection") ||
			strings.Contains(text, "database connection") ||
			strings.Contains(text, "connection refused") ||
			strings.Contains(text, "sql") && strings.Contains(text, "connection") {
			count += countValue(row)
		}
	}
	return count
}

func copyAPIFiveXX(rows []map[string]string) int {
	count := 0
	for _, row := range rows {
		if !strings.Contains(row["http.path"], "/assignments/copy") {
			continue
		}
		if strings.HasPrefix(row["http.statusCode"], "5") {
			count += countValue(row)
		}
	}
	return count
}

func (s *Service) updateHealthDownCount(service string, down bool) {
	_ = s.store.Update(func(data *state.Data) {
		if down {
			data.HealthDownCounts[service]++
			return
		}
		data.HealthDownCounts[service] = 0
	})
}

func (s *Service) healthDownCount(service string) int {
	return s.store.Snapshot().HealthDownCounts[service]
}

func countValue(row map[string]string) int {
	count, err := strconv.Atoi(strings.TrimSpace(row["count"]))
	if err != nil || count <= 0 {
		return 1
	}
	return count
}
