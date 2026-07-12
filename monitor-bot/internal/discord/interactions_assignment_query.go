package discord

import (
	"context"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

func (h *Handler) assignmentsCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, "", "올바르지 않은 courseSlug입니다.")
	}
	statusFilter, ok := security.NormalizeAssignmentStatus(optionString(interaction, "status"))
	if !ok {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, "", "지원하지 않는 status 값입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	assignments, err := h.reportAdmin.ListAssignments(ctx, courseSlug)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, "", err.Error()), notice)
		}
		return h.assignmentLogFallback(ctx, "Assignments", courseSlug, "", status, err)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminAssignments(courseSlug, statusFilter, reportadmin.FilterAssignments(assignments, statusFilter)), notice)
}

func (h *Handler) assignmentsAllCommand(ctx context.Context, interaction Interaction) string {
	window, ok := security.NormalizeAssignmentWindow(optionString(interaction, "window"))
	if !ok {
		return formatting.FormatAdminError(reportadmin.StatusError, "", "", "지원하지 않는 window 값입니다.")
	}
	courses, err := h.reportAdmin.ListCourses(ctx)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			return formatting.FormatAdminError(status, "", "", err.Error())
		}
		return h.assignmentLogFallback(ctx, "Assignments all", "", "", status, err)
	}
	summaries := make([]formatting.AdminAssignmentsAllSummary, 0, len(courses))
	totalShown := 0
	for _, course := range courses {
		if len(summaries) >= 8 || totalShown >= 20 {
			break
		}
		assignments, err := h.reportAdmin.ListAssignments(ctx, course.Slug)
		summary := formatting.AdminAssignmentsAllSummary{CourseSlug: course.Slug}
		if err != nil {
			summary.Error = reportadmin.StatusOf(err)
			summaries = append(summaries, summary)
			continue
		}
		filtered := filterAssignmentsByWindow(assignments, window)
		counts := assignmentStatusCounts(filtered)
		summary.Total = len(filtered)
		summary.Published = counts["published"]
		summary.Scheduled = counts["scheduled"]
		summary.Draft = counts["draft"]
		for _, assignment := range filtered {
			if len(summary.Shown) >= 3 || totalShown >= 20 {
				break
			}
			summary.Shown = append(summary.Shown, assignment)
			totalShown++
		}
		summaries = append(summaries, summary)
	}
	return formatting.FormatAdminAssignmentsAll(window, summaries)
}

func (h *Handler) assignmentCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	assignmentID := strings.TrimSpace(optionString(interaction, "id"))
	action := strings.TrimSpace(optionString(interaction, "action"))
	scope := strings.TrimSpace(optionString(interaction, "scope"))
	if action == "" && assignmentID == "" {
		action = "list"
	}
	if action == "list" || scope == "all" {
		if scope == "all" || courseSlug == "" {
			return h.assignmentsAllCommand(ctx, interactionForCommand("assignments-all", withDefaultOptions(interaction.Data.Options, map[string]string{"window": "today"})))
		}
		if !security.ValidateCourseSlug(courseSlug) {
			return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
		}
		return h.assignmentsCommand(ctx, interactionForCommand("assignments", withDefaultOptions(interaction.Data.Options, map[string]string{"status": "all"})))
	}
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	switch action {
	case "check":
		return h.assignmentCheckCommand(ctx, interaction)
	case "submissions":
		return h.submissionsCommand(ctx, interactionForCommand("submissions", withDefaultOptions(interaction.Data.Options, map[string]string{"assignment": assignmentID})))
	case "ack":
		return h.assignmentAckAction(courseSlug, assignmentID, interaction)
	case "unack":
		return h.assignmentUnackAction(courseSlug, assignmentID, interaction)
	case "":
	default:
		return "지원하지 않는 assignment action입니다. list, check, submissions, ack, unack 중 하나를 사용하세요."
	}
	view := optionString(interaction, "view")
	if view == "" {
		view = "summary"
	}
	if view == "events" {
		return h.assignmentEventsView(courseSlug, assignmentID)
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	assignment, err := h.reportAdmin.GetAssignment(ctx, courseSlug, assignmentID)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			finding := err.Error()
			if status == reportadmin.StatusNoData {
				finding = "no matching records"
			}
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, assignmentID, finding), notice)
		}
		return h.assignmentLogFallback(ctx, "Assignment", courseSlug, assignmentID, status, err)
	}
	switch view {
	case "summary":
		return formatting.WithAdminNotice(formatting.FormatAdminAssignment(courseSlug, assignment), notice)
	case "diagnosis":
		if h.ops == nil {
			return formatting.WithAdminNotice(formatting.FormatAdminAssignment(courseSlug, assignment), notice)
		}
		return formatting.WithAdminNotice(h.ops.DescribeAssignmentDiagnosis(courseSlug, assignment), notice)
	case "raw":
		return formatting.WithAdminNotice(formatting.FormatAdminAssignmentRaw(courseSlug, assignment), notice)
	default:
		return "지원하지 않는 assignment view입니다. summary, diagnosis, raw, events 중 하나를 사용하세요."
	}
}

func (h *Handler) assignmentCheckCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	assignmentID := strings.TrimSpace(optionString(interaction, "id"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	assignment, err := h.reportAdmin.GetAssignment(ctx, courseSlug, assignmentID)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			finding := err.Error()
			if status == reportadmin.StatusNoData {
				finding = "no matching records"
			}
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, assignmentID, finding), notice)
		}
		return h.assignmentLogFallback(ctx, "Assignment check", courseSlug, assignmentID, status, err)
	}
	botIssue := "NONE"
	if h.ops != nil {
		botIssue = h.ops.AssignmentIssueStatus(courseSlug, assignmentID)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminAssignmentCheck(courseSlug, assignment, reportadmin.CheckAssignment(assignment), botIssue), notice)
}

func (h *Handler) submissionsCommand(ctx context.Context, interaction Interaction) string {
	courseSlug := strings.TrimSpace(optionString(interaction, "course"))
	assignmentID := strings.TrimSpace(optionString(interaction, "assignment"))
	if !security.ValidateCourseSlug(courseSlug) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 courseSlug입니다.")
	}
	if !security.ValidateAssignmentID(assignmentID) {
		return formatting.FormatAdminError(reportadmin.StatusError, courseSlug, assignmentID, "올바르지 않은 assignmentId입니다.")
	}
	notice := h.courseManualNotice(ctx, courseSlug)
	summary, err := h.reportAdmin.SubmissionStatuses(ctx, courseSlug, assignmentID)
	if err != nil {
		status := reportadmin.StatusOf(err)
		if !shouldUseCloudWatchFallback(status) {
			finding := err.Error()
			if status == reportadmin.StatusNoData {
				finding = "no matching records"
			}
			return formatting.WithAdminNotice(formatting.FormatAdminError(status, courseSlug, assignmentID, finding), notice)
		}
		return h.assignmentLogFallback(ctx, "Submissions", courseSlug, assignmentID, status, err)
	}
	return formatting.WithAdminNotice(formatting.FormatAdminSubmissions(courseSlug, assignmentID, summary), notice)
}

func (h *Handler) assignmentLogFallback(ctx context.Context, title, courseSlug, assignmentID, status string, cause error) string {
	if assignmentID != "" && security.ValidateAssignmentID(assignmentID) {
		groups, groupErr := cw.LogGroupsForService(h.cfg.LogGroups, "report")
		if groupErr == nil {
			query, queryErr := cw.BuildAssignmentQuery(assignmentID)
			if queryErr == nil {
				rows, queryRunErr := h.logs.Query(ctx, groups, query, 3*time.Hour, 20)
				if queryRunErr == nil {
					return formatting.FormatCloudWatchFallback(title, rows)
				}
			}
		}
	}
	prefix := "WEB_ADMIN_API " + status + ": " + security.SanitizeText(cause.Error()) + ". "
	return formatting.FormatAdminError(status, courseSlug, assignmentID, prefix+"CloudWatch fallback result, not authoritative; 상세 로그는 `/ops logs service:report mode:errors since:30m limit:10`로 확인하세요.")
}

func shouldUseCloudWatchFallback(status string) bool {
	switch status {
	case reportadmin.StatusUpstreamError, reportadmin.StatusTimeout, reportadmin.StatusInvalidResponse:
		return true
	default:
		return false
	}
}

func (h *Handler) courseManualNotice(ctx context.Context, courseSlug string) string {
	courses, err := h.reportAdmin.ListCourses(ctx)
	if err != nil {
		return ""
	}
	now := time.Now()
	for _, course := range courses {
		if course.Slug != courseSlug {
			continue
		}
		switch classifyManualCourse(course, now) {
		case "LEGACY":
			return "참고: 이 코스는 레거시/종료 코스로 보입니다. 자동 feed 대상은 아니며 수동 조회 결과입니다."
		case "UNKNOWN":
			return "참고: 이 코스는 운영 상태 판단 필드가 부족해 UNKNOWN으로 보입니다. 자동 이벤트 발송은 제한됩니다."
		default:
			return ""
		}
	}
	return ""
}

func classifyManualCourse(course reportadmin.Course, now time.Time) string {
	status := strings.ToUpper(strings.TrimSpace(course.Status))
	switch status {
	case "CLOSED", "ARCHIVED", "ENDED", "LEGACY", "INACTIVE":
		return "LEGACY"
	}
	if end, ok := parseRFC3339(course.EndAt); ok && now.After(end) {
		return "LEGACY"
	}
	if strings.TrimSpace(course.Status) == "" && strings.TrimSpace(course.StartAt) == "" && strings.TrimSpace(course.EndAt) == "" {
		return "UNKNOWN"
	}
	return "ACTIVE"
}

func filterAssignmentsByWindow(assignments []reportadmin.Assignment, window string) []reportadmin.Assignment {
	return filterAssignmentsByWindowAt(assignments, window, time.Now())
}

func filterAssignmentsByWindowAt(assignments []reportadmin.Assignment, window string, now time.Time) []reportadmin.Assignment {
	normalized := strings.TrimSpace(window)
	if normalized == "" {
		return assignments
	}
	var start, end time.Time
	switch normalized {
	case "today":
		kst := time.FixedZone("KST", 9*60*60)
		nowKST := now.In(kst)
		start = time.Date(nowKST.Year(), nowKST.Month(), nowKST.Day(), 0, 0, 0, 0, kst)
		end = start.Add(24 * time.Hour)
	case "this-week":
		kst := time.FixedZone("KST", 9*60*60)
		nowKST := now.In(kst)
		start = time.Date(nowKST.Year(), nowKST.Month(), nowKST.Day(), 0, 0, 0, 0, kst)
		end = now.Add(7 * 24 * time.Hour)
	default:
		return assignments
	}
	filtered := make([]reportadmin.Assignment, 0, len(assignments))
	for _, assignment := range assignments {
		for _, candidate := range []string{assignment.StartAt, assignment.EndAt, assignment.PublishedAt, assignment.UpdatedAt} {
			parsed, ok := parseRFC3339(candidate)
			if ok && (parsed.Equal(start) || parsed.After(start)) && parsed.Before(end) {
				filtered = append(filtered, assignment)
				break
			}
		}
	}
	return filtered
}

func assignmentStatusCounts(assignments []reportadmin.Assignment) map[string]int {
	counts := map[string]int{"published": 0, "scheduled": 0, "draft": 0}
	for _, assignment := range assignments {
		normalized := strings.ToLower(strings.TrimSpace(assignment.Status))
		if _, ok := counts[normalized]; ok {
			counts[normalized]++
		}
	}
	return counts
}

func parseRFC3339(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
