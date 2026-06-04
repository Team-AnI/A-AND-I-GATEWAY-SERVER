# Performance Measurement

> 메인 README로 돌아가기: [README](../README.md)

본 문서는 현재 확인 가능한 성능 측정값과 향후 측정 기준을 분리합니다. 현재 before/after 측정값은 없습니다.

## 현재 측정값 상태

| 항목 | 상태 |
| :--- | :--- |
| Gateway p95 latency | 현재 before/after 측정값은 없습니다 |
| alert delivery latency | 현재 before/after 측정값은 없습니다 |
| Discord command response time | 현재 before/after 측정값은 없습니다 |
| duplicate alert count | 현재 before/after 측정값은 없습니다 |
| Gateway throughput | 현재 before/after 측정값은 없습니다 |

## 이번 단계에서 확인한 내용

Coverage / Query Tuning 단계에서는 실제 운영 CloudWatch Logs, Discord API, downstream service가 연결된 동일 환경 before/after 측정값을 확보하지 못했습니다. 따라서 Gateway latency, alert delivery latency, CloudWatch query latency 개선 수치는 작성하지 않습니다.

대신 성능 측정의 기반이 되는 CloudWatch Logs Insights query builder를 단위 테스트로 보강했습니다.

- all-service query의 log group 순서와 `maxGroups` 제한 검증
- last-log query의 service filter와 `limit 1` 검증
- query time range lookback 계산 검증
- 로그 검색어 injection성 입력 거부 검증

관련 테스트와 coverage 결과는 [Test Results](./test.md)에 기록했습니다.

## 코드상 확인된 측정 기반

Gateway structured log는 `http.latencyMs`를 남깁니다. monitor-bot의 CloudWatch dashboard summary query는 `pct(http.latencyMs, 95) as p95`와 `max(http.latencyMs)`를 계산하도록 작성되어 있습니다.

근거:

- `src/main/kotlin/com/aandi/gateway/logging/ApiLogFactory.kt`
- `monitor-bot/internal/cloudwatch/queries.go`

## 향후 측정할 지표

| 지표 | 측정 방법 후보 | 사용 목적 |
| :--- | :--- | :--- |
| Gateway p95 latency | CloudWatch Logs Insights `pct(http.latencyMs, 95)` | Gateway 정책/라우팅 지연 확인 |
| alert delivery latency | log timestamp와 Discord send timestamp 차이 | 장애 알림 전달 지연 확인 |
| duplicate alert count | state suppression count와 sent alert count 비교 | 반복 알림 억제 효과 확인 |
| command response time | Discord interaction 수신부터 follow-up 전송까지 | 운영 명령 UX 확인 |
| trace drilldown query time | CloudWatch query start부터 result까지 | trace 조회 응답성 확인 |

## 측정 절차 초안

| 항목 | 조건 |
| :--- | :--- |
| 실행 환경 | 동일 EC2 또는 동일 local/staging 환경 |
| 데이터 조건 | 같은 CloudWatch log group, 같은 조회 기간, 같은 service set |
| Gateway latency | Gateway structured log의 `http.latencyMs`를 `pct(http.latencyMs, 95)`로 집계 |
| Alert delivery latency | alert 대상 log timestamp와 Discord send timestamp 차이 기록 |
| CloudWatch query latency | query 시작 시각과 `GetQueryResults` 완료 시각 차이 기록 |
| 비교 기준 | 같은 commit 전/후 또는 같은 환경의 설정 전/후 |

## 이력서 작성 기준

현재는 다음 문장을 쓰면 안 됩니다.

- Gateway latency N% 개선
- 장애 감지 시간 N분 단축
- alert delivery latency N초 이하 달성
- duplicate alert N% 감소

실제 리포트가 생기면 측정 기간, 표본 수, before/after 조건, CloudWatch query 또는 test script를 함께 남긴 뒤 [Resume Evidence](./resume-evidence.md)에 연결합니다.
