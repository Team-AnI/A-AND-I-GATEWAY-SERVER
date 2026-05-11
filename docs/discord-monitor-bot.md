# Discord Monitor Bot

## 목적

Gateway 리포에 Go 기반 경량 Discord HTTP Interactions sidecar를 둔다. Gateway Spring Boot 애플리케이션에는 Discord bot 기능이나 로그 수집 기능을 넣지 않는다.

운영 Gateway EC2는 t3.micro이고 MemAvailable이 약 184MiB 수준이라 Python, Node.js, JVM 기반 bot은 피한다. bot은 로그 저장소가 아니며, 로그 저장소는 CloudWatch Logs다. 사용자가 slash command를 호출할 때만 CloudWatch Logs Insights를 조회한다.

CD workflow는 Gateway 이미지와 monitor-bot 이미지를 같은 ECR repository에 push한다. monitor-bot은 별도 ECR repository를 만들지 않고, `monitor-bot-${releaseTag}` 형식의 tag로만 구분한다.

태그 배포는 운영 Gateway와 monitor-bot 컨테이너를 변경하므로 사용자 승인 후에만 진행한다. 승인 전에는 `git tag`, `git push origin v*.*.*`, GitHub Actions CD 수동 실행을 하지 않는다.

## Discord Commands

- `/dashboard since:<5m|15m|30m|1h|3h>`: 전체 서비스 health, 요청 수, 에러 수, latency, last log, alarm 요약
- `/service service:<service> since:<duration>`: 특정 서비스 상세 상태, top path, 최근 에러 요약
- `/count service:<service> since:<duration> type:<all|api|error|4xx|5xx>`: 숫자 기반 로그 집계
- `/top service:<service> since:<duration> by:<path|error|status>`: 상위 path/error/status 집계
- `/slow service:<service> since:<duration> limit:<1..20> threshold_ms:<optional>`: 느린 API 조회
- `/copy-status since:<duration>`: Report assignment copy API 전용 성공/실패/latency 요약
- `/status`: gateway, report, auth, online-judge, post health 요약
- `/health service:<service>`: allowlist service health 조회
- `/logs service:<service> since:<5m|15m|30m|1h|3h> level:<INFO|WARN|ERROR>`: CloudWatch Logs 조회
- `/errors service:<optional> since:<duration>`: API_ERROR, WARN/ERROR, 4xx 이상 집계
- `/trace trace_id:<traceId>`: traceId 기준 시간순 조회
- `/alarm`: ALARM 상태 CloudWatch alarm 출력
- `/help`: 명령어 예시

`service=report`는 기본 log group `/a-and-i/prod/report`를 조회한다.

## Dashboard UX

`/dashboard`는 운영자가 Discord에서 첫 화면으로 볼 수 있는 요약이다. health endpoint와 CloudWatch Logs Insights 결과를 slash command 호출 시점에만 조회한다. background polling, scheduler, 대용량 cache는 두지 않는다.

상태 색상 기준:

- `🟢`: health UP, 5xx/ERROR 없음
- `🟡`: health UNKNOWN, 소량 ERROR, latency 높음, last log 오래됨
- `🔴`: health DOWN, 5xx 발생, CloudWatch ALARM 감지
- `⚪`: 로그 데이터 없음

예시:

```text
🟢 A&I Service Dashboard - last 30m

Service        Health     Total   4xx   5xx   ERROR   p95    Last log
gateway        🟢 UP      1240    18    0     0       92ms   12s ago
report         🟢 UP      312     4     0     0       148ms  24s ago
auth           🟡 UNKNOWN 890     22    1     1       110ms  1m ago
online-judge   🟢 UP      74      0     0     0       820ms  3m ago
post           🟡 UNKNOWN 41      0     0     0       170ms  8m ago

Alarms: none
Top issue: auth 5xx x1
```

상세/집계 명령 예시:

```text
/service service:report since:30m
/count service:report since:1h type:error
/top service:report since:1h by:error
/slow service:gateway since:30m limit:10 threshold_ms:1000
/copy-status since:1h
```

## V2 JSON Log Schema

Report 서버 로그는 Docker `awslogs` driver를 통해 CloudWatch Logs `/a-and-i/prod/report`에 한 줄 JSON으로 수집된다고 가정한다.

필수 규칙:

- `@timestamp`: Asia/Seoul offset datetime
- `env`: `prod`
- `service.name`: `report-service`
- `service.domainCode`: `4`
- 성공 API: `logType=API`, `level=INFO`
- 4xx 실패: `logType=API_ERROR`, `level=WARN`
- 5xx 실패: `logType=API_ERROR`, `level=ERROR`
- `trace.traceId`, `trace.requestId` 필수
- `http.method`, `http.path`, `http.route`, `http.statusCode`, `http.latencyMs` 필수
- 실패 로그에는 `response.error.code`, `response.error.value`, `response.error.message` 포함
- `tags`에는 가능한 경우 `report`, `assignment`, `copy`, `success/fail`, `admin` 포함

## CloudWatch Logs Insights

최근 Report API_ERROR:

```text
fields @timestamp, level, logType, env, service.name, service.domainCode, service.version, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, actor.userId, actor.role, actor.isAuthenticated, response.success, response.error.code, response.error.value, response.error.message, response.error.alert, message, tags
| filter service.name = "report-service"
| filter logType = "API_ERROR" or level = "ERROR" or http.statusCode >= 400
| sort @timestamp desc
| limit 20
```

Report trace:

```text
fields @timestamp, level, logType, service.name, service.version, trace.traceId, traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.success, response.error.code, response.error.value, response.error.message, message, tags
| filter trace.traceId = "{traceId}" or traceId = "{traceId}"
| sort @timestamp asc
| limit 100
```

Report error aggregation:

```text
fields service.name, http.path, http.statusCode, response.error.code, response.error.value, response.error.message
| filter service.name = "report-service"
| filter logType = "API_ERROR" or level = "ERROR" or http.statusCode >= 400
| stats count(*) as count by http.path, http.statusCode, response.error.code, response.error.value
| sort count desc
| limit 20
```

Dashboard/count 집계:

```text
fields service.name, logType, level, http.statusCode
| filter service.name = "report-service"
| stats count(*) as count by logType, level, http.statusCode
| sort count desc
| limit 50
```

Slow API:

```text
fields @timestamp, service.name, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.error.code, response.error.value, response.error.message, message
| filter service.name = "report-service"
| filter logType = "API" or logType = "API_ERROR"
| sort http.latencyMs desc
| limit 10
```

Assignment copy status:

```text
fields @timestamp, service.name, trace.traceId, trace.requestId, http.method, http.path, http.route, http.statusCode, http.latencyMs, response.success, response.error.code, response.error.value, response.error.message
| filter service.name = "report-service"
| filter http.method = "POST"
| filter http.path like /\/v2\/admin\/courses\/.*\/assignments\/copy/
| sort @timestamp desc
| limit 100
```

사용자 입력은 service/since/level allowlist 또는 traceId regex `^[a-zA-Z0-9._:-]{1,128}$` 검증 후에만 query에 사용한다. query는 반드시 좁은 시간 범위로 실행한다.

CloudWatch 비용과 응답 시간을 줄이기 위해 기본 조회 범위는 5m, 15m, 30m, 1h, 3h allowlist로만 제한한다. `/dashboard`는 최대 5개 log group만 조회하고, 각 query는 `CLOUDWATCH_QUERY_TIMEOUT_SECONDS` 기본 8초를 따른다.

## Discord Output Policy

출력 허용 필드:

- `@timestamp`
- `env`
- `service.name`
- `service.domainCode`
- `service.version`
- `trace.traceId`
- `trace.requestId`
- `level`
- `logType`
- `message`
- `http.method`
- `http.path`
- `http.route`
- `http.statusCode`
- `http.latencyMs`
- `actor.userId`
- `actor.role`
- `actor.isAuthenticated`
- `response.success`
- `response.error.code`
- `response.error.value`
- `response.error.message`
- `response.error.alert`
- `tags`

출력 금지:

- raw `@message`
- `headers.Authenticate`
- `headers.salt`
- `request.body`
- `response.data`
- `Authorization`, `authorization`
- `token`, `accessToken`, `refreshToken`
- `password`, `salt`, `secret`
- `cookie`, `session`
- `privateTestCases`, `hiddenTestCases`, `expectedOutput`
- `input`, `output`
- `userCode`, `sourceCode`, `code`

CloudWatch query 결과에 금지 필드가 포함되어도 formatting 단계에서 버린다. Discord 메시지는 항상 요약 형태로 출력하고 2000자 제한을 넘지 않도록 자른다.

## Required Environment

Required Secrets:

- `DISCORD_APPLICATION_ID`
- `DISCORD_PUBLIC_KEY`
- `DISCORD_BOT_TOKEN`
- `DISCORD_ALLOWED_GUILD_ID`
- `DISCORD_ALLOWED_ROLE_IDS`

Required Vars:

- `LOG_GROUP_REPORT=/a-and-i/prod/report`
- `LOG_GROUP_GATEWAY=/a-and-i/gateway`
- `LOG_GROUP_AUTH=/a-and-i/auth`
- `LOG_GROUP_ONLINE_JUDGE=/a-and-i/online-judge`
- `LOG_GROUP_POST=/a-and-i/prod/tech-blog`
- `HEALTH_URL_REPORT=http://<REPORT_PRIVATE_IP>:8080/actuator/health`
- `DISCORD_REGISTER_COMMANDS=true`
- `CLOUDWATCH_QUERY_TIMEOUT_SECONDS=8`
- `CLOUDWATCH_QUERY_POLL_INTERVAL_MS=500`
- `CLOUDWATCH_QUERY_LIMIT=20`

Runtime defaults:

- `BOT_HTTP_PORT=8088`
- `AWS_REGION=ap-northeast-2`
- `DISCORD_REGISTER_COMMANDS=false` when the var is empty

`BOT_ECR_REPOSITORY`는 사용하지 않는다.

## CD Deployment

monitor-bot은 기존 Gateway ECR repository를 사용한다. 예시:

```text
362622729632.dkr.ecr.ap-northeast-2.amazonaws.com/aandi-gateway-server:monitor-bot-v2.0.14
```

Gateway image tags:

```text
<ECR>/aandi-gateway-server:latest
<ECR>/aandi-gateway-server:<releaseTag>
```

Monitor bot image tag:

```text
<ECR>/aandi-gateway-server:monitor-bot-<releaseTag>
```

The generated compose service:

```yaml
monitor-bot:
  image: 362622729632.dkr.ecr.ap-northeast-2.amazonaws.com/aandi-gateway-server:monitor-bot-v2.0.14
  container_name: aandi-gateway-discord-bot
  restart: unless-stopped
  mem_limit: 96m
  memswap_limit: 192m
  cpus: "0.20"
  environment:
    AWS_REGION: ap-northeast-2
    BOT_HTTP_PORT: "8088"
    DISCORD_APPLICATION_ID: "${DISCORD_APPLICATION_ID}"
    DISCORD_PUBLIC_KEY: "${DISCORD_PUBLIC_KEY}"
    DISCORD_BOT_TOKEN: "${DISCORD_BOT_TOKEN}"
    DISCORD_ALLOWED_GUILD_ID: "${DISCORD_ALLOWED_GUILD_ID}"
    DISCORD_ALLOWED_ROLE_IDS: "${DISCORD_ALLOWED_ROLE_IDS}"
    DISCORD_REGISTER_COMMANDS: "${DISCORD_REGISTER_COMMANDS}"
    LOG_GROUP_GATEWAY: "/a-and-i/gateway"
    LOG_GROUP_REPORT: "/a-and-i/prod/report"
    LOG_GROUP_AUTH: "/a-and-i/auth"
    LOG_GROUP_ONLINE_JUDGE: "/a-and-i/online-judge"
    LOG_GROUP_POST: "/a-and-i/prod/tech-blog"
    HEALTH_URL_GATEWAY: "http://gateway:9090/actuator/health"
    HEALTH_URL_REPORT: "http://<REPORT_PRIVATE_IP>:8080/actuator/health"
    CLOUDWATCH_QUERY_TIMEOUT_SECONDS: "8"
    CLOUDWATCH_QUERY_POLL_INTERVAL_MS: "500"
    CLOUDWATCH_QUERY_LIMIT: "20"
  logging:
    driver: awslogs
    options:
      awslogs-region: "ap-northeast-2"
      awslogs-group: "/a-and-i/prod/monitor-bot"
      awslogs-stream: "gateway-discord-bot"
      awslogs-create-group: "true"
  healthcheck:
    test: ["/monitor-bot", "healthcheck", "--url", "http://127.0.0.1:8088/healthz"]
    interval: 30s
    timeout: 3s
    retries: 3
    start_period: 20s
```

nginx 예시:

```nginx
location = /discord/interactions {
    proxy_pass http://monitor-bot:8088/interactions;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

주의:

- `/healthz`는 외부 노출하지 않는다.
- `/actuator/` deny는 유지한다.
- 9090 management port를 외부 publish하지 않는다.
- Gateway management address는 Docker 내부 network 접근을 위해 compose에서 `MANAGEMENT_SERVER_ADDRESS=0.0.0.0`으로 설정한다.
- monitor-bot에 Docker socket을 마운트하지 않는다.
- monitor-bot host port를 publish하지 않는다.

## IAM

별도 monitor-bot ECR repository는 만들지 않으므로 ECR repository 권한 추가는 필요 없다. 기존 `aandi-gateway-server` repository push/pull 권한을 그대로 사용한다.

CloudWatch 조회 권한:

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:StartQuery",
    "logs:GetQueryResults",
    "logs:DescribeLogGroups",
    "logs:DescribeLogStreams",
    "cloudwatch:DescribeAlarms"
  ],
  "Resource": "*"
}
```

monitor-bot 자체 로그를 `awslogs`로 보낼 경우:

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:CreateLogGroup",
    "logs:CreateLogStream",
    "logs:PutLogEvents",
    "logs:DescribeLogStreams"
  ],
  "Resource": "*"
}
```

## Deployment Checks

```bash
cd /opt/aandi/gateway

sudo docker compose ps

sudo docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}'

sudo docker logs --tail=100 aandi-gateway-discord-bot

sudo docker inspect aandi-gateway-discord-bot \
  --format 'OOMKilled={{.State.OOMKilled}} RestartCount={{.RestartCount}} Health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}'

free -h
vmstat 1 5

sudo docker exec aandi-gateway-discord-bot \
  /monitor-bot healthcheck --url http://127.0.0.1:8088/healthz
```

Discord Developer Portal:

```text
Interactions Endpoint URL: https://api.aandiclub.com/discord/interactions
```
