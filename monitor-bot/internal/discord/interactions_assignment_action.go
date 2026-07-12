package discord

import (
	"strings"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

func (h *Handler) assignmentAckAction(courseSlug, assignmentID string, interaction Interaction) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	reason := optionString(interaction, "reason")
	if strings.TrimSpace(reason) == "" {
		return "assignment ack에는 reason이 필요합니다."
	}
	result, err := h.ops.AcknowledgeAssignmentIssue(
		courseSlug,
		assignmentID,
		optionString(interaction, "event"),
		optionString(interaction, "until"),
		reason,
		"discord",
	)
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) assignmentUnackAction(courseSlug, assignmentID string, interaction Interaction) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	result, err := h.ops.UnacknowledgeAssignmentIssue(courseSlug, assignmentID, optionString(interaction, "event"))
	if err != nil {
		return security.SanitizeText(err.Error())
	}
	return result
}

func (h *Handler) assignmentEventsView(courseSlug, assignmentID string) string {
	if h.ops == nil {
		return "Service Ops controller is not ready."
	}
	return h.ops.AssignmentIssueHistory(courseSlug, assignmentID)
}
