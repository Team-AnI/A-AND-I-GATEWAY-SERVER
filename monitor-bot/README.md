# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Commands

Primary 운영 command는 `/ops`입니다.

- `/ops dashboard since:<15m|30m|1h>`
- `/ops service service:report view:<summary|health>`
- `/ops logs service:<report|all> mode:<recent|errors|slow> level:<INFO|WARN|ERROR> limit:<5|10|20>`
- `/ops assignments since:<30m|1h|today>`
- `/ops assignment id:<assignmentId>`
- `/ops trace trace_id:<traceId>`
- `/ops alarms state:<ALARM|OK|INSUFFICIENT_DATA|all>`
- `/ops storage view:<usage|retention>`
- `/ops help`

기본 등록은 `/ops`만 대상으로 합니다. 임시 호환이 꼭 필요할 때만 `DISCORD_REGISTER_LEGACY_COMMANDS=true`로 기존 `/dashboard`, `/service`, `/logs`, `/errors`, `/trace`, `/alarm`, `/disk`, `/retention`, `/help` alias를 함께 등록합니다.

Phase 1은 web/report server only입니다. `auth`, `online-judge`, `tech-blog`, `gateway`는 아직 실제 조회/알림 연동 대상이 아니며 dashboard에서 `NOT_CONNECTED` 또는 `NOT_CONFIGURED`로 표시합니다.

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`; registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`
- If OOM occurs on EC2, raise to `128m`; higher is not recommended for the current t3.micro.

The container must not publish a host port and must not mount the Docker socket.
