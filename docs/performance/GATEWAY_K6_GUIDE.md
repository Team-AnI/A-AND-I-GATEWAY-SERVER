# Gateway k6 Performance Guide

This guide keeps the root `README.md` unchanged. It documents local-only k6 checks for Spring Cloud Gateway routing overhead, auth policy, error contract, and auth rate limit behavior.

## Repository Findings

Existing performance assets:

- No existing `performance/` or k6 files were present.
- No existing Mock Server or WireMock setup was present.
- Existing Docker Compose starts `nginx`, `gateway`, and `redis`; it points Gateway to real downstream locations through environment variables.

Gateway route groups in `src/main/resources/application.yaml`:

| Upstream env | Route groups |
| --- | --- |
| `AUTH_SERVICE_URI` | `/v1/auth/*`, `/v1/me`, `/v1/admin/*`, `/v1/users/**`, `/v2/auth/*`, `/activate`, `/v2/activate`, `/v2/me`, `/v2/users/lookup`, `/v2/admin/*`, `/v2/auth/users/**` |
| `REPORT_SERVICE_URI` | `/v1/report/**`, `/v1/admin/courses/**`, `/v1/courses/**`, `/v2/report/**`, `/v2/admin/courses/**`, `/v2/courses/**`, `/v2/assignments/*/course`, `/v2/post/admin/courses/**`, `/v2/post/courses/**` |
| `POST_SERVICE_URI` | `/v1/posts/**`, `/v2/post/**`, `/v2/posts/**`, `/v2/blogs/**`, `/v2/lectures/**` |
| `ONLINE_JUDGE_SERVICE_URI` | `/v1/submissions/**`, `/v1/problems/{problemId}/submissions/me`, `/v1/admin/submissions`, `/v1/admin/testcases`, `/v2/online-judge/**`, native `/v2/submissions/**`, `/v2/problems/**`, `/v2/admin/submissions`, `/v2/admin/testcases` |

URI rewrite behavior:

- Most business routes preserve the incoming path.
- OpenAPI proxy routes use `SetPath=/v3/api-docs` or `RewritePath=/v2/<service>/v3/api-docs/...`.
- `/v1/report` and `/v1/report/**` have no-op `SetPath` / `RewritePath` rules that keep the same `/v1/report` prefix.

Security and policy behavior:

- JWT verification is enabled by default through `SecurityConfig`; this performance setup does not disable it.
- Public routes include auth login/refresh/logout, activation, ping, Swagger/OpenAPI, and actuator health.
- Role-protected examples: `/v1/me` requires `USER`, `ORGANIZER`, or `ADMIN`; `/v1/admin/**`, `/v2/admin/**`, `/v2/auth/admin/**`, and online judge admin routes require `ADMIN`; draft and write blog/post/lecture routes require `ORGANIZER` or `ADMIN`.
- `GatewayRequestPolicyFilter` applies HTTPS, Host, method/path allowlist, explicit deny, and JSON Content-Type policy before the request reaches downstream.
- Host and HTTPS are controlled by `ENFORCE_HTTPS`, `ALLOWED_HOSTS`, and `ALLOW_PRIVATE_IP_HOST`.
- JSON Content-Type is required for `POST`, `PUT`, and `PATCH` except configured multipart-compatible content paths.

Rate limit behavior:

- Current auth rate limit implementation is `AuthRateLimitFilter`, an in-memory per-Gateway-instance counter.
- It applies only to `POST /v1/auth/login`, `POST /v2/auth/login`, `POST /v1/auth/refresh`, `POST /v2/auth/refresh`, `POST /v1/auth/logout`, and `POST /v2/auth/logout`.
- Current key patterns are `login:<remote-ip>:<username>` and `refresh:<remote-ip>:<sha256(refreshToken)>`.
- Redis is used for token context cache keys such as `cache:token:*` and `cache:token-index:*`; there is no Redis-backed rate limit key in the current code.
- The k6 rate limit script uses a unique username per run and treats 429 as expected after the configured login limit.
- Because the implementation uses epoch-minute windows, the local runner waits past the minute boundary when it starts at second 50 or later.

Common error contract:

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": 11001,
    "message": "...",
    "value": "AUTHENTICATION_FAILED",
    "alert": "..."
  },
  "timestamp": "..."
}
```

Gateway-owned mappings include:

| HTTP status | Gateway error value |
| --- | --- |
| 401 | `AUTHENTICATION_FAILED` |
| 403 | `ACCESS_DENIED`, `HTTPS_REQUIRED`, `HOST_NOT_ALLOWED`, or `INTERNAL_TOKEN_INVALID` depending on policy |
| 404 | `ENDPOINT_NOT_ALLOWLISTED` |
| 429 | `LOGIN_RATE_LIMIT_EXCEEDED`, `REFRESH_RATE_LIMIT_EXCEEDED`, or `LOGOUT_RATE_LIMIT_EXCEEDED` |
| 502 | `DOWNSTREAM_SERVICE_UNAVAILABLE` for Gateway-owned downstream connection, timeout, DNS, or premature-close failures |
| 500 | Downstream 500 passthrough when the downstream returns a standard error body; `INTERNAL_SERVER_ERROR` for unexpected Gateway exceptions |

Structured log fields:

- Top-level: `@timestamp`, `level`, `logType`, `message`, `env`, `service`, `trace`, `http`, `headers`, `client`, `actor`, `request`, `response`, `tags`.
- HTTP: `method`, `path`, `route`, `statusCode`, `latencyMs`.
- Trace headers: `X-Trace-Id`, `X-Request-Id`.
- Auth headers are masked in logs.

Profiles and environment overrides:

- `application.yaml` uses environment overrides for all downstream URIs: `AUTH_SERVICE_URI`, `REPORT_SERVICE_URI`, `POST_SERVICE_URI`, and `ONLINE_JUDGE_SERVICE_URI`.
- `docker-compose.performance.yml` runs Gateway with `--spring.profiles.active=performance` and points those URI variables to the local Mock Downstream.
- `run-local.sh` overrides only the performance Compose `REPORT_SERVICE_URI` to `http://127.0.0.1:65534` for the Gateway-owned 502 connection-failure contract check.
- No production route definitions are changed for performance testing.

JWT test state:

- Existing unit tests use `spring-security-test` `mockJwt()` for role checks.
- k6 protected route checks use local-only USER and ADMIN test JWTs generated by `performance/k6/tools/generate_test_jwt.py` during `run-local.sh`.
- Individual k6 runs may use `USER_ACCESS_TOKEN`, `ADMIN_ACCESS_TOKEN`, `ACCESS_TOKEN`, or `TEST_JWT`; full `run-local.sh` fails auth scenarios unless tokens are generated or `SKIP_AUTH_SCENARIOS=true` is explicit.
- JWT secrets and real access tokens must not be committed.

## Local Services

The local performance Compose file starts:

- `mock-upstream` on `http://localhost:18080`
- `redis-performance` on host port `6380`
- `gateway-performance` on `http://localhost:8080`, management on `http://localhost:9090`

The Mock Downstream is implemented in `performance/mock-upstream/server.js`, returns fixed JSON, and accepts:

- `status=200|400|401|403|404|429|500`
- `payloadBytes=<bytes>` from `0` to `1048576`
- `delayMs=<milliseconds>` from `0` to `5000`

Invalid numeric parameters and unsupported statuses return a 400 mock error. Every mock response includes `X-Mock-Upstream: true`.

Example:

```bash
curl 'http://localhost:18080/v2/blogs?status=200&payloadBytes=1024&delayMs=50'
```

## Run

Install Docker, k6, Python 3, JDK 21, and Go before running the full local workflow.

Run repository tests:

```bash
./gradlew test
cd monitor-bot && go test ./...
```

Run the local performance workflow:

```bash
./performance/k6/run-local.sh
```

Run individual scripts:

```bash
docker compose -f performance/mock-upstream/docker-compose.performance.yml up -d --build
k6 run performance/k6/direct-upstream.js
k6 run performance/k6/gateway-public-route.js
k6 run performance/k6/gateway-protected-route.js
k6 run performance/k6/gateway-error-contract.js
k6 run performance/k6/gateway-rate-limit.js
```

Warm-up:

- `run-local.sh` runs `performance/k6/warmup.js` before measured Direct/Gateway runs.
- Warm-up does not write result JSON/Markdown and is not included in comparison output.
- When `RUN_REPEAT` is 2 or greater, measured order alternates by pair: odd pairs run Direct then Gateway; even pairs run Gateway then Direct.

Compare Direct and Gateway runs:

```bash
python3 performance/compare/compare_results.py \
  --direct performance/results/direct-upstream-<run-id>-r1.json \
  --gateway performance/results/gateway-public-route-<run-id>-r1.json \
  --output-dir performance/results
```

## Environment

Supported core variables:

| Variable | Purpose |
| --- | --- |
| `BASE_URL` | Gateway base URL. Defaults to `http://localhost:8080`. |
| `UPSTREAM_BASE_URL` | Mock Downstream direct URL. Defaults to `http://localhost:18080`. |
| `ACCESS_TOKEN` / `TEST_JWT` | Optional JWT for protected route and 403 contract checks. |
| `ALLOW_REMOTE_LOAD_TEST` | Must be `true` to allow non-local targets. Keep `false` for local runs. |
| `TARGET_ENVIRONMENT` | Defaults to `local`. `prod` and `production` are always blocked. |
| `REMOTE_TARGET_ALLOWLIST` | Comma-separated exact host allowlist for approved staging-only remote tests. |
| `LOAD_VUS` | Constant VUs for direct/public/protected load scripts. |
| `TEST_DURATION` | Duration for direct/public/protected load scripts. |
| `P95_THRESHOLD_MS` | Optional P95 threshold. Blank means no P95 threshold. |
| `RESULT_DIR` | Result output directory. Defaults to `performance/results`. |

Additional local knobs:

| Variable | Purpose |
| --- | --- |
| `PUBLIC_ROUTE_PATH` | Public Gateway route for direct-vs-Gateway comparison. Defaults to `/v2/blogs`. |
| `PROTECTED_ROUTE_PATH` | Protected Gateway route. Defaults to `/v1/me`. |
| `FORBIDDEN_ROUTE_PATH` | Admin route for 403 checks. Defaults to `/v1/admin/ping`. |
| `RATE_LIMIT_PATH` | Login route for auth rate limit checks. Defaults to `/v1/auth/login`. |
| `PAYLOAD_BYTES` | Mock payload string length. |
| `MOCK_DELAY_MS` | Mock downstream delay. Use `0`, `50`, `100`, or `300` for standard runs. |
| `AUTH_LOGIN_RATE_LIMIT_PER_MINUTE` | Expected current login limit. Do not relax it for tests. |
| `SKIP_AUTH_SCENARIOS` | Full local runs require generated USER/ADMIN tokens unless this is explicitly `true`. |
| `KEEP_PERFORMANCE_STACK` | Set to `true` to keep Docker Compose services after `run-local.sh`; defaults to cleanup. |

## CI And Results

- CI static validation covers shell syntax, Mock server syntax, k6 script syntax, Python compile/unit tests, Compose config, and `git diff --check`.
- CI green for static validation does not mean k6 runtime smoke has executed.
- Runtime smoke results are written under `RESULT_DIR`; `performance/results/*`, local env files, and token-like result files are Git-ignored.
- Generated JWT values are local-only and must not be copied into JSON, Markdown, logs, or commits.

## Result Rules

- Direct and Gateway comparison is valid only when commit SHA, git dirty state, executor, route path, VUs, duration, payload size, mock delay, mock status, pair order/index, warm-up state, k6 version, and repeat count match.
- Report the comparison as Gateway additional latency: `Gateway percentile - Direct percentile`.
- Do not describe results as performance improvement.
- Do not write unexecuted latency, RPS, failure-rate, or improvement numbers.
- Monitor Bot tests are separate repository checks and are not included in Gateway load-test results.
