# Gateway Local Overhead Measurement

> 운영 최대 처리량이 아니라 로컬 Mock Downstream 기반 회귀 검증 기준

- Generated At: 2026-06-28T00:00:00+00:00
- Measurement Status: 확인 완료
- Resume Use: 사용 가능
- Commit SHA: `910def3e12d278be087140083fd255de331d92b8`
- k6: v1.7.1
- JVM: openjdk version "21.0.6" 2025-01-21
- Docker: 29.5.3
- Planned VUs: 1
- Planned Duration: 10s
- Planned Payload Bytes: [1024, 65536, 1048576]
- BASE_URL: http://localhost:8080
- UPSTREAM_BASE_URL: http://localhost:18080
- DOWNSTREAM_URL: http://localhost:18080
- Target URL Policy: localhost/127.0.0.1/mock-upstream only

## Direct vs Gateway Overhead

| Scenario | Pairs | Direct P95 | Gateway P95 | Additional P95 | Additional P99 | HTTP failed | Checks | Throughput Direct/Gateway | Resume Use |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| logging disabled | 3 | 55.489 ms | 64.829 ms | 9.512 ms | 9.357 ms | 0.000 | 1.000 | 6.461/6.121 | 사용 가능 |
| logging enabled | 3 | 55.643 ms | 66.656 ms | 10.850 ms | 11.759 ms | 0.000 | 1.000 | 6.457/6.107 | 사용 가능 |
| payload 1024 bytes | 3 | 55.838 ms | 65.817 ms | 10.906 ms | 10.716 ms | 0.000 | 1.000 | 6.457/6.130 | 사용 가능 |
| payload 65536 bytes | 3 | 56.444 ms | 64.705 ms | 8.301 ms | 8.951 ms | 0.000 | 1.000 | 6.394/6.140 | 사용 가능 |
| payload 1048576 bytes | 3 | 71.560 ms | 80.117 ms | 8.565 ms | 7.566 ms | 0.000 | 1.000 | 5.690/5.500 | 사용 가능 |
| protected route | 3 | 55.300 ms | 65.547 ms | 9.853 ms | 8.847 ms | 0.000 | 1.000 | 6.465/6.162 | 사용 가능 |
| public route | 3 | 56.263 ms | 63.095 ms | 6.639 ms | 7.141 ms | 0.000 | 1.000 | 6.441/6.206 | 사용 가능 |

## Rate Limit and Error Contract

| Scenario | Expected | Actual | Checks | HTTP failed | Resume Use |
| --- | --- | --- | ---: | ---: | --- |
| Rate limit | allow 10.000, reject 2.000 | allow 10.000, reject 2.000 | 1.000 | 0.000 | 사용 가능 |
| Downstream failure contract | 502 maintained | passed | 1.000 | 0.000 | 사용 가능 |

## Resume Sentence Candidates

- 확인된 경우: 로컬 Mock Downstream 기준 payload 1KB/64KB/1MB별 Gateway P95/P99와 추가 지연을 측정해 라우팅·정책·로깅 계층 회귀 기준을 관리
- 확인된 경우: 구조화 로깅 on/off 비교로 Gateway 요청 추적 기능의 지연 비용을 로컬 기준으로 검증
