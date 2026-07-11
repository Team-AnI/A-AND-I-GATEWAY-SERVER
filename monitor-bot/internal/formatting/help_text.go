package formatting

import (
	"fmt"
	"strings"

	"github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/monitor-bot/internal/security"
)

func HelpText() string {
	return HelpTextFor("", "", "")
}

func HelpTextFor(topic, command, query string) string {
	command = strings.ToLower(strings.TrimSpace(command))
	if command != "" {
		return helpCommandText(command)
	}
	switch strings.ToLower(strings.TrimSpace(topic)) {
	case "assignments":
		return helpAssignmentsText()
	case "logs":
		return helpLogsText()
	case "alerts":
		return helpAlertsText()
	case "dashboard":
		return helpDashboardText()
	case "routing":
		return helpRoutingText()
	case "audit":
		return helpAuditText()
	case "troubleshooting":
		return helpTroubleshootingText()
	}
	if strings.TrimSpace(query) != "" {
		return helpQueryText(query)
	}
	return strings.TrimSpace(`A&I Ops Bot 도움말

기본 명령은 5개입니다.

1. /ops dashboard
   - 전체/서비스 상태와 dashboard watch를 봅니다.
   - 예: /ops dashboard
   - 예: /ops dashboard service:report

2. /ops logs
   - 오류, CRITICAL, trace, EVENT/audit 로그를 검색합니다.
   - 예: /ops logs service:report mode:errors since:30m limit:10
   - 예: /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
   - 예: /ops logs mode:trace query:<traceId>

3. /ops alert
   - general/critical 채널과 CRITICAL role mention을 설정합니다.
   - 예: /ops alert action:channel target:general channel:#ops-log
   - 예: /ops alert action:channel target:critical channel:#ops-critical
   - 예: /ops alert action:role role:@운영팀

4. /ops assignment
   - 과제 목록, 상세, 진단, 이벤트 이력, ack, 제출 상태를 봅니다.
   - 예: /ops assignment course:3rd-cs
   - 예: /ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis
   - 예: /ops assignment course:<courseSlug> id:<assignmentId> view:events

5. /ops help
   - 상황별로 쓸 명령을 검색합니다.
   - 예: /ops help query:"과제 수정 누가"
   - 예: /ops help query:"critical role"

주의:
- 봇은 과제를 생성/수정/삭제/공개하지 않습니다.
- 누가/언제 과제를 변경했는지는 Report EVENT 로그에서 확인합니다.
- CRITICAL 서버 장애만 role mention을 사용합니다.`)
}

func helpDashboardText() string {
	return strings.TrimSpace(`A&I Ops 대시보드 도움말

/ops dashboard
→ 전체 서비스 최근 30분 상태를 봅니다.

/ops dashboard service:<service>
→ 특정 서비스 하나의 health/log/error 요약을 봅니다.

/ops dashboard action:watch channel:#ops interval:5m
→ 하나의 dashboard 메시지를 주기적으로 edit/update합니다.

/ops dashboard action:unwatch
→ dashboard watch를 해제합니다.

/ops dashboard action:status
→ 등록된 dashboard watch를 확인합니다.

이 명령은 예전 service/watch/unwatch 흐름을 대체합니다.`)
}

func helpLogsText() string {
	return strings.TrimSpace(`A&I Ops 로그 도움말

/ops logs
→ 전체 서비스 오류 로그를 최근 30분 기준으로 봅니다.

/ops logs service:report mode:recent query:<assignmentId|traceId|eventType> since:24h limit:20
→ 구조화 필드에서 검색하고 @message는 fallback 검색으로만 사용합니다.

/ops logs service:report mode:errors since:30m limit:10
→ API_ERROR/EVENT_ERROR 집계를 봅니다.

/ops logs service:report mode:critical since:30m limit:10
→ CRITICAL 장애 후보를 확인합니다.

/ops logs service:report mode:slow since:30m limit:10
→ 느린 API 로그를 확인합니다.

/ops logs service:report mode:security since:30m limit:10
→ 보안 로그를 확인합니다.

/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
→ Report assignment audit EVENT 로그에서 actor와 발생 시각을 봅니다.

/ops logs mode:trace query:<traceId>
→ traceId 기준 요청 흐름을 봅니다.

/ops logs action:watch service:report mode:errors channel:#report-logs interval:5m
/ops logs action:unwatch service:report mode:errors
/ops logs action:watches
→ 로그 feed 등록/해제/목록을 관리합니다.

분류는 structured V2 fields 기준입니다. @message는 fallback 검색/표시로만 사용합니다.`)
}

func helpAlertsText() string {
	return strings.TrimSpace(`A&I Ops 알림 도움말

/ops alert action:channel target:general channel:#ops-log
→ assignment audit, assignment issue WARN/INFO, HIGH service alert, 일반 운영 로그 채널입니다.

/ops alert action:channel target:critical channel:#ops-critical
→ CRITICAL 서버 장애 전용 채널입니다.

/ops alert action:channel target:all channel:#ops-alerts
→ general/critical을 같은 채널로 저장합니다.

/ops alert action:role role:@운영팀
→ CRITICAL 서버 장애에서만 mention할 역할을 저장합니다.

/ops alert action:role-clear
→ 저장된 role mention 설정을 지웁니다.

/ops alert action:status
→ general/critical 채널, role, fallback, cooldown, 최근 alert 상태를 봅니다.

/ops alert action:test target:general
/ops alert action:test target:critical
→ route별 테스트 메시지를 보냅니다. test는 role mention을 보내지 않습니다.

HIGH/general/audit/WARN은 role mention을 하지 않습니다. @everyone/@here는 허용하지 않습니다.`)
}

func helpRoutingText() string {
	return strings.TrimSpace(`A&I Ops 라우팅 도움말

general route:
- assignment audit 성공 이벤트
- assignment issue WARN/INFO
- HIGH service alert
- 일반 운영 로그
- role mention 없음

critical route:
- CRITICAL 서버 장애만 전송
- 설정된 role mention 사용

설정 예시:
- /ops alert action:channel target:general channel:#ops-log
- /ops alert action:channel target:critical channel:#ops-critical
- /ops alert action:role role:@운영팀

fallback:
- general: state general channel → legacy alert channel → DISCORD_ALERT_CHANNEL_ID → dashboard channel
- critical: state critical channel → legacy alert channel → DISCORD_ALERT_CHANNEL_ID`)
}

func helpAssignmentsText() string {
	return strings.TrimSpace(`과제 운영 도움말

/ops assignment
- 목적: 과제 목록, 상세, 진단, 감지 이력, 체크리스트, 제출 상태, ack/unack을 한 명령에서 처리
- list: /ops assignment course:3rd-cs action:list status:draft
- all list: /ops assignment scope:all action:list window:today
- summary: /ops assignment course:<courseSlug> id:<assignmentId>
- diagnosis: /ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis
- events: /ops assignment course:<courseSlug> id:<assignmentId> view:events
- check: /ops assignment course:<courseSlug> id:<assignmentId> action:check
- submissions: /ops assignment course:<courseSlug> id:<assignmentId> action:submissions
- ack: /ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>
- 주의: 누가 변경했는지 증명하는 audit trail이 아닙니다.

/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
- 목적: WEB Admin API가 답하지 못하는 원인을 CloudWatch 로그에서 검색

Assignment audit notifications
- 목적: 과제 등록/수정/삭제/공개/비공개를 누가 언제 했는지 확인
- source: Report EVENT logs only
- 이벤트: ASSIGNMENT_CREATED, ASSIGNMENT_UPDATED, ASSIGNMENT_DELETED, ASSIGNMENT_PUBLISHED, ASSIGNMENT_UNPUBLISHED
- 조회: /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
- 주의: bot은 과제를 생성/수정/삭제/공개하지 않습니다. actor/occurredAt은 EVENT 로그에 있을 때만 표시하고 없으면 unknown입니다.`)
}

func helpAuditText() string {
	return strings.TrimSpace(`Assignment audit 도움말

Assignment audit 알림은 자동으로 전송됩니다.
- source: Report V2 EVENT logs
- route: general
- role mention: 없음

대상 이벤트:
- ASSIGNMENT_CREATED
- ASSIGNMENT_UPDATED
- ASSIGNMENT_DELETED
- ASSIGNMENT_PUBLISHED
- ASSIGNMENT_UNPUBLISHED

표시 필드:
- actor.userId, actor.role, 안전한 actor name/displayName/loginId
- occurredAt
- traceId, requestId
- assignmentId, title
- changedFields

actor/occurredAt이 없으면 unknown으로 표시합니다. WEB Admin API snapshot에서 actor를 추측하지 않습니다.

수동 조회:
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20`)
}

func helpTroubleshootingText() string {
	return strings.TrimSpace(`A&I Ops 문제 해결 도움말

Discord command가 보이지 않음:
- 배포/등록 시점에 DISCORD_REGISTER_COMMANDS=true로 1회 등록했는지 확인합니다.
- /healthz의 discordCommandsRegistered, discordCommandRegistrationError를 확인합니다.

CRITICAL role mention이 동작하지 않음:
- /ops alert action:status로 role/channel 상태를 확인합니다.
- bot이 해당 role을 mention할 권한이 있는지 확인합니다.

과제 WARN이 많음:
- Too many assignment warnings:
- issue digest와 repeated suppressed count를 확인합니다.
- /ops assignment course:<courseSlug> id:<assignmentId> view:events로 감지 이력을 봅니다.
- 의도된 stale issue는 /ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>으로 묶습니다.

누가 과제를 수정했는지 확인:
- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20

서버 장애 원인 추적:
- traceId가 있으면 /ops logs mode:trace query:<traceId>를 사용합니다.`)
}

func helpCommandText(command string) string {
	switch command {
	case "assignment-check":
		return strings.TrimSpace(`/ops assignment action:check

역할:
특정 과제가 운영상 제출 가능한 상태인지 점검합니다.

확인하는 것:
- title 존재 여부
- assignmentStatus
- publishedAt/startAt/endAt 시간 관계
- problemId 연결 여부
- 봇이 감지한 issue와의 연결

주의:
이 명령은 왜 알림이 발생했는지 설명해야 합니다. problemId 누락만으로 DRAFT past start를 설명하지 않습니다.

다음 단계:
- 서버 로그 검색: /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
- 감지 이력 확인: /ops assignment course:<courseSlug> id:<assignmentId> view:events
- 의도된 상태면 ack: /ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>`)
	case "dashboard":
		return strings.TrimSpace(`/ops dashboard

역할:
전체/단일 서비스 상태와 dashboard watch를 관리합니다.

언제 사용:
- 현재 서비스 상태를 빠르게 볼 때
- 운영 채널에 고정 dashboard를 만들거나 해제할 때

예시:
- /ops dashboard
- /ops dashboard service:report
- /ops dashboard action:watch channel:#ops interval:5m
- /ops dashboard action:unwatch

이 명령은 예전 service/watch 흐름을 대체합니다.`)
	case "logs":
		return HelpTextFor("logs", "", "")
	case "alert":
		return strings.TrimSpace(`/ops alert

역할:
general/critical 채널, CRITICAL role mention, alert on/off/test를 설정합니다.

예시:
- /ops alert action:channel target:general channel:#ops-log
- /ops alert action:channel target:critical channel:#ops-critical
- /ops alert action:role role:@운영팀
- /ops alert action:status
- /ops alert action:test target:critical

정책:
- CRITICAL 서버 장애만 critical route와 role mention을 사용합니다.
- HIGH/general/audit/WARN은 role mention을 하지 않습니다.`)
	case "assignment":
		return strings.TrimSpace(`/ops assignment

역할:
단일 과제의 현재 상태, 진단, 봇 감지 이력, ack/unack을 처리합니다.
read-only 명령이며 과제를 생성/수정/삭제/공개하지 않습니다.

확인하는 것:
- title/status/publishedAt/startAt/endAt/problemId
- 봇 issue lifecycle와 ack/silence 상태
- 제출/채점 요약

view:
- summary: 기본 필드
- diagnosis: 봇이 이상 상태로 판단한 근거와 issue lifecycle
- events: 봇 감지 이력, 반복 억제, ack/silence 상태
- raw: 민감정보 제외 원본 주요 필드

action:
- list: 과제 목록 조회
- check: 운영 체크리스트
- submissions: 제출/채점 상태
- ack: 의도된 과제 이슈의 반복 알림 중지
- unack: ack 해제

예시:
/ops assignment course:3rd-cs action:list status:draft
/ops assignment course:<courseSlug> id:<assignmentId> view:events
/ops assignment course:<courseSlug> id:<assignmentId> action:check
/ops assignment course:<courseSlug> id:<assignmentId> action:submissions
/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>`)
	case "help":
		return strings.TrimSpace(`/ops help

역할:
상황별로 어떤 명령을 써야 하는지 검색합니다.

사용 방식:
- /ops help
- /ops help topic:<topic>
- /ops help command:<command>
- /ops help query:"과제 수정 누가"
- /ops help query:"critical role"`)
	default:
		return HelpTextFor("overview", "", "")
	}
}

func helpQueryText(query string) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	assignmentQuery := strings.Contains(normalized, "과제") || strings.Contains(normalized, "assignment")
	deleteQuery := strings.Contains(normalized, "삭제") || strings.Contains(normalized, "delete") || strings.Contains(normalized, "deleted")
	updateActorQuery := strings.Contains(normalized, "수정") || strings.Contains(normalized, "update") || strings.Contains(normalized, "updated") || strings.Contains(normalized, "누가") || strings.Contains(normalized, "who") || strings.Contains(normalized, "actor")
	criticalRoleQuery := strings.Contains(normalized, "critical") || strings.Contains(normalized, "장애") || strings.Contains(normalized, "role") || strings.Contains(normalized, "mention")
	generalChannelQuery := (strings.Contains(normalized, "일반") && strings.Contains(normalized, "채널")) ||
		(strings.Contains(normalized, "general") && strings.Contains(normalized, "channel")) ||
		(strings.Contains(normalized, "ops log") && strings.Contains(normalized, "channel")) ||
		(strings.Contains(normalized, "ops logs") && strings.Contains(normalized, "channel"))
	repeatedAlertQuery := strings.Contains(normalized, "반복") || strings.Contains(normalized, "중복") ||
		strings.Contains(normalized, "spam") || strings.Contains(normalized, "too many") ||
		strings.Contains(normalized, "noisy") || strings.Contains(normalized, "repeated") ||
		strings.Contains(normalized, "duplicate")
	var b strings.Builder
	fmt.Fprintf(&b, "검색어: %s\n\n", security.SanitizeText(query))
	switch {
	case strings.Contains(normalized, "태그") || strings.Contains(normalized, "배포"):
		b.WriteString("이 PR에서는 tag/deploy를 하지 않습니다.\n")
		b.WriteString("- git tag 또는 git push origin v* 명령을 실행하지 않습니다.\n")
		b.WriteString("- 태그 배포는 별도 release workflow입니다.\n")
		b.WriteString("- command schema 변경은 배포/등록 시점에 DISCORD_REGISTER_COMMANDS=true로 1회 등록합니다.")
	case strings.Contains(normalized, "공개 지연"):
		b.WriteString("과제 공개 지연 판단:\n")
		b.WriteString("- ASSIGNMENT_PUBLISH_DELAYED는 publishedAt이 존재하고, 현재보다 과거이며, status가 published/open/opened가 아닐 때만 사용합니다.\n")
		b.WriteString("- publishedAt unknown + DRAFT + startAt past는 ASSIGNMENT_DRAFT_PAST_START입니다.\n")
		b.WriteString("- stale draft는 반복 WARN으로 spam하지 않습니다.\n\n")
		b.WriteString("확인:\n")
		b.WriteString("- /ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis\n")
		b.WriteString("- /ops assignment course:<courseSlug> id:<assignmentId> view:events")
	case criticalRoleQuery:
		b.WriteString("CRITICAL role mention 설정:\n")
		b.WriteString("- /ops alert action:channel target:critical channel:#ops-critical\n")
		b.WriteString("- /ops alert action:role role:@운영팀\n\n")
		b.WriteString("정책:\n")
		b.WriteString("- CRITICAL only: configured role mention은 CRITICAL alert에서만 보냅니다.\n")
		b.WriteString("- HIGH/general/audit/WARN do not role-mention.")
	case generalChannelQuery:
		b.WriteString("일반 운영 로그 채널:\n")
		b.WriteString("- /ops alert action:channel target:general channel:#ops-log\n")
		b.WriteString("  → assignment audit, assignment issue WARN/INFO, HIGH service alerts, normal ops logs go to general.\n\n")
		b.WriteString("CRITICAL goes to critical:\n")
		b.WriteString("- /ops alert action:channel target:critical channel:#ops-critical")
	case assignmentQuery && deleteQuery:
		b.WriteString("과제 삭제 audit 확인:\n")
		b.WriteString("- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("  → 삭제 actor/time은 Report V2 EVENT logs에서 확인합니다.\n\n")
		b.WriteString("주의:\n")
		b.WriteString("- 삭제된 assignment는 /ops assignment course:<courseSlug> id:<assignmentId>에서 더 이상 조회되지 않을 수 있습니다.\n")
		b.WriteString("- /ops assignment는 현재 WEB Admin state만 보여줍니다.\n")
		b.WriteString("- bot은 과제를 삭제하거나 업데이트하지 않습니다.")
	case assignmentQuery && updateActorQuery:
		b.WriteString("과제 수정 actor 확인:\n")
		b.WriteString("- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("- /ops assignment course:<courseSlug> id:<assignmentId> view:events\n\n")
		b.WriteString("정책:\n")
		b.WriteString("- who/when은 Report EVENT logs에서 확인합니다.\n")
		b.WriteString("- 현재 assignment state does not prove actor.\n")
		b.WriteString("- bot은 과제를 업데이트하지 않습니다.")
	case assignmentQuery && strings.Contains(normalized, "공개"):
		b.WriteString("관련 기능:\n")
		b.WriteString("1. 과제 audit 알림\n")
		b.WriteString("   - source: Report EVENT logs\n")
		b.WriteString("   - eventType: ASSIGNMENT_CREATED/UPDATED/DELETED/PUBLISHED/UNPUBLISHED\n")
		b.WriteString("   - actor와 occurredAt이 있으면 자동 표시합니다.\n\n")
		b.WriteString("2. 수동 검색\n")
		b.WriteString("   /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("   → 과제 생성/수정/삭제/공개 EVENT 로그를 검색합니다.\n\n")
		b.WriteString("3. 과제별 이력\n")
		b.WriteString("   /ops assignment course:<courseSlug> id:<assignmentId> view:events\n")
		b.WriteString("   → 봇 감지 이력과 audit event 요약을 봅니다.\n\n")
		b.WriteString("주의:\n")
		b.WriteString("- /ops assignment는 현재 상태 조회입니다.\n")
		b.WriteString("- 누가 변경했는지는 Report EVENT 로그에서 확인합니다.\n")
		b.WriteString("- bot은 과제를 생성/수정/삭제/공개하지 않습니다.")
	case repeatedAlertQuery:
		b.WriteString("반복 알림 정책:\n")
		b.WriteString("- assignment issues are lifecycle state, not an event stream.\n")
		b.WriteString("- 같은 open issue는 cooldown마다 다시 전송하지 않습니다.\n")
		b.WriteString("- digest groups repeated assignment issues and shows repeated suppressed count.\n\n")
		b.WriteString("확인/조치:\n")
		b.WriteString("- /ops assignment course:<courseSlug> id:<assignmentId> view:events\n")
		b.WriteString("- /ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>")
	case strings.Contains(normalized, "로그") || strings.Contains(normalized, "검색") || strings.Contains(normalized, "trace"):
		b.WriteString("관련 기능:\n")
		b.WriteString("- /ops logs service:report mode:recent query:<assignmentId|traceId|eventType> since:24h limit:20\n")
		b.WriteString("  → 구조화 필드 기반 일반 로그 검색\n")
		b.WriteString("- /ops logs service:<service> mode:errors since:30m limit:10\n")
		b.WriteString("  → 서비스 오류 집계 확인\n")
		b.WriteString("- /ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20\n")
		b.WriteString("  → 과제 lifecycle EVENT 로그 검색\n")
		b.WriteString("- /ops logs mode:trace query:<traceId>\n")
		b.WriteString("  → traceId가 있을 때만 단일 요청 흐름 확인")
	default:
		b.WriteString("관련 기능을 좁히지 못했습니다.\n")
		b.WriteString("- /ops help topic:assignments\n")
		b.WriteString("- /ops help topic:alerts\n")
		b.WriteString("- /ops help topic:logs")
	}
	return TruncateDiscordMessage(b.String())
}
