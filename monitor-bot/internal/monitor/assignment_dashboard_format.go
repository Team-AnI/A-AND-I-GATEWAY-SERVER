package monitor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/formatting"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/reportadmin"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/state"
)

func formatAssignmentDashboard(result assignmentPollResult) string {
	status := "정상"
	if result.APIStatus != reportadmin.StatusOK || result.AssignmentIssues > 0 || result.GradingFailedDelta > 0 {
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
	fmt.Fprintf(&b, "상태 점검 대상: %d개\n\n", result.AssignmentIssues)
	fmt.Fprintf(&b, "채점 진행 중 과제: %d개\n", result.GradingInProgress)
	fmt.Fprintf(&b, "최근 채점 완료 업데이트: %d건\n", result.GradingCompletedDelta)
	fmt.Fprintf(&b, "채점 실패 감지: %d건\n\n", result.GradingFailedDelta)
	b.WriteString("최근 이벤트\n")
	recentGroups := groupRecentAssignmentEvents(result.RecentEvents, 5)
	if len(recentGroups) == 0 {
		b.WriteString("- 아직 이벤트 없음\n")
	} else {
		for i, group := range recentGroups {
			fmt.Fprintf(&b, "%d. %s\n", i+1, formatAssignmentRecentEventGroup(group))
		}
	}
	b.WriteString("\n상세 확인\n")
	b.WriteString("/ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis\n")
	b.WriteString("/ops assignment course:<courseSlug> id:<assignmentId> view:events\n")
	b.WriteString("/ops assignment course:<courseSlug> id:<assignmentId> action:submissions\n")
	b.WriteString("/ops logs service:report mode:events query:<assignmentId> since:24h limit:20")
	return formatting.TruncateDiscordMessage(b.String())
}

type assignmentRecentEventGroup struct {
	Key           string
	EventType     string
	Severity      string
	CourseSlug    string
	Summary       string
	ReasonCode    string
	Count         int
	FirstAt       time.Time
	LatestAt      time.Time
	AssignmentIDs []string
	IssueKey      string
	EvidenceHash  string
}

func groupRecentAssignmentEvents(events []state.AssignmentEventState, limit int) []assignmentRecentEventGroup {
	if limit <= 0 {
		limit = 5
	}
	groups := map[string]*assignmentRecentEventGroup{}
	order := make([]string, 0, len(events))
	for _, event := range events {
		key := assignmentRecentEventGroupKey(event)
		if key == "" {
			continue
		}
		group, ok := groups[key]
		if !ok {
			group = &assignmentRecentEventGroup{
				Key:        key,
				EventType:  strings.TrimSpace(event.EventType),
				Severity:   strings.TrimSpace(event.Severity),
				CourseSlug: strings.TrimSpace(event.CourseSlug),
				Summary:    strings.TrimSpace(event.Summary),
				ReasonCode: strings.TrimSpace(event.ReasonCode),
			}
			groups[key] = group
			order = append(order, key)
		}
		group.Count++
		if severityRank(event.Severity) > severityRank(group.Severity) {
			group.Severity = strings.TrimSpace(event.Severity)
		}
		if group.EventType == "" {
			group.EventType = strings.TrimSpace(event.EventType)
		}
		if group.CourseSlug == "" {
			group.CourseSlug = strings.TrimSpace(event.CourseSlug)
		}
		if group.Summary == "" {
			group.Summary = strings.TrimSpace(event.Summary)
		}
		if group.ReasonCode == "" {
			group.ReasonCode = strings.TrimSpace(event.ReasonCode)
		}
		if group.IssueKey == "" {
			group.IssueKey = strings.TrimSpace(event.IssueKey)
		}
		if group.EvidenceHash == "" {
			group.EvidenceHash = strings.TrimSpace(event.EvidenceHash)
		}
		if !event.CreatedAt.IsZero() {
			if group.FirstAt.IsZero() || event.CreatedAt.Before(group.FirstAt) {
				group.FirstAt = event.CreatedAt
			}
			if group.LatestAt.IsZero() || event.CreatedAt.After(group.LatestAt) {
				group.LatestAt = event.CreatedAt
			}
		}
		addAssignmentRecentID(group, event.AssignmentID)
	}
	result := make([]assignmentRecentEventGroup, 0, len(groups))
	for _, key := range order {
		result = append(result, *groups[key])
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].LatestAt.Equal(result[j].LatestAt) {
			return result[i].Key < result[j].Key
		}
		if result[i].LatestAt.IsZero() {
			return false
		}
		if result[j].LatestAt.IsZero() {
			return true
		}
		return result[i].LatestAt.After(result[j].LatestAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func assignmentRecentEventGroupKey(event state.AssignmentEventState) string {
	parts := []string{
		strings.TrimSpace(event.EventType),
		strings.TrimSpace(event.CourseSlug),
		strings.TrimSpace(event.Summary),
		strings.TrimSpace(event.ReasonCode),
	}
	if strings.Join(parts, "") == "" {
		parts = append(parts, strings.TrimSpace(event.Fingerprint))
	}
	for i, part := range parts {
		parts[i] = strings.ToLower(part)
	}
	return strings.Trim(strings.Join(parts, "\x00"), "\x00")
}

func addAssignmentRecentID(group *assignmentRecentEventGroup, assignmentID string) {
	assignmentID = strings.TrimSpace(assignmentID)
	if assignmentID == "" || !security.ValidateAssignmentID(assignmentID) {
		return
	}
	for _, existing := range group.AssignmentIDs {
		if existing == assignmentID {
			return
		}
	}
	group.AssignmentIDs = append(group.AssignmentIDs, assignmentID)
}

func formatAssignmentRecentEventGroup(group assignmentRecentEventGroup) string {
	var b strings.Builder
	eventType := firstNonEmpty(group.EventType, "EVENT")
	course := firstNonEmpty(group.CourseSlug, "<course>")
	count := group.Count
	if count <= 0 {
		count = 1
	}
	fmt.Fprintf(&b, "%s %s %s ×%d\n", assignmentDashboardEventIcon(group), security.SanitizeText(course), security.SanitizeText(eventType), count)
	if strings.TrimSpace(group.Summary) != "" {
		fmt.Fprintf(&b, "   %s\n", security.SanitizeText(group.Summary))
	}
	if !group.LatestAt.IsZero() {
		fmt.Fprintf(&b, "   latest=%s", formatAssignmentClock(group.LatestAt))
		if !group.FirstAt.IsZero() && !group.FirstAt.Equal(group.LatestAt) {
			fmt.Fprintf(&b, " first=%s", formatAssignmentClock(group.FirstAt))
		}
		b.WriteString("\n")
	}
	if len(group.AssignmentIDs) > 0 {
		b.WriteString("   assignments: ")
		for i, assignmentID := range group.AssignmentIDs {
			if i >= 3 {
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(security.SanitizeText(assignmentID))
		}
		if extra := len(group.AssignmentIDs) - 3; extra > 0 {
			fmt.Fprintf(&b, " (+%d)", extra)
		}
		b.WriteString("\n")
	}
	if group.IssueKey != "" {
		fmt.Fprintf(&b, "   issue: %s\n", security.SanitizeText(group.IssueKey))
	}
	if group.EvidenceHash != "" {
		fmt.Fprintf(&b, "   evidence: %s\n", security.SanitizeText(group.EvidenceHash))
	}
	if len(group.AssignmentIDs) == 0 {
		return strings.TrimRight(b.String(), "\n")
	}
	assignmentID := security.SanitizeText(group.AssignmentIDs[0])
	course = security.SanitizeText(course)
	if strings.EqualFold(group.Severity, "WARN") {
		fmt.Fprintf(&b, "   detail: /ops assignment course:%s id:%s view:diagnosis\n", course, assignmentID)
		fmt.Fprintf(&b, "   events: /ops assignment course:%s id:%s view:events\n", course, assignmentID)
		fmt.Fprintf(&b, "   ack: /ops assignment course:%s id:%s action:ack event:%s until:7d reason:<reason>\n", course, assignmentID, assignmentEventSlug(group.EventType))
	} else {
		fmt.Fprintf(&b, "   detail: /ops assignment course:%s id:%s view:events\n", course, assignmentID)
	}
	fmt.Fprintf(&b, "   logs: /ops logs service:report mode:events query:%s since:24h limit:20", assignmentID)
	return strings.TrimRight(b.String(), "\n")
}

func assignmentDashboardEventIcon(group assignmentRecentEventGroup) string {
	if strings.EqualFold(group.Severity, "WARN") || strings.EqualFold(group.Severity, "ERROR") || strings.EqualFold(group.Severity, "CRITICAL") {
		return "⚠️"
	}
	switch group.EventType {
	case "ASSIGNMENT_CREATED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_UNPUBLISHED", "ASSIGNMENT_DELETED", "ASSIGNMENT_UPDATED", "GRADING_COMPLETED":
		return "✅"
	default:
		return "ℹ️"
	}
}

func formatAssignmentClock(value time.Time) string {
	return value.In(time.FixedZone("KST", 9*60*60)).Format("15:04")
}

const assignmentIssueSource = "WEB_ADMIN_API"

type assignmentIssueDigestGroup struct {
	CourseSlug string
	EventType  string
	Severity   string
	Source     string
	Events     []state.AssignmentEventState
	Suppressed int
}

func assignmentIssueGroupKey(event state.AssignmentEventState) string {
	return strings.Join([]string{event.CourseSlug, event.EventType, event.Severity, assignmentIssueSource}, "\x00")
}

func formatAssignmentIssueDigest(group assignmentIssueDigestGroup) string {
	var b strings.Builder
	total := len(group.Events) + group.Suppressed
	fmt.Fprintf(&b, "%s 과제 상태 점검 %d건\n", eventIcon(state.AssignmentEventState{Severity: group.Severity}), total)
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(group.CourseSlug))
	fmt.Fprintf(&b, "eventType: %s\n", security.SanitizeText(group.EventType))
	fmt.Fprintf(&b, "severity: %s\n", security.SanitizeText(group.Severity))
	fmt.Fprintf(&b, "source: %s\n\n", security.SanitizeText(firstNonEmpty(group.Source, assignmentIssueSource)))
	newlyOpened := 0
	publishedAtUnknown := 0
	staleCandidate := 0
	for _, event := range group.Events {
		if event.NotifyCount <= 1 {
			newlyOpened++
		}
		if strings.TrimSpace(event.PublishedAt) == "" {
			publishedAtUnknown++
		}
		if group.EventType == "ASSIGNMENT_STALE_DRAFT" || strings.Contains(strings.ToLower(event.ReasonText), "stale") {
			staleCandidate++
		}
	}
	fmt.Fprintf(&b, "summary:\n")
	fmt.Fprintf(&b, "- newly opened: %d\n", newlyOpened)
	fmt.Fprintf(&b, "- repeated suppressed: %d\n", group.Suppressed)
	if publishedAtUnknown > 0 {
		fmt.Fprintf(&b, "- publishedAt unknown: %d\n", publishedAtUnknown)
	}
	if staleCandidate > 0 {
		fmt.Fprintf(&b, "- stale candidate: %d\n", staleCandidate)
	}
	if len(group.Events) > 0 && strings.TrimSpace(group.Events[0].ReasonText) != "" {
		fmt.Fprintf(&b, "- reason: %s\n", security.SanitizeText(group.Events[0].ReasonText))
	}
	b.WriteString("\nexamples:\n")
	for i, event := range group.Events {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&b, "%d. %s title: %s startAt: %s\n", i+1, security.SanitizeText(event.AssignmentID), security.SanitizeText(unknownIfBlank(event.Title)), security.SanitizeText(unknownIfBlank(event.StartAt)))
	}
	if extra := len(group.Events) - 5; extra > 0 {
		fmt.Fprintf(&b, "... and %d more\n", extra)
	}
	example := state.AssignmentEventState{CourseSlug: group.CourseSlug}
	if len(group.Events) > 0 {
		example = group.Events[0]
	}
	writeAssignmentDigestNextCommands(&b, example)
	return formatting.TruncateDiscordMessage(b.String())
}

func formatAssignmentEvent(event state.AssignmentEventState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", eventIcon(event), assignmentEventTitle(event.EventType))
	fmt.Fprintf(&b, "eventType: %s\n", event.EventType)
	fmt.Fprintf(&b, "severity: %s\n", event.Severity)
	fmt.Fprintf(&b, "course: %s\n", security.SanitizeText(event.CourseSlug))
	fmt.Fprintf(&b, "assignmentId: %s\n", security.SanitizeText(event.AssignmentID))
	fmt.Fprintf(&b, "title: %s\n", security.SanitizeText(unknownIfBlank(event.Title)))
	fmt.Fprintf(&b, "status: %s\n", security.SanitizeText(unknownIfBlank(event.Status)))
	fmt.Fprintf(&b, "publishedAt: %s\n", security.SanitizeText(assignmentEventPublishedAt(event)))
	fmt.Fprintf(&b, "startAt: %s\n", security.SanitizeText(unknownIfBlank(event.StartAt)))
	fmt.Fprintf(&b, "endAt: %s\n", security.SanitizeText(unknownIfBlank(event.EndAt)))
	fmt.Fprintf(&b, "problemId: %s\n", security.SanitizeText(unknownIfBlank(event.ProblemID)))
	if strings.TrimSpace(event.ProblemIDFallback) != "" {
		fmt.Fprintf(&b, "problemIdFallback: %s\n", security.SanitizeText(event.ProblemIDFallback))
	}
	fmt.Fprintf(&b, "summary: %s\n", security.SanitizeText(event.Summary))
	b.WriteString("source: WEB_ADMIN_API\n")
	if strings.TrimSpace(event.ReasonCode) != "" {
		fmt.Fprintf(&b, "\nreasonCode: %s\n", security.SanitizeText(event.ReasonCode))
		fmt.Fprintf(&b, "reasonText: %s\n", security.SanitizeText(event.ReasonText))
		if event.EventType == "ASSIGNMENT_DRAFT_PAST_START" {
			b.WriteString("note: publishedAt이 없어 공개 지연으로 단정할 수 없음\n")
		}
		if len(event.Evidence) > 0 {
			b.WriteString("evidence:\n")
			for _, evidence := range event.Evidence {
				fmt.Fprintf(&b, "- %s\n", security.SanitizeText(evidence))
			}
		}
	}
	if strings.TrimSpace(event.IssueState) != "" {
		b.WriteString("\nissue:\n")
		fmt.Fprintf(&b, "- state: %s\n", security.SanitizeText(event.IssueState))
		fmt.Fprintf(&b, "- firstDetectedAt: %s\n", formatKST(event.FirstDetectedAt))
		fmt.Fprintf(&b, "- lastDetectedAt: %s\n", formatKST(event.LastDetectedAt))
		fmt.Fprintf(&b, "- notifyCount: %d\n", event.NotifyCount)
		fmt.Fprintf(&b, "- repeatPolicy: %s\n", security.SanitizeText(event.RepeatPolicy))
	}
	writeAssignmentNextCommands(&b, event)
	return formatting.TruncateDiscordMessage(b.String())
}

func assignmentEventPublishedAt(event state.AssignmentEventState) string {
	if strings.TrimSpace(event.PublishedAt) != "" {
		return event.PublishedAt
	}
	if event.PublishedAtOmitted {
		return "summary omitted"
	}
	return "unknown"
}

func shouldSendAssignmentEvent(event state.AssignmentEventState) bool {
	if isAssignmentIssueEvent(event.EventType) {
		return event.ShouldNotify
	}
	switch event.EventType {
	case "ASSIGNMENT_CREATED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_PUBLISH_DELAYED", "ASSIGNMENT_INVALID_TIME", "GRADING_COMPLETED", "GRADING_FAILED":
		return true
	}
	return strings.HasPrefix(event.EventType, "WEB_ADMIN_API_")
}

func assignmentEventTitle(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_CREATED":
		return "과제 등록 확인"
	case "ASSIGNMENT_PUBLISHED":
		return "과제 공개 완료"
	case "ASSIGNMENT_PUBLISH_DELAYED":
		return "과제 공개 지연"
	case "ASSIGNMENT_DRAFT_PAST_START":
		return "과제 상태 점검 필요"
	case "ASSIGNMENT_STALE_DRAFT":
		return "오래된 DRAFT 과제"
	case "ASSIGNMENT_MISSING_PROBLEM":
		return "과제 problemId 누락"
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
	if strings.EqualFold(event.Severity, "WARN") || strings.EqualFold(event.Severity, "ERROR") || strings.EqualFold(event.Severity, "CRITICAL") {
		return "⚠️"
	}
	switch event.EventType {
	case "ASSIGNMENT_CREATED", "ASSIGNMENT_PUBLISHED", "ASSIGNMENT_UNPUBLISHED", "ASSIGNMENT_DELETED", "ASSIGNMENT_UPDATED", "GRADING_COMPLETED":
		return "✅"
	default:
		return "ℹ️"
	}
}

func writeAssignmentNextCommands(b *strings.Builder, event state.AssignmentEventState) {
	course := security.SanitizeText(event.CourseSlug)
	id := security.SanitizeText(event.AssignmentID)
	b.WriteString("\nnext:\n")
	switch event.EventType {
	case "ASSIGNMENT_MISSING_PROBLEM":
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s action:check\n", course, id)
		b.WriteString("   - problemId 연결과 제출 가능성 체크리스트를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops assignment course:%s id:%s action:submissions\n", course, id)
		b.WriteString("   - 제출/채점 상태가 누락 문제와 연결되는지 확인합니다.\n")
		fmt.Fprintf(b, "3. /ops logs service:report mode:events query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report 로그에서 해당 assignmentId를 검색합니다.")
	case "ASSIGNMENT_PUBLISH_DELAYED":
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s view:diagnosis\n", course, id)
		b.WriteString("   - 봇이 공개 지연으로 분류한 필드별 근거를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops assignment course:%s id:%s view:events\n", course, id)
		b.WriteString("   - firstDetectedAt, lastDetectedAt, notifyCount, ack/silence 상태를 봅니다.\n")
		fmt.Fprintf(b, "3. /ops logs service:report mode:events query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report EVENT 로그에서 이 assignmentId의 publish/update 흔적을 찾습니다.\n")
		fmt.Fprintf(b, "4. /ops assignment course:%s id:%s action:ack event:publish-delayed until:7d reason:<reason>\n", course, id)
		b.WriteString("   - 의도된 상태라면 반복 알림을 중지합니다.")
	case "ASSIGNMENT_DRAFT_PAST_START", "ASSIGNMENT_STALE_DRAFT", "ASSIGNMENT_INVALID_TIME":
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s view:diagnosis\n", course, id)
		b.WriteString("   - 공개 지연 단정이 가능한지와 부족한 필드를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops assignment course:%s id:%s view:events\n", course, id)
		b.WriteString("   - 봇 감지 이력과 반복 억제 상태를 확인합니다.\n")
		fmt.Fprintf(b, "3. /ops logs service:report mode:events query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report EVENT 로그에서 이 과제의 update/publish 이벤트를 검색합니다.\n")
		fmt.Fprintf(b, "4. /ops assignment course:%s id:%s action:ack event:%s until:7d reason:<reason>\n", course, id, assignmentEventSlug(event.EventType))
		b.WriteString("   - 오래된 draft 등 의도된 상태라면 알림을 중지합니다.")
	default:
		fmt.Fprintf(b, "1. /ops assignment course:%s id:%s view:diagnosis\n", course, id)
		b.WriteString("   - 과제 필드와 봇 판단 근거를 확인합니다.\n")
		fmt.Fprintf(b, "2. /ops logs service:report mode:events query:%s since:24h limit:20\n", id)
		b.WriteString("   - Report 로그에서 관련 이벤트를 검색합니다.")
	}
}

func writeAssignmentDigestNextCommands(b *strings.Builder, event state.AssignmentEventState) {
	course := security.SanitizeText(event.CourseSlug)
	id := security.SanitizeText(firstNonEmpty(event.AssignmentID, "<id>"))
	b.WriteString("\nnext:\n")
	fmt.Fprintf(b, "- /ops assignment course:%s id:%s view:diagnosis\n", course, id)
	b.WriteString("  - 단일 과제의 판단 근거를 확인합니다.\n")
	fmt.Fprintf(b, "- /ops assignment course:%s id:%s view:events\n", course, id)
	b.WriteString("  - 봇 감지 이력과 반복 억제 상태를 확인합니다.\n")
	fmt.Fprintf(b, "- /ops logs service:report mode:events query:%s since:24h limit:20\n", id)
	b.WriteString("  - 과제 생성/수정/삭제/공개 EVENT 로그를 확인합니다.")
}

func assignmentEventSlug(eventType string) string {
	switch eventType {
	case "ASSIGNMENT_PUBLISH_DELAYED":
		return "publish-delayed"
	case "ASSIGNMENT_DRAFT_PAST_START":
		return "draft-past-start"
	case "ASSIGNMENT_STALE_DRAFT":
		return "stale-draft"
	case "ASSIGNMENT_INVALID_TIME":
		return "invalid-time"
	case "ASSIGNMENT_MISSING_PROBLEM":
		return "missing-problem"
	case "GRADING_FAILED":
		return "grading-failed"
	default:
		return strings.ToLower(strings.TrimPrefix(eventType, "ASSIGNMENT_"))
	}
}

func formatKST(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04 KST")
}
