# Resume Evidence

> 메인 README로 돌아가기: [README](../README.md)

본 문서는 이력서에 쓸 수 있는 문장과 실제 근거를 연결합니다. 수치가 확인되지 않은 성능 개선률, 장애 감지 시간 단축률, coverage 달성률은 작성하지 않습니다.

## 이력서 연결 요약

| 이력서 문장 | README 위치 | 상세 근거 | 검증 상태 |
| :--- | :--- | :--- | :--- |
| MSA 환경에서 흩어진 로그와 운영 이벤트를 Discord에서 확인할 수 있도록 Gateway와 Monitor Bot을 구성했습니다. | [핵심 역할](../README.md#핵심-역할) | [Observability](./observability.md), `monitor-bot/internal/cloudwatch/queries.go` | JVM/Go 테스트 통과 |
| Discord Monitor Bot을 Gateway JVM과 분리된 Go sidecar로 운영해 운영 도구가 본 서버 안정성에 영향을 주지 않도록 설계했습니다. | [Discord Monitor Bot](../README.md#discord-monitor-bot) | [Architecture](./architecture.md), [Deployment](./deployment.md), `monitor-bot/Dockerfile` | Go 테스트 통과 |
| general alert와 critical alert를 분리하고, critical alert에만 역할 멘션을 허용하도록 운영 기준을 정리했습니다. | [Critical / General Alert Routing](../README.md#critical--general-alert-routing) | [Ops Alert Flow](./api-flows/ops-alert.md), `monitor-bot/internal/monitor/alerts.go` | Go 테스트 통과 |
| trace drilldown과 assignment audit feed를 통해 운영 이벤트 추적 가능성을 개선했습니다. | [Trace Drilldown](../README.md#trace-drilldown), [Assignment Audit Feed](../README.md#assignment-audit-feed) | [Trace Drilldown](./api-flows/trace-drilldown.md), [Assignment Audit Flow](./api-flows/assignment-audit.md) | JVM/Go 테스트 통과 |
| Gateway JVM 테스트와 monitor-bot Go 테스트를 CI에서 함께 실행하도록 검증 흐름을 자동화했습니다. | [검증 요약](../README.md#검증-요약) | [Test Results](./test.md), `.github/workflows/ci.yml` | 로컬 테스트 통과, CI 설정 확인 |
| JaCoCo와 Go coverage profile로 테스트 근거를 수치화하고, CloudWatch query builder의 회귀 테스트를 보강했습니다. | [검증 요약](../README.md#검증-요약) | [Test Results](./test.md), [Query Tuning](./query-tuning.md) | JVM line 91.32%, branch 65.17%, Go statements 59.2% |

## 바로 사용할 수 있는 문장

- MSA 환경에서 Gateway가 route, 인증/인가, 요청 정책, 공통 응답 계약을 적용하도록 Spring Cloud Gateway 기반 서버를 구성했습니다.
- Gateway structured log의 traceId와 error code를 CloudWatch Logs에 남기고, Discord Monitor Bot에서 trace drilldown과 alert routing으로 조회할 수 있게 했습니다.
- Discord Monitor Bot을 Gateway JVM과 분리된 Go HTTP Interactions sidecar로 구현해 운영 조회 기능을 본 서버 프로세스와 분리했습니다.
- general alert와 critical alert route를 분리하고, critical alert에만 configured role mention을 허용하는 운영 알림 정책을 구현했습니다.
- Assignment 현재 상태는 WEB Admin GET API로 조회하고, 변경 주체와 시각은 Report EVENT 로그를 source of truth로 삼도록 audit feed를 구성했습니다.
- Gateway JVM 테스트와 monitor-bot Go 테스트를 GitHub Actions CI에서 함께 실행하도록 구성했습니다.
- JaCoCo와 Go coverage profile을 생성해 JVM line coverage 91.32%, branch coverage 65.17%, monitor-bot Go statements coverage 59.2%를 로컬 리포트 기준으로 확인했습니다.

## 수치 검증 후 사용할 문장

- [ ] p95 latency N ms -> N ms
- [ ] alert delivery latency N ms
- [ ] duplicate alert count N회 -> N회
- [ ] alert detection time N분 -> N분
- [ ] CloudWatch query latency N ms -> N ms

## 아직 쓰면 안 되는 표현

- 커버리지 N% 달성
- 성능 N% 개선
- 장애 대응 시간 N% 단축
- 처리량 N배 개선
- 알림 누락률 N% 감소
- Redis 캐시로 성능 N% 개선

## 근거 링크

| 구분 | 위치 | 설명 |
| :--- | :--- | :--- |
| 코드 | `src/main/kotlin/com/aandi/gateway/security/GatewayRequestPolicyFilter.kt` | Gateway allowlist, HTTPS, Host, Content-Type 정책 |
| 코드 | `src/main/kotlin/com/aandi/gateway/common/response/GatewayResponse.kt` | 공통 응답과 Gateway error code enum |
| 코드 | `src/main/kotlin/com/aandi/gateway/logging/RequestResponseLoggingFilter.kt` | trace header 생성/재사용 |
| 코드 | `monitor-bot/internal/discord/commands.go` | `/ops` command family |
| 코드 | `monitor-bot/internal/monitor/alerts.go` | alert routing, role mention 제한, suppression |
| 코드 | `monitor-bot/internal/monitor/assignment_audit.go` | Report EVENT 기반 assignment audit feed |
| 테스트 | `src/test/kotlin/com/aandi/gateway/common/response/GatewayErrorCodeTests.kt` | 응답/에러 코드 계약 검증 |
| 테스트 | `monitor-bot/internal/monitor/alerts_test.go` | alert routing 검증 |
| 테스트 | `monitor-bot/internal/monitor/assignment_ops_test.go` | assignment issue lifecycle와 digest 검증 |
| 테스트 | `monitor-bot/internal/cloudwatch/queries_test.go` | CloudWatch query builder와 log group 제한 검증 |
| 테스트 | `monitor-bot/internal/security/validate_test.go` | 로그 검색어와 입력 validation 검증 |
| CI | `.github/workflows/ci.yml` | JVM test와 Go test 실행 |
| 문서 | [Test Results](./test.md) | 실제 테스트와 Go coverage 결과 |
| 문서 | [Performance Measurement](./performance-measurement.md) | 측정값 없음과 향후 지표 기준 |
| 문서 | [Query Tuning](./query-tuning.md) | query tuning 미적용 사유와 향후 측정 절차 |
