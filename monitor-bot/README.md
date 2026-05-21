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

기존에 별도 subcommand로 나뉘던 service, trace, watch, logs-watch, assignment-check, submissions 흐름은 위 5개 family의 option으로 흡수합니다.

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
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`.
- Registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`.
- The container must not publish a host port and must not mount the Docker socket.

## Required Environment

기존 secret/config를 재사용합니다.

- `DISCORD_BOT_TOKEN`
- `DISCORD_PUBLIC_KEY`
- `DISCORD_APPLICATION_ID`
- `DISCORD_ALLOWED_GUILD_ID`
- `DISCORD_ALLOWED_ROLE_IDS`
- `DISCORD_ALERT_CHANNEL_ID`
- `DISCORD_DASHBOARD_CHANNEL_ID`
- `REPORT_SERVICE_URI`
- `AUTH_SERVICE_URI`
- `OPS_ADMIN_REFRESH_TOKEN`

새 required secret은 없습니다. `ASSIGNMENT_STALE_GRACE_DAYS`는 stale draft grace를 조정하는 일반 env이며 기본값은 7일입니다. 별도 `ASSIGNMENT_OPS_CHANNEL_ID`는 없으며 no ASSIGNMENT_OPS_CHANNEL_ID 정책을 유지합니다.

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
- 이 패치는 slash command schema를 바꾸지 않으므로 별도 command 변경이 없다면 `DISCORD_REGISTER_COMMANDS=true`가 필요하지 않습니다.

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

## CI / Verification

PR 검증은 아래 명령을 기준으로 합니다.

```bash
cd monitor-bot
go test ./...
cd ..
./gradlew test
```

## Release Safety

- No tag/deploy in this PR.
- 이 PR은 tag를 만들지 않습니다.
- 이 PR은 deploy를 수행하지 않습니다.
- tag deployment는 별도 release workflow입니다.
- command schema 변경을 운영 Discord에 반영할 때만 배포/등록 시점에 `DISCORD_REGISTER_COMMANDS=true`로 1회 실행합니다.
- monitor-bot은 기존 Discord bot, 기존 CloudWatch log group config, 기존 alert channel/role config를 재사용합니다.

## More Docs

- Full operator guide: [../docs/discord-monitor-bot.md](../docs/discord-monitor-bot.md)
- Root project contract: [../README.md](../README.md)
