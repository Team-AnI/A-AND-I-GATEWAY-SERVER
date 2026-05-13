# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Commands

Primary 운영 command는 `/ops`입니다.

- `/ops dashboard since:<15m|30m|1h>`
- `/ops service service:report view:<summary|health>`
- `/ops logs service:<report|all> mode:<recent|errors|slow> level:<INFO|WARN|ERROR> limit:<5|10|20>`
- `/ops assignments course:<courseSlug> status:<all|published|draft|scheduled>`
- `/ops assignments-all window:<today|this-week>`
- `/ops assignment course:<courseSlug> id:<assignmentId>`
- `/ops assignment-check course:<courseSlug> id:<assignmentId>`
- `/ops submissions course:<courseSlug> assignment:<assignmentId>`
- `/ops trace trace_id:<traceId>`
- `/ops alarms state:<ALARM|OK|INSUFFICIENT_DATA|all>`
- `/ops storage view:<usage|retention>`
- `/ops help`

기본 등록은 `/ops`만 대상으로 합니다. 임시 호환이 꼭 필요할 때만 `DISCORD_REGISTER_LEGACY_COMMANDS=true`로 기존 `/dashboard`, `/service`, `/logs`, `/errors`, `/trace`, `/alarm`, `/disk`, `/retention`, `/help` alias를 함께 등록합니다.

Phase 1은 web/report server only입니다. `auth`, `online-judge`, `tech-blog`, `gateway`는 아직 실제 조회/알림 연동 대상이 아니며 dashboard에서 `NOT_CONNECTED` 또는 `NOT_CONFIGURED`로 표시합니다.

과제 운영 조회는 WEB-SERVER 기존 관리자 GET API를 우선 사용합니다. 새로 필요한 환경 변수는 `REPORT_ADMIN_BEARER_TOKEN` 하나뿐이며, base URL은 기존 `REPORT_SERVICE_URI`를 재사용합니다. token 값은 raw token만 저장하고, 요청 시 monitor-bot이 `Authorization: Bearer <token>` 형태로 붙입니다. POST/PATCH/DELETE 관리자 API는 호출하지 않습니다.

CloudWatch Logs는 `/ops logs`, `/ops trace`, admin API 실패 시 fallback 확인 용도로만 사용합니다. fallback 응답은 `fallback result, not authoritative`로 표시되며, 과제 존재/공개/제출 상태의 authoritative source는 WEB-SERVER admin API입니다.

## Assignment Ops Feed

이번 단계는 WEB-SERVER admin API only 기반의 Assignment Ops Feed입니다.

- monitor-bot이 주기적으로 `GET /v2/admin/courses`와 과제/제출 상태 조회 API를 호출합니다.
- `ACTIVE` 코스만 자동 감시합니다.
- `LEGACY` 코스는 자동 feed에서 제외하고 수동 `/ops assignments` 조회만 허용합니다.
- `UNKNOWN` 코스는 dashboard count에만 포함하고 이벤트 발송은 제한합니다.
- 첫 실행은 baseline만 저장하고 과거 과제를 대량 발송하지 않습니다.
- dashboard는 한 Discord 메시지를 edit/update하며, 이벤트 feed만 중요한 변경을 새 메시지로 보냅니다.
- 채널은 기존 `DISCORD_ALERT_CHANNEL_ID`를 우선 재사용하고, 없으면 `DISCORD_DASHBOARD_CHANNEL_ID`를 사용합니다. 별도 `ASSIGNMENT_OPS_CHANNEL_ID`는 추가하지 않았습니다.

자동 감지 이벤트:

- 과제 등록
- 과제 공개 완료
- 과제 공개 지연
- 과제 시간 설정 이상
- 제출 수 증가
- 채점 완료 수 증가
- 채점 실패
- WEB Admin API 인증/업스트림 오류

기존 `/ops` 명령은 feed에서 이상 항목을 봤을 때 상세 확인하는 용도입니다.

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`; registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`
- If OOM occurs on EC2, raise to `128m`; higher is not recommended for the current t3.micro.

The container must not publish a host port and must not mount the Docker socket.
