# Discord Monitor Bot Operator Guide

## 1. Bot Role

monitor-bot은 Gateway JVM과 분리된 Go HTTP Interactions sidecar입니다. Discord Gateway WebSocket을 열지 않고 `/discord/interactions` HTTP endpoint만 받습니다.

역할은 read-only 운영 확인입니다.

- 서비스 dashboard 확인
- CloudWatch Logs 조회
- Report assignment 현재 상태 진단
- Report V2 EVENT 로그 기반 assignment audit 알림
- general/critical alert routing

monitor-bot은 과제를 생성/수정/삭제/공개/비공개하지 않습니다. Report Admin POST/PATCH/DELETE API를 호출하지 않습니다. 예외적으로 token 갱신을 위해 `/v2/auth/refresh`만 호출할 수 있습니다.

## 2. Core UX: Five Command Families

Default operator UX exposes only five command families.

- `/ops dashboard`
- `/ops logs`
- `/ops alert`
- `/ops assignment`
- `/ops help`

Discord registration uses the `/ops` command family. 기존에 분리되어 있던 service, trace, watch, logs-watch, assignment-check, submissions 흐름은 option으로 흡수합니다.

## 3. Command Guide

### `/ops dashboard`

서비스 상태와 dashboard watch를 담당합니다.

```text
/ops dashboard
/ops dashboard service:report
/ops dashboard action:watch channel:#ops interval:5m
/ops dashboard action:unwatch
/ops dashboard action:status
```

`/ops dashboard service:<service>`는 단일 서비스 health/log/error 요약을 보여줍니다. `action:watch`는 한 메시지를 주기적으로 edit/update하는 dashboard watch를 등록합니다.

최근 장애 알림은 raw history가 아니라 grouped incident view입니다. 같은 service/severity/reason/path/errorCode에서 traceId만 다른 요청은 한 줄로 묶고, count/latest/first/대표 traceId와 `/ops logs mode:trace query:<traceId>` drilldown을 보여줍니다.

### `/ops logs`

오류, CRITICAL, slow, security, trace, EVENT/audit 검색과 log feed watch를 담당합니다.

```text
/ops logs service:<service> mode:errors since:30m limit:10
/ops logs service:<service> mode:critical since:30m limit:10
/ops logs service:<service> mode:slow since:30m limit:10
/ops logs service:<service> mode:security since:30m limit:10
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
/ops logs mode:trace query:<traceId>
/ops logs action:watch service:report mode:errors channel:#report-logs interval:5m
/ops logs action:unwatch service:report mode:errors
/ops logs action:watches
```

`mode:events`는 Report assignment audit EVENT 로그 조회에 사용합니다. `mode:trace`는 traceId가 있을 때만 사용합니다. 로그 분류는 structured V2 fields 기준이며 `@message`는 fallback search/display 용도입니다.

### Service Alert Drilldown Buttons

서비스 alert notification은 concrete trace/service 값이 있을 때 legacy Discord message components 버튼을 포함합니다.

- `Trace 상세` 버튼은 `/ops logs mode:trace query:<traceId>`와 같은 내부 조회를 실행합니다.
- `<service> 오류 30m` 버튼은 `/ops logs service:<service> mode:errors since:30m limit:10`와 같은 내부 조회를 실행합니다.
- 버튼 클릭 결과는 기본적으로 ephemeral follow-up이므로 public channel을 로그 결과로 채우지 않습니다.
- slash command fallback은 알림 본문에 남아 있으므로 버튼 UI나 interaction 문제가 있어도 수동 조회할 수 있습니다.
- slash command schema 변경이 아니므로 이 버튼 패치만으로는 `DISCORD_REGISTER_COMMANDS=true`가 필요하지 않습니다.

### `/ops alert`

general/critical 채널, role mention, on/off/status/test를 담당합니다.

```text
/ops alert action:channel target:general channel:#ops-log
/ops alert action:channel target:critical channel:#ops-critical
/ops alert action:channel target:all channel:#ops-alerts
/ops alert action:role role:@운영팀
/ops alert action:role-clear
/ops alert action:status
/ops alert action:test target:general
/ops alert action:test target:critical
```

`target:all`은 기존 단일 alert channel과 호환되도록 general/critical을 같은 채널로 저장합니다.

### `/ops assignment`

과제 목록, 상세, 진단, 감지 이력, 체크리스트, 제출 상태, ack/unack을 담당합니다. 과제 write command는 없습니다.

```text
/ops assignment course:<courseSlug>
/ops assignment course:<courseSlug> action:list
/ops assignment scope:all action:list
/ops assignment course:<courseSlug> id:<assignmentId>
/ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis
/ops assignment course:<courseSlug> id:<assignmentId> view:events
/ops assignment course:<courseSlug> id:<assignmentId> action:check
/ops assignment course:<courseSlug> id:<assignmentId> action:submissions
/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>
/ops assignment course:<courseSlug> id:<assignmentId> action:unack event:<eventType>
```

`/ops assignment`은 WEB Admin GET API 기준 현재 상태를 보여줍니다. 현재 상태는 누가 변경했는지를 증명하지 않습니다. actor/occurredAt은 Report EVENT 로그에서만 확인합니다.

### `/ops help`

상황별로 쓸 명령을 검색합니다.

```text
/ops help
/ops help topic:<topic>
/ops help command:<command>
/ops help query:"과제 수정 누가"
/ops help query:"critical role"
```

## 4. `/ops help` Behavior

Default `/ops help`는 5개 command family만 보여줍니다. legacy command를 기본 도움말에 노출하지 않습니다.

우선순위:

1. `command`
2. `topic`
3. `query`
4. overview/default

추천 검색어:

- `/ops help query:"과제 수정 누가"`
- `/ops help query:"과제 삭제 언제"`
- `/ops help query:"critical role"`
- `/ops help query:"일반 로그 채널"`
- `/ops help query:"로그 검색"`
- `/ops help query:"반복 알림"`
- `/ops help query:"과제 공개 지연"`
- `/ops help query:"태그 배포"`

## 5. Alert Routing

general route:

- assignment audit 성공 이벤트
- assignment issue WARN/INFO
- HIGH service alert
- 일반 운영 로그
- role mention 없음

critical route:

- CRITICAL server alerts only
- configured role mention

설정 예시:

```text
/ops alert action:channel target:general channel:#ops-log
/ops alert action:channel target:critical channel:#ops-critical
/ops alert action:role role:@운영팀
```

`HIGH`는 즉시 알림이지만 role mention은 없습니다. `CRITICAL`만 critical channel과 configured role mention을 사용합니다. `@everyone`/`@here`는 허용하지 않습니다.

Fallback order:

- general: state general channel → legacy alert channel → `DISCORD_ALERT_CHANNEL_ID` → dashboard channel
- critical: state critical channel → legacy alert channel → `DISCORD_ALERT_CHANNEL_ID`

## 6. Assignment Read-Only Policy

monitor-bot은 assignment operation을 실행하지 않습니다.

금지:

- `/ops assignment-create`
- `/ops assignment-update`
- `/ops assignment-delete`
- `/ops assignment-publish`
- `/ops assignment-unpublish`
- Report Admin POST/PATCH/DELETE API 호출

허용:

- WEB Admin GET API 조회
- Report V2 EVENT 로그 조회
- `/v2/auth/refresh`를 통한 admin access token 갱신

## 7. Assignment Audit Events

Assignment audit events are automatic and read-only.

Source of truth:

- Report V2 EVENT logs

대상 이벤트:

- `ASSIGNMENT_CREATED`
- `ASSIGNMENT_UPDATED`
- `ASSIGNMENT_DELETED`
- `ASSIGNMENT_PUBLISHED`
- `ASSIGNMENT_UNPUBLISHED`

표시 필드:

- `actor.userId`
- `actor.role`
- safe actor `name`/`displayName`/`loginId`
- `occurredAt`
- `traceId`
- `requestId`
- `assignmentId`
- `title`
- `changedFields`

actor/occurredAt이 없으면 `unknown`으로 표시합니다. actor is never inferred from WEB Admin API.

Audit events route to general and never role-mention.

수동 조회:

```text
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
```

삭제된 과제는 `/ops assignment` 조회가 실패할 수 있습니다. 삭제 actor/time은 Report EVENT 로그에서 확인합니다.

## 8. Assignment Issue Warnings

Assignment issue warning은 WEB Admin API snapshot과 diagnosis를 사용합니다.

주요 이벤트:

- `ASSIGNMENT_PUBLISH_DELAYED`
- `ASSIGNMENT_DRAFT_PAST_START`
- `ASSIGNMENT_STALE_DRAFT`
- `ASSIGNMENT_MISSING_PROBLEM`
- `ASSIGNMENT_INVALID_TIME`
- `GRADING_FAILED`

`ASSIGNMENT_PUBLISH_DELAYED`는 `publishedAt`이 존재하고, 현재보다 과거이며, status가 `published/open/opened`가 아닐 때만 사용합니다.

`publishedAt unknown + DRAFT + startAt past`는 publish delayed로 단정하지 않고 `ASSIGNMENT_DRAFT_PAST_START`로 분리합니다.

`endAt + stale grace`가 지난 draft는 `ASSIGNMENT_STALE_DRAFT`로 분리하고 반복 WARN spam을 만들지 않습니다.

과제 dashboard의 최근 이벤트는 eventType/course/summary/reason 기준으로 묶어서 표시합니다. WEB Admin snapshot diagnosis 이벤트는 traceId가 없을 수 있으므로 assignmentId, issueKey, evidenceHash, `/ops assignment course:<courseSlug> id:<assignmentId> view:events` 중심으로 drilldown합니다. Report EVENT audit 이벤트는 실제 EVENT 로그에 traceId/requestId가 있을 때만 trace drilldown을 표시합니다.

## 9. Repeated Alert Suppression and Digest

same assignment issue does not resend every cooldown. Assignment issue는 event stream이 아니라 lifecycle state입니다.

새 메시지를 보내는 경우:

- issue 최초 open
- resolved 후 reopen
- severity 상승
- evidenceHash의 의미 있는 변경
- ack 만료 후 재감지

같은 poll window의 noisy assignment warning은 digest로 묶습니다.

- group key: `courseSlug + eventType + severity + source`
- total count
- newly opened count
- repeated suppressed count
- stale count
- top examples
- next command explanation

조치:

```text
/ops assignment course:<courseSlug> id:<assignmentId> view:events
/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>
```

## 10. Source of Truth

- Service errors and audit logs: CloudWatch Logs structured V2 fields
- Assignment current state: WEB Admin GET API
- Assignment actor/who/when: Report V2 EVENT logs
- Assignment issue diagnosis: WEB Admin API snapshot + monitor-bot diagnosis

`@message`는 fallback search/display로만 사용합니다. alert classification에는 사용하지 않습니다.

## 11. Security / Output Policy

출력 금지:

- `request.body`
- full `response.data`
- `accessToken`
- `refreshToken`
- `Authorization`
- `Authenticate`
- `salt`
- `secret`
- `password`

Discord 일반 메시지는 mentions를 기본 억제합니다. role mention 전용 path만 configured role id를 허용합니다.

## 12. Troubleshooting

Discord command does not appear:

- command schema changes need `DISCORD_REGISTER_COMMANDS=true` once during deployment/registration.
- `/healthz`의 `discordCommandsRegistered`, `discordCommandRegistrationError`를 확인합니다.

Critical role mention does not work:

- `/ops alert action:status`를 확인합니다.
- role이 저장되어 있고 bot이 해당 role을 mention할 권한이 있는지 확인합니다.

Too many assignment warnings:

- digest의 `repeated suppressed` count를 확인합니다.
- `/ops assignment course:<courseSlug> id:<assignmentId> view:events`로 lifecycle 상태를 확인합니다.
- 의도된 stale issue는 `/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>`으로 silence합니다.

Need to know who modified an assignment:

```text
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
```

Need server failure root cause:

```text
/ops logs mode:trace query:<traceId>
```

## 13. Legacy Command Migration

Legacy commands are not primary UX. If legacy handlers remain in code, they should return deprecation guidance. If they are not registered, document them only as migration references.

| Legacy | Current |
|---|---|
| `/ops service` | `/ops dashboard service:<service>` |
| `/ops trace` | `/ops logs mode:trace query:<traceId>` |
| `/ops watch` | `/ops dashboard action:watch` |
| `/ops unwatch` | `/ops dashboard action:unwatch` |
| `/ops logs-watch` | `/ops logs action:watch` |
| `/ops logs-unwatch` | `/ops logs action:unwatch` |
| `/ops logs-watches` | `/ops logs action:watches` |
| `/ops assignments` | `/ops assignment action:list` |
| `/ops assignments-all` | `/ops assignment scope:all action:list` |
| `/ops assignment-check` | `/ops assignment course:<courseSlug> id:<assignmentId> action:check` |
| `/ops assignment-events` | `/ops assignment course:<courseSlug> id:<assignmentId> view:events` |
| `/ops assignment-ack` | `/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>` |
| `/ops assignment-unack` | `/ops assignment course:<courseSlug> id:<assignmentId> action:unack event:<eventType>` |
| `/ops submissions` | `/ops assignment course:<courseSlug> id:<assignmentId> action:submissions` |

## 14. Deployment and Registration

No tag deployment is done by this PR.

- Do not run `git tag`.
- Do not run `git push origin v*`.
- Do not manually trigger production CD as part of this PR.

Deployment and command registration are separate release operations. When command schema changes are ready to deploy, run registration once with `DISCORD_REGISTER_COMMANDS=true` during the approved deployment/registration step.

monitor-bot reuses the existing Discord bot, existing CloudWatch log group config, existing alert channel/role config, and existing Gateway ECR repository.
