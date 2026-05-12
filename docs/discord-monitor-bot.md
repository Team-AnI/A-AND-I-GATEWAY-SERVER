# Discord Monitor Bot

## 목적

Gateway 리포에 Go 기반 경량 Discord HTTP Interactions sidecar를 둔다. Gateway Spring Boot 애플리케이션에는 Discord bot 기능이나 로그 수집 기능을 넣지 않는다.

운영 Gateway EC2는 t3.micro이고 MemAvailable이 약 184MiB 수준이라 Python, Node.js, JVM 기반 bot은 피한다. bot은 로그 저장소가 아니며, 로그 저장소는 CloudWatch Logs다. 사용자가 slash command를 호출할 때만 CloudWatch Logs Insights를 조회한다.

CD workflow는 Gateway 이미지와 monitor-bot 이미지를 같은 ECR repository에 push한다. monitor-bot은 별도 ECR repository를 만들지 않고, `monitor-bot-${releaseTag}` 형식의 tag로만 구분한다.

태그 배포는 운영 Gateway와 monitor-bot 컨테이너를 변경하므로 사용자 승인 후에만 진행한다. 승인 전에는 `git tag`, `git push origin v*.*.*`, GitHub Actions CD 수동 실행을 하지 않는다.

## Discord Commands

- `/dashboard since:<5m|15m|30m|1h|3h>`: 전체 서비스 health, 요청 수, 에러 수, latency, last log, alarm 요약
- `/watch channel:<channel> interval:<5m|10m|15m>`: 지정 채널에 dashboard message 1개를 만들고 주기적으로 edit
- `/unwatch`: 지속 dashboard 갱신 중지
- `/service service:<service> since:<duration>`: 특정 서비스 상세 상태, top path, 최근 에러 요약
- `/count service:<service|all> since:<duration> type:<all|api|error|4xx|5xx>`: 숫자 기반 로그 집계
- `/top service:<service|all> since:<duration> by:<path|error|status>`: 상위 path/error/status 집계
- `/slow service:<service> since:<duration> limit:<1..20> threshold_ms:<optional>`: 느린 API 조회
- `/copy-status since:<duration>`: Report assignment copy API 전용 성공/실패/latency 요약
- `/status`: gateway, report, auth, online-judge, post health 요약
- `/health service:<service>`: allowlist service health 조회
- `/logs service:<service> since:<5m|15m|30m|1h|3h> level:<INFO|WARN|ERROR>`: CloudWatch Logs 조회
- `/errors since:<duration> service:<optional>`: API_ERROR, WARN/ERROR, 4xx 이상 집계
- `/trace trace_id:<traceId>`: traceId 기준 시간순 조회
- `/alarm`: ALARM 상태 CloudWatch alarm 출력
- `/disk`: CloudWatch log group stored bytes와 retention 조회
- `/retention`: CloudWatch log group retention 조회
- `/help`: 명령어 예시

`service=report`는 기본 log group `/a-and-i/prod/report`를 조회한다.

## Dashboard UX

`/dashboard`는 운영자가 Discord에서 첫 화면으로 볼 수 있는 요약이다. health endpoint와 CloudWatch Logs Insights 결과를 slash command 호출 시점에만 조회한다. background polling, scheduler, 대용량 cache는 두지 않는다.

Dashboard는 현재 연결된 서비스만 보여주지 않는다. 운영 대상 전체 서비스인 `gateway`, `auth`, `report`, `online-judge`, `post`를 `ServiceRegistry` 고정 순서로 항상 표시한다. health URL이나 log group이 없는 서비스도 숨기지 않고 `UNKNOWN`, `NO_LOGS`, `NOT_CONFIGURED`, `LOG_QUERY_FAILED` 중 하나로 표시한다.

상태 색상 기준:

- `🟢`: health UP, 5xx/ERROR 없음
- `🟡`: health UNKNOWN, 소량 ERROR, latency 높음, last log 오래됨
- `🔴`: health DOWN, 5xx 발생, CloudWatch ALARM 감지
- `⚪`: 로그 데이터 없음
- `⚫`: health URL과 log group 중 필요한 설정 없음

상태 정의:

- `UP`: health check 성공
- `DOWN`: health check HTTP 실패
- `UNKNOWN`: health URL이 없거나 timeout/접근 실패
- `NO_LOGS`: log group은 설정되어 있으나 지정한 `since` 범위에 로그 없음
- `NOT_CONFIGURED`: health URL과 log group 중 dashboard에 필요한 설정 없음
- `LOG_QUERY_FAILED`: CloudWatch Logs Insights query 실패

예시:

```text
🟢 A&I Service Dashboard - last 30m

Service        Health          Logs             4xx   5xx   ERROR   Last log
gateway        🟢 UP           OK               12    0     0       10s ago
report         🟢 UP           OK               4     0     0       24s ago
auth           🟡 UNKNOWN      OK               22    1     1       1m ago
online-judge   ⚪ UNKNOWN      NO_LOGS          0     0     0       -
post           ⚫ NOT_CONFIGURED NOT_CONFIGURED -     -     -       -

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

## Persistent Dashboard And Alerts

`DASHBOARD_ENABLED=true`이면 monitor-bot은 `DISCORD_DASHBOARD_CHANNEL_ID`에 dashboard message를 1개 생성하고 이후 같은 message를 edit한다. message id는 `/var/lib/monitor-bot/state.json`에 저장한다. 새 메시지를 계속 보내지 않으므로 운영 채널이 도배되지 않는다.

`/watch`는 실행한 채널 또는 지정 채널에 dashboard 갱신을 설정한다. `/unwatch`는 state에서 dashboard channel/message id를 비우고 갱신을 중지한다.

state file에는 다음 정도의 작은 상태만 저장한다.

- `dashboardChannelId`
- `dashboardMessageId`
- `lastDashboardUpdatedAt`
- alert fingerprint별 `lastSentAt`, `active`, `resolvedAt`
- health DOWN 연속 횟수

CloudWatch raw log, query result, request/response body는 저장하지 않는다.

자동 알림은 `ALERT_ENABLED=true`일 때만 동작한다. 알림 조건:

- health DOWN 연속 `ALERT_HEALTH_DOWN_CONSECUTIVE`회
- 최근 5분 5xx가 `ALERT_5XX_THRESHOLD_5M` 이상
- 최근 5분 ERROR가 `ALERT_ERROR_THRESHOLD_5M` 이상
- assignment copy API 5xx가 `ALERT_COPY_API_5XX_THRESHOLD_5M` 이상
- 같은 fingerprint는 `ALERT_COOLDOWN_SECONDS` 동안 중복 전송하지 않음
- 활성 alert가 다음 poll에서 사라지면 resolved 알림 전송

fingerprint 형식:

```text
prod:<service>:<alertType>:<path>:<errorCode>
```

운영 EC2가 t3.micro라 dashboard interval을 너무 짧게 잡지 않는다. 권장값은 5분 이상이며, `MAX_CLOUDWATCH_QUERIES_PER_TICK=6` 기본값을 유지한다.

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

## Log Retention And Usage

로그 삭제/보관은 앱 내부 Quartz Cron이나 monitor-bot 명령으로 처리하지 않는다. CloudWatch Logs는 retention policy로 관리하고, EC2 로컬 Docker 로그는 Docker log rotation으로 제한한다.

CD는 주요 log group에 `CLOUDWATCH_LOG_RETENTION_DAYS`를 적용한다. 값이 비어 있으면 기본 14일을 사용한다. retention 설정 실패는 배포를 막지 않고 warning만 남긴다.

대상 log group:

- `/a-and-i/gateway`
- `/a-and-i/prod/monitor-bot`
- `/a-and-i/prod/report`
- `/a-and-i/prod/report-mongodb`
- `/a-and-i/auth`
- `/a-and-i/online-judge`
- `/a-and-i/prod/tech-blog`

`/disk`와 `/retention`은 CloudWatch `DescribeLogGroups` 기반 조회 전용 명령이다. host Docker disk를 직접 조회하거나 삭제하지 않고, Docker socket도 마운트하지 않는다. retention이 없는 log group은 `INFINITE`로 표시한다.

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
- `HEALTH_URL_AUTH`
- `HEALTH_URL_ONLINE_JUDGE`
- `HEALTH_URL_POST`
- `DISCORD_REGISTER_COMMANDS=false`
- `CLOUDWATCH_QUERY_TIMEOUT_SECONDS=8`
- `CLOUDWATCH_QUERY_POLL_INTERVAL_MS=500`
- `CLOUDWATCH_QUERY_LIMIT=20`
- `DASHBOARD_ENABLED=true`
- `DASHBOARD_REFRESH_INTERVAL_SECONDS=300`
- `DASHBOARD_SINCE=30m`
- `ALERT_ENABLED=true`
- `ALERT_POLL_INTERVAL_SECONDS=180`
- `ALERT_COOLDOWN_SECONDS=900`
- `ALERT_5XX_THRESHOLD_5M=3`
- `ALERT_ERROR_THRESHOLD_5M=5`
- `ALERT_HEALTH_DOWN_CONSECUTIVE=2`
- `ALERT_NO_LOGS_MINUTES=30`
- `ALERT_COPY_API_5XX_THRESHOLD_5M=1`
- `MAX_CLOUDWATCH_QUERIES_PER_TICK=6`

Optional Vars:

- `CLOUDWATCH_LOG_RETENTION_DAYS=14`
- `STRICT_STARTUP_CHECKS=false`

Additional Secrets:

- `DISCORD_DASHBOARD_CHANNEL_ID`
- `DISCORD_ALERT_CHANNEL_ID`

Runtime defaults:

- `BOT_HTTP_PORT=8088`
- `AWS_REGION=ap-northeast-2`
- `DISCORD_REGISTER_COMMANDS=false` when the var is empty
- Command registration failure does not stop the process unless `STRICT_STARTUP_CHECKS=true`

`BOT_ECR_REPOSITORY`는 사용하지 않는다.

### Discord Command Registration

첫 배포는 `DISCORD_REGISTER_COMMANDS=false`를 권장한다. bot 컨테이너와 `/healthz`가 정상인지 확인한 뒤 `DISCORD_REGISTER_COMMANDS=true`로 바꿔 command registration을 한 번 수행한다. 등록 성공 후에는 다시 `false`로 내려도 이미 등록된 guild slash command는 유지된다.

Discord slash command option은 `required=true` option이 `required=false` option보다 항상 앞에 있어야 한다. 예를 들어 `/errors`는 `since`가 required이고 `service`가 optional이므로 `since`, `service` 순서로 등록한다.

registration이 HTTP 400으로 실패하면 monitor-bot은 종료하지 않고 `/healthz`의 `discordCommandRegistrationError`와 컨테이너 로그에 Discord response body를 남긴다. HTTP 429가 나오면 `retry_after`를 기록하고 해당 boot에서는 즉시 반복 재시도하지 않는다. Bot token과 Authorization header는 로그에 출력하지 않는다.

`/healthz`는 command registration 실패가 있어도 프로세스 상태를 200 JSON으로 반환한다. 주요 필드는 `ok`, `httpServer`, `awsSdkConfigured`, `discordPublicKeyProvided`, `discordCommandsRegistered`, `discordCommandRegistrationError`, `dashboardEnabled`, `alertEnabled`, `version`이다.

### Discord Interactions Endpoint

`/discord/interactions`는 Discord HTTP Interactions용 POST 전용 endpoint다. 브라우저나 `curl`로 GET 요청을 보내서 `405 Method Not Allowed`가 나오면 정상 가능성이 높다.

Discord Developer Portal의 Interactions Endpoint URL에는 다음 값을 넣는다.

```text
https://api.aandiclub.com/discord/interactions
```

Developer Portal 저장 시 Discord가 POST PING interaction을 보내며, monitor-bot은 signature 검증 후 PONG을 반환해야 한다. Discord에서 `/dashboard`, `/help` 등이 보이지 않으면 command registration 성공 여부와 `/healthz`의 `discordCommandsRegistered`, `discordCommandRegistrationError`를 먼저 확인한다.

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
    STRICT_STARTUP_CHECKS: "false"
    LOG_GROUP_GATEWAY: "/a-and-i/gateway"
    LOG_GROUP_REPORT: "/a-and-i/prod/report"
    LOG_GROUP_AUTH: "/a-and-i/auth"
    LOG_GROUP_ONLINE_JUDGE: "/a-and-i/online-judge"
    LOG_GROUP_POST: "/a-and-i/prod/tech-blog"
    HEALTH_URL_GATEWAY: "http://gateway:9090/actuator/health/readiness"
    HEALTH_URL_REPORT: "http://<REPORT_PRIVATE_IP>:8080/actuator/health"
    CLOUDWATCH_QUERY_TIMEOUT_SECONDS: "8"
    CLOUDWATCH_QUERY_POLL_INTERVAL_MS: "500"
    CLOUDWATCH_QUERY_LIMIT: "20"
    DASHBOARD_ENABLED: "true"
    DISCORD_DASHBOARD_CHANNEL_ID: "${DISCORD_DASHBOARD_CHANNEL_ID}"
    DASHBOARD_REFRESH_INTERVAL_SECONDS: "300"
    DASHBOARD_SINCE: "30m"
    ALERT_ENABLED: "true"
    DISCORD_ALERT_CHANNEL_ID: "${DISCORD_ALERT_CHANNEL_ID}"
    ALERT_POLL_INTERVAL_SECONDS: "180"
    ALERT_COOLDOWN_SECONDS: "900"
    ALERT_5XX_THRESHOLD_5M: "3"
    ALERT_ERROR_THRESHOLD_5M: "5"
    ALERT_HEALTH_DOWN_CONSECUTIVE: "2"
    ALERT_NO_LOGS_MINUTES: "30"
    ALERT_COPY_API_5XX_THRESHOLD_5M: "1"
    MAX_CLOUDWATCH_QUERIES_PER_TICK: "6"
  volumes:
    - monitor-bot-state:/var/lib/monitor-bot
  logging:
    driver: awslogs
    options:
      awslogs-region: "ap-northeast-2"
      awslogs-group: "/a-and-i/prod/monitor-bot"
      awslogs-stream: "gateway-discord-bot"
      awslogs-create-group: "true"
  healthcheck:
    test: ["CMD", "/monitor-bot", "healthcheck", "--url", "http://127.0.0.1:8088/healthz"]
    interval: 30s
    timeout: 3s
    retries: 3
    start_period: 20s
```

Compose top-level volume:

```yaml
volumes:
  monitor-bot-state:
```

nginx 예시:

```nginx
location ^~ /.well-known/acme-challenge/ {
    root /var/www/certbot;
}

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
- bootstrap nginx는 최초 인증서 발급 전용이다.
- 운영 nginx가 이미 떠 있으면 bootstrap nginx를 실행하지 않는다. 실행하면 80 포트 충돌이 날 수 있다.
- 운영 배포에서는 nginx container recreate 대신 `nginx -t` 후 `nginx -s reload`를 사용한다.
- certbot renew는 기존 nginx webroot 기반으로 수행한다.
- Redis는 monitor-bot 배포와 무관하므로 pull/up/recreate하지 않는다.

## Gateway Internal Health Check

monitor-bot과 CD health wait는 Docker 내부 network에서만 Gateway actuator readiness를 조회한다. 외부 nginx의 `/actuator/` deny 정책과 9090 host port 비공개 정책은 유지한다.

- `8080`: Gateway API port
- `9090`: Gateway 내부 management/actuator port
- CD와 monitor-bot의 Gateway 성공 기준: `/actuator/health/readiness`
- 전체 `/actuator/health`는 Redis 같은 dependency health가 포함되어 503일 수 있으므로 CD 성공 기준으로 쓰지 않는다.

운영 EC2에서 내부 접근을 확인할 때:

```bash
cd /opt/aandi/gateway

NETWORK="$(sudo docker inspect aandi-gateway-server --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')"

sudo docker run --rm --network "$NETWORK" curlimages/curl:8.10.1 \
  -i http://gateway:9090/actuator/health/liveness

sudo docker run --rm --network "$NETWORK" curlimages/curl:8.10.1 \
  -i http://gateway:9090/actuator/health/readiness

sudo docker run --rm --network "$NETWORK" curlimages/curl:8.10.1 \
  -i -H "Host: api.aandiclub.com" http://gateway:9090/actuator/health/readiness

sudo docker run --rm --network "$NETWORK" curlimages/curl:8.10.1 \
  -i http://gateway:9090/actuator/health
```

`/actuator/health/liveness`와 `/actuator/health/readiness`는 인증 없이 200 계열이어야 한다. 전체 `/actuator/health`가 Redis component 때문에 503이어도 readiness가 200이면 CD는 Gateway 배포 성공으로 판단한다. `/actuator/health` 외 actuator endpoint는 public으로 열지 않는다.

Redis health가 `NOAUTH Authentication required`로 DOWN이면 Redis password가 설정된 상태일 수 있다. Gateway가 같은 password로 Redis에 접근하는지 확인한다.

```bash
sudo docker run --rm --network gateway_default redis:7-alpine \
  redis-cli -h redis -p 6379 ping

sudo docker inspect aandi-gateway-server \
  --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -Ei 'REDIS|SPRING_DATA'
```

필요한 경우 compose 환경 변수에 Redis 인증 정보를 맞춘다. Redis 컨테이너는 monitor-bot 배포와 무관하므로 recreate하지 않는다.

```yaml
SPRING_DATA_REDIS_HOST: redis
SPRING_DATA_REDIS_PORT: "6379"
SPRING_DATA_REDIS_PASSWORD: "${REDIS_PASSWORD}"
```

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

CloudWatch retention 설정 권한:

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:CreateLogGroup",
    "logs:PutRetentionPolicy",
    "logs:DescribeLogGroups"
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

## Pre-Deploy Checklist

- `DISCORD_REGISTER_COMMANDS=false`로 monitor-bot을 먼저 배포한다.
- `GET /discord/interactions`의 405는 POST 전용 endpoint 특성상 정상 가능성이 있다.
- `/healthz`에서 `httpServer=true`, `discordPublicKeyProvided=true`, `awsSdkConfigured=true`인지 확인한다.
- Gateway 상태 확인 URL은 `http://gateway:9090/actuator/health/readiness`를 사용한다.
- 전체 `/actuator/health`는 Redis health DOWN으로 503일 수 있다.
- 외부 nginx의 `/actuator/` deny와 9090 host port 비공개 정책을 유지한다.
- command 변경이 있을 때만 `DISCORD_REGISTER_COMMANDS=true`로 1회 등록하고, 성공 후 다시 `false`로 내린다.
- registration 400이 나면 로그의 sanitized response body에서 어떤 command payload가 거절됐는지 확인한다.
