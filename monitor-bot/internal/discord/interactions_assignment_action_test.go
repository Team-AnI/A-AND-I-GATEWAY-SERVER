package discord

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type assignmentActionOpsRecorder struct {
	OpsController
	ackCall     *assignmentAckCall
	ackResult   string
	ackErr      error
	unackCall   *assignmentUnackCall
	unackResult string
	unackErr    error
	historyCall *assignmentHistoryCall
	history     string
}

type assignmentAckCall struct {
	course, assignment, event, until, reason, actor string
}

type assignmentUnackCall struct {
	course, assignment, event string
}

type assignmentHistoryCall struct {
	course, assignment string
}

func (r *assignmentActionOpsRecorder) AcknowledgeAssignmentIssue(course, assignment, event, until, reason, actor string) (string, error) {
	r.ackCall = &assignmentAckCall{course, assignment, event, until, reason, actor}
	return r.ackResult, r.ackErr
}

func (r *assignmentActionOpsRecorder) UnacknowledgeAssignmentIssue(course, assignment, event string) (string, error) {
	r.unackCall = &assignmentUnackCall{course, assignment, event}
	return r.unackResult, r.unackErr
}

func (r *assignmentActionOpsRecorder) AssignmentIssueHistory(course, assignment string) string {
	r.historyCall = &assignmentHistoryCall{course, assignment}
	return r.history
}

func TestOpsAssignmentAckAndUnackForwardControllerArguments(t *testing.T) {
	const assignmentID = "11111111-1111-1111-1111-111111111111"
	ops := &assignmentActionOpsRecorder{ackResult: "acked", unackResult: "unacked"}
	h := &Handler{ops: ops}

	ack := h.assignmentCommand(context.Background(), assignmentInteraction("ack",
		stringInteractionOption("course", "course-1"),
		stringInteractionOption("id", assignmentID),
		stringInteractionOption("event", "stale"),
		stringInteractionOption("until", "2h"),
		stringInteractionOption("reason", "investigating"),
	))
	if ack != "acked" || ops.ackCall == nil || *ops.ackCall != (assignmentAckCall{"course-1", assignmentID, "stale", "2h", "investigating", "discord"}) {
		t.Fatalf("unexpected ack result=%q call=%#v", ack, ops.ackCall)
	}

	unack := h.assignmentCommand(context.Background(), assignmentInteraction("unack",
		stringInteractionOption("course", "course-1"),
		stringInteractionOption("id", assignmentID),
		stringInteractionOption("event", "stale"),
	))
	if unack != "unacked" || ops.unackCall == nil || *ops.unackCall != (assignmentUnackCall{"course-1", assignmentID, "stale"}) {
		t.Fatalf("unexpected unack result=%q call=%#v", unack, ops.unackCall)
	}
}

func TestOpsAssignmentEventsDelegatesToController(t *testing.T) {
	const assignmentID = "11111111-1111-1111-1111-111111111111"
	ops := &assignmentActionOpsRecorder{history: "history"}
	h := &Handler{ops: ops}

	got := h.assignmentCommand(context.Background(), assignmentInteraction("",
		stringInteractionOption("course", "course-1"),
		stringInteractionOption("id", assignmentID),
		stringInteractionOption("view", "events"),
	))

	if got != "history" || ops.historyCall == nil || *ops.historyCall != (assignmentHistoryCall{"course-1", assignmentID}) {
		t.Fatalf("unexpected events result=%q call=%#v", got, ops.historyCall)
	}
}

func TestOpsAssignmentControllerActionsPreserveValidationAndErrors(t *testing.T) {
	const assignmentID = "11111111-1111-1111-1111-111111111111"

	t.Run("reason required before controller call", func(t *testing.T) {
		ops := &assignmentActionOpsRecorder{}
		h := &Handler{ops: ops}
		got := h.assignmentCommand(context.Background(), assignmentInteraction("ack",
			stringInteractionOption("course", "course-1"), stringInteractionOption("id", assignmentID),
		))
		if got != "assignment ack에는 reason이 필요합니다." || ops.ackCall != nil {
			t.Fatalf("unexpected missing reason result=%q call=%#v", got, ops.ackCall)
		}
	})

	t.Run("controller required", func(t *testing.T) {
		h := &Handler{}
		got := h.assignmentCommand(context.Background(), assignmentInteraction("unack",
			stringInteractionOption("course", "course-1"), stringInteractionOption("id", assignmentID),
		))
		if got != "Service Ops controller is not ready." {
			t.Fatalf("unexpected missing controller result: %s", got)
		}
	})

	t.Run("controller error sanitized", func(t *testing.T) {
		ops := &assignmentActionOpsRecorder{ackErr: errors.New("failed token=super-secret")}
		h := &Handler{ops: ops}
		got := h.assignmentCommand(context.Background(), assignmentInteraction("ack",
			stringInteractionOption("course", "course-1"), stringInteractionOption("id", assignmentID), stringInteractionOption("reason", "test"),
		))
		if got != "failed token=[REDACTED]" {
			t.Fatalf("unexpected sanitized error: %s", got)
		}
	})

	t.Run("invalid identifiers", func(t *testing.T) {
		h := &Handler{}
		got := h.assignmentCommand(context.Background(), assignmentInteraction("ack",
			stringInteractionOption("course", "bad course"), stringInteractionOption("id", "bad id"),
		))
		if !strings.Contains(got, "올바르지 않은 courseSlug") {
			t.Fatalf("unexpected validation response: %s", got)
		}
	})
}

func assignmentInteraction(action string, options ...ApplicationCommandOpt) Interaction {
	return interactionForCommand("assignment", append([]ApplicationCommandOpt{stringInteractionOption("action", action)}, options...))
}
