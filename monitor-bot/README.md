# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Commands

Primary 운영 command는 `/ops`입니다.

- `/ops dashboard since:<15m|30m|1h>`
- `/ops service service:<gateway|auth|report|blog> view:<summary|health>`
- `/ops logs [service:<all|gateway|auth|report|blog>] mode:<recent|errors|slow|security|events> level:<INFO|WARN|ERROR> query:<assignmentId|traceId|eventType|errorCode> limit:<5|10|20>`
- `/ops alarms state:<ALARM|OK|INSUFFICIENT_DATA|all>`
- `/ops storage view:<usage|retention>`
- `/ops help topic:<overview|dashboard|logs|alerts|assignments|feeds>`
- `/ops help command:<dashboard|service|logs|trace|alert|watch|logs-watch|assignments|assignment|assignment-check|assignment-events|assignment-ack|submissions>`

Drilldown/fallback commands:

- `/ops trace trace_id:<traceId>`: `/ops logs` 또는 logs-watch 결과에 traceId가 표시될 때만 사용하는 요청 단위 드릴다운
- `/ops assignments`, `/ops assignments-all`, `/ops assignment view:<summary|diagnosis|raw>`, `/ops assignment-check`, `/ops assignment-events`, `/ops assignment-ack`, `/ops assignment-unack`, `/ops submissions`: 과제 이벤트 feed 이후 상세 확인 또는 fallback 용도

Service Ops 자동화 설정:

- `/ops watch scope:all channel:#ops interval:5m`
- `/ops watch scope:service service:blog channel:#blog-ops interval:5m`
- `/ops unwatch scope:all`
- `/ops watches`
- `/ops alert action:channel channel:#ops-alerts`
- `/ops alert action:channel target:general channel:#ops-log`
- `/ops alert action:channel target:critical channel:#ops-critical`
- `/ops alert action:role role:@운영팀`
- `/ops alert action:on|off|status|test target:<general|critical>`
- `/ops logs-watch service:blog mode:<errors|slow|recent|security> channel:#blog-logs interval:5m since:30m limit:10`
- `/ops logs-unwatch service:blog mode:errors`
- `/ops logs-watches`

Discord command registration은 `/ops`만 등록합니다. 기존 `/dashboard`, `/service`, `/logs`, `/errors`, `/trace`, `/alarm`, `/disk`, `/retention`, `/help` 같은 top-level legacy command는 제거했습니다.

현재 V2 로그 기반 실제 조회/알림 대상은 `gateway`, `auth`, `report/web`, `blog/post`입니다. `online-judge`는 service catalog/dashboard에는 계속 표시하지만, V2 로그 연동이 확인되기 전까지 자동 조회/알림 대상에 포함하지 않습니다.

과제 운영 조회는 WEB-SERVER 기존 관리자 GET API를 우선 사용합니다. admin 인증 환경 변수는 `OPS_ADMIN_REFRESH_TOKEN` 하나만 사용하며, base URL은 기존 `REPORT_SERVICE_URI`를 재사용합니다. refresh token 값은 raw token만 저장하고, monitor-bot이 `/v2/auth/refresh`만 POST로 호출해 access token을 메모리에서 발급/갱신합니다. 관리자 GET API 요청 시에는 메모리 access token으로 `Authorization: Bearer <accessToken>`와 `Authenticate: Bearer <accessToken>`을 함께 붙입니다. `deviceOS`, `timestamp`는 V2 API 호환용으로 자동 생성하고, `salt`는 현재 관리자 조회에 필요하지 않아 보내지 않습니다. 관리자 POST/PATCH/DELETE API는 호출하지 않습니다.

CloudWatch Logs는 `/ops logs`, `/ops trace`, admin API 실패 시 fallback 확인 용도로만 사용합니다. fallback 응답은 `fallback result, not authoritative`로 표시되며, 과제 존재/공개/제출 상태의 authoritative source는 WEB-SERVER admin API입니다.

## Service Ops Dashboard

Service Ops Dashboard는 명령어를 매번 입력하지 않아도 Discord 운영 채널에서 서비스 상태를 확인하기 위한 자동 대시보드입니다.

- dashboard는 한 메시지만 유지하고, 주기마다 기존 메시지를 edit/update합니다.
- 정상 상태에서는 새 메시지를 보내지 않습니다.
- 장애, 5xx 증가, ERROR 로그 증가, health 연속 실패 같은 중요한 상황에서만 새 alert 메시지를 보냅니다.
- alert는 `/ops alert action:channel target:general channel:#ops-log`와 `/ops alert action:channel target:critical channel:#ops-critical`로 일반 운영 로그와 CRITICAL 서버 장애 로그를 분리할 수 있습니다.
- 기존 `/ops alert action:channel channel:#채널`은 `target:all`처럼 동작해 general/critical 채널을 모두 같은 채널로 저장합니다.
- general route는 과제 audit, 과제 issue WARN, 일반 운영 로그, WARN/HIGH 알림을 전송하며 role mention을 하지 않습니다.
- critical route는 CRITICAL 서버 장애 알림만 전송합니다.
- dashboard는 `/ops watch ... channel:#채널`로 저장한 채널/messageId를 우선 사용하고, 없으면 기존 `DISCORD_DASHBOARD_CHANNEL_ID`를 사용합니다. `channel` 옵션을 생략하면 명령어를 실행한 현재 채널에 고정합니다.
- interval은 기존 `DASHBOARD_REFRESH_INTERVAL_SECONDS`를 재사용합니다.
- alert cooldown은 기존 `ALERT_COOLDOWN_SECONDS`를 재사용합니다.
- explicit `CRITICAL` alert만 `/ops alert action:role`로 저장한 role을 mention합니다. 없으면 `DISCORD_ALLOWED_ROLE_IDS`의 첫 번째 role을 fallback으로 사용하고, role 값이 없으면 mention 없이 전송합니다.
- `HIGH` alert는 general channel로 전송하지만 role mention은 하지 않습니다.
- state는 `/var/lib/monitor-bot/state.json`에 저장하며 dashboard watch, alert 설정, logs-watch fingerprint를 보관합니다. 저장은 atomic rename 방식으로 수행하고, 깨진 state는 `.corrupt.*` 파일로 보존한 뒤 빈 state로 graceful fallback합니다.
- logs-watch는 최초 등록 시 기존 결과를 baseline fingerprint로 저장하고, 이후 새 항목부터 전송합니다.

compact table 형식은 유지합니다.

```text
Service   Health  Logs    4xx  5xx  Err Last
gateway   ⚪ UNK   NOLOG     0    0    0 -
auth      ⚪ UNK   NOLOG     0    0    0 -
report    🟢 UP    OK        2    0    0 1m
judge     ⚪ UNK   NOLOG     0    0    0 -
blog      ⚪ UNK   NOLOG     0    0    0 -
```

실제 자동 감시 대상은 `gateway`, `auth`, `report/web`, `blog/post`입니다. `blog`는 Discord 사용자 표시명이고, state와 CloudWatch 설정의 canonical key는 기존 호환성을 위해 `post`를 유지합니다. `online-judge`는 service catalog에는 항상 표시하지만, 아직 미연동 상태이므로 `UNK`/`NO_V2`로 표시하고 자동 장애 알림 대상에서 제외합니다.

상세 분석은 자동 alert에 raw log를 싣지 않고 slash command로 진행합니다. `/ops trace`는 직접 외워서 쓰는 명령이 아니라, `/ops logs` 또는 logs-watch 결과에 traceId가 있을 때만 Next command로 노출되는 드릴다운입니다.

```text
/ops logs service:report mode:errors since:30m limit:10
/ops logs service:blog mode:slow since:30m limit:10
/ops trace trace_id:<traceId>
```

## Assignment Ops Feed

이번 단계는 WEB-SERVER admin API only 기반의 Assignment Ops Feed입니다.

- monitor-bot이 주기적으로 `GET /v2/admin/courses`와 과제/제출 상태 조회 API를 호출합니다.
- `ACTIVE` 코스만 자동 감시합니다.
- `LEGACY` 코스는 자동 feed에서 제외하고 수동 `/ops assignments` 조회만 허용합니다.
- `UNKNOWN` 코스는 dashboard count에만 포함하고 이벤트 발송은 제한합니다.
- 첫 실행은 baseline만 저장하고 과거 과제를 대량 발송하지 않습니다.
- dashboard는 한 Discord 메시지를 edit/update하며, 이벤트 feed만 중요한 변경을 새 메시지로 보냅니다.
- assignment alert는 cooldown마다 같은 WARN을 다시 보내지 않고 issue lifecycle로 관리합니다. 같은 `assignment:<eventType>:<courseSlug>:<assignmentId>`가 open 상태이면 dashboard/count와 `lastDetectedAt`만 갱신하고, 새 Discord feed는 억제합니다.
- 새 메시지는 issue가 처음 open 될 때, resolved 후 reopen 될 때, severity가 상승할 때, evidenceHash가 의미 있게 바뀔 때만 보냅니다.
- `/ops assignment-ack ... until:<1h|6h|1d|7d|forever> reason:<reason>`으로 의도된 상태를 acknowledge/silence할 수 있습니다. ack 상태여도 dashboard와 `/ops assignment-events`에서는 이슈 상태를 확인할 수 있습니다.
- 채널은 general route를 사용합니다. `/ops alert action:channel target:general channel:#채널` 값을 우선 사용하고, 없으면 legacy alert channel, 기존 `DISCORD_ALERT_CHANNEL_ID`, 그다음 `DISCORD_DASHBOARD_CHANNEL_ID`를 사용합니다. 별도 `ASSIGNMENT_OPS_CHANNEL_ID`는 추가하지 않았습니다.
- poll 주기는 기존 alert poll interval을 재사용합니다. 기본값은 보통 3분이며, 첫 성공 poll은 baseline만 저장해서 과거 과제를 대량 발송하지 않습니다.
- stale draft 판단 grace는 일반 환경 변수 `ASSIGNMENT_STALE_GRACE_DAYS`로 조정할 수 있으며 기본값은 7일입니다. 새 secret은 필요하지 않습니다.

자동 감지 이벤트:

- 과제 등록
- 과제 공개 완료
- `ASSIGNMENT_PUBLISH_DELAYED`: `publishedAt`이 존재하고 현재보다 과거이며 status가 `published/open/opened`가 아닌 경우입니다.
- `ASSIGNMENT_DRAFT_PAST_START`: `publishedAt`이 없고 status가 `DRAFT`이며 `startAt`이 지났습니다. 이 경우 공개 지연으로 단정하지 않고 "공개 지연으로 단정할 수 없음"을 메시지에 표시합니다.
- `ASSIGNMENT_STALE_DRAFT`: `endAt + ASSIGNMENT_STALE_GRACE_DAYS`가 지난 `DRAFT` 과제입니다. dashboard/count 대상이며 반복 WARN feed를 보내지 않습니다.
- `ASSIGNMENT_MISSING_PROBLEM`: 공개/open 과제의 `problemId`가 비어 있어 제출/채점 연결 점검이 필요한 경우입니다.
- 과제 시간 설정 이상
- 제출 수 증가
- 채점 완료 수 증가
- 채점 실패
- WEB Admin API 인증/업스트림 오류

기존 assignment 관련 `/ops` 명령은 관리자 페이지를 대체하는 상태 확인 UI가 아닙니다. feed에서 이상 항목을 봤을 때 상세 확인하거나, admin API 장애 시 fallback 확인하는 용도입니다.

과제 alert 메시지는 `title`, `status`, `publishedAt`, `startAt`, `endAt`, `problemId`, `reasonCode`, `reasonText`, evidence, issue state, `firstDetectedAt`, `lastDetectedAt`, `notifyCount`, repeat policy를 포함합니다. recommended next commands는 명령어만 나열하지 않고 각 명령을 실행해야 하는 이유를 함께 표시합니다.

예시:

```text
/ops assignment course:3rd-cs id:<assignmentId> view:diagnosis
→ 필드별 판단 근거와 공개 지연 단정 가능 여부를 확인합니다.

/ops assignment-events course:3rd-cs id:<assignmentId>
→ 봇 감지 이력, 반복 억제, ack/silence 상태를 확인합니다.

/ops logs service:report mode:recent query:<assignmentId> since:24h limit:20
→ 구조화 필드에서 해당 assignmentId를 검색합니다.
```

`/ops logs ... query:<value>`는 가능한 경우 구조화 필드(`trace.traceId`, `trace.requestId`, `event.eventType`, `assignmentId`, `request.pathVariables.assignmentId`, `response.error.code`, `response.error.value`, `http.path`, `http.route`)를 검색합니다. `@message` 검색은 fallback search로만 사용하며 alert 판단 조건에는 사용하지 않습니다.

### Assignment Audit Event Feed

Assignment issue warning과 별도로, Report V2 `EVENT` 로그 기반 audit feed를 전송합니다. 이 feed는 과제 상태 이상이 아니라 "누가 어떤 운영 행위를 언제 했는지"를 알리는 read-only 알림입니다.

- monitor-bot은 과제를 생성/수정/삭제/공개/비공개하는 명령을 제공하지 않습니다.
- Report admin POST/PATCH/DELETE API를 호출하지 않습니다.
- actor와 발생 시각은 WEB Admin API snapshot에서 추측하지 않고 Report `EVENT` 로그의 구조화 필드에서만 가져옵니다.
- actor 또는 occurredAt이 로그에 없으면 `unknown`으로 표시합니다.
- audit 알림은 INFO 성격이며 role mention을 하지 않습니다.

대상 이벤트:

- `ASSIGNMENT_CREATED`
- `ASSIGNMENT_UPDATED`
- `ASSIGNMENT_DELETED`
- `ASSIGNMENT_PUBLISHED`
- `ASSIGNMENT_UNPUBLISHED`

알림에는 `eventType`, `course`, `assignmentId`, `title`, `actor.userId`, `actor.role`, 안전한 actor 표시명, `occurredAt`, `trace.traceId`, `trace.requestId`, `source: REPORT_EVENT_LOG`, 수정 시 `changedFields`를 표시합니다.

수동 조회:

```text
/ops logs service:report mode:events query:<assignmentId|traceId|actorId|eventType> since:24h limit:20
```

`mode:events`는 현재 Report assignment audit 이벤트 조회 용도입니다. 일반 장애 분석은 기존 `mode:recent|errors|slow|security`를 사용합니다.

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`; registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`
- If OOM occurs on EC2, raise to `128m`; higher is not recommended for the current t3.micro.

The container must not publish a host port and must not mount the Docker socket.

## CI and Release Safety

- GitHub Actions CI runs the existing Gradle checks and `cd monitor-bot && go test ./...`.
- This monitor-bot integration reuses the existing Discord bot, existing Discord secrets, existing CloudWatch log group config, and existing alert channel/role config.
- Do not add a new Discord bot, do not add new required secrets, and do not create a tag for this implementation PR.
- This document does not imply production deployment; deployment remains controlled by the repository release/tag workflow.
