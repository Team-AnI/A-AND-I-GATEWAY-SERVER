package formatting

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
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

type AdminAssignmentsAllSummary struct {
	CourseSlug string
	Total      int
	Published  int
	Scheduled  int
	Draft      int
	Shown      []reportadmin.Assignment
	Error      string
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
	return FormatDashboardWithMetaAndAlerts(since, services, alarmNames, updatedAt, nextRefresh, nil)
}

func FormatDashboardWithMetaAndAlerts(since string, services []DashboardServiceInput, alarmNames []string, updatedAt time.Time, nextRefresh time.Duration, recentAlerts []string) string {
	var b strings.Builder
	statusIcon := "🟢"
	overall := "정상"
	topIssue := "none"
	for _, service := range services {
		if isUnconnectedDashboardInput(service) {
			continue
		}
		summary := SummarizeRows(service.Rows)
		logStatus := strings.ToUpper(strings.TrimSpace(service.LogStatus))
		if service.Alarm || strings.EqualFold(service.Health.State, "DOWN") || summary.FiveXX > 0 || isDashboardLogFailure(logStatus) {
			statusIcon = "🔴"
			overall = "장애"
			topIssue = dashboardServiceTopIssue(service, summary, logStatus)
			break
		}
		if strings.EqualFold(service.Health.State, "UNKNOWN") || strings.EqualFold(service.Health.State, "NOT_CONNECTED") || summary.Error > 0 || summary.P95 >= 1000 || service.LogStatus == "NOT_CONFIGURED" || service.LogStatus == "NOT_CONNECTED" {
			statusIcon = "🟡"
			overall = "주의"
			if topIssue == "none" {
				topIssue = dashboardServiceTopIssue(service, summary, logStatus)
			}
		}
	}
	fmt.Fprintf(&b, "📌 A&I 서비스 운영 대시보드\n\n")
	if !updatedAt.IsZero() {
		fmt.Fprintf(&b, "마지막 업데이트: %s KST\n", updatedAt.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(&b, "전체 상태: %s %s\n", statusIcon, overall)
	if nextRefresh > 0 {
		fmt.Fprintf(&b, "업데이트 주기: %s\n", formatDurationCompact(nextRefresh))
	}
	fmt.Fprintf(&b, "조회 범위: 최근 %s\n\n", since)
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
				logStatus = "NO_V2_LOG"
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
	b.WriteString("\nCloudWatch 알람: ")
	if len(alarmNames) == 0 {
		b.WriteString("none")
	} else {
		b.WriteString(strings.Join(alarmNames, ", "))
	}
	if topIssue == "none" && len(recentAlerts) > 0 {
		topIssue = dashboardRecentTopIssue(recentAlerts[0])
	}
	b.WriteString("\nTop issue: " + topIssue)
	b.WriteString("\n\n최근 장애 알림\n")
	if len(recentAlerts) == 0 {
		b.WriteString("1. 없음\n")
	} else {
		for i, alert := range recentAlerts {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "%d. %s\n", i+1, security.SanitizeText(alert))
		}
	}
	b.WriteString("\n상세 확인\n")
	b.WriteString("/ops logs service:report mode:errors since:" + since + " limit:10\n")
	b.WriteString("/ops logs service:blog mode:slow since:" + since + " limit:10\n")
	b.WriteString("/ops logs mode:trace query:<traceId>")
	return TruncateDiscordMessage(b.String())
}

func isUnconnectedDashboardInput(service DashboardServiceInput) bool {
	return strings.Contains(strings.ToLower(service.Health.Detail), "not connected") && (strings.EqualFold(service.LogStatus, "NO_LOGS") || strings.EqualFold(service.LogStatus, "NO_V2_LOG"))
}

func dashboardServiceTopIssue(service DashboardServiceInput, summary LogSummary, logStatus string) string {
	name := dashboardServiceName(service.Service, service.DisplayName)
	switch {
	case service.Alarm:
		return name + " CloudWatch alarm"
	case strings.EqualFold(service.Health.State, "DOWN"):
		return name + " health DOWN"
	case summary.FiveXX > 0:
		return fmt.Sprintf("%s 5xx x%d", name, summary.FiveXX)
	case summary.Error > 0:
		return fmt.Sprintf("%s error x%d", name, summary.Error)
	case summary.P95 >= 1000:
		return fmt.Sprintf("%s p95 %dms", name, summary.P95)
	case isDashboardLogFailure(logStatus):
		return name + " logs " + strings.ToUpper(strings.TrimSpace(logStatus))
	case strings.EqualFold(service.Health.State, "UNKNOWN"), strings.EqualFold(service.Health.State, "NOT_CONNECTED"):
		return name + " health " + strings.ToUpper(strings.TrimSpace(service.Health.State))
	case strings.EqualFold(service.LogStatus, "NOT_CONFIGURED"), strings.EqualFold(service.LogStatus, "NOT_CONNECTED"):
		return name + " logs " + strings.ToUpper(strings.TrimSpace(service.LogStatus))
	default:
		return name + " warning"
	}
}

func dashboardRecentTopIssue(recent string) string {
	line := strings.TrimSpace(strings.SplitN(recent, "\n", 2)[0])
	if line == "" {
		return "recent alert"
	}
	return security.SanitizeText(line)
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
	return FormatTopSummaryWithLimit(service, since, by, rows, 10)
}

func FormatTopSummaryWithLimit(service, since, by string, rows []map[string]string, limit int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔥 Top %s %s - last %s\n\n", service, by, since)
	writeTopRowsWithLimit(&b, rows, limit)
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
			redactPath(firstNonEmpty(value(row, "http.route", ""), value(row, "http.path", "-"))),
			value(row, "http.latencyMs", "-"),
			value(row, "http.statusCode", "-"),
			value(row, "trace.traceId", "-"),
		)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatAssignmentsSummary(since string, rows []map[string]string, forcedStatus, forcedFinding string) string {
	status := strings.TrimSpace(forcedStatus)
	finding := strings.TrimSpace(forcedFinding)
	if status == "" {
		switch {
		case len(rows) == 0:
			status = "NO_DATA"
			finding = "과제 관련 로그 없음"
		case SummarizeRows(rows).FiveXX > 0 || SummarizeRows(rows).Error > 0:
			status = "ERROR"
			finding = "과제 관련 ERROR 또는 5xx 로그 확인"
		case SummarizeRows(rows).FourXX > 0 || SummarizeRows(rows).Warn > 0 || SummarizeRows(rows).APIError > 0:
			status = "WARN"
			finding = "과제 관련 WARN/API_ERROR 로그 확인"
		default:
			status = "OK"
			finding = "과제 관련 주요 오류 없음"
		}
	}
	if finding == "" {
		finding = "과제 관련 이벤트 요약"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", status)
	fmt.Fprintf(&b, "service: report\n")
	fmt.Fprintf(&b, "window: %s\n", since)
	fmt.Fprintf(&b, "key findings: %s\n\n", security.SanitizeText(finding))
	if len(rows) > 0 {
		b.WriteString("top assignment events:\n")
		writeTopRowsWithLimit(&b, rows, 10)
	}
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`\n")
	b.WriteString("- `/ops logs service:report mode:slow since:30m limit:10`\n")
	b.WriteString("- `/ops logs mode:trace query:<traceId>`")
	return TruncateDiscordMessage(b.String())
}

func FormatAssignmentDetail(assignmentID string, rows []map[string]string, forcedStatus, forcedFinding string) string {
	status := strings.TrimSpace(forcedStatus)
	finding := strings.TrimSpace(forcedFinding)
	if status == "" {
		if len(rows) == 0 {
			status = "NO_DATA"
			finding = "no matching records"
		} else {
			summary := SummarizeRows(rows)
			switch {
			case summary.FiveXX > 0 || summary.Error > 0:
				status = "ERROR"
			case summary.FourXX > 0 || summary.Warn > 0 || summary.APIError > 0:
				status = "WARN"
			default:
				status = "OK"
			}
			finding = "assignmentId 관련 로그 확인"
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", status)
	fmt.Fprintf(&b, "service: report\n")
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(assignmentID))
	fmt.Fprintf(&b, "key findings: %s\n\n", security.SanitizeText(finding))
	if len(rows) > 0 {
		b.WriteString("recent records:\n")
		for i, row := range rows {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "%d. ", i+1)
			writeCompactRow(&b, row)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`\n")
	b.WriteString("- `/ops logs mode:trace query:<traceId>`")
	return TruncateDiscordMessage(b.String())
}

func FormatAdminAssignments(courseSlug, statusFilter string, assignments []reportadmin.Assignment) string {
	statusCounts := countAssignmentStatuses(assignments)
	status := "OK"
	finding := fmt.Sprintf("%d assignments found", len(assignments))
	if len(assignments) == 0 {
		status = "NO_DATA"
		finding = "matching assignments 없음"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", status)
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "filter: %s\n", security.SanitizeText(firstNonEmpty(statusFilter, "all")))
	fmt.Fprintf(&b, "key findings: %s\n", finding)
	fmt.Fprintf(&b, "%d published, %d scheduled, %d draft\n\n", statusCounts["published"], statusCounts["scheduled"], statusCounts["draft"])
	if len(assignments) > 0 {
		b.WriteString("assignments:\n")
		for i, assignment := range assignments {
			if i >= 10 {
				fmt.Fprintf(&b, "- ... %d more\n", len(assignments)-i)
				break
			}
			fmt.Fprintf(&b, "- id=%s status=%s problem=%s start=%s end=%s updated=%s\n",
				shortValue(assignment.ID),
				shortValue(assignment.Status),
				shortValue(assignment.ProblemID),
				shortValue(assignment.StartAt),
				shortValue(assignment.EndAt),
				shortValue(assignment.UpdatedAt),
			)
		}
	}
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:<assignmentId>`\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:<assignmentId> action:submissions`\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`")
	return TruncateDiscordMessage(b.String())
}

func FormatAdminAssignmentsAll(window string, summaries []AdminAssignmentsAllSummary) string {
	status := "OK"
	finding := fmt.Sprintf("%d courses checked", len(summaries))
	if len(summaries) == 0 {
		status = "NO_DATA"
		finding = "course 목록 또는 과제 목록 없음"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", status)
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "window: %s\n", security.SanitizeText(window))
	fmt.Fprintf(&b, "key findings: %s\n\n", finding)
	for i, summary := range summaries {
		if i >= 8 {
			fmt.Fprintf(&b, "- ... %d more courses\n", len(summaries)-i)
			break
		}
		fmt.Fprintf(&b, "- %s total=%d published=%d scheduled=%d draft=%d", security.SanitizeText(summary.CourseSlug), summary.Total, summary.Published, summary.Scheduled, summary.Draft)
		if summary.Error != "" {
			fmt.Fprintf(&b, " status=%s", security.SanitizeText(summary.Error))
		}
		b.WriteByte('\n')
		for j, assignment := range summary.Shown {
			if j >= 3 {
				break
			}
			fmt.Fprintf(&b, "  - id=%s status=%s start=%s end=%s\n", shortValue(assignment.ID), shortValue(assignment.Status), shortValue(assignment.StartAt), shortValue(assignment.EndAt))
		}
	}
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops assignment course:<courseSlug> action:list status:all`\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`")
	return TruncateDiscordMessage(b.String())
}

func FormatAdminAssignment(courseSlug string, assignment reportadmin.Assignment) string {
	status := "OK"
	if strings.TrimSpace(assignment.ID) == "" {
		status = "NO_DATA"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", status)
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", shortValue(assignment.ID))
	fmt.Fprintf(&b, "key findings: assignment detail 조회\n\n")
	fmt.Fprintf(&b, "title: %s\n", shortValue(assignment.Title))
	fmt.Fprintf(&b, "assignmentStatus: %s\n", shortValue(assignment.Status))
	fmt.Fprintf(&b, "publishedAt: %s\n", shortValue(assignment.PublishedAt))
	fmt.Fprintf(&b, "startAt: %s\n", shortValue(assignment.StartAt))
	fmt.Fprintf(&b, "endAt: %s\n", shortValue(assignment.EndAt))
	fmt.Fprintf(&b, "problemId: %s\n", shortValue(assignment.ProblemID))
	fmt.Fprintf(&b, "updatedAt: %s\n", shortValue(assignment.UpdatedAt))
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + " view:diagnosis` - 봇 판단 근거를 확인합니다.\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + " view:events` - 감지 이력과 반복 억제 상태를 확인합니다.\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + " action:check` - 운영 체크리스트를 확인합니다.\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + " action:submissions`")
	return TruncateDiscordMessage(b.String())
}

func FormatAdminAssignmentRaw(courseSlug string, assignment reportadmin.Assignment) string {
	var b strings.Builder
	b.WriteString("status: OK\n")
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", shortValue(assignment.ID))
	b.WriteString("raw sanitized fields used by bot:\n")
	fmt.Fprintf(&b, "- title: %s\n", shortValue(assignment.Title))
	fmt.Fprintf(&b, "- status: %s\n", shortValue(assignment.Status))
	fmt.Fprintf(&b, "- publishedAt: %s\n", shortValue(assignment.PublishedAt))
	fmt.Fprintf(&b, "- startAt: %s\n", shortValue(assignment.StartAt))
	fmt.Fprintf(&b, "- endAt: %s\n", shortValue(assignment.EndAt))
	fmt.Fprintf(&b, "- problemId: %s\n", shortValue(assignment.ProblemID))
	fmt.Fprintf(&b, "- updatedAt: %s\n", shortValue(assignment.UpdatedAt))
	return TruncateDiscordMessage(b.String())
}

func FormatAdminAssignmentCheck(courseSlug string, assignment reportadmin.Assignment, check reportadmin.AssignmentCheck, botIssueValues ...string) string {
	botIssue := "NONE"
	if len(botIssueValues) > 0 && strings.TrimSpace(botIssueValues[0]) != "" {
		botIssue = botIssueValues[0]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", check.Status)
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", shortValue(assignment.ID))
	b.WriteString("checks:\n")
	fmt.Fprintf(&b, "- title: %s\n", okMissing(assignment.Title))
	fmt.Fprintf(&b, "- status: %s\n", shortValue(assignment.Status))
	fmt.Fprintf(&b, "- publishedAt: %s\n", timeCheck(assignment.PublishedAt, false))
	fmt.Fprintf(&b, "- startAt: %s\n", timeCheck(assignment.StartAt, false))
	fmt.Fprintf(&b, "- endAt: %s\n", timeCheck(assignment.EndAt, true))
	fmt.Fprintf(&b, "- problemId: %s\n", okMissing(assignment.ProblemID))
	fmt.Fprintf(&b, "- botIssue: %s\n", shortValue(botIssue))
	b.WriteString("\nkey findings:\n")
	for _, finding := range check.Findings {
		fmt.Fprintf(&b, "- %s\n", security.SanitizeText(finding))
	}
	if strings.TrimSpace(assignment.ProblemID) == "" {
		b.WriteString("- problemId is missing, but this does not explain ASSIGNMENT_DRAFT_PAST_START by itself.\n")
	}
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + " view:diagnosis` - 필드별 판단 근거 확인\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + " view:events` - 봇 감지 이력 확인\n")
	b.WriteString("- `/ops logs service:report mode:events query:" + shortValue(assignment.ID) + " since:24h limit:20` - Report EVENT 로그 검색")
	return TruncateDiscordMessage(b.String())
}

func FormatAdminSubmissions(courseSlug, assignmentID string, summary reportadmin.SubmissionSummary) string {
	status := "OK"
	finding := "submission status summary"
	if summary.UnsupportedShape {
		status = "WARN"
		finding = "unsupported response shape; raw count만 확인 가능"
	}
	if summary.TotalStudents == 0 && summary.RawCount == 0 && !summary.UnsupportedShape {
		status = "NO_DATA"
		finding = "submission records 없음"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", status)
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(assignmentID))
	fmt.Fprintf(&b, "key findings: %s\n\n", finding)
	fmt.Fprintf(&b, "total students: %d\n", summary.TotalStudents)
	fmt.Fprintf(&b, "submitted: %d\n", summary.Submitted)
	fmt.Fprintf(&b, "not submitted: %d\n", summary.NotSubmitted)
	fmt.Fprintf(&b, "graded: %d\n", summary.Graded)
	fmt.Fprintf(&b, "pending: %d\n", summary.Pending)
	fmt.Fprintf(&b, "failed: %d\n", summary.Failed)
	fmt.Fprintf(&b, "average score: %s\n", shortValue(summary.AverageScore))
	fmt.Fprintf(&b, "highest score: %s\n", shortValue(summary.HighestScore))
	fmt.Fprintf(&b, "lowest score: %s\n", shortValue(summary.LowestScore))
	fmt.Fprintf(&b, "recent gradedAt: %s\n", shortValue(summary.RecentGradedAt))
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + security.SanitizeText(assignmentID) + "`\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`")
	return TruncateDiscordMessage(b.String())
}

func FormatAssignmentAuditRows(rows []map[string]string, query string) string {
	var b strings.Builder
	b.WriteString("Assignment audit events\n")
	if strings.TrimSpace(query) != "" {
		fmt.Fprintf(&b, "query: %s\n", security.SanitizeText(query))
	}
	if len(rows) == 0 {
		b.WriteString("\n조회 결과가 없습니다.")
		return TruncateDiscordMessage(b.String())
	}
	for i, row := range rows {
		if i >= 10 {
			break
		}
		eventType := value(row, "event.eventType", "unknown")
		assignmentID := firstNonEmpty(
			value(row, "event.assignmentId", ""),
			value(row, "event.resourceId", ""),
			value(row, "assignmentId", ""),
			value(row, "assignment.assignmentId", ""),
			value(row, "request.pathVariables.assignmentId", ""),
			"unknown",
		)
		course := firstNonEmpty(
			value(row, "event.courseSlug", ""),
			value(row, "assignment.courseSlug", ""),
			value(row, "courseSlug", ""),
			value(row, "request.pathVariables.courseSlug", ""),
			value(row, "request.pathVariables.course", ""),
			"unknown",
		)
		title := firstNonEmpty(value(row, "event.title", ""), value(row, "assignment.title", ""), "unknown")
		actorID := firstNonEmpty(value(row, "actor.userId", ""), value(row, "actor.id", ""), "unknown")
		actorName := firstNonEmpty(value(row, "actor.name", ""), value(row, "actor.displayName", ""), value(row, "actor.loginId", ""), "unknown")
		actorRole := value(row, "actor.role", "unknown")
		occurredAt := firstNonEmpty(value(row, "event.occurredAt", ""), value(row, "@timestamp", ""), "unknown")
		traceID := value(row, "trace.traceId", "unknown")
		fmt.Fprintf(&b, "\n%d. %s course=%s assignmentId=%s\n", i+1, eventType, course, assignmentID)
		fmt.Fprintf(&b, "   title=%s\n", title)
		fmt.Fprintf(&b, "   actor=userId:%s role:%s name:%s\n", actorID, actorRole, actorName)
		fmt.Fprintf(&b, "   occurredAt=%s traceId=%s\n", occurredAt, traceID)
	}
	return TruncateDiscordMessage(b.String())
}

func FormatAdminError(status, courseSlug, assignmentID, finding string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", security.SanitizeText(status))
	b.WriteString("source: WEB_ADMIN_API\n")
	if courseSlug != "" {
		fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	}
	if assignmentID != "" {
		fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(assignmentID))
	}
	fmt.Fprintf(&b, "key findings: %s\n", security.SanitizeText(finding))
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`\n")
	b.WriteString("- `/ops logs mode:trace query:<traceId>`")
	return TruncateDiscordMessage(b.String())
}

func WithAdminNotice(content, notice string) string {
	trimmed := strings.TrimSpace(security.SanitizeText(notice))
	if trimmed == "" {
		return content
	}
	return TruncateDiscordMessage(trimmed + "\n\n" + content)
}

func FormatCloudWatchFallback(title string, rows []map[string]string) string {
	content := FormatLogRows(title+" fallback result, not authoritative", rows)
	if len(rows) == 0 {
		content = title + "\nsource: CLOUDWATCH_FALLBACK\nkey findings: fallback result, not authoritative; matching logs 없음"
	}
	return TruncateDiscordMessage(content + "\n\nsource: CLOUDWATCH_FALLBACK\nnote: fallback result, not authoritative")
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
		statusCode := parseInt(row["http.statusCode"])
		if logType == "API" {
			summary.API += count
		}
		if logType == "API_ERROR" {
			summary.APIError += count
		}
		if logType == "API_ERROR" || logType == "EVENT_ERROR" {
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
	return HelpTextFor("", "", "")
}

func HelpTextFor(topic, command, query string) string {
	command = strings.ToLower(strings.TrimSpace(command))
	if command != "" {
		return helpCommandText(command)
	}
	switch strings.ToLower(strings.TrimSpace(topic)) {
	case "assignments":
		return helpAssignmentsText()
	case "logs":
		return helpLogsText()
	case "alerts":
		return helpAlertsText()
	case "dashboard":
		return helpDashboardText()
	case "routing":
		return helpRoutingText()
	case "audit":
		return helpAuditText()
	case "troubleshooting":
		return helpTroubleshootingText()
	}
	if strings.TrimSpace(query) != "" {
		return helpQueryText(query)
	}
	return strings.TrimSpace(`A&I Ops Bot 도움말

기본 명령은 5개입니다.

1. /ops dashboard
   - 전체/서비스 상태와 dashboard watch를 봅니다.
   - 예: /ops dashboard
   - 예: /ops dashboard service:report

2. /ops logs
   - 오류, CRITICAL, trace, EVENT/audit 로그를 검색합니다.
   - 예: /ops logs service:report mode:errors since:30m limit:10
   - 예: /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
   - 예: /ops logs mode:trace query:<traceId>

3. /ops alert
   - general/critical 채널과 CRITICAL role mention을 설정합니다.
   - 예: /ops alert action:channel target:general channel:#ops-log
   - 예: /ops alert action:channel target:critical channel:#ops-critical
   - 예: /ops alert action:role role:@운영팀

4. /ops assignment
   - 과제 목록, 상세, 진단, 이벤트 이력, ack, 제출 상태를 봅니다.
   - 예: /ops assignment course:3rd-cs
   - 예: /ops assignment course:<course> id:<assignmentId> view:diagnosis
   - 예: /ops assignment course:<course> id:<assignmentId> view:events

5. /ops help
   - 상황별로 쓸 명령을 검색합니다.
   - 예: /ops help query:"과제 수정 누가"
   - 예: /ops help query:"critical role"

주의:
- 봇은 과제를 생성/수정/삭제/공개하지 않습니다.
- 누가/언제 과제를 변경했는지는 Report EVENT 로그에서 확인합니다.
- CRITICAL 서버 장애만 role mention을 사용합니다.`)
}

func helpDashboardText() string {
	return strings.TrimSpace(`A&I Ops 대시보드 도움말

/ops dashboard
→ 전체 서비스 최근 30분 상태를 봅니다.

/ops dashboard service:<service>
→ 특정 서비스 하나의 health/log/error 요약을 봅니다.

/ops dashboard action:watch channel:#ops interval:5m
→ 하나의 dashboard 메시지를 주기적으로 edit/update합니다.

/ops dashboard action:unwatch
→ dashboard watch를 해제합니다.

/ops dashboard action:status
→ 등록된 dashboard watch를 확인합니다.

이 명령은 예전 service/watch/unwatch 흐름을 대체합니다.`)
}

func helpLogsText() string {
	return strings.TrimSpace(`A&I Ops 로그 도움말

/ops logs
→ 전체 서비스 오류 로그를 최근 30분 기준으로 봅니다.

/ops logs service:report mode:recent query:<assignmentId|traceId|eventType> since:24h limit:20
→ 구조화 필드에서 검색하고 @message는 fallback 검색으로만 사용합니다.

/ops logs service:report mode:errors since:30m limit:10
→ API_ERROR/EVENT_ERROR 집계를 봅니다.

/ops logs service:report mode:critical since:30m limit:10
→ CRITICAL 장애 후보를 확인합니다.

/ops logs service:report mode:slow since:30m limit:10
→ 느린 API 로그를 확인합니다.

/ops logs service:report mode:security since:30m limit:10
→ 보안 로그를 확인합니다.

/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
→ Report assignment audit EVENT 로그에서 actor와 발생 시각을 봅니다.

/ops logs mode:trace query:<traceId>
→ traceId 기준 요청 흐름을 봅니다.

/ops logs action:watch service:report mode:errors channel:#report-logs interval:5m
/ops logs action:unwatch service:report mode:errors
/ops logs action:watches
→ 로그 feed 등록/해제/목록을 관리합니다.

분류는 structured V2 fields 기준입니다. @message는 fallback 검색/표시로만 사용합니다.`)
}

func helpAlertsText() string {
	return strings.TrimSpace(`A&I Ops 알림 도움말

/ops alert action:channel target:general channel:#ops-log
→ assignment audit, assignment issue WARN/INFO, HIGH service alert, 일반 운영 로그 채널입니다.

/ops alert action:channel target:critical channel:#ops-critical
→ CRITICAL 서버 장애 전용 채널입니다.

/ops alert action:channel target:all channel:#ops-alerts
→ general/critical을 같은 채널로 저장합니다.

/ops alert action:role role:@운영팀
→ CRITICAL 서버 장애에서만 mention할 역할을 저장합니다.

/ops alert action:role-clear
→ 저장된 role mention 설정을 지웁니다.

/ops alert action:status
→ general/critical 채널, role, fallback, cooldown, 최근 alert 상태를 봅니다.

/ops alert action:test target:general
/ops alert action:test target:critical
→ route별 테스트 메시지를 보냅니다. test는 role mention을 보내지 않습니다.

HIGH/general/audit/WARN은 role mention을 하지 않습니다. @everyone/@here는 허용하지 않습니다.`)
}

func helpRoutingText() string {
	return strings.TrimSpace(`A&I Ops 라우팅 도움말

general route:
- assignment audit 성공 이벤트
- assignment issue WARN/INFO
- HIGH service alert
- 일반 운영 로그
- role mention 없음

critical route:
- CRITICAL 서버 장애만 전송
- 설정된 role mention 사용

설정 예시:
- /ops alert action:channel target:general channel:#ops-log
- /ops alert action:channel target:critical channel:#ops-critical
- /ops alert action:role role:@운영팀

fallback:
- general: state general channel → legacy alert channel → DISCORD_ALERT_CHANNEL_ID → dashboard channel
- critical: state critical channel → legacy alert channel → DISCORD_ALERT_CHANNEL_ID`)
}

func helpAssignmentsText() string {
	return strings.TrimSpace(`과제 운영 도움말

/ops assignment
- 목적: 과제 목록, 상세, 진단, 감지 이력, 체크리스트, 제출 상태, ack/unack을 한 명령에서 처리
- list: /ops assignment course:3rd-cs action:list status:draft
- all list: /ops assignment scope:all action:list window:today
- summary: /ops assignment course:<course> id:<assignmentId>
- diagnosis: /ops assignment course:<course> id:<assignmentId> view:diagnosis
- events: /ops assignment course:<course> id:<assignmentId> view:events
- check: /ops assignment course:<course> id:<assignmentId> action:check
- submissions: /ops assignment course:<course> id:<assignmentId> action:submissions
- ack: /ops assignment course:<course> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>
- 주의: 누가 변경했는지 증명하는 audit trail이 아닙니다.

/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
- 목적: WEB Admin API가 답하지 못하는 원인을 CloudWatch 로그에서 검색

Assignment audit notifications
- 목적: 과제 등록/수정/삭제/공개/비공개를 누가 언제 했는지 확인
- source: Report EVENT logs only
- 이벤트: ASSIGNMENT_CREATED, ASSIGNMENT_UPDATED, ASSIGNMENT_DELETED, ASSIGNMENT_PUBLISHED, ASSIGNMENT_UNPUBLISHED
- 조회: /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
- 주의: bot은 과제를 생성/수정/삭제/공개하지 않습니다. actor/occurredAt은 EVENT 로그에 있을 때만 표시하고 없으면 unknown입니다.`)
}

func helpAuditText() string {
	return strings.TrimSpace(`Assignment audit 도움말

Assignment audit 알림은 자동으로 전송됩니다.
- source: Report V2 EVENT logs
- route: general
- role mention: 없음

대상 이벤트:
- ASSIGNMENT_CREATED
- ASSIGNMENT_UPDATED
- ASSIGNMENT_DELETED
- ASSIGNMENT_PUBLISHED
- ASSIGNMENT_UNPUBLISHED

표시 필드:
- actor.userId, actor.role, 안전한 actor name/displayName/loginId
- occurredAt
- traceId, requestId
- assignmentId, title
- changedFields

actor/occurredAt이 없으면 unknown으로 표시합니다. WEB Admin API snapshot에서 actor를 추측하지 않습니다.

수동 조회:
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20`)
}

func helpTroubleshootingText() string {
	return strings.TrimSpace(`A&I Ops 문제 해결 도움말

Discord command가 보이지 않음:
- 배포/등록 시점에 DISCORD_REGISTER_COMMANDS=true로 1회 등록했는지 확인합니다.
- /healthz의 discordCommandsRegistered, discordCommandRegistrationError를 확인합니다.

CRITICAL role mention이 동작하지 않음:
- /ops alert action:status로 role/channel 상태를 확인합니다.
- bot이 해당 role을 mention할 권한이 있는지 확인합니다.

과제 WARN이 많음:
- Too many assignment warnings:
- issue digest와 repeated suppressed count를 확인합니다.
- /ops assignment course:<course> id:<assignmentId> view:events로 감지 이력을 봅니다.
- 의도된 stale issue는 /ops assignment course:<course> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>으로 묶습니다.

누가 과제를 수정했는지 확인:
- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20

서버 장애 원인 추적:
- traceId가 있으면 /ops logs mode:trace query:<traceId>를 사용합니다.`)
}

func helpCommandText(command string) string {
	switch command {
	case "assignment-check":
		return strings.TrimSpace(`/ops assignment action:check

역할:
특정 과제가 운영상 제출 가능한 상태인지 점검합니다.

확인하는 것:
- title 존재 여부
- assignmentStatus
- publishedAt/startAt/endAt 시간 관계
- problemId 연결 여부
- 봇이 감지한 issue와의 연결

주의:
이 명령은 왜 알림이 발생했는지 설명해야 합니다. problemId 누락만으로 DRAFT past start를 설명하지 않습니다.

다음 단계:
- 서버 로그 검색: /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
- 감지 이력 확인: /ops assignment course:<course> id:<assignmentId> view:events
- 의도된 상태면 ack: /ops assignment course:<course> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>`)
	case "dashboard":
		return strings.TrimSpace(`/ops dashboard

역할:
전체/단일 서비스 상태와 dashboard watch를 관리합니다.

언제 사용:
- 현재 서비스 상태를 빠르게 볼 때
- 운영 채널에 고정 dashboard를 만들거나 해제할 때

예시:
- /ops dashboard
- /ops dashboard service:report
- /ops dashboard action:watch channel:#ops interval:5m
- /ops dashboard action:unwatch

이 명령은 예전 service/watch 흐름을 대체합니다.`)
	case "logs":
		return HelpTextFor("logs", "", "")
	case "alert":
		return strings.TrimSpace(`/ops alert

역할:
general/critical 채널, CRITICAL role mention, alert on/off/test를 설정합니다.

예시:
- /ops alert action:channel target:general channel:#ops-log
- /ops alert action:channel target:critical channel:#ops-critical
- /ops alert action:role role:@운영팀
- /ops alert action:status
- /ops alert action:test target:critical

정책:
- CRITICAL 서버 장애만 critical route와 role mention을 사용합니다.
- HIGH/general/audit/WARN은 role mention을 하지 않습니다.`)
	case "assignment":
		return strings.TrimSpace(`/ops assignment

역할:
단일 과제의 현재 상태, 진단, 봇 감지 이력, ack/unack을 처리합니다.
read-only 명령이며 과제를 생성/수정/삭제/공개하지 않습니다.

확인하는 것:
- title/status/publishedAt/startAt/endAt/problemId
- 봇 issue lifecycle와 ack/silence 상태
- 제출/채점 요약

view:
- summary: 기본 필드
- diagnosis: 봇이 이상 상태로 판단한 근거와 issue lifecycle
- events: 봇 감지 이력, 반복 억제, ack/silence 상태
- raw: 민감정보 제외 원본 주요 필드

action:
- list: 과제 목록 조회
- check: 운영 체크리스트
- submissions: 제출/채점 상태
- ack: 의도된 과제 이슈의 반복 알림 중지
- unack: ack 해제

예시:
/ops assignment course:3rd-cs action:list status:draft
/ops assignment course:<course> id:<assignmentId> view:events
/ops assignment course:<course> id:<assignmentId> action:check
/ops assignment course:<course> id:<assignmentId> action:submissions
/ops assignment course:<course> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>`)
	case "help":
		return strings.TrimSpace(`/ops help

역할:
상황별로 어떤 명령을 써야 하는지 검색합니다.

사용 방식:
- /ops help
- /ops help topic:<dashboard|logs|alerts|assignments|routing|audit|troubleshooting>
- /ops help command:<dashboard|logs|alert|assignment|help>
- /ops help query:"과제 수정 누가"
- /ops help query:"critical role"`)
	default:
		return HelpTextFor("overview", "", "")
	}
}

func helpQueryText(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	assignmentQuery := strings.Contains(normalized, "과제") || strings.Contains(normalized, "assignment")
	deleteQuery := strings.Contains(normalized, "삭제") || strings.Contains(normalized, "delete") || strings.Contains(normalized, "deleted")
	updateActorQuery := strings.Contains(normalized, "수정") || strings.Contains(normalized, "update") || strings.Contains(normalized, "updated") || strings.Contains(normalized, "누가") || strings.Contains(normalized, "who") || strings.Contains(normalized, "actor")
	criticalRoleQuery := strings.Contains(normalized, "critical") || strings.Contains(normalized, "장애") || strings.Contains(normalized, "role") || strings.Contains(normalized, "mention")
	generalChannelQuery := (strings.Contains(normalized, "일반") && strings.Contains(normalized, "채널")) ||
		(strings.Contains(normalized, "general") && strings.Contains(normalized, "channel")) ||
		(strings.Contains(normalized, "ops log") && strings.Contains(normalized, "channel")) ||
		(strings.Contains(normalized, "ops logs") && strings.Contains(normalized, "channel"))
	repeatedAlertQuery := strings.Contains(normalized, "반복") || strings.Contains(normalized, "중복") ||
		strings.Contains(normalized, "spam") || strings.Contains(normalized, "too many") ||
		strings.Contains(normalized, "noisy") || strings.Contains(normalized, "repeated") ||
		strings.Contains(normalized, "duplicate")
	var b strings.Builder
	fmt.Fprintf(&b, "검색어: %s\n\n", security.SanitizeText(query))
	switch {
	case strings.Contains(normalized, "태그") || strings.Contains(normalized, "배포"):
		b.WriteString("이 PR에서는 tag/deploy를 하지 않습니다.\n")
		b.WriteString("- git tag 또는 git push origin v* 명령을 실행하지 않습니다.\n")
		b.WriteString("- 태그 배포는 별도 release workflow입니다.\n")
		b.WriteString("- command schema 변경은 배포/등록 시점에 DISCORD_REGISTER_COMMANDS=true로 1회 등록합니다.")
	case strings.Contains(normalized, "공개 지연"):
		b.WriteString("과제 공개 지연 판단:\n")
		b.WriteString("- ASSIGNMENT_PUBLISH_DELAYED는 publishedAt이 존재하고, 현재보다 과거이며, status가 published/open/opened가 아닐 때만 사용합니다.\n")
		b.WriteString("- publishedAt unknown + DRAFT + startAt past는 ASSIGNMENT_DRAFT_PAST_START입니다.\n")
		b.WriteString("- stale draft는 반복 WARN으로 spam하지 않습니다.\n\n")
		b.WriteString("확인:\n")
		b.WriteString("- /ops assignment course:<course> id:<assignmentId> view:diagnosis\n")
		b.WriteString("- /ops assignment course:<course> id:<assignmentId> view:events")
	case criticalRoleQuery:
		b.WriteString("CRITICAL role mention 설정:\n")
		b.WriteString("- /ops alert action:channel target:critical channel:#ops-critical\n")
		b.WriteString("- /ops alert action:role role:@운영팀\n\n")
		b.WriteString("정책:\n")
		b.WriteString("- CRITICAL only: configured role mention은 CRITICAL alert에서만 보냅니다.\n")
		b.WriteString("- HIGH/general/audit/WARN do not role-mention.")
	case generalChannelQuery:
		b.WriteString("일반 운영 로그 채널:\n")
		b.WriteString("- /ops alert action:channel target:general channel:#ops-log\n")
		b.WriteString("  → assignment audit, assignment issue WARN/INFO, HIGH service alerts, normal ops logs go to general.\n\n")
		b.WriteString("CRITICAL goes to critical:\n")
		b.WriteString("- /ops alert action:channel target:critical channel:#ops-critical")
	case assignmentQuery && deleteQuery:
		b.WriteString("과제 삭제 audit 확인:\n")
		b.WriteString("- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("  → 삭제 actor/time은 Report V2 EVENT logs에서 확인합니다.\n\n")
		b.WriteString("주의:\n")
		b.WriteString("- 삭제된 assignment는 /ops assignment course:<course> id:<assignmentId>에서 더 이상 조회되지 않을 수 있습니다.\n")
		b.WriteString("- /ops assignment는 현재 WEB Admin state만 보여줍니다.\n")
		b.WriteString("- bot은 과제를 삭제하거나 업데이트하지 않습니다.")
	case assignmentQuery && updateActorQuery:
		b.WriteString("과제 수정 actor 확인:\n")
		b.WriteString("- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("- /ops assignment course:<course> id:<assignmentId> view:events\n\n")
		b.WriteString("정책:\n")
		b.WriteString("- who/when은 Report EVENT logs에서 확인합니다.\n")
		b.WriteString("- 현재 assignment state does not prove actor.\n")
		b.WriteString("- bot은 과제를 업데이트하지 않습니다.")
	case assignmentQuery && strings.Contains(normalized, "공개"):
		b.WriteString("관련 기능:\n")
		b.WriteString("1. 과제 audit 알림\n")
		b.WriteString("   - source: Report EVENT logs\n")
		b.WriteString("   - eventType: ASSIGNMENT_CREATED/UPDATED/DELETED/PUBLISHED/UNPUBLISHED\n")
		b.WriteString("   - actor와 occurredAt이 있으면 자동 표시합니다.\n\n")
		b.WriteString("2. 수동 검색\n")
		b.WriteString("   /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("   → 과제 생성/수정/삭제/공개 EVENT 로그를 검색합니다.\n\n")
		b.WriteString("3. 과제별 이력\n")
		b.WriteString("   /ops assignment course:<course> id:<assignmentId> view:events\n")
		b.WriteString("   → 봇 감지 이력과 audit event 요약을 봅니다.\n\n")
		b.WriteString("주의:\n")
		b.WriteString("- /ops assignment는 현재 상태 조회입니다.\n")
		b.WriteString("- 누가 변경했는지는 Report EVENT 로그에서 확인합니다.\n")
		b.WriteString("- bot은 과제를 생성/수정/삭제/공개하지 않습니다.")
	case repeatedAlertQuery:
		b.WriteString("반복 알림 정책:\n")
		b.WriteString("- assignment issues are lifecycle state, not an event stream.\n")
		b.WriteString("- 같은 open issue는 cooldown마다 다시 전송하지 않습니다.\n")
		b.WriteString("- digest groups repeated assignment issues and shows repeated suppressed count.\n\n")
		b.WriteString("확인/조치:\n")
		b.WriteString("- /ops assignment course:<course> id:<assignmentId> view:events\n")
		b.WriteString("- /ops assignment course:<course> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>")
	case strings.Contains(normalized, "로그") || strings.Contains(normalized, "검색") || strings.Contains(normalized, "trace"):
		b.WriteString("관련 기능:\n")
		b.WriteString("- /ops logs service:report mode:recent query:<assignmentId|traceId|eventType> since:24h limit:20\n")
		b.WriteString("  → 구조화 필드 기반 일반 로그 검색\n")
		b.WriteString("- /ops logs service:<service> mode:errors since:30m limit:10\n")
		b.WriteString("  → 서비스 오류 집계 확인\n")
		b.WriteString("- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("  → 과제 lifecycle EVENT 로그 검색\n")
		b.WriteString("- /ops logs mode:trace query:<traceId>\n")
		b.WriteString("  → traceId가 있을 때만 단일 요청 흐름 확인")
	default:
		b.WriteString("관련 기능을 좁히지 못했습니다.\n")
		b.WriteString("- /ops help topic:assignments\n")
		b.WriteString("- /ops help topic:alerts\n")
		b.WriteString("- /ops help topic:logs")
	}
	return TruncateDiscordMessage(b.String())
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
	writeTopRowsWithLimit(b, rows, 10)
}

func writeTopRowsWithLimit(b *strings.Builder, rows []map[string]string, limit int) {
	if len(rows) == 0 {
		b.WriteString("- none\n")
		return
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	for i, row := range rows {
		if i >= limit {
			break
		}
		count := value(row, "count", "1")
		path := redactPath(firstNonEmpty(value(row, "http.route", ""), value(row, "http.path", "")))
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
	case logStatus == "NOT_CONNECTED" || healthState == "NOT_CONNECTED":
		return "⚫"
	case alarm || healthState == "DOWN" || isDashboardLogFailure(logStatus) || summary.FiveXX > 0:
		return "🔴"
	case logStatus == "NO_LOGS" || logStatus == "NO_V2_LOG":
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
	case "NO_V2_LOG":
		return "NO_V2"
	case "NOT_CONFIGURED":
		return "NOCFG"
	case "NOT_CONNECTED":
		return "NCONN"
	case "LOG_QUERY_FAILED":
		return "QFAIL"
	case "ERR":
		return "ERR"
	case "AUTH":
		return "AUTH"
	case "TIMEOUT":
		return "TOUT"
	default:
		return "UNK"
	}
}

func isDashboardLogFailure(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "LOG_QUERY_FAILED", "ERR", "AUTH", "TIMEOUT":
		return true
	default:
		return false
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
	case "NO_V2_LOG":
		return "UNK"
	case "NOT_CONFIGURED":
		return "NOCFG"
	case "NOT_CONNECTED":
		return "NCONN"
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
	case "post":
		return "blog"
	case "gateway", "auth", "report":
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
	if logStatus == "NOT_CONFIGURED" || logStatus == "NOT_CONNECTED" || isDashboardLogFailure(logStatus) {
		return "-"
	}
	return strconv.Itoa(value)
}

func dashboardLastLogShort(value, logStatus string) string {
	if logStatus == "NOT_CONFIGURED" || logStatus == "NOT_CONNECTED" || logStatus == "NO_LOGS" || logStatus == "NO_V2_LOG" || isDashboardLogFailure(logStatus) {
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

func shortValue(value string) string {
	trimmed := strings.TrimSpace(security.SanitizeText(value))
	if trimmed == "" {
		return "unknown"
	}
	runes := []rune(trimmed)
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return trimmed
}

func okMissing(value string) string {
	if strings.TrimSpace(value) == "" {
		return "MISSING"
	}
	return "OK"
}

func timeCheck(value string, staleAware bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "MISSING"
	}
	parsed, ok := parseTimestamp(trimmed)
	if !ok {
		return "INVALID"
	}
	if time.Now().After(parsed) {
		if staleAware && time.Since(parsed) > 7*24*time.Hour {
			return "STALE"
		}
		return "PAST"
	}
	return "OK"
}

func countAssignmentStatuses(assignments []reportadmin.Assignment) map[string]int {
	counts := map[string]int{"published": 0, "scheduled": 0, "draft": 0}
	for _, assignment := range assignments {
		normalized := strings.ToLower(strings.TrimSpace(assignment.Status))
		if _, ok := counts[normalized]; ok {
			counts[normalized]++
		}
	}
	return counts
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
