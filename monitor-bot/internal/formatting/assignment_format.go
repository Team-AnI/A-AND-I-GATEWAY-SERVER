package formatting

import (
	"fmt"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

type AdminAssignmentsAllSummary struct {
	CourseSlug string
	Total      int
	Published  int
	Scheduled  int
	Draft      int
	Shown      []reportadmin.Assignment
	Error      string
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
