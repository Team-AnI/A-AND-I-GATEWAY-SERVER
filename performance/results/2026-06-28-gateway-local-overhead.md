# Gateway Local Overhead Measurement

> 운영 최대 처리량이 아니라 로컬 Mock Downstream 기반 회귀 검증 기준

- Generated At: 2026-06-28T00:00:00+00:00
- Measurement Status: 측정 필요
- Resume Use: 사용 비추천
- Commit SHA: `9c3bcf803fc653df84fd42b62ad8dda5d0967380`
- k6: k6 v2.0.0 (commit/devel, go1.26.3, darwin/arm64)
- JVM: openjdk version "21.0.6" 2025-01-21
- Docker: unavailable: docker daemon is not running
- Planned VUs: 1
- Planned Duration: 10s
- Planned Payload Bytes: [1024, 65536, 1048576]
- BASE_URL: http://localhost:8080
- UPSTREAM_BASE_URL: http://localhost:18080
- DOWNSTREAM_URL: http://localhost:18080
- Target URL Policy: localhost/127.0.0.1/mock-upstream only

## Measurement Blocked

- Docker daemon unavailable: docker API socket is not running
- Installed k6 is k6 v2.0.0 (commit/devel, go1.26.3, darwin/arm64); expected official v1.7.1

## Missing Required Dimensions

- payload-overhead / 1024
- payload-overhead / 65536
- payload-overhead / 1048576
- route-overhead / public
- route-overhead / protected
- logging-overhead / enabled
- logging-overhead / disabled

## Direct vs Gateway Overhead

| Scenario | Pairs | Direct P95 | Gateway P95 | Additional P95 | Additional P99 | HTTP failed | Checks | Throughput Direct/Gateway | Resume Use |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| [측정 필요] | 0 | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | 사용 비추천 |

## Rate Limit and Error Contract

| Scenario | Expected | Actual | Checks | HTTP failed | Resume Use |
| --- | --- | --- | ---: | ---: | --- |
| Rate limit | allow [측정 필요], reject [측정 필요] | allow [측정 필요], reject [측정 필요] | [측정 필요] | [측정 필요] | 사용 비추천 |
| Downstream failure contract | 502 maintained | [측정 필요] | [측정 필요] | [측정 필요] | 사용 비추천 |

## Resume Sentence Candidates

- 확인된 경우: 로컬 Mock Downstream 기준 payload 1KB/64KB/1MB별 Gateway P95/P99와 추가 지연을 측정해 라우팅·정책·로깅 계층 회귀 기준을 관리
- 확인된 경우: 구조화 로깅 on/off 비교로 Gateway 요청 추적 기능의 지연 비용을 로컬 기준으로 검증
- 측정 부족 시: [측정 필요] Mock Downstream 기반 k6 시나리오로 Gateway 라우팅·인증·오류 계약의 성능 회귀를 검증
