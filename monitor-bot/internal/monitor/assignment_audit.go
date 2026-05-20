package monitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	cw "github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/cloudwatch"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

const assignmentAuditSource = "REPORT_EVENT_LOG"

func (s *Service) refreshAssignmentAuditEvents(ctx context.Context, channelID string) {
	groups, err := cw.LogGroupsForService(s.cfg.LogGroups, "report")
	if err != nil {
		log.Printf("assignment audit log group unavailable: %v", err)
		return
	}
	query, err := cw.BuildAssignmentAuditEventsQuery("", 20)
	if err != nil {
		log.Printf("assignment audit query build failed: %v", err)
		return
	}
	rows, err := s.logs.Query(ctx, groups, query, s.assignmentAuditWindow(), 20)
	if err != nil {
		log.Printf("assignment audit query failed: %v", err)
		return
	}
	events := collectAssignmentAuditEvents(rows, s.store.Snapshot())
	if len(events) == 0 {
		return
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if _, err := s.discord.SendChannelMessage(ctx, s.client, s.cfg.DiscordBotToken, channelID, formatAssignmentAuditEvent(event)); err != nil {
			log.Printf("assignment audit event send failed: %v", err)
			continue
		}
		if err := s.persistAssignmentAuditEvent(event, time.Now()); err != nil {
			log.Printf("assignment audit event state save failed: %v", err)
		}
	}
}

func (s *Service) assignmentAuditWindow() time.Duration {
	window := s.assignmentOpsInterval() + 5*time.Minute
	if window < 15*time.Minute {
		return 15 * time.Minute
	}
	if window > time.Hour {
		return time.Hour
	}
	return window
}

func collectAssignmentAuditEvents(rows []map[string]string, snapshot state.Data) []state.AssignmentAuditEventState {
	events := make([]state.AssignmentAuditEventState, 0, len(rows))
	seen := make(map[string]struct{})
	for _, row := range rows {
		event, ok := parseAssignmentAuditEvent(row)
		if !ok {
			continue
		}
		if _, known := snapshot.AssignmentAuditFingerprints[event.Fingerprint]; known {
			continue
		}
		if _, duplicate := seen[event.Fingerprint]; duplicate {
			continue
		}
		seen[event.Fingerprint] = struct{}{}
		events = append(events, event)
	}
	return events
}

func (s *Service) persistAssignmentAuditEvent(event state.AssignmentAuditEventState, now time.Time) error {
	return s.store.Update(func(data *state.Data) {
		data.AssignmentAuditFingerprints[event.Fingerprint] = state.AlertState{Active: true, LastSentAt: now}
		event.CreatedAt = now
		data.RecentAssignmentAuditEvents = append([]state.AssignmentAuditEventState{event}, data.RecentAssignmentAuditEvents...)
		if len(data.RecentAssignmentAuditEvents) > 20 {
			data.RecentAssignmentAuditEvents = data.RecentAssignmentAuditEvents[:20]
		}
	})
}

func parseAssignmentAuditEvent(row map[string]string) (state.AssignmentAuditEventState, bool) {
	eventType := strings.TrimSpace(rowValue(row, "event.eventType", "eventType"))
	if !isAssignmentAuditEventType(eventType) {
		return state.AssignmentAuditEventState{}, false
	}
	occurredAt := parseAuditTime(rowValue(row, "event.occurredAt", "@timestamp", "timestamp"))
	event := state.AssignmentAuditEventState{
		EventType:     eventType,
		CourseSlug:    sanitizeAuditValue(rowValue(row, "event.courseSlug", "assignment.courseSlug", "courseSlug", "request.pathVariables.courseSlug")),
		AssignmentID:  sanitizeAuditValue(rowValue(row, "event.assignmentId", "event.resourceId", "assignmentId", "assignment.assignmentId", "request.pathVariables.assignmentId")),
		Title:         sanitizeAuditValue(rowValue(row, "event.title", "assignment.title")),
		ActorID:       sanitizeAuditValue(rowValue(row, "actor.userId", "actor.id")),
		ActorName:     sanitizeAuditValue(rowValue(row, "actor.name", "actor.displayName", "actor.loginId")),
		ActorRole:     sanitizeAuditValue(rowValue(row, "actor.role")),
		OccurredAt:    occurredAt,
		TraceID:       sanitizeAuditValue(rowValue(row, "trace.traceId")),
		RequestID:     sanitizeAuditValue(rowValue(row, "trace.requestId")),
		ChangedFields: parseAuditChangedFields(rowValue(row, "changedFields", "changes")),
		Source:        assignmentAuditSource,
	}
	event.Fingerprint = assignmentAuditFingerprint(event)
	return event, event.Fingerprint != ""
}

func isAssignmentAuditEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "ASSIGNMENT_CREATED", "ASSIGNMENT_UPDATED", "ASSIGNMENT_DELETED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_UNPUBLISHED":
		return true
	default:
		return false
	}
}

func assignmentAuditFingerprint(event state.AssignmentAuditEventState) string {
	if strings.TrimSpace(event.TraceID) != "" {
		return "assignment-audit:" + event.EventType + ":trace:" + event.TraceID
	}
	occurredAt := ""
	if !event.OccurredAt.IsZero() {
		occurredAt = event.OccurredAt.UTC().Format(time.RFC3339Nano)
	}
	changedHash := assignmentChangedFieldsHash(event.ChangedFields)
	parts := []string{"assignment-audit", event.EventType, event.CourseSlug, event.AssignmentID, occurredAt, event.ActorID, changedHash}
	if strings.TrimSpace(event.AssignmentID) == "" && strings.TrimSpace(event.ActorID) == "" && occurredAt == "" && changedHash == "" {
		return ""
	}
	return strings.Join(parts, ":")
}

func assignmentChangedFieldsHash(changed map[string]state.AssignmentChangedField) string {
	if len(changed) == 0 {
		return ""
	}
	keys := make([]string, 0, len(changed))
	for key := range changed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		value := changed[key]
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(value.Before)
		b.WriteString("->")
		b.WriteString(value.After)
		b.WriteByte(';')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

func parseAuditChangedFields(value string) map[string]state.AssignmentChangedField {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(value), &object); err == nil {
		changed := make(map[string]state.AssignmentChangedField)
		for key, raw := range object {
			field := sanitizeAuditFieldName(key)
			if field == "" {
				continue
			}
			changed[field] = changedFieldFromRaw(raw)
		}
		if len(changed) > 0 {
			return changed
		}
	}
	var list []any
	if err := json.Unmarshal([]byte(value), &list); err == nil {
		changed := make(map[string]state.AssignmentChangedField)
		for _, raw := range list {
			field := sanitizeAuditFieldName(fmt.Sprint(raw))
			if field != "" {
				changed[field] = state.AssignmentChangedField{}
			}
		}
		if len(changed) > 0 {
			return changed
		}
	}
	return nil
}

func changedFieldFromRaw(raw any) state.AssignmentChangedField {
	object, ok := raw.(map[string]any)
	if !ok {
		return state.AssignmentChangedField{After: sanitizeAuditValue(formatAuditValue(raw))}
	}
	return state.AssignmentChangedField{
		Before: sanitizeAuditValue(firstAuditObjectValue(object, "before", "old", "from", "previous", "previousValue")),
		After:  sanitizeAuditValue(firstAuditObjectValue(object, "after", "new", "to", "current", "currentValue")),
	}
}

func firstAuditObjectValue(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return formatAuditValue(value)
		}
	}
	return ""
}

func formatAuditValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func sanitizeAuditFieldName(value string) string {
	value = strings.TrimSpace(security.SanitizeText(value))
	if value == "" || security.IsForbiddenField(value) {
		return ""
	}
	if len([]rune(value)) > 64 {
		return string([]rune(value)[:64])
	}
	return value
}

func sanitizeAuditValue(value string) string {
	value = strings.TrimSpace(security.SanitizeText(value))
	if value == "" {
		return ""
	}
	if len([]rune(value)) > 160 {
		return string([]rune(value)[:157]) + "..."
	}
	return value
}

func parseAuditTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func rowValue(row map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(row[key]); value != "" {
			return value
		}
	}
	return ""
}

func formatAssignmentAuditEvent(event state.AssignmentAuditEventState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", assignmentAuditIcon(event.EventType), assignmentAuditTitle(event.EventType))
	fmt.Fprintf(&b, "eventType: %s\n", event.EventType)
	fmt.Fprintf(&b, "course: %s\n", unknownAuditValue(event.CourseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", unknownAuditValue(event.AssignmentID))
	fmt.Fprintf(&b, "title: %s\n", unknownAuditValue(event.Title))
	b.WriteString("\nactor:\n")
	fmt.Fprintf(&b, "- userId: %s\n", unknownAuditValue(event.ActorID))
	fmt.Fprintf(&b, "- role: %s\n", unknownAuditValue(event.ActorRole))
	fmt.Fprintf(&b, "- name: %s\n", unknownAuditValue(event.ActorName))
	fmt.Fprintf(&b, "\noccurredAt: %s\n", formatKST(event.OccurredAt))
	fmt.Fprintf(&b, "traceId: %s\n", unknownAuditValue(event.TraceID))
	fmt.Fprintf(&b, "requestId: %s\n", unknownAuditValue(event.RequestID))
	fmt.Fprintf(&b, "source: %s\n", assignmentAuditSource)
	if len(event.ChangedFields) > 0 {
		b.WriteString("\nchangedFields:\n")
		keys := make([]string, 0, len(event.ChangedFields))
		for key := range event.ChangedFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := event.ChangedFields[key]
			fmt.Fprintf(&b, "- %s: %s -> %s\n", key, unknownAuditValue(value.Before), unknownAuditValue(value.After))
		}
	}
	if event.EventType == "ASSIGNMENT_DELETED" {
		b.WriteString("\nnote: 삭제된 과제는 /ops assignment 조회가 실패할 수 있습니다.\n")
	}
	b.WriteString("\nnext:\n")
	query := firstNonEmpty(event.TraceID, event.AssignmentID)
	fmt.Fprintf(&b, "1. /ops logs service:report mode:recent query:%s since:24h limit:20\n", unknownAuditValue(query))
	b.WriteString("   - 이 운영 행위의 서버 로그를 확인합니다.")
	return formatting.TruncateDiscordMessage(b.String())
}

func assignmentAuditTitle(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_CREATED":
		return "과제 등록"
	case "ASSIGNMENT_UPDATED":
		return "과제 수정"
	case "ASSIGNMENT_DELETED":
		return "과제 삭제"
	case "ASSIGNMENT_PUBLISHED":
		return "과제 공개"
	case "ASSIGNMENT_UNPUBLISHED":
		return "과제 비공개"
	default:
		return "과제 audit"
	}
}

func assignmentAuditIcon(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_CREATED":
		return "📝"
	case "ASSIGNMENT_UPDATED":
		return "✏️"
	case "ASSIGNMENT_DELETED":
		return "🗑️"
	case "ASSIGNMENT_PUBLISHED":
		return "📣"
	case "ASSIGNMENT_UNPUBLISHED":
		return "🔒"
	default:
		return "ℹ️"
	}
}

func unknownAuditValue(value string) string {
	value = strings.TrimSpace(security.SanitizeText(value))
	if value == "" {
		return "unknown"
	}
	return value
}
