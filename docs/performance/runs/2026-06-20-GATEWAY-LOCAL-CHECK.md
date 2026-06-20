# Gateway Local k6 Check - 2026-06-20

이 기록은 Gateway의 최대 처리량이나 성능 개선율을 주장하기 위한 자료가 아니다. 구조화 로그, traceId/requestId 기반 장애 추적, 오류 분류, 운영 알림 흐름을 보조하는 로컬 회귀 검증 기록이다.

## Environment

| 항목 | 값 |
| --- | --- |
| Measurement Target SHA | `63b74ea80fcdee45db16dea830016357bf398254` |
| k6 Version | `v1.7.1` |
| Git Dirty | `false` |
| Docker Compose | `performance/mock-upstream/docker-compose.performance.yml` |
| Mock Delay | `50 ms` |
| Payload Size | `1024 bytes` |
| VUs | `5` |
| Duration | `1m` |
| Sleep | `0.1s` |
| Run Repeat | `3` |
| Run Order | `alternating` |
| Public Route | `/v2/blogs` |
| Protected Route | `/v1/me` |
| Admin Route | `/v1/admin/ping` |

Warm-up은 측정 전에 Mock Direct `/v2/blogs`, Gateway `/v2/blogs`, USER `/v1/me`를 각각 10회 호출했으며, 측정 P95에는 포함하지 않았다.

## Direct vs Gateway

| Run | Order | Direct P50 | Direct P95 | Direct P99 | Gateway P50 | Gateway P95 | Gateway P99 | Additional P50 | Additional P95 | Additional P99 | Direct RPS | Gateway RPS | HTTP Fail | Check |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | Direct -> Gateway | 54.722 ms | 58.106 ms | 60.993 ms | 61.702 ms | 70.245 ms | 74.341 ms | 6.980 ms | 12.139 ms | 13.349 ms | 31.935 | 30.497 | 0.000 | 1.000 |
| 2 | Gateway -> Direct | 54.089 ms | 56.959 ms | 59.038 ms | 59.252 ms | 65.357 ms | 67.106 ms | 5.163 ms | 8.399 ms | 8.068 ms | 32.132 | 30.992 | 0.000 | 1.000 |
| 3 | Direct -> Gateway | 53.878 ms | 56.174 ms | 57.409 ms | 58.295 ms | 61.185 ms | 63.882 ms | 4.417 ms | 5.010 ms | 6.473 ms | 32.237 | 31.377 | 0.000 | 1.000 |

## Median

| 항목 | 중앙값 |
| --- | ---: |
| Direct P50 | 54.089 ms |
| Direct P95 | 56.959 ms |
| Direct P99 | 59.038 ms |
| Gateway P50 | 59.252 ms |
| Gateway P95 | 65.357 ms |
| Gateway P99 | 67.106 ms |
| Gateway Additional P50 | 5.163 ms |
| Gateway Additional P95 | 8.399 ms |
| Gateway Additional P99 | 8.068 ms |
| Direct RPS | 32.132 |
| Gateway RPS | 30.992 |

Gateway Additional P95 대표값은 각 pair의 `Gateway P95 - Direct P95` 값을 먼저 구한 뒤 3개 값의 중앙값을 사용했다. 이 값은 동일 로컬 조건에서 관측된 Gateway 추가 지연시간이며 처리량 또는 성능 개선율로 해석하지 않는다.

## Error Contract

| Scenario | Result |
| --- | --- |
| Token 없음 + `GET /v1/me` | PASS, `401 AUTHENTICATION_FAILED` |
| USER Token + `GET /v1/me` | PASS, `200` |
| USER Token + `GET /v1/admin/ping` | PASS, `403 ACCESS_DENIED` |
| ADMIN Token + `GET /v1/admin/ping` | PASS, `200` |
| Allowlist 밖의 경로 | PASS, `404 ENDPOINT_NOT_ALLOWLISTED` |
| Downstream `500` | PASS, downstream `500` passthrough |
| Downstream 연결 실패 | PASS, `502 DOWNSTREAM_SERVICE_UNAVAILABLE` |
| Login 제한 초과 | PASS, `429 LOGIN_RATE_LIMIT_EXCEEDED` |

오류 계약 검증은 latency 비교와 분리했다.

## Rate Limit

| 항목 | 값 |
| --- | ---: |
| Allowed responses | 10 |
| Rejected responses | 2 |
| Unexpected responses | 0 |
| HTTP failure rate | 0.000 |
| Check rate | 1.000 |

현재 Rate Limit은 JVM in-memory key 기반이므로 Redis key cleanup은 수행하지 않았다. 매 실행마다 고유 username을 사용하고 분 경계 근처 실행을 회피했다.

## Artifacts

| Artifact | Path |
| --- | --- |
| Aggregate JSON | `docs/performance/data/2026-06-20-gateway-local-check.json` |
| Deterministic SVG | `docs/assets/performance/gateway-k6-overhead.svg` |

Raw 결과는 `performance/results`에 생성되며 Git에는 커밋하지 않는다.

## Verification

| Command | Result |
| --- | --- |
| `./gradlew test` | PASS |
| `cd monitor-bot && go test ./...` | PASS |
| `./gradlew bootJar` | PASS |
| `bash -n performance/k6/run-local.sh` | PASS |
| `node --check performance/mock-upstream/server.js` | PASS |
| `find performance/k6 -name '*.js' -print0 \| xargs -0 -n1 node --check` | PASS |
| `python3 -m unittest performance.compare.test_compare_results` | PASS |
| `python3 -m unittest discover -s performance/aggregate -p 'test_*.py' -v` | PASS |
| `python3 -m unittest discover -s performance/report -p 'test_*.py' -v` | PASS |
| `docker compose -f performance/mock-upstream/docker-compose.performance.yml config` | PASS |
| `k6 inspect performance/k6/*.js` | PASS |
| `./performance/k6/run-local.sh performance/k6/env.local` | PASS |

## Limitations

- 이 실행은 로컬 Mock Downstream 기반의 보조 회귀 검증이다.
- 실제 Auth, Report, Post, Online Judge 서비스에는 요청하지 않았다.
- Gateway 최대 처리량, 운영 처리량, 성능 개선율을 주장하지 않는다.
- Monitor Bot 부하 테스트 결과는 Gateway 비교 결과에 포함하지 않았다.
- 이전 k6 v2.0.0 측정 결과는 이번 k6 v1.7.1 재측정 결과로 교체했다.
