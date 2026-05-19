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
			if topIssue == "none" {
				topIssue = fmt.Sprintf("%s 5xx x%d", service.Service, summary.FiveXX)
			}
			break
		}
		if strings.EqualFold(service.Health.State, "UNKNOWN") || strings.EqualFold(service.Health.State, "NOT_CONNECTED") || summary.Error > 0 || summary.P95 >= 1000 || service.LogStatus == "NOT_CONFIGURED" || service.LogStatus == "NOT_CONNECTED" {
			statusIcon = "🟡"
			overall = "주의"
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
	b.WriteString("/ops trace trace_id:<traceId>")
	return TruncateDiscordMessage(b.String())
}

func isUnconnectedDashboardInput(service DashboardServiceInput) bool {
	return strings.Contains(strings.ToLower(service.Health.Detail), "not connected") && (strings.EqualFold(service.LogStatus, "NO_LOGS") || strings.EqualFold(service.LogStatus, "NO_V2_LOG"))
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
	b.WriteString("- `/ops trace trace_id:<traceId>`")
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
	b.WriteString("- `/ops trace trace_id:<traceId>`")
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
	b.WriteString("- `/ops submissions course:" + security.SanitizeText(courseSlug) + " assignment:<assignmentId>`\n")
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
	b.WriteString("- `/ops assignments course:<courseSlug> status:all`\n")
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
	b.WriteString("- `/ops assignment-check course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + "`\n")
	b.WriteString("- `/ops submissions course:" + security.SanitizeText(courseSlug) + " assignment:" + shortValue(assignment.ID) + "`")
	return TruncateDiscordMessage(b.String())
}

func FormatAdminAssignmentCheck(courseSlug string, assignment reportadmin.Assignment, check reportadmin.AssignmentCheck) string {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", check.Status)
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", shortValue(assignment.ID))
	b.WriteString("key findings:\n")
	for _, finding := range check.Findings {
		fmt.Fprintf(&b, "- %s\n", security.SanitizeText(finding))
	}
	b.WriteString("\nrecommended next commands:\n")
	b.WriteString("- `/ops assignment course:" + security.SanitizeText(courseSlug) + " id:" + shortValue(assignment.ID) + "`\n")
	b.WriteString("- `/ops logs service:report mode:errors since:30m limit:10`")
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
	b.WriteString("- `/ops trace trace_id:<traceId>`")
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
	return strings.TrimSpace(`A&I Ops Incident Flow

1. 전체 상태 확인
   /ops dashboard since:30m
2. 전체 서비스 에러 빠른 확인
   /ops logs service:all mode:errors since:15m limit:10
3. 특정 서비스 에러 분석
   /ops logs service:blog mode:errors since:30m limit:10
4. 느린 API 확인
   /ops logs service:auth mode:slow since:30m limit:10

Automation setup
- /ops watch scope:all channel:#ops interval:5m
- /ops alert action:channel channel:#ops-alerts
- /ops alert action:role role:@운영팀
- /ops alert action:on
- /ops logs-watch service:blog mode:errors channel:#blog-logs interval:5m since:30m limit:10
- /ops logs-watches

Trace drilldown은 /ops logs 또는 logs-watch 결과에 traceId가 있을 때만 사용하세요.
Assignment Ops는 수동 상태 확인보다 과제 등록/공개/채점 이벤트 feed가 기본입니다.
수동 assignment 명령은 알림 이후 상세 확인 또는 fallback 용도입니다.`)
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
