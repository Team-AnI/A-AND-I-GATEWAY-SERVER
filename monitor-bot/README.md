# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Commands

Primary 운영 command는 `/ops`입니다.

- `/ops dashboard since:<15m|30m|1h>`
- `/ops service service:report view:<summary|health>`
- `/ops logs service:<report|all> mode:<recent|errors|slow> level:<INFO|WARN|ERROR> limit:<5|10|20>`
- `/ops alarms state:<ALARM|OK|INSUFFICIENT_DATA|all>`
- `/ops storage view:<usage|retention>`
- `/ops help`

Drilldown/fallback commands:

- `/ops trace trace_id:<traceId>`: `/ops logs` 또는 logs-watch 결과에 traceId가 표시될 때만 사용하는 요청 단위 드릴다운
- `/ops assignments`, `/ops assignments-all`, `/ops assignment`, `/ops assignment-check`, `/ops submissions`: 과제 이벤트 feed 이후 상세 확인 또는 fallback 용도

Service Ops 자동화 설정:

- `/ops watch scope:all channel:#ops interval:5m`
- `/ops watch scope:service service:report channel:#report-ops interval:5m`
- `/ops unwatch scope:all`
- `/ops watches`
- `/ops alert action:channel channel:#ops-alerts`
- `/ops alert action:role role:@운영팀`
- `/ops alert action:on|off|status|test`
- `/ops logs-watch service:report mode:<errors|slow|recent> channel:#report-logs interval:5m since:30m limit:10`
- `/ops logs-unwatch service:report mode:errors`
- `/ops logs-watches`

Discord command registration은 `/ops`만 등록합니다. 기존 `/dashboard`, `/service`, `/logs`, `/errors`, `/trace`, `/alarm`, `/disk`, `/retention`, `/help` 같은 top-level legacy command는 제거했습니다.

Phase 1은 web/report server only입니다. `auth`, `online-judge`, `tech-blog`, `gateway`는 아직 실제 조회/알림 연동 대상이 아니며 dashboard에서 `UNK`/`NOLOG`로 표시합니다.

과제 운영 조회는 WEB-SERVER 기존 관리자 GET API를 우선 사용합니다. admin 인증 환경 변수는 `OPS_ADMIN_REFRESH_TOKEN` 하나만 사용하며, base URL은 기존 `REPORT_SERVICE_URI`를 재사용합니다. refresh token 값은 raw token만 저장하고, monitor-bot이 `/v2/auth/refresh`만 POST로 호출해 access token을 메모리에서 발급/갱신합니다. 관리자 GET API 요청 시에는 메모리 access token으로 `Authorization: Bearer <accessToken>`와 `Authenticate: Bearer <accessToken>`을 함께 붙입니다. `deviceOS`, `timestamp`는 V2 API 호환용으로 자동 생성하고, `salt`는 현재 관리자 조회에 필요하지 않아 보내지 않습니다. 관리자 POST/PATCH/DELETE API는 호출하지 않습니다.

CloudWatch Logs는 `/ops logs`, `/ops trace`, admin API 실패 시 fallback 확인 용도로만 사용합니다. fallback 응답은 `fallback result, not authoritative`로 표시되며, 과제 존재/공개/제출 상태의 authoritative source는 WEB-SERVER admin API입니다.

## Service Ops Dashboard

Service Ops Dashboard는 명령어를 매번 입력하지 않아도 Discord 운영 채널에서 서비스 상태를 확인하기 위한 자동 대시보드입니다.

- dashboard는 한 메시지만 유지하고, 주기마다 기존 메시지를 edit/update합니다.
- 정상 상태에서는 새 메시지를 보내지 않습니다.
- 장애, 5xx 증가, ERROR 로그 증가, health 연속 실패 같은 중요한 상황에서만 새 alert 메시지를 보냅니다.
- alert는 `/ops alert action:channel channel:#채널`로 저장한 채널을 우선 사용하고, 없으면 기존 `DISCORD_ALERT_CHANNEL_ID`로 전송합니다. `channel` 옵션을 생략하면 명령어를 실행한 현재 채널을 저장합니다.
- dashboard는 `/ops watch ... channel:#채널`로 저장한 채널/messageId를 우선 사용하고, 없으면 기존 `DISCORD_DASHBOARD_CHANNEL_ID`를 사용합니다. `channel` 옵션을 생략하면 명령어를 실행한 현재 채널에 고정합니다.
- interval은 기존 `DASHBOARD_REFRESH_INTERVAL_SECONDS`를 재사용합니다.
- alert cooldown은 기존 `ALERT_COOLDOWN_SECONDS`를 재사용합니다.
- 중요한 alert에는 `/ops alert action:role`로 저장한 role을 mention합니다. 없으면 `DISCORD_ALLOWED_ROLE_IDS`의 첫 번째 role을 fallback으로 사용하고, role 값이 없으면 mention 없이 전송합니다.
- state는 `/var/lib/monitor-bot/state.json`에 저장하며 dashboard watch, alert 설정, logs-watch fingerprint를 보관합니다. 저장은 atomic rename 방식으로 수행하고, 깨진 state는 `.corrupt.*` 파일로 보존한 뒤 빈 state로 graceful fallback합니다.
- logs-watch는 최초 등록 시 기존 결과를 baseline fingerprint로 저장하고, 이후 새 항목부터 전송합니다.

compact table 형식은 유지합니다.

```text
Service   Health  Logs    4xx  5xx  Err Last
gateway   ⚪ UNK   NOLOG     0    0    0 -
auth      ⚪ UNK   NOLOG     0    0    0 -
report    🟢 UP    OK        2    0    0 1m
judge     ⚪ UNK   NOLOG     0    0    0 -
post      ⚪ UNK   NOLOG     0    0    0 -
```

현재 실제 자동 감시 대상은 `report/web`입니다. `gateway`, `auth`, `online-judge`, `post`는 service catalog에는 항상 표시하지만, 아직 미연동 상태이므로 `UNK`/`NOLOG`로 표시하고 자동 장애 알림 대상에서 제외합니다. 각 서비스의 CloudWatch log group, health URL, `service.name`, traceId 정책이 준비되면 별도 task/PR에서 순차 연동합니다.

상세 분석은 자동 alert에 raw log를 싣지 않고 slash command로 진행합니다. `/ops trace`는 직접 외워서 쓰는 명령이 아니라, `/ops logs` 또는 logs-watch 결과에 traceId가 있을 때만 Next command로 노출되는 드릴다운입니다.

```text
/ops logs service:report mode:errors since:30m limit:10
/ops logs service:report mode:slow since:30m limit:10
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
- 채널은 `/ops alert action:channel channel:#채널`로 저장한 state 값을 우선 사용합니다. 없으면 기존 `DISCORD_ALERT_CHANNEL_ID`, 그다음 `DISCORD_DASHBOARD_CHANNEL_ID`를 사용합니다. 별도 `ASSIGNMENT_OPS_CHANNEL_ID`는 추가하지 않았습니다.
- poll 주기는 기존 alert poll interval을 재사용합니다. 기본값은 보통 3분이며, 첫 성공 poll은 baseline만 저장해서 과거 과제를 대량 발송하지 않습니다.

자동 감지 이벤트:

- 과제 등록
- 과제 공개 완료
- 과제 공개 지연
- 과제 시간 설정 이상
- 제출 수 증가
- 채점 완료 수 증가
- 채점 실패
- WEB Admin API 인증/업스트림 오류

기존 assignment 관련 `/ops` 명령은 관리자 페이지를 대체하는 상태 확인 UI가 아닙니다. feed에서 이상 항목을 봤을 때 상세 확인하거나, admin API 장애 시 fallback 확인하는 용도입니다.

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`; registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`
- If OOM occurs on EC2, raise to `128m`; higher is not recommended for the current t3.micro.

The container must not publish a host port and must not mount the Docker socket.
