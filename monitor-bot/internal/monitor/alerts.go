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
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/opslog"
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
	V2Log       *opslog.V2OpsLog
	V2Decision  opslog.AlertDecision
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
	if !s.alertsEnabled() {
		return nil
	}
	channelID := s.alertChannelID()
	if strings.TrimSpace(channelID) == "" {
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
	registry := s.cfg.ServiceRegistry
	if len(registry) == 0 {
		registry = config.BuildServiceRegistry(s.cfg.LogGroups, s.cfg.HealthURLs)
	}
	queries := newQueryBudget(s.cfg.Dashboard.MaxCloudWatchQueries)
	for _, service := range registry {
		if !isServiceOpsConnected(service) {
			continue
		}
		if strings.TrimSpace(service.LogGroup) == "" || !queries.Allow() {
			continue
		}
		query, err := cw.BuildAlertQuery(service.Name)
		if err != nil {
			continue
		}
		rows, err := s.logs.Query(ctx, []string{service.LogGroup}, query, 5*time.Minute, 100)
		if err != nil {
			continue
		}
		for _, row := range rows {
			alert := makeV2Alert(row)
			if alert.Fingerprint == "" {
				continue
			}
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

func makeV2Alert(row map[string]string) Alert {
	log := opslog.RowToV2OpsLog(row)
	decision := opslog.DecideV2Alert(log)
	if !decision.Alert {
		return Alert{}
	}
	traceID := ""
	if log.Trace != nil {
		traceID = strings.TrimSpace(log.Trace.TraceID)
	}
	path := ""
	if log.HTTP != nil {
		path = firstNonEmpty(log.HTTP.Route, log.HTTP.Path)
	}
	code := ""
	if log.Response != nil && log.Response.Error != nil && log.Response.Error.Code != 0 {
		code = strconv.Itoa(log.Response.Error.Code)
	}
	fpKey := firstNonEmpty(traceID, strings.Join([]string{log.Timestamp, log.LogType, path, code}, ":"))
	return Alert{
		Fingerprint: fmt.Sprintf("prod:%s:v2:%s:%s", decision.Domain, log.LogType, fpKey),
		Service:     decision.Domain,
		AlertType:   "v2-log",
		Severity:    decision.Severity,
		Reason:      decision.Reason,
		Path:        path,
		ErrorCode:   code,
		FiveXX:      boolCount(log.HTTP != nil && log.HTTP.StatusCode >= 500),
		Error:       1,
		Traces:      traces([]map[string]string{row}, 1),
		V2Log:       &log,
		V2Decision:  decision,
	}
}

func (s *Service) shouldSendAlert(fingerprint string, now time.Time) bool {
	snapshot := s.store.Snapshot()
	existing := snapshot.Alerts[fingerprint]
	cooldown := s.alertCooldown(snapshot)
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
			data.ServiceAlerts.LastSent[alert.Fingerprint] = time.Now()
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
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, s.alertChannelID(), content); err != nil {
			log.Printf("send resolved alert failed: %v", err)
			continue
		}
		_ = s.markAlertSent(Alert{Fingerprint: fingerprint}, false)
	}
}

func (s *Service) sendAlert(ctx context.Context, alert Alert) error {
	mention := ""
	if shouldMentionAlert(alert) {
		mention = s.alertRoleMention()
	}
	content := formatAlert(alert, mention)
	_, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, s.alertChannelID(), content)
	return err
}

func (s *Service) alertRoleMention() string {
	if roleID := strings.TrimSpace(s.store.Snapshot().ServiceAlerts.RoleID); roleID != "" {
		if validRoleID(roleID) {
			return "<@&" + roleID + ">\n"
		}
	}
	if len(s.cfg.DiscordAllowedRoleIDs) == 0 {
		return ""
	}
	roleID := strings.TrimSpace(s.cfg.DiscordAllowedRoleIDs[0])
	if roleID == "" {
		return ""
	}
	return "<@&" + roleID + ">\n"
}

func (s *Service) ConfigureAlert(ctx context.Context, channelID, action, roleID string) (string, error) {
	action = strings.TrimSpace(action)
	switch action {
	case "channel":
		if strings.TrimSpace(channelID) == "" {
			return "", fmt.Errorf("channel id is required")
		}
		if err := s.store.Update(func(data *state.Data) {
			data.ServiceAlerts.ChannelID = strings.TrimSpace(channelID)
		}); err != nil {
			return "", err
		}
		return "✅ 서비스 알림 채널을 현재 채널로 설정했습니다.", nil
	case "role":
		roleID = normalizeRoleID(roleID)
		if !validRoleID(roleID) {
			return "", fmt.Errorf("올바른 role을 선택하세요. @everyone, @here는 허용하지 않습니다")
		}
		if err := s.store.Update(func(data *state.Data) {
			data.ServiceAlerts.RoleID = roleID
		}); err != nil {
			return "", err
		}
		return "✅ 서비스 알림 role을 <@&" + roleID + "> 로 설정했습니다.", nil
	case "role-clear":
		if err := s.store.Update(func(data *state.Data) {
			data.ServiceAlerts.RoleID = ""
		}); err != nil {
			return "", err
		}
		return "✅ 서비스 알림 role mention을 제거했습니다.", nil
	case "on":
		if err := s.store.Update(func(data *state.Data) {
			data.ServiceAlerts.Enabled = true
			if data.ServiceAlerts.CooldownSec <= 0 {
				data.ServiceAlerts.CooldownSec = int(s.defaultAlertCooldown().Seconds())
			}
		}); err != nil {
			return "", err
		}
		return "✅ 서비스 알림을 켰습니다.", nil
	case "off":
		if err := s.store.Update(func(data *state.Data) {
			data.ServiceAlerts.Enabled = false
		}); err != nil {
			return "", err
		}
		return "✅ 서비스 알림을 껐습니다.", nil
	case "status":
		return s.FormatAlertStatus(), nil
	case "test":
		target := s.alertChannelID()
		if target == "" {
			target = strings.TrimSpace(channelID)
		}
		if target == "" {
			return "", fmt.Errorf("alert channel이 설정되지 않았습니다. 먼저 /ops alert action:channel 을 실행하세요")
		}
		content := "✅ 서비스 알림 테스트\n\n서비스 운영 알림 채널 설정이 정상입니다.\nrole mention은 CRITICAL alert에서만 적용됩니다."
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, target, content); err != nil {
			return "", err
		}
		return "✅ 테스트 알림을 전송했습니다.", nil
	default:
		return "", fmt.Errorf("지원하지 않는 alert action입니다")
	}
}

func (s *Service) FormatAlertStatus() string {
	snapshot := s.store.Snapshot()
	enabled := s.alertsEnabled()
	channelID := s.alertChannelID()
	roleID := strings.TrimSpace(snapshot.ServiceAlerts.RoleID)
	cooldown := s.alertCooldown(snapshot)
	var b strings.Builder
	b.WriteString("🔔 Service Alert Status\n\n")
	fmt.Fprintf(&b, "enabled: %t\n", enabled)
	if channelID == "" {
		b.WriteString("channel: NOT_CONFIGURED\n")
	} else {
		fmt.Fprintf(&b, "channel: <#%s>\n", channelID)
	}
	if roleID == "" {
		b.WriteString("role mention: none\n")
	} else {
		fmt.Fprintf(&b, "role mention: <@&%s>\n", roleID)
	}
	fmt.Fprintf(&b, "cooldown: %s\n", formatKoreanDuration(cooldown))
	fmt.Fprintf(&b, "recent alert fingerprints: %d", len(snapshot.ServiceAlerts.LastSent))
	return formatting.TruncateDiscordMessage(b.String())
}

func (s *Service) alertsEnabled() bool {
	snapshot := s.store.Snapshot()
	return snapshot.ServiceAlerts.Enabled || s.cfg.Alert.Enabled || strings.TrimSpace(s.cfg.Alert.ChannelID) != ""
}

func (s *Service) alertChannelID() string {
	snapshot := s.store.Snapshot()
	return strings.TrimSpace(firstNonEmpty(snapshot.ServiceAlerts.ChannelID, s.cfg.Alert.ChannelID))
}

func (s *Service) alertCooldown(snapshot state.Data) time.Duration {
	if snapshot.ServiceAlerts.CooldownSec > 0 {
		return time.Duration(snapshot.ServiceAlerts.CooldownSec) * time.Second
	}
	return s.defaultAlertCooldown()
}

func (s *Service) defaultAlertCooldown() time.Duration {
	if s.cfg.Alert.Cooldown > 0 {
		return s.cfg.Alert.Cooldown
	}
	return 15 * time.Minute
}

func normalizeRoleID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<@&")
	value = strings.TrimSuffix(value, ">")
	return value
}

func validRoleID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "everyone") || strings.EqualFold(value, "here") {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func formatAlert(alert Alert, mention string) string {
	if alert.AlertType == "v2-log" && alert.V2Log != nil {
		return formatting.TruncateDiscordMessage(opslog.FormatV2Alert(*alert.V2Log, alert.V2Decision, mention))
	}
	var b strings.Builder
	if shouldMentionAlert(alert) {
		b.WriteString(mention)
	}
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
	if service == "post" {
		return "blog"
	}
	return service
}

func shouldMentionAlert(alert Alert) bool {
	if alert.AlertType == "v2-log" {
		return alert.V2Decision.Alert && alert.V2Decision.Mention && alert.V2Decision.Severity == opslog.SeverityCrit
	}
	return alert.Severity == opslog.SeverityCrit || alert.Severity == "P0"
}

func serviceAlertSummary(alert Alert) string {
	if alert.AlertType == "v2-log" {
		return fmt.Sprintf("%s %s - %s", displayServiceName(alert.Service), alert.Severity, alert.Reason)
	}
	return fmt.Sprintf("%s %s - %s", displayServiceName(alert.Service), alertStatus(alert), alertReasonText(alert))
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

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
