# Discord Monitor Bot

## 목적

Gateway 리포에 Go 기반 경량 Discord HTTP Interactions sidecar를 둔다. Gateway Spring Boot 애플리케이션에는 Discord bot 기능이나 로그 수집 기능을 넣지 않는다.

운영 Gateway EC2는 t3.micro이고 MemAvailable이 약 184MiB 수준이라 Python, Node.js, JVM 기반 bot은 피한다. bot은 로그 저장소가 아니며, 로그 저장소는 CloudWatch Logs다. 사용자가 slash command를 호출할 때만 CloudWatch Logs Insights를 조회한다.

이 PR에서는 운영 배포를 하지 않는다. 새 tag를 만들지 않고, CD workflow를 실행하지 않는다.

## Discord Commands

- `/status`: gateway, report, auth, online-judge, post health 요약
- `/health service:<service>`: allowlist service health 조회
- `/logs service:<service> since:<5m|15m|30m|1h|3h> level:<INFO|WARN|ERROR>`: CloudWatch Logs 조회
- `/errors service:<optional> since:<duration>`: API_ERROR, WARN/ERROR, 4xx 이상 집계
- `/trace trace_id:<traceId>`: traceId 기준 시간순 조회
- `/alarm`: ALARM 상태 CloudWatch alarm 출력
- `/help`: 명령어 예시

`service=report`는 기본 log group `/a-and-i/prod/report`를 조회한다.

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

사용자 입력은 service/since/level allowlist 또는 traceId regex `^[a-zA-Z0-9._:-]{1,128}$` 검증 후에만 query에 사용한다. query는 반드시 좁은 시간 범위로 실행한다.

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

- `BOT_HTTP_PORT=8088`
- `DISCORD_APPLICATION_ID`
- `DISCORD_PUBLIC_KEY`
- `DISCORD_BOT_TOKEN`
- `DISCORD_ALLOWED_GUILD_ID`
- `DISCORD_ALLOWED_ROLE_IDS`
- `DISCORD_REGISTER_COMMANDS=false`
- `AWS_REGION=ap-northeast-2`
- `LOG_GROUP_REPORT=/a-and-i/prod/report`
- `HEALTH_URL_REPORT=http://<REPORT_PRIVATE_IP>:8080/actuator/health`

기타 log group override:

- `LOG_GROUP_GATEWAY`
- `LOG_GROUP_AUTH`
- `LOG_GROUP_ONLINE_JUDGE`
- `LOG_GROUP_POST`

## Future Manual Deployment Example

이 PR에서는 `cd.yml`에 monitor-bot 배포를 붙이지 않는다. 운영 배포는 별도 승인 후 수동으로만 한다.

```yaml
monitor-bot:
  image: <ECR>/aandi-gateway-monitor-bot:<tag-or-sha>
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
    LOG_GROUP_REPORT: "/a-and-i/prod/report"
    HEALTH_URL_REPORT: "http://<REPORT_PRIVATE_IP>:8080/actuator/health"
    CLOUDWATCH_QUERY_TIMEOUT_SECONDS: "8"
    CLOUDWATCH_QUERY_POLL_INTERVAL_MS: "500"
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
- Gateway management address를 sidecar 접근용으로 `0.0.0.0`으로 바꾸는 것은 향후 배포 시에만 검토한다.
- monitor-bot에 Docker socket을 마운트하지 않는다.

## IAM

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
