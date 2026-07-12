package monitor

import (
	"context"
	"fmt"
	"log"
	"sort"
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
	SuppressedIssueCounts map[string]int
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
	s.sendAssignmentEventNotifications(ctx, channelID, result)
	s.refreshAssignmentAuditEvents(ctx, channelID)
	return nil
}

func (s *Service) sendAssignmentEventNotifications(ctx context.Context, channelID string, result assignmentPollResult) {
	issueGroups := map[string]assignmentIssueDigestGroup{}
	for _, event := range result.Events {
		if isAssignmentIssueEvent(event.EventType) {
			if shouldSendAssignmentEvent(event) {
				key := assignmentIssueGroupKey(event)
				group := issueGroups[key]
				group.CourseSlug = event.CourseSlug
				group.EventType = event.EventType
				group.Severity = event.Severity
				group.Source = assignmentIssueSource
				group.Events = append(group.Events, event)
				group.Suppressed = result.SuppressedIssueCounts[key]
				issueGroups[key] = group
			}
			continue
		}
		if shouldSendAssignmentEvent(event) {
			if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, formatAssignmentEvent(event)); err != nil {
				log.Printf("assignment event send failed: %v", err)
			}
		}
	}
	keys := make([]string, 0, len(issueGroups))
	for key := range issueGroups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, formatAssignmentIssueDigest(issueGroups[key])); err != nil {
			log.Printf("assignment issue digest send failed: %v", err)
		}
	}
}

func (s *Service) collectAssignmentOps(ctx context.Context, now time.Time) assignmentPollResult {
	result := assignmentPollResult{
		UpdatedAt:             now,
		APIStatus:             reportadmin.StatusOK,
		Snapshots:             map[string]state.AssignmentSnapshot{},
		IssueStates:           map[string]state.AssignmentIssueState{},
		SuppressedIssueCounts: map[string]int{},
		RecentEvents:          s.store.Snapshot().RecentAssignmentEvents,
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
	detailBudget := 30
	submissionBudget := 30
	for _, course := range activeCourses {
		assignments, err := s.report.ListAssignments(ctx, course.Slug)
		if err != nil {
			result.APIStatus = reportadmin.StatusOf(err)
			result.APIFinding = "course " + security.SanitizeText(course.Slug) + " 조회 실패: " + result.APIStatus
			continue
		}
		for _, assignment := range assignments {
			if assignment.PublishedAtOmitted && strings.TrimSpace(assignment.ID) != "" && detailBudget > 0 {
				if detail, err := s.report.GetAssignment(ctx, course.Slug, assignment.ID); err == nil {
					if hasAssignmentDetail(detail) {
						assignment = mergeAssignmentDetail(assignment, detail)
					}
				}
				detailBudget--
			}
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
