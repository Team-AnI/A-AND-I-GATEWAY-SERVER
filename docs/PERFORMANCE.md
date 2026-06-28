# Gateway 성능 측정

> 이 문서는 Gateway의 운영 최대 처리량을 주장하지 않습니다.
>
> 동일한 로컬 Mock Downstream을 직접 호출한 경우와 Gateway를 경유한 경우를 비교해, 정책·라우팅·로깅 계층의 회귀를 확인하기 위한 기준입니다.

[README로 돌아가기](../README.md)

## 무엇을 확인했나

Gateway 성능 검증은 세 가지를 분리해 실행합니다.

1. Mock Downstream 직접 호출과 Gateway 경유 호출의 지연 차이
2. 인증·권한·allowlist·downstream failure 오류 계약
3. 로그인 요청 제한 동작

실제 Auth, Report, Blog, Online Judge 서버에는 부하를 보내지 않습니다.

기존 baseline runner의 부하 테스트 대상은 로컬 또는 명시적으로 허용한 staging으로 제한하며 production 대상 실행은 스크립트에서 차단합니다.

추가 overhead runner는 staging도 기본 금지하고 `BASE_URL`을 `localhost` 또는 `127.0.0.1`로 제한합니다.

## 측정 환경

| 항목 | 값 |
| :--- | :--- |
| 측정일 | 2026-06-20 KST |
| 측정 대상 SHA | `63b74ea80fcdee45db16dea830016357bf398254` |
| k6 | v1.7.1 |
| Mock 응답 | 지연 50ms, payload 1KB |
| 부하 | 5 VUs, 1분 |
| 반복 | Direct/Gateway 순서를 바꾸며 3회 |
| Public route | `/v2/blogs` |
| Protected route | `/v1/me` |
| Admin route | `/v1/admin/ping` |

각 측정 전 Direct, Gateway, protected route를 10회씩 warm-up했습니다.

Warm-up 결과는 P95 계산에서 제외했습니다.

## Direct와 Gateway 비교

| 3회 중앙값 | 결과 |
| :--- | ---: |
| Direct P50 | 54.089 ms |
| Direct P95 | 56.959 ms |
| Direct P99 | 59.038 ms |
| Gateway P50 | 59.252 ms |
| Gateway P95 | 65.357 ms |
| Gateway P99 | 67.106 ms |
| Gateway 추가 P50 | 5.163 ms |
| Gateway 추가 P95 | **8.399 ms** |
| HTTP 실패율 | **0.00%** |
| Check 성공률 | **100.00%** |

Gateway 추가 지연은 각 실행 쌍에서 `Gateway percentile - Direct percentile`을 계산한 뒤 세 값의 중앙값을 사용합니다.

![Gateway k6 overhead](./assets/performance/gateway-k6-overhead.svg)

## 추가 Overhead 시나리오

기존 Direct P95 `56.959ms`, Gateway P95 `65.357ms`, Gateway 추가 P95 `8.399ms`는 위 2026-06-20 baseline과 [docs/performance/data/2026-06-20-gateway-local-check.json](./performance/data/2026-06-20-gateway-local-check.json)을 근거로 유지합니다.

PR 3에서는 기존 값을 새로 생성하지 않고, 다음 시나리오를 로컬 Mock Downstream 전용 runner로 추가했습니다.

### 2026-06-28 Overhead 측정 환경

| 항목 | 값 |
| :--- | :--- |
| 측정일 | 2026-06-28 KST |
| 측정 대상 SHA | `910def3e12d278be087140083fd255de331d92b8` |
| k6 | v1.7.1 |
| JVM | OpenJDK 21.0.6 |
| Docker | 29.5.3 |
| Mock 응답 | payload 1KB/64KB/1MB, 지연 50ms |
| 부하 | 1 VU, 10초 |
| 반복 | Direct/Gateway 순서를 바꾸며 3회 |
| Target URL | `http://localhost:8080`, `http://localhost:18080` |

### 2026-06-28 Overhead 결과

| 시나리오 | Direct P95 | Gateway P95 | Gateway 추가 P95 | Gateway 추가 P99 | HTTP 실패율 | Check 성공률 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| payload 1KB | 55.838 ms | 65.817 ms | 10.906 ms | 10.716 ms | 0.00% | 100.00% |
| payload 64KB | 56.444 ms | 64.705 ms | 8.301 ms | 8.951 ms | 0.00% | 100.00% |
| payload 1MB | 71.560 ms | 80.117 ms | 8.565 ms | 7.566 ms | 0.00% | 100.00% |
| public route | 56.263 ms | 63.095 ms | 6.639 ms | 7.141 ms | 0.00% | 100.00% |
| protected route | 55.300 ms | 65.547 ms | 9.853 ms | 8.847 ms | 0.00% | 100.00% |
| logging enabled | 55.643 ms | 66.656 ms | 10.850 ms | 11.759 ms | 0.00% | 100.00% |
| logging disabled | 55.489 ms | 64.829 ms | 9.512 ms | 9.357 ms | 0.00% | 100.00% |

Rate limit 검증에서는 12건 중 10건을 허용하고 2건을 차단했으며 예상하지 않은 응답은 없었습니다.

Downstream failure 오류 계약 검증은 check 성공률 100.00%, HTTP 실패율 0.00%로 통과했습니다.

결과 파일은 측정 완료 상태를 명시합니다.

- [performance/results/2026-06-28-gateway-local-overhead.json](../performance/results/2026-06-28-gateway-local-overhead.json)
- [performance/results/2026-06-28-gateway-local-overhead.md](../performance/results/2026-06-28-gateway-local-overhead.md)
- [docs/performance/data/2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json)

이 결과는 운영 최대 처리량이 아니라 로컬 Mock Downstream 기반 회귀 검증 기준입니다.

## 오류 계약

| 시나리오 | 결과 |
| :--- | :--- |
| Token 없이 `GET /v1/me` | `401 AUTHENTICATION_FAILED` |
| USER Token으로 `GET /v1/me` | `200` |
| USER Token으로 `GET /v1/admin/ping` | `403 ACCESS_DENIED` |
| ADMIN Token으로 `GET /v1/admin/ping` | `200` |
| allowlist 밖의 경로 | `404 ENDPOINT_NOT_ALLOWLISTED` |
| Downstream 500 | 500 응답 전달 |
| Downstream 연결 실패 | `502 DOWNSTREAM_SERVICE_UNAVAILABLE` |
| 로그인 제한 초과 | `429 LOGIN_RATE_LIMIT_EXCEEDED` |

로그인 제한 검증에서는 12건 중 10건을 허용하고 2건을 차단했으며 예상하지 않은 응답은 없었습니다.

## 실행

필요 도구는 Docker, JDK 21, Go, Python 3, k6입니다.

```bash
./gradlew test
cd monitor-bot && go test ./...
cd ..
./performance/k6/run-local.sh

scripts/performance/install_pinned_k6.sh
export K6_BIN="$PWD/.tools/k6/$(cat performance/k6/K6_VERSION)/k6"
./performance/k6/run-local-overhead.sh
```

개별 시나리오도 실행할 수 있습니다.

```bash
k6 run performance/k6/direct-upstream.js
k6 run performance/k6/gateway-public-route.js
k6 run performance/k6/gateway-protected-route.js
k6 run performance/k6/gateway-payload-overhead.js
k6 run performance/k6/gateway-route-overhead.js
k6 run performance/k6/gateway-logging-overhead.js
k6 run performance/k6/gateway-error-contract.js
k6 run performance/k6/gateway-rate-limit.js
```

추가 overhead 결과 집계는 다음 스크립트가 수행합니다.

```bash
python3 performance/aggregate/summarize_gateway_overhead.py \
  --input performance/results \
  --run-prefix "$RUN_ID" \
  --out-json performance/results/YYYY-MM-DD-gateway-local-overhead.json \
  --out-md performance/results/YYYY-MM-DD-gateway-local-overhead.md \
  --docs-json docs/performance/data/YYYY-MM-DD-gateway-local-overhead.json
```

## 결과를 비교할 때 지켜야 할 기준

다음 조건이 모두 같을 때만 before/after 차이를 계산합니다.

- commit SHA와 git dirty 상태
- k6 버전과 executor
- VUs, duration, 실행 순서, warm-up 여부
- route, Mock 지연, payload 크기와 상태 코드
- JVM, Docker, 하드웨어 환경

이 결과는 로컬 회귀 baseline입니다.

Gateway 최대 처리량, 운영 처리량, 성능 개선율로 표현하지 않습니다.

## 다음 측정

현재 public GET 경로의 추가 지연은 확인했습니다.

다음 측정은 같은 로컬 Mock Downstream 조건을 유지한 채 Gateway 2개 인스턴스 이상에서 분산 동작을 검증합니다.

- Gateway 2개 인스턴스에서 분산 Rate Limit 정확도
- 동일 critical 이벤트 반복 시 Discord role mention 억제율
