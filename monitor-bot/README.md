# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Purpose

monitor-bot은 운영자가 Discord에서 서비스 상태, 로그, 과제 상태, 과제 audit 이벤트를 확인하기 위한 read-only 운영 도구입니다.

- CloudWatch Logs와 WEB Admin GET API를 조회합니다.
- 과제 생성/수정/삭제/공개/비공개 명령을 제공하지 않습니다.
- Report Admin POST/PATCH/DELETE API를 호출하지 않습니다.
- token, request body, full response data, secret 값은 Discord 응답이나 로그에 노출하지 않습니다.

## UX Contract

기본 운영 UX는 5개 command family만 노출합니다.

- `/ops dashboard`
- `/ops logs`
- `/ops alert`
- `/ops assignment`
- `/ops help`

service, trace, watch, logs-watch, assignment-check, submissions 흐름은 위 5개 family의 option으로 제공합니다.

## Core Commands

```text
/ops dashboard
/ops dashboard service:report
/ops dashboard action:watch channel:#ops interval:5m
/ops dashboard action:unwatch
/ops dashboard action:status

/ops logs service:<service> mode:errors since:30m limit:10
/ops logs service:<service> mode:critical since:30m limit:10
/ops logs service:<service> mode:slow since:30m limit:10
/ops logs service:<service> mode:security since:30m limit:10
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
/ops logs mode:trace query:<traceId>
/ops logs action:watch service:report mode:errors channel:#report-logs interval:5m
/ops logs action:watches

/ops alert action:channel target:general channel:#ops-log
/ops alert action:channel target:critical channel:#ops-critical
/ops alert action:channel target:all channel:#ops-alerts
/ops alert action:role role:@운영팀
/ops alert action:status

/ops assignment course:<courseSlug>
/ops assignment course:<courseSlug> id:<assignmentId>
/ops assignment course:<courseSlug> id:<assignmentId> view:diagnosis
/ops assignment course:<courseSlug> id:<assignmentId> view:events
/ops assignment course:<courseSlug> id:<assignmentId> action:check
/ops assignment course:<courseSlug> id:<assignmentId> action:submissions
/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>

/ops help
/ops help topic:<topic>
/ops help command:<command>
/ops help query:"과제 수정 누가"
```

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- `/healthz`는 HTTP 서버, AWS SDK, `DISCORD_APPLICATION_ID`, 유효한 Discord 서명 키, interaction handler가 모두 준비되기 전까지 `503`을 반환합니다.
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`.
- command 등록 결과와 실패 사유는 `/healthz` 응답에 포함됩니다. 등록 실패는 `STRICT_STARTUP_CHECKS=true`일 때만 프로세스를 종료합니다.

## Environment

### Always Required

| 변수 | 기본값 | Secret | 용도 |
| :--- | :--- | :---: | :--- |
| `DISCORD_PUBLIC_KEY` | 없음 | 아니요 | interaction 서명 검증에 사용합니다. 누락되거나 유효한 Ed25519 공개 키가 아니면 `/healthz`는 `503`입니다. |
| `DISCORD_APPLICATION_ID` | 없음 | 아니요 | deferred interaction의 follow-up 응답 주소에 사용합니다. |

CloudWatch를 사용하려면 AWS SDK 기본 credential chain이 유효해야 합니다. 운영 환경에서는 static access key보다 workload IAM role을 사용합니다. `AWS_REGION` 기본값은 `ap-northeast-2`입니다.

### Access and Discord Features

| 변수 | 기본값 | Secret | 적용 조건 |
| :--- | :--- | :---: | :--- |
| `DISCORD_ALLOWED_GUILD_ID` | 빈 값 | 아니요 | 설정하면 해당 guild만 허용합니다. command 등록을 켤 때는 필수입니다. |
| `DISCORD_ALLOWED_ROLE_IDS` | 빈 값 | 아니요 | 쉼표로 구분합니다. 빈 값이면 role 제한을 적용하지 않으므로 운영 환경에서는 설정을 권장합니다. |
| `DISCORD_BOT_TOKEN` | 빈 값 | 예 | command 등록 또는 channel/dashboard/alert 메시지 전송을 사용할 때 필요합니다. interaction follow-up 자체에는 사용하지 않습니다. |
| `DISCORD_REGISTER_COMMANDS` | `false` | 아니요 | `true`일 때 guild command를 등록합니다. |
| `DISCORD_COMMAND_SCOPE` | `guild` | 아니요 | 등록을 켠 경우 `guild`만 지원합니다. |
| `DISCORD_EPHEMERAL_RESPONSES` | `true` | 아니요 | 일반 interaction 응답의 ephemeral 여부입니다. 버튼 응답은 항상 ephemeral입니다. |
| `STRICT_STARTUP_CHECKS` | `false` | 아니요 | AWS 설정 또는 command 등록 실패 시 즉시 종료할지 결정합니다. |

`DISCORD_REGISTER_COMMANDS=true`이면 `DISCORD_BOT_TOKEN`, `DISCORD_APPLICATION_ID`, `DISCORD_ALLOWED_GUILD_ID`가 모두 필요합니다. 등록 오류는 `/healthz` JSON에 기록되며, `STRICT_STARTUP_CHECKS=false`이면 readiness 상태와 별도로 보고됩니다.

### Assignment Operations

| 변수 | 기본값 | Secret | 적용 조건 |
| :--- | :--- | :---: | :--- |
| `REPORT_SERVICE_URI` | 빈 값 | 아니요 | assignment Admin GET API를 사용할 때 필요합니다. Gateway도 같은 이름의 route 설정을 사용합니다. |
| `AUTH_SERVICE_URI` | 빈 값 | 아니요 | Admin access token 갱신을 사용할 때 필요합니다. Gateway도 같은 이름의 route 설정을 사용합니다. |
| `OPS_ADMIN_REFRESH_TOKEN` | 빈 값 | 예 | assignment 조회와 access token 갱신을 사용할 때 필요합니다. |
| `ASSIGNMENT_STALE_GRACE_DAYS` | `7` | 아니요 | stale draft 판정 유예 기간입니다. |

### Runtime and Query Defaults

| 변수 | 기본값 | 설명 |
| :--- | :--- | :--- |
| `BOT_HTTP_PORT` | `8088` | HTTP listener port |
| `MONITOR_BOT_STATE_PATH` | `/var/lib/monitor-bot/state.json` | persistent state 파일 경로 |
| `HEALTH_REQUEST_TIMEOUT_MS` | `2000` | downstream health 요청 timeout |
| `CLOUDWATCH_QUERY_TIMEOUT_SECONDS` | `8` | Logs Insights query timeout |
| `CLOUDWATCH_QUERY_POLL_INTERVAL_MS` | `500` | query 결과 poll 간격 |
| `CLOUDWATCH_QUERY_LIMIT` | `20` | command query 기본 결과 제한 |
| `CLOUDWATCH_MAX_LOG_GROUPS_PER_QUERY` | `5` | 한 query에서 선택할 최대 log group 수 |
| `MAX_CLOUDWATCH_QUERIES_PER_TICK` | `6` | dashboard/alert tick당 query budget |

양수가 아닌 정수나 파싱할 수 없는 값은 위 기본값으로 돌아갑니다.

서비스별 CloudWatch log group 기본값은 다음과 같습니다.

| 변수 | 기본값 |
| :--- | :--- |
| `LOG_GROUP_GATEWAY` | `/a-and-i/gateway` |
| `LOG_GROUP_AUTH` | `/a-and-i/auth` |
| `LOG_GROUP_REPORT` | `/a-and-i/prod/report` |
| `LOG_GROUP_ONLINE_JUDGE` | `/a-and-i/online-judge` |
| `LOG_GROUP_POST` | `/a-and-i/prod/tech-blog` |

health URL은 `HEALTH_URL_GATEWAY`만 `http://gateway:9090/actuator/health/readiness` 기본값을 가지며, `HEALTH_URL_AUTH`, `HEALTH_URL_REPORT`, `HEALTH_URL_ONLINE_JUDGE`, `HEALTH_URL_POST`는 빈 값이 기본입니다.

### Dashboard and Alerts

| 변수 | 기본값 | 적용 조건 |
| :--- | :--- | :--- |
| `DASHBOARD_ENABLED` | `false` | env 기반 기본 dashboard watch를 사용할 때 `true` |
| `DISCORD_DASHBOARD_CHANNEL_ID` | 빈 값 | `DASHBOARD_ENABLED=true`일 때 channel 지정 |
| `DASHBOARD_REFRESH_INTERVAL_SECONDS` | `300` | dashboard 갱신 간격 |
| `DASHBOARD_SINCE` | `30m` | dashboard 조회 범위 |
| `ALERT_ENABLED` | `false` | env 기반 service alert를 사용할 때 `true`. state에서 활성화된 alert도 별도로 동작합니다. |
| `DISCORD_ALERT_CHANNEL_ID` | 빈 값 | env 기반 alert channel fallback. 값이 있으면 `ALERT_ENABLED=false`여도 alert poll이 활성화됩니다. |
| `ALERT_POLL_INTERVAL_SECONDS` | `180` | alert 및 assignment poll 간격 |
| `ALERT_COOLDOWN_SECONDS` | `900` | 같은 alert와 assignment issue의 재전송 억제 시간 |
| `ALERT_5XX_THRESHOLD_5M` | `3` | 5분간 5xx alert 기준 |
| `ALERT_ERROR_THRESHOLD_5M` | `5` | 5분간 error alert 기준 |
| `ALERT_HEALTH_DOWN_CONSECUTIVE` | `2` | health 연속 실패 alert 기준 |
| `ALERT_NO_LOGS_MINUTES` | `30` | no-logs alert 기준 시간 |
| `ALERT_COPY_API_5XX_THRESHOLD_5M` | `1` | copy API 5xx alert 기준 |

dashboard와 alert channel/role은 `/ops` 명령으로 state에 저장할 수도 있습니다. env 값은 초기 설정 및 fallback으로 사용합니다.

## State

state는 `/var/lib/monitor-bot/state.json`에 저장합니다.

- dashboard watch message/channel
- general/critical alert channel
- CRITICAL role mention
- log feed fingerprint
- assignment issue lifecycle
- assignment audit fingerprint

저장은 atomic rename 방식이고, 깨진 JSON은 `.corrupt.*`로 보존한 뒤 빈 state로 graceful fallback합니다.

## Dashboard Recent Sections

서비스 dashboard의 최근 장애 알림은 raw history 5건이 아니라 incident key 기준으로 묶어서 표시합니다. 같은 service/severity/reason/path/errorCode에서 traceId만 다른 요청은 한 incident로 묶고, 대표 traceId가 있으면 `/ops logs mode:trace query:<traceId>` drilldown을 함께 보여줍니다.

과제 dashboard의 최근 이벤트도 eventType/course/summary/reason 기준으로 묶습니다. WEB Admin snapshot diagnosis 이벤트는 traceId가 없을 수 있으므로 assignmentId, issueKey, evidenceHash, `/ops assignment course:<courseSlug> id:<assignmentId> view:events` 중심으로 drilldown합니다. Report EVENT audit 이벤트는 실제 로그 필드에 있을 때만 traceId/requestId를 표시합니다.

## Alert Drilldown Buttons

서비스 alert 알림은 concrete trace/service 값이 있을 때 Discord 버튼을 함께 보냅니다.

- `Trace 상세`: `/ops logs mode:trace query:<traceId>`와 같은 조회를 ephemeral follow-up으로 실행합니다.
- `<service> 오류 30m`: `/ops logs service:<service> mode:errors since:30m limit:10`와 같은 조회를 ephemeral follow-up으로 실행합니다.
- 버튼이 보이지 않거나 실패할 때를 대비해 slash command fallback은 알림 본문에 유지합니다.

## Assignment Ops

Assignment issue warning은 WEB Admin GET API snapshot과 diagnosis를 사용합니다.

- `ACTIVE` 코스만 자동 감시합니다.
- `LEGACY` 코스는 자동 feed에서 제외하고 수동 `/ops assignment action:list` 조회만 허용합니다.
- 첫 성공 poll은 baseline만 저장하고 과거 과제를 대량 발송하지 않습니다.
- 같은 poll 안의 issue는 `course + eventType + severity + source` 기준 digest로 묶습니다.
- 같은 open issue는 cooldown마다 다시 보내지 않습니다.
- 새 메시지는 최초 open, resolved 후 reopen, severity 상승, evidence 변경, ack 만료 등 의미 있는 변화에만 보냅니다.
- `publishedAt unknown + DRAFT + startAt past`는 publish delayed로 단정하지 않고 `ASSIGNMENT_DRAFT_PAST_START`로 분리합니다.
- 오래된 draft는 `ASSIGNMENT_STALE_DRAFT`로 분리하고 반복 WARN spam을 만들지 않습니다.
- `/ops assignment course:<courseSlug> id:<assignmentId> action:ack event:<eventType> until:7d reason:<reason>`으로 의도된 stale issue를 silence할 수 있습니다.

## Assignment Audit Events

Assignment audit feed는 Report V2 `EVENT` 로그를 source of truth로 사용합니다.

대상 이벤트:

- `ASSIGNMENT_CREATED`
- `ASSIGNMENT_UPDATED`
- `ASSIGNMENT_DELETED`
- `ASSIGNMENT_PUBLISHED`
- `ASSIGNMENT_UNPUBLISHED`

알림에는 가능한 경우 `actor.userId`, `actor.role`, 안전한 actor 표시명, `occurredAt`, `traceId`, `requestId`, `assignmentId`, `title`, `changedFields`를 표시합니다. actor/occurredAt이 없으면 `unknown`으로 표시하며, WEB Admin API snapshot에서 actor를 추측하지 않습니다.

수동 조회:

```text
/ops logs service:report mode:events query:<assignmentId|traceId|actorId> since:24h limit:20
```

## Alert Routing

`/ops alert target`으로 general/critical route를 분리합니다.

- general: assignment audit, assignment issue WARN/INFO, HIGH service alert, 일반 운영 로그
- critical: CRITICAL server alerts only
- role mention: CRITICAL only
- HIGH/general/audit/WARN: role mention 없음
- `@everyone`/`@here`: 허용하지 않음

예시:

```text
/ops alert action:channel target:general channel:#ops-log
/ops alert action:channel target:critical channel:#ops-critical
/ops alert action:role role:@운영팀
```

현재 V2 로그 기반 실제 조회/알림 대상은 `gateway`, `auth`, `report/web`, `blog/post`입니다. `blog`는 Discord 사용자 표시명이고 canonical key는 기존 호환성을 위해 `post`입니다. `online-judge`는 catalog/dashboard에는 계속 표시하지만, V2 로그 연동이 확인되기 전까지 자동 조회/알림 대상에 포함하지 않습니다.

## Verification

변경 후 아래 명령으로 Gateway와 Monitor Bot의 회귀를 확인합니다.

```bash
cd monitor-bot
go test ./...
cd ..
./gradlew test
```

## Command Registration

- 평상시에는 `DISCORD_REGISTER_COMMANDS=false`로 실행합니다.
- command schema 변경을 Discord guild에 반영할 때만 `DISCORD_REGISTER_COMMANDS=true`로 한 번 실행한 뒤 다시 `false`로 되돌립니다.
- 안전한 등록 범위를 위해 `DISCORD_COMMAND_SCOPE=guild`만 지원합니다.

## More Docs

- Root project contract: [../README.md](../README.md)
