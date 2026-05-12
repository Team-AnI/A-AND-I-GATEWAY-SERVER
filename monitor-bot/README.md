# A&I Discord Monitor Bot

Go 기반 Discord HTTP Interactions sidecar입니다. Gateway JVM 프로세스와 분리된 컨테이너로 실행되며, Discord Gateway WebSocket을 열지 않고 nginx가 `/discord/interactions`만 프록시합니다.

## Commands

- `/status`
- `/health service:<gateway|auth|report|online-judge|post>`
- `/logs service:<service> since:<5m|15m|30m|1h|3h> level:<INFO|WARN|ERROR>`
- `/errors service:<optional> since:<duration>`
- `/trace trace_id:<traceId>`
- `/alarm`
- `/help`

## Runtime

- Default port: `8088`
- Health endpoint: `GET /healthz`
- Discord endpoint: `POST /interactions`
- `DISCORD_REGISTER_COMMANDS` defaults to `false`; registration failures are reported in `/healthz` and do not restart the process unless `STRICT_STARTUP_CHECKS=true`.
- Default memory limit in compose: `96m`
- If OOM occurs on EC2, raise to `128m`; higher is not recommended for the current t3.micro.

The container must not publish a host port and must not mount the Docker socket.
