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
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/discord"
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

const (
	alertTargetAll      = "all"
	alertTargetGeneral  = "general"
	alertTargetCritical = "critical"
)

type alertRoute struct {
	Kind      string
	ChannelID string
	RoleID    string
	Mention   bool
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
	if s.generalAlertChannelID() == "" && s.criticalAlertChannelID() == "" {
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
				IncidentKey: serviceAlertIncidentKey(alert),
				Severity:    alert.Severity,
				Service:     alert.Service,
				AlertType:   alert.AlertType,
				Summary:     serviceAlertSummary(alert),
				TraceIDs:    serviceAlertTraceIDs(alert),
				Reason:      alert.Reason,
				Path:        alert.Path,
				ErrorCode:   alert.ErrorCode,
				CreatedAt:   time.Now(),
			}}, data.RecentServiceAlerts...)
		}
		data.Alerts[alert.Fingerprint] = stateAlert
		data.LastAlertSentAt = time.Now()
	})
}

func (s *Service) sendResolvedAlerts(ctx context.Context, active map[string]Alert) {
	snapshot := s.store.Snapshot()
	channelID := s.generalAlertChannelID()
	if channelID == "" {
		return
	}
	for fingerprint, existing := range snapshot.Alerts {
		if !existing.Active {
			continue
		}
		if _, ok := active[fingerprint]; ok {
			continue
		}
		content := fmt.Sprintf("🟢 [PROD] alert resolved\n\nFingerprint: `%s`", fingerprint)
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, content); err != nil {
			log.Printf("send resolved alert failed: %v", err)
			continue
		}
		_ = s.markAlertSent(Alert{Fingerprint: fingerprint}, false)
	}
}

func (s *Service) sendAlert(ctx context.Context, alert Alert) error {
	content := formatAlert(alert, "")
	components := alertDrilldownComponents(alert)
	route := s.alertRoute(alert)
	if route.ChannelID == "" {
		return fmt.Errorf("%s alert channel is not configured", route.Kind)
	}
	if route.Mention {
		if route.RoleID != "" {
			if len(components) > 0 {
				_, err := s.discord.SendChannelMessageWithRoleMentionAndComponents(ctx, s.client, s.cfg.DiscordBotToken, route.ChannelID, content, route.RoleID, components)
				return err
			}
			_, err := s.discord.SendChannelMessageWithRoleMention(ctx, s.client, s.cfg.DiscordBotToken, route.ChannelID, content, route.RoleID)
			return err
		}
	}
	if len(components) > 0 {
		_, err := s.discord.SendChannelMessageWithComponents(ctx, s.client, s.cfg.DiscordBotToken, route.ChannelID, content, components)
		return err
	}
	_, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, route.ChannelID, content)
	return err
}

func alertDrilldownComponents(alert Alert) []discord.MessageComponent {
	var buttons []discord.MessageComponent
	if traceID := alertTraceID(alert); traceID != "" {
		if customID, ok := discord.OpsTraceButtonCustomID(traceID); ok {
			buttons = append(buttons, discord.PrimaryButton("Trace 상세", customID))
		}
	}
	if service := alertButtonService(alert); service != "" {
		if customID, ok := discord.OpsServiceErrorsButtonCustomID(service); ok {
			buttons = append(buttons, discord.SecondaryButton(alertButtonServiceLabel(service)+" 오류 30m", customID))
		}
	}
	if len(buttons) == 0 {
		return nil
	}
	return []discord.MessageComponent{discord.ActionRow(buttons...)}
}

func alertTraceID(alert Alert) string {
	if alert.V2Log != nil && alert.V2Log.Trace != nil {
		traceID := strings.TrimSpace(alert.V2Log.Trace.TraceID)
		if security.ValidateTraceID(traceID) {
			return traceID
		}
	}
	for _, traceID := range alert.Traces {
		traceID = strings.TrimSpace(traceID)
		if security.ValidateTraceID(traceID) {
			return traceID
		}
	}
	return ""
}

func alertButtonService(alert Alert) string {
	if strings.EqualFold(strings.TrimSpace(alert.Service), "cloudwatch") {
		return ""
	}
	service, ok := security.NormalizeService(alert.Service)
	if !ok || !isServiceOpsNameConnected(service) {
		return ""
	}
	return service
}

func alertButtonServiceLabel(service string) string {
	if service == "post" {
		return "blog"
	}
	return service
}

func (s *Service) alertRoute(alert Alert) alertRoute {
	if isCriticalAlert(alert) {
		return alertRoute{
			Kind:      alertTargetCritical,
			ChannelID: s.criticalAlertChannelID(),
			RoleID:    s.alertMentionRoleID(),
			Mention:   shouldMentionAlert(alert),
		}
	}
	return alertRoute{
		Kind:      alertTargetGeneral,
		ChannelID: s.generalAlertChannelID(),
	}
}

func (s *Service) alertMentionRoleID() string {
	if roleID := strings.TrimSpace(s.store.Snapshot().ServiceAlerts.RoleID); roleID != "" {
		if validRoleID(roleID) {
			return roleID
		}
		return ""
	}
	if len(s.cfg.DiscordAllowedRoleIDs) == 0 {
		return ""
	}
	roleID := strings.TrimSpace(s.cfg.DiscordAllowedRoleIDs[0])
	if !validRoleID(roleID) {
		return ""
	}
	return roleID
}

func (s *Service) ConfigureAlert(ctx context.Context, channelID, action, roleID, target string) (string, error) {
	action = strings.TrimSpace(action)
	target, err := normalizeAlertTarget(target)
	if err != nil {
		return "", err
	}
	switch action {
	case "channel":
		if strings.TrimSpace(channelID) == "" {
			return "", fmt.Errorf("channel id is required")
		}
		if err := s.store.Update(func(data *state.Data) {
			channelID = strings.TrimSpace(channelID)
			switch target {
			case alertTargetAll:
				data.ServiceAlerts.ChannelID = channelID
				data.ServiceAlerts.GeneralChannelID = channelID
				data.ServiceAlerts.CriticalChannelID = channelID
			case alertTargetGeneral:
				data.ServiceAlerts.GeneralChannelID = channelID
			case alertTargetCritical:
				data.ServiceAlerts.CriticalChannelID = channelID
			}
		}); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ 서비스 알림 채널을 %s route로 설정했습니다.", target), nil
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
		routeChannelID := s.alertTestChannelID(target)
		if routeChannelID == "" {
			routeChannelID = strings.TrimSpace(channelID)
		}
		if routeChannelID == "" {
			return "", fmt.Errorf("alert channel이 설정되지 않았습니다. 먼저 /ops alert action:channel 을 실행하세요")
		}
		content := fmt.Sprintf("✅ 서비스 알림 테스트\n\ntarget: %s\nrole mention은 CRITICAL alert에서만 적용되며 test에서는 전송하지 않습니다.", target)
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, routeChannelID, content); err != nil {
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
	generalChannelID := s.generalAlertChannelID()
	criticalChannelID := s.criticalAlertChannelID()
	legacyChannelID := strings.TrimSpace(snapshot.ServiceAlerts.ChannelID)
	roleID := strings.TrimSpace(snapshot.ServiceAlerts.RoleID)
	effectiveRoleID := s.alertMentionRoleID()
	cooldown := s.alertCooldown(snapshot)
	var b strings.Builder
	b.WriteString("🔔 Service Alert Status\n\n")
	fmt.Fprintf(&b, "enabled: %t\n", enabled)
	if generalChannelID == "" {
		b.WriteString("general channel: NOT_CONFIGURED\n")
	} else {
		fmt.Fprintf(&b, "general channel: <#%s>\n", generalChannelID)
	}
	if criticalChannelID == "" {
		b.WriteString("critical channel: NOT_CONFIGURED\n")
	} else {
		fmt.Fprintf(&b, "critical channel: <#%s>\n", criticalChannelID)
	}
	if legacyChannelID != "" {
		fmt.Fprintf(&b, "legacy fallback channel: <#%s>\n", legacyChannelID)
	}
	if roleID == "" {
		if effectiveRoleID == "" {
			b.WriteString("role mention: none\n")
		} else {
			fmt.Fprintf(&b, "role mention: <@&%s> (fallback)\n", effectiveRoleID)
		}
	} else if !validRoleID(roleID) {
		b.WriteString("role mention: INVALID_CONFIGURED_ROLE\n")
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

func (s *Service) generalAlertChannelID() string {
	snapshot := s.store.Snapshot()
	return strings.TrimSpace(firstNonEmpty(snapshot.ServiceAlerts.GeneralChannelID, snapshot.ServiceAlerts.ChannelID, s.cfg.Alert.ChannelID, s.cfg.Dashboard.ChannelID))
}

func (s *Service) criticalAlertChannelID() string {
	snapshot := s.store.Snapshot()
	return strings.TrimSpace(firstNonEmpty(snapshot.ServiceAlerts.CriticalChannelID, snapshot.ServiceAlerts.ChannelID, s.cfg.Alert.ChannelID))
}

func (s *Service) alertTestChannelID(target string) string {
	switch target {
	case alertTargetCritical:
		return s.criticalAlertChannelID()
	default:
		return s.generalAlertChannelID()
	}
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

func normalizeAlertTarget(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return alertTargetAll, nil
	}
	switch value {
	case alertTargetAll, alertTargetGeneral, alertTargetCritical:
		return value, nil
	default:
		return "", fmt.Errorf("지원하지 않는 alert target입니다")
	}
}

func serviceAlertIncidentKey(alert Alert) string {
	parts := []string{
		strings.TrimSpace(alert.Service),
		strings.TrimSpace(alert.Severity),
		strings.TrimSpace(alert.AlertType),
		strings.TrimSpace(alertReasonText(alert)),
		strings.TrimSpace(alert.Path),
		strings.TrimSpace(alert.ErrorCode),
	}
	for i, part := range parts {
		parts[i] = strings.ToLower(part)
	}
	key := strings.Trim(strings.Join(parts, "\x00"), "\x00")
	if key == "" {
		return strings.TrimSpace(alert.Fingerprint)
	}
	return key
}

func serviceAlertTraceIDs(alert Alert) []string {
	seen := map[string]struct{}{}
	traceIDs := make([]string, 0, len(alert.Traces))
	for _, trace := range alert.Traces {
		trace = strings.TrimSpace(trace)
		if trace == "" || !security.ValidateTraceID(trace) {
			continue
		}
		if _, ok := seen[trace]; ok {
			continue
		}
		seen[trace] = struct{}{}
		traceIDs = append(traceIDs, trace)
	}
	return traceIDs
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
	var fallbacks []string
	if alert.Service == "cloudwatch" {
		fallbacks = append(fallbacks, "/ops dashboard action:status")
	} else if service := alertButtonService(alert); service != "" {
		fallbacks = append(fallbacks, "/ops logs service:"+alertButtonServiceLabel(service)+" mode:errors since:30m limit:10")
	}
	traceID := alertTraceID(alert)
	if traceID != "" {
		fallbacks = append([]string{"/ops logs mode:trace query:" + traceID}, fallbacks...)
	}
	if len(fallbacks) > 0 {
		b.WriteString("\nNext\n")
		if traceID != "" || alertButtonService(alert) != "" {
			b.WriteString("버튼으로 상세 로그를 확인하세요.\n\n")
		} else {
			b.WriteString("fallback 명령으로 상태를 확인하세요.\n\n")
		}
		b.WriteString("fallback:\n")
		b.WriteString(strings.Join(fallbacks, "\n"))
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
	return isCriticalAlert(alert)
}

func isCriticalAlert(alert Alert) bool {
	if strings.EqualFold(alert.Severity, opslog.SeverityCrit) || alert.Severity == "P0" {
		return true
	}
	if alert.V2Log != nil {
		return strings.EqualFold(alert.V2Log.Level, opslog.SeverityCrit)
	}
	if alert.AlertType == "v2-log" {
		return strings.EqualFold(alert.V2Decision.Severity, opslog.SeverityCrit)
	}
	return false
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
