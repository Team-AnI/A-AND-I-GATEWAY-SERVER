# Gateway Local k6 Check - 2026-06-20

이 기록은 Gateway의 최대 처리량이나 성능 개선율을 주장하기 위한 자료가 아니다. 구조화 로그, traceId/requestId 기반 장애 추적, 오류 분류, 운영 알림 흐름을 보조하는 로컬 회귀 검증 기록이다.

## Environment

| 항목 | 값 |
| --- | --- |
| Commit SHA | `a21084d09e8cb7cda2a99668082ca5e52f90aab6` |
| k6 Version | `v2.0.0` |
| Docker Compose | `performance/mock-upstream/docker-compose.performance.yml` |
| Mock Delay | `50 ms` |
| Payload Size | `1024 bytes` |
| VUs | `5` |
| Duration | `1m` |
| Run Repeat | `3` |
| Public Route | `/v2/blogs` |
| Protected Route | `/v1/me` |
| Admin Route | `/v1/admin/ping` |

Warm-up은 측정 전에 Mock Direct `/v2/blogs`, Gateway `/v2/blogs`, USER `/v1/me`를 각각 10회 호출했으며, 측정 P95에는 포함하지 않았다.

## Direct vs Gateway

| Run | Order | Direct P50 | Direct P95 | Direct P99 | Gateway P50 | Gateway P95 | Gateway P99 | Additional P50 | Additional P95 | Additional P99 | Direct RPS | Gateway RPS | HTTP Fail | Check |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | Direct -> Gateway | 54.662 ms | 57.517 ms | 58.819 ms | 62.730 ms | 70.878 ms | 74.676 ms | 8.068 ms | 13.361 ms | 15.857 ms | 32.102 | 30.375 | 0.000 | 1.000 |
| 2 | Gateway -> Direct | 54.681 ms | 58.932 ms | 61.881 ms | 59.520 ms | 65.099 ms | 70.743 ms | 4.839 ms | 6.168 ms | 8.862 ms | 31.901 | 30.935 | 0.000 | 1.000 |
| 3 | Direct -> Gateway | 54.560 ms | 58.457 ms | 60.952 ms | 59.690 ms | 65.397 ms | 67.802 ms | 5.130 ms | 6.941 ms | 6.849 ms | 31.911 | 30.924 | 0.000 | 1.000 |

## Median

| 항목 | 중앙값 |
| --- | ---: |
| Direct P95 | 58.457 ms |
| Gateway P95 | 65.397 ms |
| Gateway Additional P95 | 6.941 ms |

Gateway Additional P95 공식은 `Gateway P95 - Direct P95`이다. 이 값은 동일 로컬 조건에서 관측된 Gateway 추가 지연시간이며 처리량 또는 성능 개선율로 해석하지 않는다.

## Error Contract

| Scenario | Result |
| --- | --- |
| Token 없음 + `GET /v1/me` | PASS, `401 AUTHENTICATION_FAILED` |
| USER Token + `GET /v1/me` | PASS, `200` |
| USER Token + `GET /v1/admin/ping` | PASS, `403 ACCESS_DENIED` |
| ADMIN Token + `GET /v1/admin/ping` | PASS, `200` |
| Allowlist 밖의 경로 | PASS, `404 ENDPOINT_NOT_ALLOWLISTED` |
| Downstream 연결 실패 | PASS, `502 DOWNSTREAM_SERVICE_UNAVAILABLE` |
| Login 제한 초과 | PASS, `429 LOGIN_RATE_LIMIT_EXCEEDED` |

오류 계약 검증은 처리량 비교와 분리했다.

## Rate Limit

| 항목 | 값 |
| --- | ---: |
| Expected allowed responses | 10 |
| Observed allowed responses | 10 |
| Expected rejected responses | 2 |
| Observed rejected responses | 2 |
| HTTP failure rate | 0.000 |
| Check rate | 1.000 |

현재 Rate Limit은 JVM in-memory key 기반이므로 Redis key cleanup은 수행하지 않았다. 매 실행마다 고유 username을 사용하고 분 경계 근처 실행을 회피했다.

## Verification

| Command | Result |
| --- | --- |
| `./gradlew test` | PASS |
| `cd monitor-bot && go test ./...` | PASS |
| `./gradlew bootJar` | PASS |
| `docker compose -f performance/mock-upstream/docker-compose.performance.yml config` | PASS |
| `./performance/k6/run-local.sh` | PASS |

Raw 결과는 `performance/results`에 생성되며 Git에는 커밋하지 않는다.

## Limitations

- 이 실행은 로컬 Mock Downstream 기반의 보조 회귀 검증이다.
- 실제 Auth, Report, Post, Online Judge 서비스에는 요청하지 않았다.
- Gateway 최대 처리량, 운영 처리량, 성능 개선율을 주장하지 않는다.
- Monitor Bot 부하 테스트 결과는 Gateway 비교 결과에 포함하지 않았다.
