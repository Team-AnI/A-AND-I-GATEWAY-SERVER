package monitor

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

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
