# Gateway Observability Metrics

> 운영 CloudWatch, Discord, 운영 URL을 호출하지 않고 로컬 테스트와 mock/fake 기반으로 검증한 수치입니다.

## Safety

| 항목 | 값 |
| :--- | :--- |
| actualAwsCloudWatchAccess | `false` |
| actualDiscordApiAccess | `false` |
| productionUrlAccess | `false` |
| productionSecretStored | `false` |

## Metrics

| 영역 | 테스트 수 | 근거 파일 |
| :--- | ---: | :--- |
| traceId/requestId tests | 2 | `src/test/kotlin/com/aandi/gateway/observability/TraceIdRequestIdTest.kt` (2) |
| Gateway error contract tests | 7 | `src/test/kotlin/com/aandi/gateway/errorcontract/GatewayErrorContractTest.kt` (7) |
| structured log redaction tests | 2 | `src/test/kotlin/com/aandi/gateway/logging/StructuredLogRedactionTest.kt` (2) |
| existing Gateway logging contract tests | 19 | `src/test/kotlin/com/aandi/gateway/logging/ApiLoggingContractTests.kt` (19) |
| Monitor Bot mock tests | 93 | `monitor-bot/internal/discord/observability_handlers_test.go` (4)<br>`monitor-bot/internal/discord/interactions_test.go` (9)<br>`monitor-bot/internal/discord/commands_test.go` (23)<br>`monitor-bot/internal/cloudwatch/queries_test.go` (20)<br>`monitor-bot/internal/monitor/alerts_test.go` (23)<br>`monitor-bot/internal/monitor/dashboard_test.go` (14) |

## Summary

- traceId/requestId 테스트 수: `2`
- 오류 계약 테스트 수: `7`
- 로그 redaction 테스트 수: `2`
- Gateway 관측 가능성 관련 테스트 수: `30`
- Monitor Bot mock 테스트 수: `93`
- 실제 AWS/Discord 접근 여부: `false`
- 운영 URL 접근 여부: `false`

## Verified Coverage

- Gateway 응답과 downstream 전달 header의 `X-Trace-Id`, `X-Request-Id`를 로컬 mock downstream으로 검증합니다.
- 구조화 로그의 `path`, `method`, `status`, `latencyMs`, `traceId`, `requestId`와 민감정보 redaction을 captured log output으로 검증합니다.
- 401, 403, 404, 415, 429, 502, 500 오류 응답 envelope와 trace/request header를 검증합니다.
- Monitor Bot dashboard/logs/alert/help handler는 fake CloudWatch, fake Ops controller, local httptest health server로 검증합니다.
- 기존 Monitor Bot alert 테스트가 critical 반복 이벤트의 cooldown과 role mention 중복 억제를 검증합니다.

## Resume Sentence Candidates

- traceId/requestId 기반 구조화 로그와 공통 오류 응답 계약을 테스트로 검증해 요청 추적성과 장애 분석 흐름을 관리
- Mock CloudWatch/Discord 기반 Monitor Bot 테스트로 장애 알림 cooldown과 중복 mention 억제 동작을 검증
