package monitor

import (
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

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
	if result != nil && result.SuppressedIssueCounts == nil {
		result.SuppressedIssueCounts = map[string]int{}
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
			} else if result != nil && event.ShouldCount {
				result.SuppressedIssueCounts[assignmentIssueGroupKey(event)]++
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
		IssueKey:           event.IssueKey,
		EventType:          event.EventType,
		Severity:           event.Severity,
		CourseSlug:         event.CourseSlug,
		AssignmentID:       event.AssignmentID,
		Title:              event.Title,
		Status:             event.Status,
		PublishedAt:        event.PublishedAt,
		PublishedAtOmitted: event.PublishedAtOmitted,
		StartAt:            event.StartAt,
		EndAt:              event.EndAt,
		ProblemID:          event.ProblemID,
		ProblemIDFallback:  event.ProblemIDFallback,
		FirstDetectedAt:    existing.FirstDetectedAt,
		LastDetectedAt:     now,
		LastNotifiedAt:     existing.LastNotifiedAt,
		State:              "open",
		NotifyCount:        existing.NotifyCount,
		EvidenceHash:       event.EvidenceHash,
		ReasonCode:         event.ReasonCode,
		ReasonText:         event.ReasonText,
		AckBy:              existing.AckBy,
		AckReason:          existing.AckReason,
		AckUntil:           existing.AckUntil,
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
