package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/config"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/health"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

type fakeReportAdmin struct {
	courses     []reportadmin.Course
	assignments map[string][]reportadmin.Assignment
	submissions map[string]reportadmin.SubmissionSummary
	err         error
}

func (f *fakeReportAdmin) ListCourses(context.Context) ([]reportadmin.Course, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.courses, nil
}

func (f *fakeReportAdmin) ListAssignments(_ context.Context, courseSlug string) ([]reportadmin.Assignment, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.assignments[courseSlug], nil
}

func (f *fakeReportAdmin) SubmissionStatuses(_ context.Context, courseSlug, assignmentID string) (reportadmin.SubmissionSummary, error) {
	if f.err != nil {
		return reportadmin.SubmissionSummary{}, f.err
	}
	return f.submissions[courseSlug+":"+assignmentID], nil
}

func TestClassifyCourse(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	if got := ClassifyCourse(reportadmin.Course{Status: "CLOSED"}, now); got != CourseLegacy {
		t.Fatalf("closed course = %s", got)
	}
	if got := ClassifyCourse(reportadmin.Course{EndAt: "2026-05-12T00:00:00Z"}, now); got != CourseLegacy {
		t.Fatalf("ended course = %s", got)
	}
	if got := ClassifyCourse(reportadmin.Course{}, now); got != CourseUnknown {
		t.Fatalf("empty course = %s", got)
	}
	if got := ClassifyCourse(reportadmin.Course{Status: "OPEN"}, now); got != CourseActive {
		t.Fatalf("open course = %s", got)
	}
}

func TestAssignmentOpsBaselineDoesNotEmitHistoricalEvents(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "5주차", Status: "draft", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {Submitted: 1, Graded: 0, Pending: 1}},
	})
	result := service.collectAssignmentOps(context.Background(), time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC))
	if len(result.Events) != 0 {
		t.Fatalf("baseline should not emit events: %#v", result.Events)
	}
	if !store.Snapshot().AssignmentBaselineInitialized {
		t.Fatal("baseline should be initialized")
	}
}

func TestAssignmentOpsDetectsAssignmentAndGradingEvents(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "5주차", Status: "draft", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {Submitted: 1, Graded: 0, Pending: 1}},
	})
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	_ = service.collectAssignmentOps(context.Background(), now)

	service.report = &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}, {Slug: "legacy", Status: "CLOSED"}, {Slug: "unknown"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {
				{ID: "a1", Title: "5주차", Status: "published", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z", ProblemID: "p1"},
				{ID: "a2", Title: "6주차", Status: "draft", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z"},
			},
			"legacy": {{ID: "old", Status: "published", ProblemID: "p-old"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{
			"kotlin:a1": {Submitted: 2, Graded: 1, Pending: 1},
			"kotlin:a2": {Submitted: 0, Graded: 0},
		},
	}
	result := service.collectAssignmentOps(context.Background(), now.Add(time.Minute))
	types := map[string]bool{}
	for _, event := range result.Events {
		types[event.EventType] = true
		if event.CourseSlug == "legacy" {
			t.Fatalf("legacy course event should not be emitted: %#v", event)
		}
	}
	for _, want := range []string{"ASSIGNMENT_CREATED", "ASSIGNMENT_PUBLISHED", "GRADING_COMPLETED", "SUBMISSION_COUNT_CHANGED"} {
		if !types[want] {
			t.Fatalf("missing event %s in %#v", want, result.Events)
		}
	}
	if result.UnknownCourses != 1 {
		t.Fatalf("unknown course should be counted, got %d", result.UnknownCourses)
	}
}

func TestAssignmentOpsDetectsWarningsAndDedupes(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "bad", Status: "draft", PublishedAt: "2026-05-13T09:00:00Z", StartAt: "2026-05-15T09:00:00Z", EndAt: "2026-05-14T09:00:00Z"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {Failed: 0}},
	})
	_ = service.collectAssignmentOps(context.Background(), now.Add(-time.Minute))
	service.report = &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "bad", Status: "draft", PublishedAt: "2026-05-13T09:00:00Z", StartAt: "2026-05-15T09:00:00Z", EndAt: "2026-05-14T09:00:00Z"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {Failed: 1}},
	}
	result := service.collectAssignmentOps(context.Background(), now)
	types := map[string]bool{}
	for _, event := range result.Events {
		types[event.EventType] = true
	}
	for _, want := range []string{"ASSIGNMENT_PUBLISH_DELAYED", "ASSIGNMENT_INVALID_TIME", "GRADING_FAILED"} {
		if !types[want] {
			t.Fatalf("missing warning %s in %#v", want, result.Events)
		}
	}
	again := service.collectAssignmentOps(context.Background(), now.Add(time.Minute))
	if len(again.Events) != 0 {
		t.Fatalf("duplicate events should be suppressed: %#v", again.Events)
	}
}

func TestAssignmentIssueLifecycleSuppressesSameOpenIssueAfterCooldown(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	report := &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "delayed", Status: "draft", PublishedAt: "2026-05-20T09:00:00Z", StartAt: "2026-05-21T09:00:00Z", EndAt: "2026-05-22T09:00:00Z"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {}},
	}
	service := newAssignmentOpsTestService(store, report)
	_ = service.collectAssignmentOps(context.Background(), now.Add(-time.Minute))

	first := service.collectAssignmentOps(context.Background(), now)
	if !hasAssignmentEvent(first.Events, "ASSIGNMENT_PUBLISH_DELAYED") {
		t.Fatalf("first open issue should notify: %#v", first.Events)
	}
	again := service.collectAssignmentOps(context.Background(), now.Add(2*time.Hour))
	if hasAssignmentEvent(again.Events, "ASSIGNMENT_PUBLISH_DELAYED") {
		t.Fatalf("same open issue should not resend after cooldown: %#v", again.Events)
	}
	issue := store.Snapshot().AssignmentIssues[assignmentIssueKey("ASSIGNMENT_PUBLISH_DELAYED", "kotlin", "a1")]
	if issue.NotifyCount != 1 || issue.State != "open" {
		t.Fatalf("issue state mismatch: %#v", issue)
	}
}

func TestAssignmentIssueResolvesAndReopens(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	publishedAt := now.Add(-3 * time.Hour).Format(time.RFC3339)
	startAt := now.Add(24 * time.Hour).Format(time.RFC3339)
	endAt := now.Add(48 * time.Hour).Format(time.RFC3339)
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Status: "draft", PublishedAt: publishedAt, StartAt: startAt, EndAt: endAt}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {}},
	})
	_ = service.collectAssignmentOps(context.Background(), now.Add(-time.Minute))
	_ = service.collectAssignmentOps(context.Background(), now)

	service.report = &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Status: "published", ProblemID: "p1", PublishedAt: publishedAt, StartAt: startAt, EndAt: endAt}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {}},
	}
	_ = service.collectAssignmentOps(context.Background(), now.Add(time.Hour))
	key := assignmentIssueKey("ASSIGNMENT_PUBLISH_DELAYED", "kotlin", "a1")
	if got := store.Snapshot().AssignmentIssues[key].State; got != "resolved" {
		t.Fatalf("issue should resolve, got %q", got)
	}

	service.report = &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Status: "draft", PublishedAt: publishedAt, StartAt: startAt, EndAt: endAt}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {}},
	}
	reopened := service.collectAssignmentOps(context.Background(), now.Add(2*time.Hour))
	if !hasAssignmentEvent(reopened.Events, "ASSIGNMENT_PUBLISH_DELAYED") {
		t.Fatalf("resolved issue should notify when reopened: %#v", reopened.Events)
	}
}

func TestAssignmentDiagnosisSplitsDraftPastStartAndStaleDraft(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	draftPastStart := diagnoseAssignment(state.AssignmentSnapshot{
		CourseSlug:   "3rd-cs",
		AssignmentID: "a1",
		Status:       "DRAFT",
		StartAt:      "2026-05-20T09:00:00Z",
		EndAt:        "2026-05-25T09:00:00Z",
	}, now, 7*24*time.Hour)
	if len(draftPastStart) == 0 || draftPastStart[0].EventType != "ASSIGNMENT_DRAFT_PAST_START" {
		t.Fatalf("draft past start diagnosis mismatch: %#v", draftPastStart)
	}
	if !strings.Contains(draftPastStart[0].ReasonText, "공개 지연으로 단정할 수 없습니다") {
		t.Fatalf("draft past start should not assert publish delay: %#v", draftPastStart[0])
	}

	stale := diagnoseAssignment(state.AssignmentSnapshot{
		CourseSlug:   "3rd-cs",
		AssignmentID: "a1",
		Status:       "DRAFT",
		StartAt:      "2025-05-19T09:00:00+09:00",
		EndAt:        "2025-05-23T18:00:00+09:00",
	}, now, 7*24*time.Hour)
	if len(stale) == 0 || stale[0].EventType != "ASSIGNMENT_STALE_DRAFT" || stale[0].ShouldNotify {
		t.Fatalf("stale draft should be dashboard-only: %#v", stale)
	}
}

func TestAssignmentDiagnosisMissingProblemOnPublished(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	diagnoses := diagnoseAssignment(state.AssignmentSnapshot{
		CourseSlug:   "kotlin",
		AssignmentID: "a1",
		Status:       "open",
		StartAt:      "2026-05-20T09:00:00Z",
		EndAt:        "2026-05-21T09:00:00Z",
	}, now, 7*24*time.Hour)
	if !hasDiagnosis(diagnoses, "ASSIGNMENT_MISSING_PROBLEM") {
		t.Fatalf("published/open missing problem should be diagnosed: %#v", diagnoses)
	}
}

func TestAssignmentAckSuppressesUntilExpiry(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	publishedAt := now.Add(-3 * time.Hour).Format(time.RFC3339)
	startAt := now.Add(24 * time.Hour).Format(time.RFC3339)
	endAt := now.Add(48 * time.Hour).Format(time.RFC3339)
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Status: "draft", PublishedAt: publishedAt, StartAt: startAt, EndAt: endAt}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {}},
	})
	_ = service.collectAssignmentOps(context.Background(), now.Add(-time.Minute))
	if _, err := service.AcknowledgeAssignmentIssue("kotlin", "a1", "publish-delayed", "1h", "known", "test"); err != nil {
		t.Fatal(err)
	}
	suppressed := service.collectAssignmentOps(context.Background(), now)
	if hasAssignmentEvent(suppressed.Events, "ASSIGNMENT_PUBLISH_DELAYED") {
		t.Fatalf("acked issue should be suppressed: %#v", suppressed.Events)
	}
	expired := service.collectAssignmentOps(context.Background(), now.Add(2*time.Hour))
	if !hasAssignmentEvent(expired.Events, "ASSIGNMENT_PUBLISH_DELAYED") {
		t.Fatalf("expired ack should allow notification: %#v", expired.Events)
	}
}

func TestAssignmentAlertIncludesEvidenceAndExplainedNextCommands(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	snapshot := state.AssignmentSnapshot{
		CourseSlug:   "3rd-cs",
		AssignmentID: "1d74df8d-c501-405e-9327-d8f39b4d98cb",
		Status:       "DRAFT",
		StartAt:      "2026-05-20T09:00:00Z",
		EndAt:        "2026-05-25T09:00:00Z",
	}
	diagnosis := diagnoseAssignment(snapshot, now, 7*24*time.Hour)[0]
	event := makeAssignmentIssueEvent(snapshot, diagnosis, now)
	issue, include := applyAssignmentIssueState(state.AssignmentIssueState{}, event, now)
	if !include {
		t.Fatal("first issue should be included")
	}
	got := formatAssignmentEvent(enrichAssignmentIssueEvent(event, issue))
	for _, want := range []string{"title: unknown", "publishedAt: unknown", "reasonCode: PUBLISHED_AT_MISSING_DRAFT_START_PAST", "공개 지연으로 단정할 수 없음", "evidence:", "/ops assignment course:3rd-cs id:1d74df8d-c501-405e-9327-d8f39b4d98cb view:diagnosis", "- 봇 감지 이력과 반복 억제 상태를 확인합니다."} {
		if !strings.Contains(got, want) {
			t.Fatalf("assignment alert missing %q: %s", want, got)
		}
	}
}

func TestAssignmentDashboardReusesMessageIDAndLimitsRecentEvents(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "5주차", Status: "published", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z", ProblemID: "p1"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {Submitted: 1, Graded: 1}},
	})
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord
	if err := service.RefreshAssignmentOps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.RefreshAssignmentOps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fakeDiscord.sends != 1 || fakeDiscord.edits != 1 {
		t.Fatalf("dashboard should send once and edit after, sends=%d edits=%d", fakeDiscord.sends, fakeDiscord.edits)
	}
	content := formatAssignmentDashboard(assignmentPollResult{
		UpdatedAt:     time.Now(),
		APIStatus:     reportadmin.StatusOK,
		ActiveCourses: 1,
		RecentEvents:  make([]state.AssignmentEventState, 10),
	})
	if strings.Count(content, ". ") > 5 {
		t.Fatalf("dashboard should display recent 5 events only: %s", content)
	}
	if !strings.Contains(content, "📌 A&I 과제 운영 대시보드") || !strings.Contains(content, "상세 확인") {
		t.Fatalf("dashboard should be Korean single-message format: %s", content)
	}
}

func TestAssignmentOpsUsesConfiguredAlertChannelFromState(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(func(data *state.Data) {
		data.ServiceAlerts.ChannelID = "state-alert-channel"
	}); err != nil {
		t.Fatal(err)
	}
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "5주차", Status: "published", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z", ProblemID: "p1"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"kotlin:a1": {Submitted: 1, Graded: 1}},
	})
	service.cfg.Alert.ChannelID = ""
	service.cfg.Dashboard.ChannelID = ""
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	if err := service.RefreshAssignmentOps(context.Background()); err != nil {
		t.Fatal(err)
	}

	if fakeDiscord.sends != 1 {
		t.Fatalf("expected assignment dashboard send, got sends=%d", fakeDiscord.sends)
	}
	if got := fakeDiscord.sentChannels[0]; got != "state-alert-channel" {
		t.Fatalf("assignment ops should use state alert channel, got %q", got)
	}
}

func newAssignmentOpsTestService(store *state.Store, report ReportAdminAPI) *Service {
	cfg := config.Config{
		DiscordBotToken: "bot-token",
		Alert: config.AlertConfig{
			Enabled:      true,
			ChannelID:    "ops-channel",
			PollInterval: time.Minute,
			Cooldown:     15 * time.Minute,
		},
		HealthURLs: map[string]string{},
		LogGroups:  map[string]string{"report": "/a-and-i/prod/report"},
	}
	service := NewService(cfg, health.NewClient(map[string]string{}, time.Millisecond), &fakeLogs{}, fakeAlarms{}, store, nil)
	service.report = report
	return service
}

func hasAssignmentEvent(events []state.AssignmentEventState, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func hasDiagnosis(diagnoses []AssignmentDiagnosis, eventType string) bool {
	for _, diagnosis := range diagnoses {
		if diagnosis.EventType == eventType {
			return true
		}
	}
	return false
}
