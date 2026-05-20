package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

const (
	CourseActive  = "ACTIVE"
	CourseLegacy  = "LEGACY"
	CourseUnknown = "UNKNOWN"
)

type assignmentPollResult struct {
	UpdatedAt             time.Time
	APIStatus             string
	APIFinding            string
	ActiveCourses         int
	LegacyCourses         int
	UnknownCourses        int
	TodayPlanned          int
	PublishedToday        int
	PublishDelayed        int
	AssignmentIssues      int
	GradingInProgress     int
	GradingCompletedDelta int
	GradingFailedDelta    int
	Snapshots             map[string]state.AssignmentSnapshot
	IssueStates           map[string]state.AssignmentIssueState
	Events                []state.AssignmentEventState
	RecentEvents          []state.AssignmentEventState
}

type AssignmentDiagnosis struct {
	EventType    string
	Severity     string
	ReasonCode   string
	ReasonText   string
	Evidence     []string
	ShouldNotify bool
	ShouldCount  bool
}

func (s *Service) assignmentOpsLoop(ctx context.Context) {
	for {
		if err := s.RefreshAssignmentOps(ctx); err != nil {
			log.Printf("assignment ops refresh failed: %v", err)
		}
		timer := time.NewTimer(s.assignmentOpsInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) assignmentOpsInterval() time.Duration {
	if s.cfg.Alert.PollInterval > 0 {
		return s.cfg.Alert.PollInterval
	}
	if s.cfg.Dashboard.RefreshInterval > 0 {
		return s.cfg.Dashboard.RefreshInterval
	}
	return 3 * time.Minute
}

func (s *Service) assignmentStaleGrace() time.Duration {
	if s.cfg.Alert.AssignmentStaleGrace > 0 {
		return s.cfg.Alert.AssignmentStaleGrace
	}
	return 7 * 24 * time.Hour
}

func (s *Service) assignmentOpsChannelID() string {
	return s.generalAlertChannelID()
}

func (s *Service) RefreshAssignmentOps(ctx context.Context) error {
	channelID := s.assignmentOpsChannelID()
	if channelID == "" {
		return nil
	}
	result := s.collectAssignmentOps(ctx, time.Now())
	if err := s.upsertAssignmentDashboard(ctx, channelID, formatAssignmentDashboard(result)); err != nil {
		log.Printf("assignment dashboard update failed: %v", err)
	}
	for _, event := range result.Events {
		if shouldSendAssignmentEvent(event) {
			if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, formatAssignmentEvent(event)); err != nil {
				log.Printf("assignment event send failed: %v", err)
			}
		}
	}
	s.refreshAssignmentAuditEvents(ctx, channelID)
	return nil
}

func (s *Service) collectAssignmentOps(ctx context.Context, now time.Time) assignmentPollResult {
	result := assignmentPollResult{
		UpdatedAt:    now,
		APIStatus:    reportadmin.StatusOK,
		Snapshots:    map[string]state.AssignmentSnapshot{},
		IssueStates:  map[string]state.AssignmentIssueState{},
		RecentEvents: s.store.Snapshot().RecentAssignmentEvents,
	}
	courses, err := s.report.ListCourses(ctx)
	if err != nil {
		result.APIStatus = reportadmin.StatusOf(err)
		result.APIFinding = security.SanitizeText(err.Error())
		result.Events = append(result.Events, state.AssignmentEventState{
			Fingerprint:  "web-admin-api:" + result.APIStatus,
			EventType:    "WEB_ADMIN_API_" + result.APIStatus,
			Severity:     severityForAPIStatus(result.APIStatus),
			Summary:      "WEB Admin API 조회 실패: " + result.APIStatus,
			ShouldNotify: true,
			CreatedAt:    now,
		})
		result.Events = s.applyAssignmentEventLifecycle(result.Events, nil, now, &result)
		_ = s.persistAssignmentOps(result)
		return withRecentAssignmentEvents(result, s.store.Snapshot().RecentAssignmentEvents)
	}
	activeCourses := make([]reportadmin.Course, 0, len(courses))
	for _, course := range courses {
		class := ClassifyCourse(course, now)
		switch class {
		case CourseActive:
			result.ActiveCourses++
			activeCourses = append(activeCourses, course)
		case CourseLegacy:
			result.LegacyCourses++
		default:
			result.UnknownCourses++
		}
	}

	previous := s.store.Snapshot()
	submissionBudget := 30
	for _, course := range activeCourses {
		assignments, err := s.report.ListAssignments(ctx, course.Slug)
		if err != nil {
			result.APIStatus = reportadmin.StatusOf(err)
			result.APIFinding = "course " + security.SanitizeText(course.Slug) + " 조회 실패: " + result.APIStatus
			continue
		}
		for _, assignment := range assignments {
			snapshot := assignmentSnapshot(course, assignment, now)
			if submissionBudget > 0 {
				if summary, err := s.report.SubmissionStatuses(ctx, course.Slug, assignment.ID); err == nil {
					snapshot.Submitted = summary.Submitted
					snapshot.Graded = summary.Graded
					snapshot.Pending = summary.Pending
					snapshot.Failed = summary.Failed
					snapshot.AverageScore = summary.AverageScore
					submissionBudget--
				}
			}
			result.Snapshots[snapshotKey(course.Slug, assignment.ID)] = snapshot
			result.TodayPlanned += boolInt(isToday(snapshot.PublishedAt, now) || isToday(snapshot.StartAt, now))
			result.PublishedToday += boolInt(isPublished(snapshot.Status) && (isToday(snapshot.PublishedAt, now) || isToday(snapshot.UpdatedAt, now)))
			for _, diagnosis := range diagnoseAssignment(snapshot, now, s.assignmentStaleGrace()) {
				if diagnosis.ShouldCount {
					result.AssignmentIssues++
				}
				if diagnosis.EventType == "ASSIGNMENT_PUBLISH_DELAYED" && diagnosis.ShouldCount {
					result.PublishDelayed++
				}
				if previous.AssignmentBaselineInitialized {
					result.Events = append(result.Events, makeAssignmentIssueEvent(snapshot, diagnosis, now))
				}
			}
			if snapshot.Pending > 0 {
				result.GradingInProgress++
			}
			if previous.AssignmentBaselineInitialized {
				prev, existed := previous.AssignmentSnapshots[snapshotKey(course.Slug, assignment.ID)]
				result.Events = append(result.Events, diffAssignmentSnapshot(prev, snapshot, existed, now)...)
			}
		}
	}
	result.Events = s.applyAssignmentEventLifecycle(result.Events, result.Snapshots, now, &result)
	for _, event := range result.Events {
		if event.EventType == "GRADING_COMPLETED" {
			result.GradingCompletedDelta++
		}
		if event.EventType == "GRADING_FAILED" {
			result.GradingFailedDelta++
		}
	}
	_ = s.persistAssignmentOps(result)
	return withRecentAssignmentEvents(result, s.store.Snapshot().RecentAssignmentEvents)
}

func (s *Service) persistAssignmentOps(result assignmentPollResult) error {
	return s.store.Update(func(data *state.Data) {
		if data.AssignmentSnapshots == nil {
			data.AssignmentSnapshots = map[string]state.AssignmentSnapshot{}
		}
		if result.APIStatus == reportadmin.StatusOK {
			data.AssignmentSnapshots = result.Snapshots
			data.AssignmentBaselineInitialized = true
		}
		if result.IssueStates != nil {
			if data.AssignmentIssues == nil {
				data.AssignmentIssues = map[string]state.AssignmentIssueState{}
			}
			for key, issue := range result.IssueStates {
				data.AssignmentIssues[key] = issue
			}
		}
		data.LastAssignmentOpsUpdatedAt = result.UpdatedAt
		for _, event := range result.Events {
			if !isAssignmentIssueEvent(event.EventType) {
				data.AssignmentEventFingerprints[event.Fingerprint] = state.AlertState{Active: true, LastSentAt: result.UpdatedAt}
			}
			data.RecentAssignmentEvents = append([]state.AssignmentEventState{event}, data.RecentAssignmentEvents...)
		}
		if len(data.RecentAssignmentEvents) > 20 {
			data.RecentAssignmentEvents = data.RecentAssignmentEvents[:20]
		}
	})
}

func (s *Service) upsertAssignmentDashboard(ctx context.Context, channelID, content string) error {
	snapshot := s.store.Snapshot()
	if messageID := strings.TrimSpace(snapshot.AssignmentOpsMessageID); messageID != "" {
		if err := s.discord.EditChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, messageID, content); err == nil {
			return s.store.Update(func(data *state.Data) {
				data.LastAssignmentOpsUpdatedAt = time.Now()
			})
		}
	}
	msg, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, content)
	if err != nil {
		return err
	}
	return s.store.Update(func(data *state.Data) {
		data.AssignmentOpsMessageID = msg.ID
		data.LastAssignmentOpsUpdatedAt = time.Now()
	})
}

func (s *Service) applyAssignmentEventLifecycle(events []state.AssignmentEventState, snapshots map[string]state.AssignmentSnapshot, now time.Time, result *assignmentPollResult) []state.AssignmentEventState {
	snapshot := s.store.Snapshot()
	cooldown := s.cfg.Alert.Cooldown
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	filtered := make([]state.AssignmentEventState, 0, len(events))
	observedIssues := make(map[string]struct{})
	if result != nil && result.IssueStates == nil {
		result.IssueStates = map[string]state.AssignmentIssueState{}
	}
	for _, event := range events {
		if isAssignmentIssueEvent(event.EventType) {
			updated, include := applyAssignmentIssueState(snapshot.AssignmentIssues[event.IssueKey], event, now)
			observedIssues[event.IssueKey] = struct{}{}
			if result != nil {
				result.IssueStates[event.IssueKey] = updated
			}
			if include {
				filtered = append(filtered, enrichAssignmentIssueEvent(event, updated))
			}
			continue
		}
		existing := snapshot.AssignmentEventFingerprints[event.Fingerprint]
		if !existing.LastSentAt.IsZero() && now.Sub(existing.LastSentAt) < cooldown {
			continue
		}
		filtered = append(filtered, event)
	}
	if snapshots != nil && result != nil {
		for key, issue := range snapshot.AssignmentIssues {
			if _, ok := observedIssues[key]; ok || issue.IssueKey == "" || issue.State == "resolved" {
				continue
			}
			if _, ok := snapshots[snapshotKey(issue.CourseSlug, issue.AssignmentID)]; !ok {
				continue
			}
			issue.State = "resolved"
			issue.ResolvedAt = now
			issue.LastDetectedAt = now
			result.IssueStates[key] = issue
		}
	}
	return filtered
}

func applyAssignmentIssueState(existing state.AssignmentIssueState, event state.AssignmentEventState, now time.Time) (state.AssignmentIssueState, bool) {
	first := existing.IssueKey == ""
	wasResolved := existing.State == "resolved"
	ackActive := assignmentIssueAckActive(existing, now)
	ackExpired := existing.State == "acknowledged" && !assignmentIssueAckActive(existing, now)
	severityIncreased := !first && severityRank(event.Severity) > severityRank(existing.Severity)
	evidenceChanged := !first && strings.TrimSpace(event.EvidenceHash) != "" && existing.EvidenceHash != "" && event.EvidenceHash != existing.EvidenceHash

	updated := state.AssignmentIssueState{
		IssueKey:        event.IssueKey,
		EventType:       event.EventType,
		Severity:        event.Severity,
		CourseSlug:      event.CourseSlug,
		AssignmentID:    event.AssignmentID,
		Title:           event.Title,
		Status:          event.Status,
		PublishedAt:     event.PublishedAt,
		StartAt:         event.StartAt,
		EndAt:           event.EndAt,
		ProblemID:       event.ProblemID,
		FirstDetectedAt: existing.FirstDetectedAt,
		LastDetectedAt:  now,
		LastNotifiedAt:  existing.LastNotifiedAt,
		State:           "open",
		NotifyCount:     existing.NotifyCount,
		EvidenceHash:    event.EvidenceHash,
		ReasonCode:      event.ReasonCode,
		ReasonText:      event.ReasonText,
		AckBy:           existing.AckBy,
		AckReason:       existing.AckReason,
		AckUntil:        existing.AckUntil,
	}
	if updated.FirstDetectedAt.IsZero() || wasResolved {
		updated.FirstDetectedAt = now
	}
	if ackActive {
		updated.State = existing.State
		if updated.State == "" {
			updated.State = "acknowledged"
		}
		return updated, false
	}
	if ackExpired {
		updated.AckBy = ""
		updated.AckReason = ""
		updated.AckUntil = time.Time{}
	}

	shouldNotify := event.ShouldNotify && (first || wasResolved || severityIncreased || evidenceChanged || ackExpired)
	if shouldNotify {
		updated.LastNotifiedAt = now
		updated.NotifyCount++
		return updated, true
	}
	if first && !event.ShouldNotify {
		return updated, true
	}
	return updated, false
}

func assignmentIssueAckActive(issue state.AssignmentIssueState, now time.Time) bool {
	switch issue.State {
	case "silenced":
		return true
	case "acknowledged":
		return !issue.AckUntil.IsZero() && now.Before(issue.AckUntil)
	default:
		return false
	}
}

func enrichAssignmentIssueEvent(event state.AssignmentEventState, issue state.AssignmentIssueState) state.AssignmentEventState {
	event.IssueState = issue.State
	event.FirstDetectedAt = issue.FirstDetectedAt
	event.LastDetectedAt = issue.LastDetectedAt
	event.LastNotifiedAt = issue.LastNotifiedAt
	event.NotifyCount = issue.NotifyCount
	event.RepeatPolicy = "same open issue is suppressed until resolved, acknowledged, or evidence changes"
	if !event.ShouldNotify {
		event.RepeatPolicy = "dashboard/count only; Discord feed suppressed"
	}
	return event
}

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
		CourseSlug:   course.Slug,
		CourseClass:  CourseActive,
		AssignmentID: assignment.ID,
		Title:        assignment.Title,
		Status:       assignment.Status,
		PublishedAt:  assignment.PublishedAt,
		StartAt:      assignment.StartAt,
		EndAt:        assignment.EndAt,
		ProblemID:    assignment.ProblemID,
		UpdatedAt:    assignment.UpdatedAt,
		LastSeenAt:   now,
	}
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
		Fingerprint:  fingerprint,
		EventType:    eventType,
		Severity:     severity,
		CourseSlug:   snapshot.CourseSlug,
		AssignmentID: snapshot.AssignmentID,
		Title:        snapshot.Title,
		Status:       snapshot.Status,
		PublishedAt:  snapshot.PublishedAt,
		StartAt:      snapshot.StartAt,
		EndAt:        snapshot.EndAt,
		ProblemID:    snapshot.ProblemID,
		Summary:      summary,
		ShouldNotify: true,
		CreatedAt:    now,
	}
}

func assignmentMajorFieldsChanged(prev, cur state.AssignmentSnapshot) bool {
	return prev.Title != cur.Title ||
		prev.Status != cur.Status ||
		prev.StartAt != cur.StartAt ||
		prev.EndAt != cur.EndAt ||
		prev.PublishedAt != cur.PublishedAt ||
		prev.ProblemID != cur.ProblemID
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
	if strings.TrimSpace(snapshot.PublishedAt) == "" && isDraft(snapshot.Status) {
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
		Fingerprint:  issueKey + ":" + evidenceHash,
		IssueKey:     issueKey,
		EventType:    diagnosis.EventType,
		Severity:     diagnosis.Severity,
		CourseSlug:   snapshot.CourseSlug,
		AssignmentID: snapshot.AssignmentID,
		Title:        snapshot.Title,
		Status:       snapshot.Status,
		PublishedAt:  snapshot.PublishedAt,
		StartAt:      snapshot.StartAt,
		EndAt:        snapshot.EndAt,
		ProblemID:    snapshot.ProblemID,
		Summary:      assignmentDiagnosisSummary(diagnosis.EventType),
		ReasonCode:   diagnosis.ReasonCode,
		ReasonText:   diagnosis.ReasonText,
		Evidence:     diagnosis.Evidence,
		EvidenceHash: evidenceHash,
		ShouldNotify: diagnosis.ShouldNotify,
		ShouldCount:  diagnosis.ShouldCount,
		CreatedAt:    now,
	}
}

func assignmentEvidence(snapshot state.AssignmentSnapshot) []string {
	return []string{
		"status=" + unknownIfBlank(snapshot.Status),
		"publishedAt=" + unknownIfBlank(snapshot.PublishedAt),
		"startAt=" + unknownIfBlank(snapshot.StartAt),
		"endAt=" + unknownIfBlank(snapshot.EndAt),
		"problemId=" + unknownIfBlank(snapshot.ProblemID),
	}
}

func assignmentEvidenceHash(snapshot state.AssignmentSnapshot, diagnosis AssignmentDiagnosis) string {
	source := strings.Join([]string{
		diagnosis.EventType,
		diagnosis.Severity,
		diagnosis.ReasonCode,
		snapshot.Title,
		snapshot.Status,
		snapshot.PublishedAt,
		snapshot.StartAt,
		snapshot.EndAt,
		snapshot.ProblemID,
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

func formatAssignmentDashboard(result assignmentPollResult) string {
	status := "정상"
	if result.APIStatus != reportadmin.StatusOK || result.AssignmentIssues > 0 || result.GradingFailedDelta > 0 {
		status = "주의"
	}
	if result.APIStatus == reportadmin.StatusAuthError || result.APIStatus == reportadmin.StatusForbidden || result.APIStatus == reportadmin.StatusUpstreamError {
		status = "장애"
	}
	var b strings.Builder
	b.WriteString("📌 A&I 과제 운영 대시보드\n\n")
	fmt.Fprintf(&b, "마지막 업데이트: %s KST\n", result.UpdatedAt.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "상태: %s\n", status)
	if result.APIStatus != reportadmin.StatusOK {
		fmt.Fprintf(&b, "WEB Admin API: %s %s\n", result.APIStatus, security.SanitizeText(result.APIFinding))
	}
	fmt.Fprintf(&b, "\n운영 중인 코스: %d개\n", result.ActiveCourses)
	fmt.Fprintf(&b, "레거시 코스: %d개\n", result.LegacyCourses)
	fmt.Fprintf(&b, "판단 불가 코스: %d개\n\n", result.UnknownCourses)
	fmt.Fprintf(&b, "오늘 공개 예정 과제: %d개\n", result.TodayPlanned)
	fmt.Fprintf(&b, "공개 완료: %d개\n", result.PublishedToday)
	fmt.Fprintf(&b, "공개 지연: %d개\n\n", result.PublishDelayed)
	fmt.Fprintf(&b, "상태 점검 대상: %d개\n\n", result.AssignmentIssues)
	fmt.Fprintf(&b, "채점 진행 중 과제: %d개\n", result.GradingInProgress)
	fmt.Fprintf(&b, "최근 채점 완료 업데이트: %d건\n", result.GradingCompletedDelta)
	fmt.Fprintf(&b, "채점 실패 감지: %d건\n\n", result.GradingFailedDelta)
	b.WriteString("최근 이벤트\n")
	recent := result.RecentEvents
	if len(recent) == 0 {
		b.WriteString("- 아직 이벤트 없음\n")
	} else {
		for i, event := range recent {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "%d. %s %s %s\n", i+1, eventIcon(event), security.SanitizeText(event.CourseSlug), security.SanitizeText(event.Summary))
		}
	}
	b.WriteString("\n상세 확인\n")
	b.WriteString("/ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis\n")
	b.WriteString("/ops assignment-events course:<courseSlug> id:<assignmentId>\n")
	b.WriteString("/ops submissions course:<courseSlug> assignment:<assignmentId>\n")
	b.WriteString("/ops logs service:report mode:recent query:<assignmentId> since:24h limit:20")
	return formatting.TruncateDiscordMessage(b.String())
}

func formatAssignmentEvent(event state.AssignmentEventState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", eventIcon(event), assignmentEventTitle(event.EventType))
	fmt.Fprintf(&b, "eventType: %s\n", event.EventType)
	fmt.Fprintf(&b, "severity: %s\n", event.Severity)
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(event.CourseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(event.AssignmentID))
	fmt.Fprintf(&b, "title: %s\n", security.SanitizeText(unknownIfBlank(event.Title)))
	fmt.Fprintf(&b, "status: %s\n", security.SanitizeText(unknownIfBlank(event.Status)))
	fmt.Fprintf(&b, "publishedAt: %s\n", security.SanitizeText(unknownIfBlank(event.PublishedAt)))
	fmt.Fprintf(&b, "startAt: %s\n", security.SanitizeText(unknownIfBlank(event.StartAt)))
	fmt.Fprintf(&b, "endAt: %s\n", security.SanitizeText(unknownIfBlank(event.EndAt)))
	fmt.Fprintf(&b, "problemId: %s\n", security.SanitizeText(unknownIfBlank(event.ProblemID)))
	fmt.Fprintf(&b, "summary: %s\n", security.SanitizeText(event.Summary))
	b.WriteString("source: WEB_ADMIN_API\n")
	if strings.TrimSpace(event.ReasonCode) != "" {
		fmt.Fprintf(&b, "\nreasonCode: %s\n", security.SanitizeText(event.ReasonCode))
		fmt.Fprintf(&b, "reasonText: %s\n", security.SanitizeText(event.ReasonText))
		if event.EventType == "ASSIGNMENT_DRAFT_PAST_START" {
			b.WriteString("note: publishedAt이 없어 공개 지연으로 단정할 수 없음\n")
		}
		if len(event.Evidence) > 0 {
			b.WriteString("evidence:\n")
			for _, evidence := range event.Evidence {
				fmt.Fprintf(&b, "- %s\n", security.SanitizeText(evidence))
			}
		}
	}
	if strings.TrimSpace(event.IssueState) != "" {
		b.WriteString("\nissue:\n")
		fmt.Fprintf(&b, "- state: %s\n", security.SanitizeText(event.IssueState))
		fmt.Fprintf(&b, "- firstDetectedAt: %s\n", formatKST(event.FirstDetectedAt))
		fmt.Fprintf(&b, "- lastDetectedAt: %s\n", formatKST(event.LastDetectedAt))
		fmt.Fprintf(&b, "- notifyCount: %d\n", event.NotifyCount)
		fmt.Fprintf(&b, "- repeatPolicy: %s\n", security.SanitizeText(event.RepeatPolicy))
	}
	writeAssignmentNextCommands(&b, event)
	return formatting.TruncateDiscordMessage(b.String())
}

func shouldSendAssignmentEvent(event state.AssignmentEventState) bool {
	if isAssignmentIssueEvent(event.EventType) {
		return event.ShouldNotify
	}
	switch event.EventType {
	case "ASSIGNMENT_CREATED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_PUBLISH_DELAYED", "ASSIGNMENT_INVALID_TIME", "GRADING_COMPLETED", "GRADING_FAILED":
		return true
	}
	return strings.HasPrefix(event.EventType, "WEB_ADMIN_API_")
}

func assignmentEventTitle(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_CREATED":
		return "과제 등록 확인"
	case "ASSIGNMENT_PUBLISHED":
		return "과제 공개 완료"
	case "ASSIGNMENT_PUBLISH_DELAYED":
		return "과제 공개 지연"
	case "ASSIGNMENT_DRAFT_PAST_START":
		return "과제 상태 점검 필요"
	case "ASSIGNMENT_STALE_DRAFT":
		return "오래된 DRAFT 과제"
	case "ASSIGNMENT_MISSING_PROBLEM":
		return "과제 problemId 누락"
	case "ASSIGNMENT_INVALID_TIME":
		return "과제 시간 설정 이상"
	case "GRADING_COMPLETED":
		return "채점 완료 업데이트"
	case "GRADING_FAILED":
		return "채점 실패 감지"
	default:
		return "WEB Admin API 상태"
	}
}

func eventIcon(event state.AssignmentEventState) string {
	if event.Severity == "WARN" {
		return "⚠️"
	}
	if event.EventType == "GRADING_COMPLETED" {
		return "🧪"
	}
	return "✅"
}

func writeAssignmentNextCommands(b *strings.Builder, event state.AssignmentEventState) {
	course := security.SanitizeText(event.CourseSlug)
	id := security.SanitizeText(event.AssignmentID)
	b.WriteString("\nnext:\n")
	switch event.EventType {
	case "ASSIGNMENT_MISSING_PROBLEM":
		fmt.Fprintf(b, "1. /ops assignment-check course:%s id:%s\n", course, id)
		b.WriteString("   - problemId 연결과 제출 가능성 체크리스트를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops submissions course:%s assignment:%s\n", course, id)
		b.WriteString("   - 제출/채점 상태가 누락 문제와 연결되는지 확인합니다.\n")
		fmt.Fprintf(b, "3. /ops logs service:report mode:recent query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report 로그에서 해당 assignmentId를 검색합니다.")
	case "ASSIGNMENT_PUBLISH_DELAYED":
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s view:diagnosis\n", course, id)
		b.WriteString("   - 봇이 공개 지연으로 분류한 필드별 근거를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops assignment-events course:%s id:%s\n", course, id)
		b.WriteString("   - firstDetectedAt, lastDetectedAt, notifyCount, ack/silence 상태를 봅니다.\n")
		fmt.Fprintf(b, "3. /ops logs service:report mode:recent query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report 로그에서 이 assignmentId의 publish/update 흔적을 찾습니다.\n")
		fmt.Fprintf(b, "4. /ops assignment-ack course:%s id:%s event:publish-delayed until:7d reason:<reason>\n", course, id)
		b.WriteString("   - 의도된 상태라면 반복 알림을 중지합니다.")
	case "ASSIGNMENT_DRAFT_PAST_START", "ASSIGNMENT_STALE_DRAFT", "ASSIGNMENT_INVALID_TIME":
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s view:diagnosis\n", course, id)
		b.WriteString("   - 공개 지연 단정이 가능한지와 부족한 필드를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops assignment-events course:%s id:%s\n", course, id)
		b.WriteString("   - 봇 감지 이력과 반복 억제 상태를 확인합니다.\n")
		fmt.Fprintf(b, "3. /ops logs service:report mode:recent query:%s since:24h limit:20\n", id)
		b.WriteString("   - 서버 로그에서 이 과제의 update/publish 이벤트를 검색합니다.\n")
		fmt.Fprintf(b, "4. /ops assignment-ack course:%s id:%s event:%s until:7d reason:<reason>\n", course, id, assignmentEventSlug(event.EventType))
		b.WriteString("   - 오래된 draft 등 의도된 상태라면 알림을 중지합니다.")
	default:
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s view:diagnosis\n", course, id)
		b.WriteString("   - 과제 필드와 봇 판단 근거를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops logs service:report mode:recent query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report 로그에서 관련 이벤트를 검색합니다.")
	}
}

func assignmentEventSlug(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_PUBLISH_DELAYED":
		return "publish-delayed"
	case "ASSIGNMENT_DRAFT_PAST_START":
		return "draft-past-start"
	case "ASSIGNMENT_STALE_DRAFT":
		return "stale-draft"
	case "ASSIGNMENT_INVALID_TIME":
		return "invalid-time"
	case "ASSIGNMENT_MISSING_PROBLEM":
		return "missing-problem"
	case "GRADING_FAILED":
		return "grading-failed"
	default:
		return strings.ToLower(strings.TrimPrefix(eventType, "ASSIGNMENT_"))
	}
}

func formatKST(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04 KST")
}

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
	fmt.Fprintf(&b, "publishedAt: %s\n", security.SanitizeText(unknownIfBlank(assignment.PublishedAt)))
	fmt.Fprintf(&b, "startAt: %s\n", security.SanitizeText(unknownIfBlank(assignment.StartAt)))
	fmt.Fprintf(&b, "endAt: %s\n", security.SanitizeText(unknownIfBlank(assignment.EndAt)))
	fmt.Fprintf(&b, "problemId: %s\n", security.SanitizeText(unknownIfBlank(assignment.ProblemID)))
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
	fmt.Fprintf(&b, "1. /ops assignment-events course:%s id:%s\n", security.SanitizeText(courseSlug), security.SanitizeText(assignment.ID))
	b.WriteString("   - 봇 감지 이력과 반복 억제 상태를 확인합니다.\n")
	fmt.Fprintf(&b, "2. /ops logs service:report mode:recent query:%s since:24h limit:20\n", security.SanitizeText(assignment.ID))
	b.WriteString("   - Report 로그에서 이 assignmentId를 검색합니다.")
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
