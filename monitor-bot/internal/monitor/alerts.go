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
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

type Alert struct {
	Fingerprint string
	Service     string
	AlertType   string
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
			_ = s.markAlertSent(alert.Fingerprint, true)
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
		healthStatus := s.health.Check(ctx, service.Name)
		s.updateHealthDownCount(service.Name, strings.EqualFold(healthStatus.State, "DOWN"))
		if s.healthDownCount(service.Name) >= s.cfg.Alert.HealthDownConsecutive && s.cfg.Alert.HealthDownConsecutive > 0 {
			alerts = append(alerts, Alert{
				Fingerprint: fmt.Sprintf("prod:%s:health-down:-:-", service.Name),
				Service:     service.Name,
				AlertType:   "health-down",
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
				})
				continue
			}
		}
		summary := formatting.SummarizeRows(rows)
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

func (s *Service) markAlertSent(fingerprint string, active bool) error {
	return s.store.Update(func(data *state.Data) {
		alert := data.Alerts[fingerprint]
		alert.Active = active
		alert.LastSentAt = time.Now()
		if !active {
			alert.ResolvedAt = time.Now()
		}
		data.Alerts[fingerprint] = alert
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
		_ = s.markAlertSent(fingerprint, false)
	}
}

func (s *Service) sendAlert(ctx context.Context, alert Alert) error {
	content := formatAlert(alert)
	_, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, s.cfg.Alert.ChannelID, content)
	return err
}

func formatAlert(alert Alert) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔴 [PROD] %s %s detected\n\n", alert.Service, alert.AlertType)
	fmt.Fprintf(&b, "Service: %s\n", alert.Service)
	b.WriteString("Window: last 5m\n")
	fmt.Fprintf(&b, "5xx: %d\n", alert.FiveXX)
	fmt.Fprintf(&b, "ERROR: %d\n", alert.Error)
	fmt.Fprintf(&b, "Top path:\n%s\n\n", formatting.TruncateDiscordMessage(alert.Path))
	b.WriteString("Recent traces:\n")
	if len(alert.Traces) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, trace := range alert.Traces {
			fmt.Fprintf(&b, "- %s\n", trace)
		}
	}
	b.WriteString("\nNext commands:\n")
	fmt.Fprintf(&b, "/errors service:%s since:15m\n", alert.Service)
	fmt.Fprintf(&b, "/logs service:%s since:15m level:ERROR\n", alert.Service)
	if len(alert.Traces) > 0 {
		fmt.Fprintf(&b, "/trace trace_id:%s", alert.Traces[0])
	}
	return formatting.TruncateDiscordMessage(b.String())
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
