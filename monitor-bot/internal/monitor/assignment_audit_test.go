package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func TestParseAssignmentAuditEventCreatedWithActorAndOccurredAt(t *testing.T) {
	event, ok := parseAssignmentAuditEvent(map[string]string{
		"@timestamp":         "2026-05-20T03:04:13Z",
		"logType":            "EVENT",
		"event.eventType":    "ASSIGNMENT_CREATED",
		"event.assignmentId": "a1",
		"event.courseSlug":   "3rd-cs",
		"assignment.title":   "midterm",
		"actor.userId":       "12345",
		"actor.name":         "홍길동",
		"actor.role":         "ADMIN",
		"event.occurredAt":   "2026-05-20T03:04:13Z",
		"trace.traceId":      "trace-1",
	})
	if !ok {
		t.Fatal("expected audit event")
	}
	if event.EventType != "ASSIGNMENT_CREATED" || event.ActorID != "12345" || event.ActorName != "홍길동" || event.ActorRole != "ADMIN" {
		t.Fatalf("unexpected actor event: %#v", event)
	}
	if event.OccurredAt.IsZero() || event.Source != assignmentAuditSource {
		t.Fatalf("occurredAt/source missing: %#v", event)
	}
	if event.Fingerprint != "assignment-audit:ASSIGNMENT_CREATED:trace:trace-1" {
		t.Fatalf("trace fingerprint mismatch: %s", event.Fingerprint)
	}
}

func TestParseAssignmentAuditEventUpdatedWithChangedFields(t *testing.T) {
	event, ok := parseAssignmentAuditEvent(map[string]string{
		"@timestamp":      "2026-05-20T03:10:02Z",
		"event.eventType": "ASSIGNMENT_UPDATED",
		"assignmentId":    "a1",
		"courseSlug":      "3rd-cs",
		"actor.userId":    "12345",
		"changedFields":   `{"endAt":{"before":"2026-05-23T18:00:00+09:00","after":"2026-05-24T18:00:00+09:00"},"request.body":{"before":"secret","after":"secret"}}`,
	})
	if !ok {
		t.Fatal("expected audit event")
	}
	if got := event.ChangedFields["endAt"]; got.Before == "" || got.After == "" {
		t.Fatalf("changed field missing: %#v", event.ChangedFields)
	}
	if _, leaked := event.ChangedFields["request.body"]; leaked {
		t.Fatalf("sensitive changed field should be dropped: %#v", event.ChangedFields)
	}
}

func TestParseAssignmentAuditEventDeletedAndTimestampFallback(t *testing.T) {
	event, ok := parseAssignmentAuditEvent(map[string]string{
		"@timestamp":       "2026-05-20T03:15:44Z",
		"event.eventType":  "ASSIGNMENT_DELETED",
		"event.resourceId": "a1",
		"event.courseSlug": "3rd-cs",
		"event.title":      "old assignment",
		"actor.userId":     "12345",
	})
	if !ok {
		t.Fatal("expected audit event")
	}
	if event.EventType != "ASSIGNMENT_DELETED" || event.AssignmentID != "a1" || event.Title != "old assignment" {
		t.Fatalf("deleted event mismatch: %#v", event)
	}
	if event.OccurredAt.IsZero() {
		t.Fatalf("@timestamp fallback should be used: %#v", event)
	}
}

func TestParseAssignmentAuditEventRejectsNonAssignmentEventAndDoesNotGuessActor(t *testing.T) {
	if _, ok := parseAssignmentAuditEvent(map[string]string{"event.eventType": "COURSE_UPDATED"}); ok {
		t.Fatal("non-assignment event accepted")
	}
	event, ok := parseAssignmentAuditEvent(map[string]string{
		"@timestamp":      "2026-05-20T03:04:13Z",
		"event.eventType": "ASSIGNMENT_PUBLISHED",
		"assignmentId":    "a1",
	})
	if !ok {
		t.Fatal("expected published event")
	}
	if event.ActorID != "" || event.ActorName != "" || event.ActorRole != "" {
		t.Fatalf("actor should not be guessed: %#v", event)
	}
	got := formatAssignmentAuditEvent(event)
	if !strings.Contains(got, "userId: unknown") || !strings.Contains(got, "name: unknown") {
		t.Fatalf("missing actor should be shown as unknown: %s", got)
	}
}

func TestAssignmentAuditFeedSendsOnceWithoutRoleMention(t *testing.T) {
	store := state.NewStore(t.TempDir() + "/state.json")
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	service := newAssignmentOpsTestService(store, &fakeReportAdmin{
		courses: []reportadmin.Course{{Slug: "3rd-cs", Status: "OPEN"}},
		assignments: map[string][]reportadmin.Assignment{
			"3rd-cs": {{ID: "a1", Status: "published", ProblemID: "p1"}},
		},
		submissions: map[string]reportadmin.SubmissionSummary{"3rd-cs:a1": {}},
	})
	service.logs = &fakeLogs{rows: []map[string]string{{
		"@timestamp":         "2026-05-20T03:04:13Z",
		"event.eventType":    "ASSIGNMENT_CREATED",
		"event.assignmentId": "a1",
		"event.courseSlug":   "3rd-cs",
		"actor.userId":       "12345",
		"actor.role":         "ADMIN",
		"trace.traceId":      "trace-1",
	}}}
	fakeDiscord := &fakeDiscord{}
	service.discord = fakeDiscord

	service.refreshAssignmentAuditEvents(context.Background(), "ops-channel")
	service.refreshAssignmentAuditEvents(context.Background(), "ops-channel")

	if fakeDiscord.sends != 1 {
		t.Fatalf("same audit event should send once, sends=%d", fakeDiscord.sends)
	}
	if fakeDiscord.roleSends != 0 {
		t.Fatalf("audit event must not role mention, roleSends=%d", fakeDiscord.roleSends)
	}
	if !strings.Contains(fakeDiscord.sentContents[0], "과제 등록") || !strings.Contains(fakeDiscord.sentContents[0], "source: REPORT_EVENT_LOG") {
		t.Fatalf("audit message missing expected content: %s", fakeDiscord.sentContents[0])
	}
}

func TestFormatAssignmentAuditEventTitles(t *testing.T) {
	cases := map[string]string{
		"ASSIGNMENT_CREATED":     "과제 등록",
		"ASSIGNMENT_UPDATED":     "과제 수정",
		"ASSIGNMENT_DELETED":     "과제 삭제",
		"ASSIGNMENT_PUBLISHED":   "과제 공개",
		"ASSIGNMENT_UNPUBLISHED": "과제 비공개",
	}
	for eventType, title := range cases {
		got := formatAssignmentAuditEvent(state.AssignmentAuditEventState{
			EventType:    eventType,
			CourseSlug:   "3rd-cs",
			AssignmentID: "a1",
			ActorID:      "12345",
			OccurredAt:   time.Date(2026, 5, 20, 3, 4, 13, 0, time.UTC),
			TraceID:      "trace-1",
			Source:       assignmentAuditSource,
		})
		for _, want := range []string{title, "actor:", "occurredAt:", "source: REPORT_EVENT_LOG", "/ops logs service:report mode:events query:trace-1"} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s message missing %q: %s", eventType, want, got)
			}
		}
	}
}
