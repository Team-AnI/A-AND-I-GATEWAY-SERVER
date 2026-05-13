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
				{ID: "a1", Title: "5주차", Status: "published", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z"},
				{ID: "a2", Title: "6주차", Status: "draft", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z"},
			},
			"legacy": {{ID: "old", Status: "published"}},
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

func TestAssignmentDashboardReusesMessageIDAndLimitsRecentEvents(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "kotlin", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"kotlin": {{ID: "a1", Title: "5주차", Status: "published", StartAt: "2026-05-14T09:00:00Z", EndAt: "2026-05-15T09:00:00Z"}},
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
