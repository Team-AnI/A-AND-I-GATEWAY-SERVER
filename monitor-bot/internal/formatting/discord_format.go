package formatting

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

const DiscordMessageLimit = 2000

type ServiceStatus struct {
	Service string
	State   string
	Detail  string
}

type DashboardServiceInput struct {
	Service     string
	DisplayName string
	Health      ServiceStatus
	LogStatus   string
	Rows        []map[string]string
	Alarm       bool
}

type ServiceDetailInput struct {
	Service   string
	LogGroup  string
	Since     string
	Health    ServiceStatus
	CountRows []map[string]string
	TopRows   []map[string]string
	ErrorRows []map[string]string
}

type LogSummary struct {
	Total    int
	API      int
	APIError int
	Warn     int
	Error    int
	FourXX   int
	FiveXX   int
	P95      int
	LastLog  string
}

type LogGroupRetention struct {
	Name          string
	RetentionDays *int32
	StoredBytes   int64
}

func TruncateDiscordMessage(message string) string {
	const suffix = "\n...(truncated)"
	if len([]rune(message)) <= DiscordMessageLimit {
		return message
	}
	runes := []rune(message)
	return string(runes[:DiscordMessageLimit-len([]rune(suffix))]) + suffix
}

func FormatStatus(statuses []ServiceStatus) string {
	var b strings.Builder
	b.WriteString("A&I 서비스 상태\n")
	for _, status := range statuses {
		icon := "🟡"
		switch strings.ToUpper(status.State) {
		case "UP":
			icon = "🟢"
		case "DOWN":
			icon = "🔴"
		}
		detail := strings.TrimSpace(security.SanitizeText(status.Detail))
		if detail == "" {
			detail = status.State
		}
		fmt.Fprintf(&b, "%s `%s` %s - %s\n", icon, status.Service, strings.ToUpper(status.State), detail)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatLogRows(title string, rows []map[string]string) string {
	if len(rows) == 0 {
		return title + "\n조회 결과가 없습니다."
	}
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for i, row := range rows {
		fmt.Fprintf(&b, "\n%d. ", i+1)
		writeCompactRow(&b, row)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatErrors(rows []map[string]string) string {
	return FormatLogRows("상위 에러", rows)
}

func FormatTrace(rows []map[string]string) string {
	return FormatLogRows("Trace 조회 결과", rows)
}

func FormatAlarms(names []string) string {
	if len(names) == 0 {
		return "현재 ALARM 상태 없음"
	}
	sort.Strings(names)
	return TruncateDiscordMessage("ALARM 상태 알람\n- " + strings.Join(names, "\n- "))
}

func FormatRetention(title string, groups []LogGroupRetention) string {
	if len(groups) == 0 {
		return title + "\n조회 결과가 없습니다."
	}
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, group := range groups {
		retention := "INFINITE"
		if group.RetentionDays != nil {
			retention = fmt.Sprintf("%dd", *group.RetentionDays)
		}
		fmt.Fprintf(&b, "%-35s %-8s %s\n", security.SanitizeText(group.Name), retention, humanBytes(group.StoredBytes))
	}
	return TruncateDiscordMessage(b.String())
}

func FormatDashboard(since string, services []DashboardServiceInput, alarmNames []string) string {
	return FormatDashboardWithMeta(since, services, alarmNames, time.Time{}, 0)
}

func FormatDashboardWithMeta(since string, services []DashboardServiceInput, alarmNames []string, updatedAt time.Time, nextRefresh time.Duration) string {
	var b strings.Builder
	statusIcon := "🟢"
	topIssue := "none"
	for _, service := range services {
		summary := SummarizeRows(service.Rows)
		if service.Alarm || strings.EqualFold(service.Health.State, "DOWN") || summary.FiveXX > 0 || service.LogStatus == "LOG_QUERY_FAILED" {
			statusIcon = "🔴"
			if topIssue == "none" {
				topIssue = fmt.Sprintf("%s 5xx x%d", service.Service, summary.FiveXX)
			}
			break
		}
		if strings.EqualFold(service.Health.State, "UNKNOWN") || summary.Error > 0 || summary.P95 >= 1000 || service.LogStatus == "NO_LOGS" || service.LogStatus == "NOT_CONFIGURED" {
			statusIcon = "🟡"
		}
	}
	fmt.Fprintf(&b, "%s A&I Service Dashboard - last %s\n\n", statusIcon, since)
	if !updatedAt.IsZero() {
		fmt.Fprintf(&b, "Last updated: %s KST\n", updatedAt.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04:05"))
	}
	if nextRefresh > 0 {
		fmt.Fprintf(&b, "Next refresh: %s\n", formatDurationCompact(nextRefresh))
	}
	if !updatedAt.IsZero() || nextRefresh > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("```txt\n")
	b.WriteString(dashboardTableHeader())
	for _, service := range services {
		summary := SummarizeRows(service.Rows)
		state := strings.ToUpper(service.Health.State)
		if state == "" {
			state = "UNKNOWN"
		}
		logStatus := strings.ToUpper(strings.TrimSpace(service.LogStatus))
		if logStatus == "" {
			if len(service.Rows) == 0 {
				logStatus = "NO_LOGS"
			} else {
				logStatus = "OK"
			}
		}
		icon := dashboardIcon(state, logStatus, summary, service.Alarm)
		fmt.Fprintf(
			&b,
			"%-9s %-7s %-6s %4s %4s %4s %-5s\n",
			dashboardServiceName(service.Service, service.DisplayName),
			icon+" "+dashboardShortStatus(state),
			formatLogStatusShort(logStatus),
			dashboardNumber(summary.FourXX, logStatus),
			dashboardNumber(summary.FiveXX, logStatus),
			dashboardNumber(summary.Error, logStatus),
			dashboardLastLogShort(summary.LastLog, logStatus),
		)
	}
	b.WriteString("```")
	b.WriteString("\nAlarms: ")
	if len(alarmNames) == 0 {
		b.WriteString("none")
	} else {
		b.WriteString(strings.Join(alarmNames, ", "))
	}
	b.WriteString("\nTop issue: " + topIssue)
	b.WriteString("\n\nNext: `/ops logs service:report mode:errors since:" + since + "` 또는 `/ops service service:report view:copy since:" + since + "`")
	return TruncateDiscordMessage(b.String())
}

func dashboardTableHeader() string {
	return fmt.Sprintf("%-9s %-7s %-6s %4s %4s %4s %-5s\n", "Service", "Health", "Logs", "4xx", "5xx", "Err", "Last")
}

func FormatServiceDetail(input ServiceDetailInput) string {
	summary := SummarizeRows(input.CountRows)
	var b strings.Builder
	icon := serviceHealthIcon(strings.ToUpper(input.Health.State), summary, false)
	fmt.Fprintf(&b, "%s %s detail - last %s\n\n", icon, input.Service, input.Since)
	fmt.Fprintf(&b, "Health: %s\n", strings.ToUpper(input.Health.State))
	fmt.Fprintf(&b, "Log group: %s\n", input.LogGroup)
	fmt.Fprintf(&b, "Last log: %s\n", formatLastLog(summary.LastLog))
	fmt.Fprintf(&b, "Total requests: %d\n", summary.Total)
	fmt.Fprintf(&b, "API_ERROR: %d\n", summary.APIError)
	fmt.Fprintf(&b, "4xx/5xx: %d/%d\n", summary.FourXX, summary.FiveXX)
	fmt.Fprintf(&b, "p95 latency: %s\n\n", formatLatency(summary.P95))
	b.WriteString("Top paths:\n")
	writeTopRows(&b, input.TopRows)
	b.WriteString("\nRecent error summary:\n")
	writeTopRows(&b, input.ErrorRows)
	fmt.Fprintf(&b, "\nNext:\n/ops logs service:%s mode:errors since:%s", input.Service, input.Since)
	return TruncateDiscordMessage(b.String())
}

func FormatCountSummary(service, since, countType string, rows []map[string]string) string {
	summary := SummarizeRows(rows)
	var b strings.Builder
	fmt.Fprintf(&b, "📊 %s log count - last %s\n\n", service, since)
	fmt.Fprintf(&b, "type: %s\n", countType)
	fmt.Fprintf(&b, "total: %d\n", summary.Total)
	fmt.Fprintf(&b, "API: %d\n", summary.API)
	fmt.Fprintf(&b, "API_ERROR: %d\n", summary.APIError)
	fmt.Fprintf(&b, "WARN: %d\n", summary.Warn)
	fmt.Fprintf(&b, "ERROR: %d\n", summary.Error)
	fmt.Fprintf(&b, "4xx: %d\n", summary.FourXX)
	fmt.Fprintf(&b, "5xx: %d", summary.FiveXX)
	return TruncateDiscordMessage(b.String())
}

func FormatTopSummary(service, since, by string, rows []map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔥 Top %s %s - last %s\n\n", service, by, since)
	writeTopRows(&b, rows)
	return TruncateDiscordMessage(b.String())
}

func FormatSlowSummary(service, since string, rows []map[string]string) string {
	if len(rows) == 0 {
		return fmt.Sprintf("🐢 Slow %s APIs - last %s\n\n조회 결과가 없습니다.", service, since)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🐢 Slow %s APIs - last %s\n\n", service, since)
	for i, row := range rows {
		if i >= 20 {
			break
		}
		fmt.Fprintf(
			&b,
			"%d. %s %s\n   latency=%sms status=%s trace=%s\n",
			i+1,
			value(row, "http.method", "-"),
			redactPath(value(row, "http.path", "-")),
			value(row, "http.latencyMs", "-"),
			value(row, "http.statusCode", "-"),
			value(row, "trace.traceId", "-"),
		)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatCopyStatus(since string, rows []map[string]string) string {
	summary := SummarizeRows(rows)
	success := 0
	duplicate := 0
	var failed []string
	for _, row := range rows {
		status := parseInt(row["http.statusCode"])
		if status >= 200 && status < 400 {
			success++
			continue
		}
		if status == 409 {
			duplicate++
		}
		if len(failed) < 5 && status >= 400 {
			failed = append(failed, fmt.Sprintf("- trace=%s status=%d code=%s", value(row, "trace.traceId", "-"), status, value(row, "response.error.code", "-")))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📦 Assignment Copy API - last %s\n\n", since)
	fmt.Fprintf(&b, "total: %d\n", summary.Total)
	fmt.Fprintf(&b, "success: %d\n", success)
	fmt.Fprintf(&b, "fail: %d\n", summary.FourXX+summary.FiveXX)
	fmt.Fprintf(&b, "409 duplicate: %d\n", duplicate)
	fmt.Fprintf(&b, "5xx: %d\n", summary.FiveXX)
	fmt.Fprintf(&b, "p95 latency: %s\n\n", formatLatency(summary.P95))
	b.WriteString("Recent failed traces:\n")
	if len(failed) == 0 {
		b.WriteString("- none")
	} else {
		b.WriteString(strings.Join(failed, "\n"))
	}
	return TruncateDiscordMessage(b.String())
}

func SummarizeRows(rows []map[string]string) LogSummary {
	var summary LogSummary
	for _, row := range rows {
		count := parseInt(row["count"])
		if count <= 0 {
			count = 1
		}
		summary.Total += count
		logType := strings.ToUpper(strings.TrimSpace(row["logType"]))
		level := strings.ToUpper(strings.TrimSpace(row["level"]))
		statusCode := parseInt(row["http.statusCode"])
		if logType == "API" {
			summary.API += count
		}
		if logType == "API_ERROR" {
			summary.APIError += count
		}
		if level == "WARN" {
			summary.Warn += count
		}
		if level == "ERROR" {
			summary.Error += count
		}
		if statusCode >= 400 && statusCode < 500 {
			summary.FourXX += count
		}
		if statusCode >= 500 {
			summary.FiveXX += count
		}
		for _, key := range []string{"p95", "maxLatency", "http.latencyMs"} {
			if latency := parseFloatInt(row[key]); latency > summary.P95 {
				summary.P95 = latency
			}
		}
		if candidate := firstNonEmpty(row["lastLog"], row["@timestamp"]); newerTimestamp(candidate, summary.LastLog) {
			summary.LastLog = candidate
		}
	}
	return summary
}

func HelpText() string {
	return strings.TrimSpace(`A&I Ops Commands

/ops dashboard
/ops service service:report
/ops logs service:report mode:errors
/ops trace trace_id:<traceId>
/ops alarms
/ops storage view:usage
/ops help

Drilldown flow: dashboard -> service -> logs -> trace
Legacy commands still work as aliases during Phase 1.`)
}

func writeCompactRow(b *strings.Builder, row map[string]string) {
	pairs := security.FilterDisplayPairs(row)
	if len(pairs) == 0 {
		b.WriteString("표시 가능한 필드 없음")
		return
	}
	for i, pair := range pairs {
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(b, "%s=%s", pair[0], pair[1])
	}
}

func writeTopRows(b *strings.Builder, rows []map[string]string) {
	if len(rows) == 0 {
		b.WriteString("- none\n")
		return
	}
	for i, row := range rows {
		if i >= 10 {
			break
		}
		count := value(row, "count", "1")
		path := redactPath(value(row, "http.path", ""))
		status := value(row, "http.statusCode", "")
		code := value(row, "response.error.code", "")
		message := security.SanitizeText(value(row, "response.error.message", ""))
		if path == "" {
			path = "status=" + status
		}
		fmt.Fprintf(b, "%d. %s", i+1, path)
		if status != "" {
			fmt.Fprintf(b, " status=%s", status)
		}
		if code != "" {
			fmt.Fprintf(b, " code=%s", code)
		}
		fmt.Fprintf(b, " count=%s", count)
		if message != "" {
			fmt.Fprintf(b, "\n   message=%s", message)
		}
		b.WriteByte('\n')
	}
}

func serviceHealthIcon(state string, summary LogSummary, alarm bool) string {
	switch {
	case alarm || state == "DOWN" || summary.FiveXX > 0:
		return "🔴"
	case state == "UNKNOWN" || state == "" || summary.Error > 0 || summary.P95 >= 1000 || summary.LastLog == "":
		return "🟡"
	case summary.Total == 0:
		return "⚪"
	default:
		return "🟢"
	}
}

func dashboardIcon(healthState, logStatus string, summary LogSummary, alarm bool) string {
	switch {
	case logStatus == "NOT_CONFIGURED":
		return "⚫"
	case alarm || healthState == "DOWN" || logStatus == "LOG_QUERY_FAILED" || summary.FiveXX > 0:
		return "🔴"
	case logStatus == "NO_LOGS":
		return "⚪"
	case healthState == "UNKNOWN" || healthState == "" || summary.Error > 0 || summary.P95 >= 1000:
		return "🟡"
	default:
		return "🟢"
	}
}

func formatLogStatusShort(status string) string {
	switch status {
	case "OK":
		return "OK"
	case "NO_LOGS":
		return "NOLOG"
	case "NOT_CONFIGURED":
		return "NOCFG"
	case "LOG_QUERY_FAILED":
		return "QFAIL"
	default:
		return "UNK"
	}
}

func dashboardShortStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP":
		return "UP"
	case "DOWN":
		return "DOWN"
	case "UNKNOWN":
		return "UNK"
	case "NO_LOGS":
		return "NOLOG"
	case "NOT_CONFIGURED":
		return "NOCFG"
	case "LOG_QUERY_FAILED":
		return "QFAIL"
	default:
		return "UNK"
	}
}

func dashboardServiceName(service, displayName string) string {
	normalized := strings.ToLower(strings.TrimSpace(firstNonEmpty(service, displayName)))
	switch normalized {
	case "online-judge":
		return "judge"
	case "gateway", "auth", "report", "post":
		return normalized
	default:
		name := strings.ToLower(strings.TrimSpace(firstNonEmpty(displayName, service)))
		if name == "online-judge" {
			return "judge"
		}
		if len([]rune(name)) > 9 {
			return string([]rune(name)[:9])
		}
		return security.SanitizeText(name)
	}
}

func dashboardNumber(value int, logStatus string) string {
	if logStatus == "NOT_CONFIGURED" || logStatus == "LOG_QUERY_FAILED" {
		return "-"
	}
	return strconv.Itoa(value)
}

func dashboardLastLogShort(value, logStatus string) string {
	if logStatus == "NOT_CONFIGURED" || logStatus == "NO_LOGS" || logStatus == "LOG_QUERY_FAILED" {
		return "-"
	}
	return formatLastLogCompact(value)
}

func formatLatency(value int) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dms", value)
}

func formatDurationCompact(value time.Duration) string {
	if value%time.Hour == 0 && value >= time.Hour {
		return fmt.Sprintf("%dh", int(value.Hours()))
	}
	if value%time.Minute == 0 && value >= time.Minute {
		return fmt.Sprintf("%dm", int(value.Minutes()))
	}
	return fmt.Sprintf("%ds", int(value.Seconds()))
}

func humanBytes(bytes int64) string {
	if bytes < 0 {
		bytes = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", bytes, units[unit])
	}
	if value >= 10 {
		return fmt.Sprintf("%.0f%s", value, units[unit])
	}
	return fmt.Sprintf("%.1f%s", value, units[unit])
}

func formatLastLog(value string) string {
	if strings.TrimSpace(value) == "" {
		return "no data"
	}
	parsed, ok := parseTimestamp(value)
	if !ok {
		return security.SanitizeText(value)
	}
	ago := time.Since(parsed)
	switch {
	case ago < time.Minute:
		return fmt.Sprintf("%ds ago", int(ago.Seconds()))
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(ago.Hours()))
	}
}

func formatLastLogCompact(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	parsed, ok := parseTimestamp(value)
	if !ok {
		return security.SanitizeText(value)
	}
	ago := time.Since(parsed)
	switch {
	case ago < time.Minute:
		return fmt.Sprintf("%ds", int(ago.Seconds()))
	case ago < time.Hour:
		return fmt.Sprintf("%dm", int(ago.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(ago.Hours()))
	}
}

func parseTimestamp(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.000"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	if unixFloat, err := strconv.ParseFloat(trimmed, 64); err == nil && unixFloat > 0 {
		seconds, fraction := math.Modf(unixFloat)
		return time.Unix(int64(seconds), int64(fraction*1e9)), true
	}
	return time.Time{}, false
}

func newerTimestamp(candidate, current string) bool {
	if strings.TrimSpace(candidate) == "" {
		return false
	}
	candidateTime, candidateOK := parseTimestamp(candidate)
	currentTime, currentOK := parseTimestamp(current)
	if candidateOK && currentOK {
		return candidateTime.After(currentTime)
	}
	if candidateOK && !currentOK {
		return true
	}
	return current == "" || candidate > current
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseFloatInt(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return int(math.Round(parsed))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func value(row map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(security.SanitizeText(row[key])); value != "" {
		return value
	}
	return fallback
}

var coursePathPattern = regexp.MustCompile(`/v2/admin/courses/[^/]+/assignments/copy`)

func redactPath(path string) string {
	return coursePathPattern.ReplaceAllString(security.SanitizeText(path), "/v2/admin/courses/*/assignments/copy")
}
