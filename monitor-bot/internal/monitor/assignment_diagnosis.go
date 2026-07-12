package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func severityRank(severity string) int {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "CRITICAL":
		return 4
	case "ERROR":
		return 3
	case "WARN":
		return 2
	case "INFO":
		return 1
	default:
		return 0
	}
}

func ClassifyCourse(course reportadmin.Course, now time.Time) string {
	status := strings.ToUpper(strings.TrimSpace(course.Status))
	switch status {
	case "CLOSED", "ARCHIVED", "ENDED", "LEGACY", "INACTIVE":
		return CourseLegacy
	}
	if end, ok := parseAssignmentTime(course.EndAt); ok && now.After(end) {
		return CourseLegacy
	}
	if strings.TrimSpace(course.Status) == "" && strings.TrimSpace(course.StartAt) == "" && strings.TrimSpace(course.EndAt) == "" {
		return CourseUnknown
	}
	return CourseActive
}

func assignmentSnapshot(course reportadmin.Course, assignment reportadmin.Assignment, now time.Time) state.AssignmentSnapshot {
	return state.AssignmentSnapshot{
		CourseSlug:         course.Slug,
		CourseClass:        CourseActive,
		AssignmentID:       assignment.ID,
		Title:              assignment.Title,
		Status:             assignment.Status,
		PublishedAt:        assignment.PublishedAt,
		PublishedAtOmitted: assignment.PublishedAtOmitted,
		StartAt:            assignment.StartAt,
		EndAt:              assignment.EndAt,
		ProblemID:          assignment.ProblemID,
		ProblemIDFallback:  assignment.ProblemIDFallback,
		UpdatedAt:          assignment.UpdatedAt,
		LastSeenAt:         now,
	}
}

func mergeAssignmentDetail(summary, detail reportadmin.Assignment) reportadmin.Assignment {
	merged := summary
	if strings.TrimSpace(detail.ID) != "" {
		merged.ID = detail.ID
	}
	if strings.TrimSpace(detail.Title) != "" {
		merged.Title = detail.Title
	}
	if strings.TrimSpace(detail.Status) != "" {
		merged.Status = detail.Status
	}
	if strings.TrimSpace(detail.PublishedAt) != "" {
		merged.PublishedAt = detail.PublishedAt
	}
	merged.PublishedAtOmitted = false
	if strings.TrimSpace(detail.StartAt) != "" {
		merged.StartAt = detail.StartAt
	}
	if strings.TrimSpace(detail.EndAt) != "" {
		merged.EndAt = detail.EndAt
	}
	if strings.TrimSpace(detail.ProblemID) != "" {
		merged.ProblemID = detail.ProblemID
		merged.ProblemIDFallback = detail.ProblemIDFallback
	}
	if strings.TrimSpace(detail.UpdatedAt) != "" {
		merged.UpdatedAt = detail.UpdatedAt
	}
	if detail.Raw != nil {
		merged.Raw = detail.Raw
	}
	return merged
}

func hasAssignmentDetail(assignment reportadmin.Assignment) bool {
	return strings.TrimSpace(assignment.ID) != "" ||
		strings.TrimSpace(assignment.Title) != "" ||
		strings.TrimSpace(assignment.Status) != "" ||
		strings.TrimSpace(assignment.PublishedAt) != "" ||
		strings.TrimSpace(assignment.StartAt) != "" ||
		strings.TrimSpace(assignment.EndAt) != "" ||
		strings.TrimSpace(assignment.ProblemID) != "" ||
		strings.TrimSpace(assignment.UpdatedAt) != ""
}

func diffAssignmentSnapshot(prev, cur state.AssignmentSnapshot, existed bool, now time.Time) []state.AssignmentEventState {
	var events []state.AssignmentEventState
	if !existed {
		events = append(events, makeAssignmentEvent("ASSIGNMENT_CREATED", "INFO", cur, "과제 등록 확인", "created", cur.AssignmentID, now))
	}
	if existed && !isPublished(prev.Status) && isPublished(cur.Status) {
		events = append(events, makeAssignmentEvent("ASSIGNMENT_PUBLISHED", "INFO", cur, "과제 공개 완료", "status", cur.Status, now))
	}
	if existed && assignmentMajorFieldsChanged(prev, cur) {
		events = append(events, makeAssignmentEvent("ASSIGNMENT_UPDATED", "INFO", cur, "과제 주요 필드 변경", "updated", cur.UpdatedAt+cur.Status+cur.StartAt+cur.EndAt+cur.ProblemID, now))
	}
	if existed && cur.Submitted > prev.Submitted {
		events = append(events, makeAssignmentEvent("SUBMISSION_COUNT_CHANGED", "INFO", cur, fmt.Sprintf("제출 수 +%d", cur.Submitted-prev.Submitted), "submitted", fmt.Sprint(cur.Submitted), now))
	}
	if existed && cur.Graded > prev.Graded {
		events = append(events, makeAssignmentEvent("GRADING_COMPLETED", "INFO", cur, fmt.Sprintf("채점 완료 +%d명", cur.Graded-prev.Graded), "graded", fmt.Sprint(cur.Graded), now))
	}
	if existed && cur.Failed > prev.Failed {
		events = append(events, makeAssignmentEvent("GRADING_FAILED", "WARN", cur, fmt.Sprintf("채점 실패 +%d건", cur.Failed-prev.Failed), "failed", fmt.Sprint(cur.Failed), now))
	}
	return events
}

func makeAssignmentEvent(eventType, severity string, snapshot state.AssignmentSnapshot, summary, changedField, newValue string, now time.Time) state.AssignmentEventState {
	fingerprint := strings.Join([]string{eventType, snapshot.CourseSlug, snapshot.AssignmentID, changedField, newValue}, ":")
	return state.AssignmentEventState{
		Fingerprint:        fingerprint,
		EventType:          eventType,
		Severity:           severity,
		CourseSlug:         snapshot.CourseSlug,
		AssignmentID:       snapshot.AssignmentID,
		Title:              snapshot.Title,
		Status:             snapshot.Status,
		PublishedAt:        snapshot.PublishedAt,
		PublishedAtOmitted: snapshot.PublishedAtOmitted,
		StartAt:            snapshot.StartAt,
		EndAt:              snapshot.EndAt,
		ProblemID:          snapshot.ProblemID,
		ProblemIDFallback:  snapshot.ProblemIDFallback,
		Summary:            summary,
		ShouldNotify:       true,
		CreatedAt:          now,
	}
}

func assignmentMajorFieldsChanged(prev, cur state.AssignmentSnapshot) bool {
	return prev.Title != cur.Title ||
		prev.Status != cur.Status ||
		prev.StartAt != cur.StartAt ||
		prev.EndAt != cur.EndAt ||
		prev.PublishedAt != cur.PublishedAt ||
		prev.PublishedAtOmitted != cur.PublishedAtOmitted ||
		prev.ProblemID != cur.ProblemID ||
		prev.ProblemIDFallback != cur.ProblemIDFallback
}

func invalidAssignmentTime(snapshot state.AssignmentSnapshot) bool {
	start, startOK := parseAssignmentTime(snapshot.StartAt)
	end, endOK := parseAssignmentTime(snapshot.EndAt)
	if !startOK || !endOK {
		return true
	}
	return end.Before(start)
}

func diagnoseAssignment(snapshot state.AssignmentSnapshot, now time.Time, staleGrace time.Duration) []AssignmentDiagnosis {
	if staleGrace <= 0 {
		staleGrace = 7 * 24 * time.Hour
	}
	diagnoses := make([]AssignmentDiagnosis, 0, 2)
	if invalidAssignmentTime(snapshot) {
		diagnoses = append(diagnoses, AssignmentDiagnosis{
			EventType:    "ASSIGNMENT_INVALID_TIME",
			Severity:     "WARN",
			ReasonCode:   "ASSIGNMENT_TIME_INVALID",
			ReasonText:   "startAt/endAt이 비어 있거나 endAt이 startAt보다 빠릅니다.",
			Evidence:     assignmentEvidence(snapshot),
			ShouldNotify: true,
			ShouldCount:  true,
		})
	}
	if isPublished(snapshot.Status) {
		if strings.TrimSpace(snapshot.ProblemID) == "" {
			diagnoses = append(diagnoses, AssignmentDiagnosis{
				EventType:    "ASSIGNMENT_MISSING_PROBLEM",
				Severity:     "WARN",
				ReasonCode:   "PUBLISHED_ASSIGNMENT_PROBLEM_MISSING",
				ReasonText:   "공개된 과제에 problemId가 없어 제출/채점 연결을 확인해야 합니다.",
				Evidence:     assignmentEvidence(snapshot),
				ShouldNotify: true,
				ShouldCount:  true,
			})
		}
		return diagnoses
	}
	if isDraft(snapshot.Status) {
		if end, ok := parseAssignmentTime(snapshot.EndAt); ok && now.After(end.Add(staleGrace)) {
			diagnoses = append(diagnoses, AssignmentDiagnosis{
				EventType:    "ASSIGNMENT_STALE_DRAFT",
				Severity:     "INFO",
				ReasonCode:   "ASSIGNMENT_WINDOW_STALE",
				ReasonText:   "endAt과 stale grace가 모두 지난 DRAFT 과제입니다. 공개 지연 feed가 아니라 정리/수동 점검 대상으로 분류합니다.",
				Evidence:     assignmentEvidence(snapshot),
				ShouldNotify: false,
				ShouldCount:  true,
			})
			return diagnoses
		}
	}
	if publishedAt, ok := parseAssignmentTime(snapshot.PublishedAt); ok && now.After(publishedAt) {
		diagnoses = append(diagnoses, AssignmentDiagnosis{
			EventType:    "ASSIGNMENT_PUBLISH_DELAYED",
			Severity:     "WARN",
			ReasonCode:   "PUBLISHED_AT_PAST_STATUS_NOT_PUBLISHED",
			ReasonText:   "publishedAt이 현재보다 과거이고 status가 published/open이 아닙니다.",
			Evidence:     assignmentEvidence(snapshot),
			ShouldNotify: true,
			ShouldCount:  true,
		})
		return diagnoses
	}
	if strings.TrimSpace(snapshot.PublishedAt) == "" && isDraft(snapshot.Status) && !snapshot.PublishedAtOmitted {
		if startAt, ok := parseAssignmentTime(snapshot.StartAt); ok && now.After(startAt) {
			diagnoses = append(diagnoses, AssignmentDiagnosis{
				EventType:    "ASSIGNMENT_DRAFT_PAST_START",
				Severity:     "WARN",
				ReasonCode:   "PUBLISHED_AT_MISSING_DRAFT_START_PAST",
				ReasonText:   "publishedAt이 없어 공개 지연으로 단정할 수 없습니다. status가 DRAFT이고 startAt이 지났으므로 draft-past-start로 분류합니다.",
				Evidence:     assignmentEvidence(snapshot),
				ShouldNotify: true,
				ShouldCount:  true,
			})
		}
	}
	return diagnoses
}

func makeAssignmentIssueEvent(snapshot state.AssignmentSnapshot, diagnosis AssignmentDiagnosis, now time.Time) state.AssignmentEventState {
	issueKey := assignmentIssueKey(diagnosis.EventType, snapshot.CourseSlug, snapshot.AssignmentID)
	evidenceHash := assignmentEvidenceHash(snapshot, diagnosis)
	return state.AssignmentEventState{
		Fingerprint:        issueKey + ":" + evidenceHash,
		IssueKey:           issueKey,
		EventType:          diagnosis.EventType,
		Severity:           diagnosis.Severity,
		CourseSlug:         snapshot.CourseSlug,
		AssignmentID:       snapshot.AssignmentID,
		Title:              snapshot.Title,
		Status:             snapshot.Status,
		PublishedAt:        snapshot.PublishedAt,
		PublishedAtOmitted: snapshot.PublishedAtOmitted,
		StartAt:            snapshot.StartAt,
		EndAt:              snapshot.EndAt,
		ProblemID:          snapshot.ProblemID,
		ProblemIDFallback:  snapshot.ProblemIDFallback,
		Summary:            assignmentDiagnosisSummary(diagnosis.EventType),
		ReasonCode:         diagnosis.ReasonCode,
		ReasonText:         diagnosis.ReasonText,
		Evidence:           diagnosis.Evidence,
		EvidenceHash:       evidenceHash,
		ShouldNotify:       diagnosis.ShouldNotify,
		ShouldCount:        diagnosis.ShouldCount,
		CreatedAt:          now,
	}
}

func assignmentEvidence(snapshot state.AssignmentSnapshot) []string {
	evidence := []string{
		"status=" + unknownIfBlank(snapshot.Status),
		"publishedAt=" + assignmentPublishedAtEvidence(snapshot),
		"startAt=" + unknownIfBlank(snapshot.StartAt),
		"endAt=" + unknownIfBlank(snapshot.EndAt),
		"problemId=" + unknownIfBlank(snapshot.ProblemID),
	}
	if strings.TrimSpace(snapshot.ProblemIDFallback) != "" {
		evidence = append(evidence, "problemIdFallback: "+snapshot.ProblemIDFallback)
	}
	return evidence
}

func assignmentPublishedAtEvidence(snapshot state.AssignmentSnapshot) string {
	if strings.TrimSpace(snapshot.PublishedAt) != "" {
		return snapshot.PublishedAt
	}
	if snapshot.PublishedAtOmitted {
		return "summary omitted"
	}
	return "unknown"
}

func assignmentEvidenceHash(snapshot state.AssignmentSnapshot, diagnosis AssignmentDiagnosis) string {
	source := strings.Join([]string{
		diagnosis.EventType,
		diagnosis.Severity,
		diagnosis.ReasonCode,
		snapshot.Title,
		snapshot.Status,
		snapshot.PublishedAt,
		fmt.Sprint(snapshot.PublishedAtOmitted),
		snapshot.StartAt,
		snapshot.EndAt,
		snapshot.ProblemID,
		snapshot.ProblemIDFallback,
	}, "\x00")
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:8])
}

func assignmentDiagnosisSummary(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_PUBLISH_DELAYED":
		return "과제 공개 예정 시간이 지났지만 공개 상태가 아닙니다."
	case "ASSIGNMENT_DRAFT_PAST_START":
		return "공개 지연으로 단정할 수 없는 DRAFT 과제입니다."
	case "ASSIGNMENT_STALE_DRAFT":
		return "오래된 DRAFT 과제입니다."
	case "ASSIGNMENT_INVALID_TIME":
		return "과제 시간 설정 이상"
	case "ASSIGNMENT_MISSING_PROBLEM":
		return "공개 과제의 problemId가 비어 있습니다."
	default:
		return "과제 상태 점검 필요"
	}
}

func isAssignmentIssueEvent(eventType string) bool {
	switch eventType {
	case "ASSIGNMENT_PUBLISH_DELAYED", "ASSIGNMENT_DRAFT_PAST_START", "ASSIGNMENT_STALE_DRAFT", "ASSIGNMENT_INVALID_TIME", "ASSIGNMENT_MISSING_PROBLEM":
		return true
	default:
		return false
	}
}

func isDraft(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "draft")
}

func assignmentIssueKey(eventType, courseSlug, assignmentID string) string {
	return strings.Join([]string{"assignment", eventType, courseSlug, assignmentID}, ":")
}

func unknownIfBlank(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func isPublished(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	return normalized == "published" || normalized == "open" || normalized == "opened"
}

func parseAssignmentTime(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05-07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func isToday(value string, now time.Time) bool {
	parsed, ok := parseAssignmentTime(value)
	if !ok {
		return false
	}
	kst := time.FixedZone("KST", 9*60*60)
	p, n := parsed.In(kst), now.In(kst)
	return p.Year() == n.Year() && p.YearDay() == n.YearDay()
}
