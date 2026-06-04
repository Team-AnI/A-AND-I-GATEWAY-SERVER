# Query Tuning

> 문서 목차로 돌아가기: [Gateway Docs](./README.md)

본 프로젝트는 Gateway와 Discord Monitor Bot 중심이며, 애플리케이션 코드에 RDB/MongoDB repository query가 없습니다. 이번 단계에서는 DB index나 query plan을 변경하지 않았고, CloudWatch Logs Insights query builder의 안전성과 측정 가능성을 테스트로 보강했습니다.

## 대상 쿼리

| 기능 | 쿼리/조건 | 문제 후보 | 근거 |
| :--- | :--- | :--- | :--- |
| Service log search | `BuildRecentLogsQueryWithSearch` | raw sensitive field 노출 또는 query injection | `monitor-bot/internal/cloudwatch/queries_test.go` |
| Trace drilldown | `BuildTraceQuery` | 검증되지 않은 traceId로 Logs Insights query 생성 | `monitor-bot/internal/cloudwatch/queries_test.go` |
| Alert polling | `BuildAlertQuery` | `@message` fallback 기반 분류로 오탐 또는 민감정보 노출 | `monitor-bot/internal/cloudwatch/queries_test.go` |
| Dashboard all-service query | `LogGroupsForOptionalService` | 너무 많은 log group 조회로 비용/지연 증가 | `monitor-bot/internal/cloudwatch/queries_test.go` |
| Last log check | `BuildLastLogQuery` | service filter 누락 또는 과도한 result scan | `monitor-bot/internal/cloudwatch/queries_test.go` |

## Before

| 지표 | 값 |
| :--- | :--- |
| DB `EXPLAIN` / `executionStats` | 해당 없음 |
| CloudWatch 실제 query latency | 현재 before/after 측정값은 없습니다 |
| Go total statements coverage | 58.7% |
| `internal/cloudwatch` coverage | 50.2% |
| `internal/security` coverage | 72.9% |

## 변경 내용

- DB index, Redis cache, Gateway route logic은 변경하지 않았습니다.
- CloudWatch query builder와 validation helper의 기존 동작을 보존하는 단위 테스트를 추가했습니다.
- all-service log group 조회가 configured order와 `maxGroups` 제한을 지키는지 검증했습니다.
- last-log query가 검증된 service filter와 `limit 1`을 유지하는지 검증했습니다.
- 로그 검색어 validation과 positive int fallback을 검증했습니다.
- JVM coverage gate 없이 JaCoCo report 생성 task를 추가했습니다.

## After

| 지표 | 값 |
| :--- | :--- |
| DB `EXPLAIN` / `executionStats` | 해당 없음 |
| CloudWatch 실제 query latency | 현재 before/after 측정값은 없습니다 |
| Go total statements coverage | 59.2% |
| `internal/cloudwatch` coverage | 58.6% |
| `internal/security` coverage | 78.1% |
| JVM line coverage | 91.32% |
| JVM branch coverage | 65.17% |

## 결과 해석

이번 변경은 실제 query latency 개선이 아니라, 운영자가 사용하는 CloudWatch query 생성 로직의 회귀 방지 범위를 넓힌 것입니다. 성능 개선률, latency 감소율, 비용 감소율은 측정하지 않았으므로 이력서에 쓰지 않습니다.

DB query tuning은 대상 repository query가 이 저장소에 없어서 수행하지 않았습니다. CloudWatch Logs Insights는 RDB/MongoDB의 `EXPLAIN` 대상이 아니므로, 실제 튜닝은 같은 log group과 같은 기간을 고정한 query latency 측정으로만 비교해야 합니다.

## 이력서 반영 가능 여부

- [x] 측정 명령이 남아 있다.
- [x] coverage before/after가 로컬 리포트 기준으로 남아 있다.
- [ ] 동일 환경 query latency before/after 비교다.
- [ ] 원본 CloudWatch query 실행 로그가 남아 있다.
- [ ] 운영 데이터와 synthetic 데이터를 구분했다.

## 다음 측정 절차

1. staging 또는 운영과 동일한 log group 사본을 정합니다.
2. 같은 `service`, `since`, `limit`, `query` 조건을 고정합니다.
3. `StartQuery` 호출 시각과 `GetQueryResults` 완료 시각을 기록합니다.
4. 변경 전/후 commit hash, log group, 조회 기간, row 수를 함께 저장합니다.
5. p95 query latency를 20회 이상 반복 측정한 뒤에만 성능 개선 문장을 작성합니다.
