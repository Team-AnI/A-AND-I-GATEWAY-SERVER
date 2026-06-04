# Test Results

> 메인 README로 돌아가기: [README](../README.md)

테스트 기준일은 2026-06-04 KST입니다. 아래 결과는 로컬 저장소 `/Users/dh/Desktop/Code/A-AND-I-GATEWAY-SERVER`에서 직접 실행한 명령 기준입니다.

## 실행 결과

| 명령 | 결과 | 메모 |
| :--- | :--- | :--- |
| `./gradlew clean test` | PASS | `BUILD SUCCESSFUL in 16s` |
| `./gradlew jacocoTestReport` | FAIL -> PASS | JaCoCo 설정 전에는 task 없음, 설정 후 XML/HTML 리포트 생성 |
| `./gradlew clean test jacocoTestReport` | PASS | `BUILD SUCCESSFUL` |
| `./gradlew jacocoTestCoverageVerification` | PASS | coverage threshold rule은 아직 설정하지 않음 |
| `cd monitor-bot && go test ./...` | PASS | 11개 Go package 테스트 통과 |
| `cd monitor-bot && go test ./... -cover` | PASS | package별 statements coverage 출력 |
| `cd monitor-bot && go test ./... -coverprofile=coverage.out` | PASS | 전체 coverage profile 생성 |
| `cd monitor-bot && go tool cover -func=coverage.out` | PASS | 전체 statements coverage `59.2%` |
| `INTERNAL_EVENT_TOKEN=local-test-token SERVER_PORT=18080 MANAGEMENT_SERVER_PORT=19090 ./gradlew bootRun` | PASS | Gateway `18080`, management `19090` 로컬 기동 |
| `curl -sS -i http://localhost:19090/actuator/health` | PASS | `HTTP/1.1 200 OK`, `status":"UP"` |
| `curl -sS -i http://localhost:18080/not-allowlisted` | PASS | `15001 ENDPOINT_NOT_ALLOWLISTED` 공통 실패 응답 확인 |

## JVM Test

`./gradlew clean test`는 Gateway Kotlin/Spring 테스트를 실행했습니다.

검증된 주요 영역:

- Gateway application context
- 공통 응답과 5자리 Gateway error code 계약
- structured API logging과 trace header 전파
- JWT bearer token converter
- actuator health public policy
- Spring Security role policy
- token context cache header filter

실행 중 Jackson API deprecation warning과 JVM dynamic agent warning이 출력됐지만 테스트 실패는 발생하지 않았습니다.

## JVM Coverage

기존에는 `jacocoTestReport` task가 없어 JVM coverage를 확인할 수 없었습니다. 이번 단계에서 coverage threshold 없이 JaCoCo 리포트 생성만 추가했습니다.

리포트 위치:

- `build/reports/jacoco/test/jacocoTestReport.xml`
- `build/reports/jacoco/test/html/index.html`

`jacocoTestReport.xml` 기준 결과:

| Metric | Covered / Total | Coverage |
| :--- | :--- | ---: |
| Instruction | 12,796 / 14,281 | 89.60% |
| Branch | 421 / 646 | 65.17% |
| Line | 1,526 / 1,671 | 91.32% |

이 수치는 로컬 테스트 리포트 기준이며, coverage gate는 추가하지 않았습니다.

## Go Test

`cd monitor-bot && go test ./...`는 다음 package를 통과했습니다.

- `cmd/monitor-bot`
- `internal/cloudwatch`
- `internal/config`
- `internal/discord`
- `internal/formatting`
- `internal/health`
- `internal/monitor`
- `internal/opslog`
- `internal/reportadmin`
- `internal/security`
- `internal/state`

## Go Coverage

`go test ./... -coverprofile=coverage.out`와 `go tool cover -func=coverage.out` 기준 전체 statements coverage는 `59.2%`입니다.

이번 단계 전 baseline:

- 전체 statements coverage: `58.7%`
- `internal/cloudwatch`: `50.2%`
- `internal/security`: `72.9%`

이번 단계 후:

| Package | Coverage |
| :--- | ---: |
| `cmd/monitor-bot` | 19.8% |
| `internal/cloudwatch` | 58.6% |
| `internal/config` | 53.8% |
| `internal/discord` | 34.2% |
| `internal/formatting` | 72.1% |
| `internal/health` | 64.5% |
| `internal/monitor` | 63.7% |
| `internal/opslog` | 80.9% |
| `internal/reportadmin` | 75.7% |
| `internal/security` | 78.1% |
| `internal/state` | 67.9% |
| Total statements | 59.2% |

## 추가한 테스트

coverage를 위한 무의미한 getter/setter 테스트는 추가하지 않았습니다. 운영 로그 조회 안전성과 input validation을 직접 검증하는 테스트만 추가했습니다.

| 파일 | 테스트 | 검증 내용 |
| :--- | :--- | :--- |
| `monitor-bot/internal/cloudwatch/queries_test.go` | `TestLogGroupsForOptionalServiceUsesConfiguredOrderAndLimit` | all-service CloudWatch query에서 log group 순서와 최대 개수 제한 |
| `monitor-bot/internal/cloudwatch/queries_test.go` | `TestLogGroupsForOptionalServiceDelegatesExplicitService` | `blog` alias가 `post` log group으로 정규화되는지 |
| `monitor-bot/internal/cloudwatch/queries_test.go` | `TestBuildLastLogQueryUsesValidatedServiceFilter` | last-log query가 검증된 service filter와 `limit 1`을 사용하는지 |
| `monitor-bot/internal/cloudwatch/queries_test.go` | `TestTimeRangeUsesRequestedLookback` | CloudWatch query time range가 요청한 lookback 범위를 유지하는지 |
| `monitor-bot/internal/security/validate_test.go` | `TestValidateLogSearchQuery` | 로그 검색어가 허용 문자만 통과하고 injection성 입력을 거부하는지 |
| `monitor-bot/internal/security/validate_test.go` | `TestParsePositiveInt` | 양수 정수만 파싱하고 빈 값/0/음수/문자는 fallback하는지 |

## CI 검증

`.github/workflows/ci.yml`에서 확인된 자동 검증:

- JVM test: `./gradlew test`
- Go test: `cd monitor-bot && go test ./...`
- JVM build: `./gradlew build -x test`
- 실패 시 artifact: `build/reports/tests/test/`

`.github/workflows/cd.yml`에서도 배포 전에 JVM test와 monitor-bot Go test를 실행합니다.

CI는 아직 JaCoCo/Go coverage 리포트를 artifact로 업로드하지 않습니다. 이번 단계에서는 리포트 생성 방법과 로컬 수치를 문서화했고, gate는 추가하지 않았습니다.

## Local Smoke Check

운영 비밀값 없이 `INTERNAL_EVENT_TOKEN`만 로컬 더미값으로 설정해 Gateway를 실행했습니다. management health는 `UP`을 반환했고, allowlist에 없는 경로는 `success=false`, `error.code=15001`, `value=ENDPOINT_NOT_ALLOWLISTED` 구조로 응답했습니다.

## 실행 중 발견해 수정한 항목

`monitor-bot/internal/monitor/assignment_ops_test.go`의 `TestAssignmentIssueNotificationsAreBatchedWithSuppressedCount`는 과거 고정 날짜를 테스트 데이터로 사용해 현재 날짜가 지나면 stale draft로 분류될 수 있었습니다. 테스트 의도는 publish delayed digest와 suppression count 검증이므로, 해당 테스트 데이터만 현재 시각 기준 상대 날짜로 바꿔 날짜 의존성을 제거했습니다.

## 부족한 테스트 영역

- CI는 JVM/Go coverage를 실행하거나 artifact로 업로드하지 않습니다.
- `cmd/monitor-bot`와 `internal/discord`는 실제 process lifecycle, Discord interaction command dispatch, follow-up 흐름이 많아 여전히 낮은 coverage입니다.
- Discord 실제 API, CloudWatch 실제 query, 운영 nginx 프록시 동작은 unit test가 아니라 배포 환경 검증이 필요합니다.
- alert delivery latency, duplicate alert count, command response time은 테스트 결과가 아니라 운영 측정 지표가 필요합니다.
