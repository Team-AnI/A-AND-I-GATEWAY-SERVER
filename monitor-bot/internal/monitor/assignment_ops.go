package monitor

import (
	"context"
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
	GradingInProgress     int
	GradingCompletedDelta int
	GradingFailedDelta    int
	Snapshots             map[string]state.AssignmentSnapshot
	Events                []state.AssignmentEventState
	RecentEvents          []state.AssignmentEventState
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

func (s *Service) RefreshAssignmentOps(ctx context.Context) error {
	channelID := strings.TrimSpace(firstNonEmpty(s.cfg.Alert.ChannelID, s.cfg.Dashboard.ChannelID))
	if channelID == "" {
		return nil
	}
	result := s.collectAssignmentOps(ctx, time.Now())
	if err := s.upsertAssignmentDashboard(ctx, channelID, formatAssignmentDashboard(result)); err != nil {
		log.Printf("assignment dashboard update failed: %v", err)
	}
	for _, event := range result.Events {
		if shouldSendAssignmentEvent(event.EventType) {
			if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, formatAssignmentEvent(event)); err != nil {
				log.Printf("assignment event send failed: %v", err)
			}
		}
	}
	return nil
}

func (s *Service) collectAssignmentOps(ctx context.Context, now time.Time) assignmentPollResult {
	result := assignmentPollResult{
		UpdatedAt:    now,
		APIStatus:    reportadmin.StatusOK,
		Snapshots:    map[string]state.AssignmentSnapshot{},
		RecentEvents: s.store.Snapshot().RecentAssignmentEvents,
	}
	courses, err := s.report.ListCourses(ctx)
	if err != nil {
		result.APIStatus = reportadmin.StatusOf(err)
		result.APIFinding = security.SanitizeText(err.Error())
		result.Events = append(result.Events, state.AssignmentEventState{
			Fingerprint: "web-admin-api:" + result.APIStatus,
			EventType:   "WEB_ADMIN_API_" + result.APIStatus,
			Severity:    severityForAPIStatus(result.APIStatus),
			Summary:     "WEB Admin API 조회 실패: " + result.APIStatus,
			CreatedAt:   now,
		})
		result.Events = s.dedupeAssignmentEvents(result.Events, now)
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
			if delayedPublish(snapshot, now) {
				result.PublishDelayed++
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
	result.Events = s.dedupeAssignmentEvents(result.Events, now)
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
		data.LastAssignmentOpsUpdatedAt = result.UpdatedAt
		for _, event := range result.Events {
			data.AssignmentEventFingerprints[event.Fingerprint] = state.AlertState{Active: true, LastSentAt: result.UpdatedAt}
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

func (s *Service) dedupeAssignmentEvents(events []state.AssignmentEventState, now time.Time) []state.AssignmentEventState {
	snapshot := s.store.Snapshot()
	cooldown := s.cfg.Alert.Cooldown
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	filtered := make([]state.AssignmentEventState, 0, len(events))
	for _, event := range events {
		existing := snapshot.AssignmentEventFingerprints[event.Fingerprint]
		if !existing.LastSentAt.IsZero() && now.Sub(existing.LastSentAt) < cooldown {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
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
	if delayedPublish(cur, now) {
		events = append(events, makeAssignmentEvent("ASSIGNMENT_PUBLISH_DELAYED", "WARN", cur, "과제 공개 지연", "publish", cur.Status, now))
	}
	if invalidAssignmentTime(cur) {
		events = append(events, makeAssignmentEvent("ASSIGNMENT_INVALID_TIME", "WARN", cur, "과제 시간 설정 이상", "time", cur.StartAt+"|"+cur.EndAt, now))
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
		Summary:      summary,
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

func delayedPublish(snapshot state.AssignmentSnapshot, now time.Time) bool {
	if isPublished(snapshot.Status) {
		return false
	}
	for _, candidate := range []string{snapshot.PublishedAt, snapshot.StartAt} {
		if expected, ok := parseAssignmentTime(candidate); ok && now.After(expected) {
			return true
		}
	}
	return false
}

func invalidAssignmentTime(snapshot state.AssignmentSnapshot) bool {
	start, startOK := parseAssignmentTime(snapshot.StartAt)
	end, endOK := parseAssignmentTime(snapshot.EndAt)
	if !startOK || !endOK {
		return true
	}
	return end.Before(start)
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
	if result.APIStatus != reportadmin.StatusOK || result.PublishDelayed > 0 || result.GradingFailedDelta > 0 {
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
	b.WriteString("/ops assignment course:<courseSlug> id:<assignmentId>\n")
	b.WriteString("/ops submissions course:<courseSlug> assignment:<assignmentId>\n")
	b.WriteString("/ops logs service:report mode:errors since:30m limit:10")
	return formatting.TruncateDiscordMessage(b.String())
}

func formatAssignmentEvent(event state.AssignmentEventState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", eventIcon(event), assignmentEventTitle(event.EventType))
	fmt.Fprintf(&b, "eventType: %s\n", event.EventType)
	fmt.Fprintf(&b, "severity: %s\n", event.Severity)
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(event.CourseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(event.AssignmentID))
	if strings.TrimSpace(event.Title) != "" {
		fmt.Fprintf(&b, "assignment: %s\n", security.SanitizeText(event.Title))
	}
	fmt.Fprintf(&b, "summary: %s\n", security.SanitizeText(event.Summary))
	b.WriteString("source: WEB_ADMIN_API\n")
	fmt.Fprintf(&b, "next: /ops assignment course:%s id:%s", security.SanitizeText(event.CourseSlug), security.SanitizeText(event.AssignmentID))
	return formatting.TruncateDiscordMessage(b.String())
}

func shouldSendAssignmentEvent(eventType string) bool {
	switch eventType {
	case "ASSIGNMENT_CREATED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_PUBLISH_DELAYED", "ASSIGNMENT_INVALID_TIME", "GRADING_COMPLETED", "GRADING_FAILED":
		return true
	}
	return strings.HasPrefix(eventType, "WEB_ADMIN_API_")
}

func assignmentEventTitle(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_CREATED":
		return "과제 등록 확인"
	case "ASSIGNMENT_PUBLISHED":
		return "과제 공개 완료"
	case "ASSIGNMENT_PUBLISH_DELAYED":
		return "과제 공개 지연"
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
		return "🚨"
	}
	if event.EventType == "GRADING_COMPLETED" {
		return "🧪"
	}
	return "✅"
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
