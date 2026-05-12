# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Commands

Primary 운영 command는 `/ops`입니다.

- `/ops dashboard since:<5m|15m|30m|1h|3h> view:<summary|errors|latency>`
- `/ops service service:<gateway|auth|report|online-judge|post> view:<summary|health|count|top|errors|slow|copy>`
- `/ops logs service:<service|all> mode:<recent|errors|top|slow> level:<INFO|WARN|ERROR>`
- `/ops trace trace_id:<traceId>`
- `/ops alarms state:<ALARM|OK|INSUFFICIENT_DATA|all>`
- `/ops storage view:<usage|retention>`
- `/ops help`

Phase 1에서는 기존 `/dashboard`, `/service`, `/logs`, `/errors`, `/trace`, `/alarm`, `/disk`, `/retention`, `/help`도 legacy alias로 유지합니다.

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`; registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`
- If OOM occurs on EC2, raise to `128m`; higher is not recommended for the current t3.micro.

The container must not publish a host port and must not mount the Docker socket.
