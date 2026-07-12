package monitor

import (
	"fmt"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func withRecentAssignmentEvents(result assignmentPollResult, recent []state.AssignmentEventState) assignmentPollResult {
	result.RecentEvents = recent
	return result
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func severityForAPIStatus(status string) string {
	switch status {
	case reportadmin.StatusAuthError, reportadmin.StatusForbidden, reportadmin.StatusUpstreamError:
		return "ERROR"
	default:
		return "WARN"
	}
}

func snapshotKey(courseSlug, assignmentID string) string {
	return courseSlug + ":" + assignmentID
}

func assignmentPublishedAtSummary(assignment reportadmin.Assignment) string {
	if strings.TrimSpace(assignment.PublishedAt) != "" {
		return assignment.PublishedAt
	}
	if assignment.PublishedAtOmitted {
		return "summary omitted"
	}
	return "unknown"
}

func (s *Service) DescribeAssignmentDiagnosis(courseSlug string, assignment reportadmin.Assignment) string {
	now := time.Now()
	snapshot := assignmentSnapshot(reportadmin.Course{Slug: courseSlug}, assignment, now)
	diagnoses := diagnoseAssignment(snapshot, now, s.assignmentStaleGrace())
	issues := assignmentIssuesFor(s.store.Snapshot(), courseSlug, assignment.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", assignmentDiagnosisStatus(diagnoses))
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(unknownIfBlank(assignment.ID)))
	fmt.Fprintf(&b, "title: %s\n", security.SanitizeText(unknownIfBlank(assignment.Title)))
	fmt.Fprintf(&b, "assignmentStatus: %s\n", security.SanitizeText(unknownIfBlank(assignment.Status)))
	fmt.Fprintf(&b, "publishedAt: %s\n", security.SanitizeText(assignmentPublishedAtSummary(assignment)))
	fmt.Fprintf(&b, "startAt: %s\n", security.SanitizeText(unknownIfBlank(assignment.StartAt)))
	fmt.Fprintf(&b, "endAt: %s\n", security.SanitizeText(unknownIfBlank(assignment.EndAt)))
	fmt.Fprintf(&b, "problemId: %s\n", security.SanitizeText(unknownIfBlank(assignment.ProblemID)))
	if strings.TrimSpace(assignment.ProblemIDFallback) != "" {
		fmt.Fprintf(&b, "problemIdFallback: %s\n", security.SanitizeText(assignment.ProblemIDFallback))
	}
	b.WriteString("\ndiagnosis:\n")
	if len(diagnoses) == 0 {
		b.WriteString("- current issue: NONE\n")
	} else {
		for _, diagnosis := range diagnoses {
			fmt.Fprintf(&b, "- %s severity=%s notify=%t count=%t\n", diagnosis.EventType, diagnosis.Severity, diagnosis.ShouldNotify, diagnosis.ShouldCount)
			fmt.Fprintf(&b, "  reasonCode: %s\n", security.SanitizeText(diagnosis.ReasonCode))
			fmt.Fprintf(&b, "  reasonText: %s\n", security.SanitizeText(diagnosis.ReasonText))
			if diagnosis.EventType == "ASSIGNMENT_DRAFT_PAST_START" {
				b.WriteString("  note: publishedAt이 없어 공개 지연으로 단정할 수 없음\n")
			}
		}
	}
	b.WriteString("\nissue lifecycle:\n")
	if len(issues) == 0 {
		b.WriteString("- stored issue: NONE\n")
	} else {
		for _, issue := range issues {
			fmt.Fprintf(&b, "- %s state=%s first=%s last=%s notified=%d\n",
				issue.EventType,
				security.SanitizeText(unknownIfBlank(issue.State)),
				formatKST(issue.FirstDetectedAt),
				formatKST(issue.LastDetectedAt),
				issue.NotifyCount,
			)
			if issue.AckReason != "" {
				fmt.Fprintf(&b, "  ack: %s until=%s\n", security.SanitizeText(issue.AckReason), formatKST(issue.AckUntil))
			}
		}
	}
	b.WriteString("\nnext:\n")
	fmt.Fprintf(&b, "1. /ops assignment course:%s id:%s view:events\n", security.SanitizeText(courseSlug), security.SanitizeText(assignment.ID))
	b.WriteString("   - 봇 감지 이력과 반복 억제 상태를 확인합니다.\n")
	fmt.Fprintf(&b, "2. /ops logs service:report mode:events query:%s since:24h limit:20\n", security.SanitizeText(assignment.ID))
	b.WriteString("   - Report EVENT 로그에서 이 assignmentId를 검색합니다.")
	return formatting.TruncateDiscordMessage(b.String())
}

func (s *Service) AssignmentIssueHistory(courseSlug, assignmentID string) string {
	snapshot := s.store.Snapshot()
	issues := activeAssignmentIssues(assignmentIssuesFor(snapshot, courseSlug, assignmentID))
	var b strings.Builder
	b.WriteString("Assignment issue history\n\n")
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(courseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(assignmentID))
	if len(issues) == 0 {
		b.WriteString("currentState: none\n")
	} else {
		b.WriteString("openIssues:\n")
		for i, issue := range issues {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "%d. %s\n", i+1, issue.EventType)
			fmt.Fprintf(&b, "   severity: %s\n", security.SanitizeText(issue.Severity))
			fmt.Fprintf(&b, "   state: %s\n", security.SanitizeText(unknownIfBlank(issue.State)))
			fmt.Fprintf(&b, "   firstDetectedAt: %s\n", formatKST(issue.FirstDetectedAt))
			fmt.Fprintf(&b, "   lastDetectedAt: %s\n", formatKST(issue.LastDetectedAt))
			fmt.Fprintf(&b, "   lastNotifiedAt: %s\n", formatKST(issue.LastNotifiedAt))
			fmt.Fprintf(&b, "   notifyCount: %d\n", issue.NotifyCount)
			fmt.Fprintf(&b, "   reason: %s\n", security.SanitizeText(issue.ReasonText))
			if assignmentIssueAckActive(issue, time.Now()) {
				fmt.Fprintf(&b, "   ack: %s until=%s\n", security.SanitizeText(issue.AckReason), formatKST(issue.AckUntil))
			}
			if !issue.LastDetectedAt.IsZero() && !issue.LastNotifiedAt.IsZero() && issue.LastDetectedAt.After(issue.LastNotifiedAt) {
				b.WriteString("   repeated: suppressed\n")
			}
		}
	}
	b.WriteString("\nrecentEvents:\n")
	count := 0
	for _, event := range snapshot.RecentAssignmentEvents {
		if event.CourseSlug != courseSlug || event.AssignmentID != assignmentID {
			continue
		}
		count++
		fmt.Fprintf(&b, "- %s %s %s\n", formatKST(event.CreatedAt), security.SanitizeText(event.EventType), security.SanitizeText(event.Summary))
		if count >= 5 {
			break
		}
	}
	if count == 0 {
		b.WriteString("- none\n")
	}
	return formatting.TruncateDiscordMessage(b.String())
}

func (s *Service) AssignmentIssueStatus(courseSlug, assignmentID string) string {
	issues := assignmentIssuesFor(s.store.Snapshot(), courseSlug, assignmentID)
	if len(issues) == 0 {
		return "NONE"
	}
	now := time.Now()
	for _, issue := range issues {
		if issue.State == "silenced" {
			return "SILENCED"
		}
		if assignmentIssueAckActive(issue, now) {
			return "ACKED"
		}
		if issue.State == "open" || issue.State == "" {
			return "OPEN"
		}
	}
	return "NONE"
}

func (s *Service) AcknowledgeAssignmentIssue(courseSlug, assignmentID, eventSlug, until, reason, actor string) (string, error) {
	eventType, ok := assignmentEventTypeFromSlug(eventSlug)
	if !ok {
		return "", fmt.Errorf("지원하지 않는 assignment event입니다")
	}
	ackUntil, silenced, ok := parseAssignmentAckUntil(until, time.Now())
	if !ok {
		return "", fmt.Errorf("지원하지 않는 until 값입니다")
	}
	reason = strings.TrimSpace(security.SanitizeText(reason))
	if reason == "" {
		return "", fmt.Errorf("reason is required")
	}
	key := assignmentIssueKey(eventType, courseSlug, assignmentID)
	err := s.store.Update(func(data *state.Data) {
		issue := data.AssignmentIssues[key]
		issue.IssueKey = key
		issue.EventType = eventType
		issue.CourseSlug = courseSlug
		issue.AssignmentID = assignmentID
		issue.AckBy = firstNonEmpty(actor, "discord")
		issue.AckReason = reason
		issue.AckUntil = ackUntil
		if silenced {
			issue.State = "silenced"
		} else {
			issue.State = "acknowledged"
		}
		data.AssignmentIssues[key] = issue
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("assignment issue acknowledged\ncourse: %s\nassignmentId: %s\nevent: %s\nuntil: %s\nreason: %s", security.SanitizeText(courseSlug), security.SanitizeText(assignmentID), eventSlug, formatKST(ackUntil), reason), nil
}

func (s *Service) UnacknowledgeAssignmentIssue(courseSlug, assignmentID, eventSlug string) (string, error) {
	eventType, ok := assignmentEventTypeFromSlug(eventSlug)
	if !ok {
		return "", fmt.Errorf("지원하지 않는 assignment event입니다")
	}
	key := assignmentIssueKey(eventType, courseSlug, assignmentID)
	err := s.store.Update(func(data *state.Data) {
		issue := data.AssignmentIssues[key]
		issue.IssueKey = key
		issue.EventType = eventType
		issue.CourseSlug = courseSlug
		issue.AssignmentID = assignmentID
		issue.State = "open"
		issue.AckBy = ""
		issue.AckReason = ""
		issue.AckUntil = time.Time{}
		data.AssignmentIssues[key] = issue
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("assignment issue ack cleared\ncourse: %s\nassignmentId: %s\nevent: %s", security.SanitizeText(courseSlug), security.SanitizeText(assignmentID), eventSlug), nil
}

func assignmentIssuesFor(data state.Data, courseSlug, assignmentID string) []state.AssignmentIssueState {
	issues := make([]state.AssignmentIssueState, 0, 4)
	for _, issue := range data.AssignmentIssues {
		if issue.CourseSlug == courseSlug && issue.AssignmentID == assignmentID {
			issues = append(issues, issue)
		}
	}
	return issues
}

func activeAssignmentIssues(issues []state.AssignmentIssueState) []state.AssignmentIssueState {
	active := make([]state.AssignmentIssueState, 0, len(issues))
	for _, issue := range issues {
		if issue.State != "resolved" {
			active = append(active, issue)
		}
	}
	return active
}

func assignmentDiagnosisStatus(diagnoses []AssignmentDiagnosis) string {
	status := reportadmin.StatusOK
	for _, diagnosis := range diagnoses {
		if diagnosis.Severity == "WARN" {
			status = reportadmin.StatusWarn
		}
		if diagnosis.Severity == "ERROR" {
			return reportadmin.StatusError
		}
	}
	return status
}

func assignmentEventTypeFromSlug(slug string) (string, bool) {
	switch strings.TrimSpace(slug) {
	case "publish-delayed":
		return "ASSIGNMENT_PUBLISH_DELAYED", true
	case "draft-past-start":
		return "ASSIGNMENT_DRAFT_PAST_START", true
	case "stale-draft":
		return "ASSIGNMENT_STALE_DRAFT", true
	case "invalid-time":
		return "ASSIGNMENT_INVALID_TIME", true
	case "missing-problem":
		return "ASSIGNMENT_MISSING_PROBLEM", true
	case "grading-failed":
		return "GRADING_FAILED", true
	default:
		return "", false
	}
}

func parseAssignmentAckUntil(value string, now time.Time) (time.Time, bool, bool) {
	switch strings.TrimSpace(value) {
	case "1h":
		return now.Add(time.Hour), false, true
	case "6h":
		return now.Add(6 * time.Hour), false, true
	case "1d":
		return now.Add(24 * time.Hour), false, true
	case "7d":
		return now.Add(7 * 24 * time.Hour), false, true
	case "forever":
		return time.Time{}, true, true
	default:
		return time.Time{}, false, false
	}
}
