# A-AND-I-GATEWAY-SERVER

본 프로젝트는 A&I MSA 환경에서 외부 요청을 내부 서비스로 라우팅하고, 인증/인가/요청 정책/공통 응답 계약을 Gateway에서 일관되게 적용하기 위한 Spring Cloud Gateway 서버입니다. 운영 관찰가능성을 위해 Gateway JVM과 분리된 Go 기반 Discord Monitor Bot sidecar를 함께 배포하고, CloudWatch Logs와 read-only Admin API를 Discord 운영 명령으로 연결합니다.

## 핵심 역할

- **MSA Gateway**: Auth, Report, Blog, Online Judge 서비스 앞단에서 route allowlist, Host/HTTPS 정책, JSON Content-Type 정책, JWT role 정책을 적용합니다.
- **운영 관찰가능성**: Gateway structured log의 `traceId`, `requestId`, `error.code`, `http.latencyMs`를 CloudWatch Logs에 남기고 Discord에서 조회합니다.
- **Discord Monitor Bot**: Go HTTP Interactions sidecar로 운영 조회 명령, alert routing, assignment audit feed를 담당합니다.
- **응답 표준화**: Gateway에서 직접 반환하는 실패 응답은 `success/data/error/timestamp` 공통 구조와 5자리 error code 정책을 따릅니다.

## 기술 스택

| 영역 | 사용 기술 |
| :--- | :--- |
| Gateway | Kotlin, Java 21, Spring Boot 4, Spring Cloud Gateway WebFlux, Spring Security |
| Cache / Policy | Redis reactive, JWT validation, route allowlist, rate limit |
| Monitor Bot | Go 1.24, Discord HTTP Interactions, AWS SDK for CloudWatch Logs |
| CI/CD | GitHub Actions, Gradle, Go test, Docker, ECR, EC2, nginx |

## 주요 기능

### Gateway Routing

Gateway는 외부 API 경로를 Auth, Report, Blog, Online Judge 서비스로 라우팅하면서 인증이 필요한 경로와 공개 경로를 분리합니다. `GatewayRequestPolicyFilter`는 허용된 method/path만 통과시키고, HTTPS/Host/Content-Type 정책 위반은 Gateway 공통 에러 응답으로 차단합니다.

> **🎬 동작 화면**
> - GIF 위치: `docs/assets/gifs/gateway-routing-demo.gif`
> - 대체 이미지: `docs/assets/images/gateway-routing-result.png`
> - 촬영 범위: allowlisted route 통과, denylisted route의 `15001 ENDPOINT_NOT_ALLOWLISTED` 응답, trace header 확인

> **핵심 구현 포인트**
> - Spring Cloud Gateway route 정의: `src/main/resources/application.yaml`
> - 요청 정책 필터: `src/main/kotlin/com/aandi/gateway/security/GatewayRequestPolicyFilter.kt`
> - JWT role 기반 보안 정책: `src/main/kotlin/com/aandi/gateway/security/SecurityConfig.kt`

> 자세한 구조는 [Architecture](./docs/architecture.md)에서 확인할 수 있습니다.

### Discord Monitor Bot

Discord Monitor Bot은 Gateway JVM에 붙은 라이브러리가 아니라 별도 Go 컨테이너로 실행되는 HTTP Interactions sidecar입니다. 운영자는 Discord slash command로 서비스 상태, 로그, 과제 상태, audit 이벤트를 조회하지만, bot은 과제 생성/수정/삭제/공개 같은 write command를 제공하지 않습니다.

> **🎬 동작 화면**
> - GIF 위치: `docs/assets/gifs/discord-dashboard-demo.gif`
> - 대체 이미지: `docs/assets/images/discord-command-mock.png`
> - 촬영 범위: `/ops dashboard`, `/ops logs`, `/ops assignment`, `/ops help` 응답과 민감정보 마스킹

> **핵심 구현 포인트**
> - Discord command schema: `monitor-bot/internal/discord/commands.go`
> - Interactions handler와 signature 검증: `monitor-bot/internal/discord/interactions.go`
> - 운영자 가이드: [monitor-bot/README.md](./monitor-bot/README.md), [Discord Monitor Bot](./docs/discord-monitor-bot.md)
> - 운영 계약: bot never creates/updates/deletes/publishes assignments, assignment audit source는 Report V2 EVENT logs, CRITICAL server alerts만 critical route와 role mention을 사용

> 자세한 흐름은 [Discord Monitor Bot](./docs/discord-monitor-bot.md)에서 확인할 수 있습니다.

### Critical / General Alert Routing

Monitor Bot은 structured V2 log의 severity와 error code를 기준으로 일반 운영 알림과 critical 장애 알림을 분리합니다. 일반 알림은 general route로 보내고, `CRITICAL` 또는 P0 계열 알림만 critical route와 configured role mention을 사용할 수 있습니다.

> **🎬 동작 화면**
> - GIF 위치: `docs/assets/gifs/critical-alert-demo.gif`
> - 대체 이미지: `docs/assets/images/critical-alert-result.png`
> - 촬영 범위: `/ops alert action:channel target:general`, `target:critical`, `action:role`, critical alert fallback command

> **핵심 구현 포인트**
> - alert 수집/라우팅: `monitor-bot/internal/monitor/alerts.go`
> - V2 log severity 판단: `monitor-bot/internal/opslog/v2.go`
> - `@everyone`, `@here` role mention 차단

> 자세한 흐름은 [Ops Alert Flow](./docs/api-flows/ops-alert.md)에서 확인할 수 있습니다.

### Trace Drilldown

Gateway는 요청마다 trace header를 재사용하거나 새로 생성해 downstream 요청과 응답 로그에 연결합니다. Monitor Bot은 alert에 유효한 traceId가 있으면 `Trace 상세` 버튼과 `/ops logs mode:trace query:<traceId>` fallback 명령을 함께 제공합니다.

> **🎬 동작 화면**
> - GIF 위치: `docs/assets/gifs/trace-drilldown-demo.gif`
> - 대체 이미지: `docs/assets/images/trace-drilldown-example.png`
> - 촬영 범위: alert 메시지의 traceId, `Trace 상세` 버튼, trace query 결과

> **핵심 구현 포인트**
> - trace header 전파: `src/main/kotlin/com/aandi/gateway/logging/RequestResponseLoggingFilter.kt`
> - CloudWatch trace query: `monitor-bot/internal/cloudwatch/queries.go`
> - 버튼 fallback 처리: `monitor-bot/internal/discord/interactions.go`

> 자세한 흐름은 [Trace Drilldown](./docs/api-flows/trace-drilldown.md)에서 확인할 수 있습니다.

### Assignment Audit Feed

Assignment audit feed는 현재 상태 조회와 변경 주체 증명을 분리합니다. 현재 상태와 진단은 WEB Admin GET API snapshot을 사용하고, 누가 언제 과제를 생성/수정/삭제/공개/비공개했는지는 Report V2 `EVENT` 로그를 source of truth로 사용합니다.

> **🎬 동작 화면**
> - GIF 위치: `docs/assets/gifs/assignment-audit-demo.gif`
> - 대체 이미지: `docs/assets/images/assignment-audit-result.png`
> - 촬영 범위: `/ops assignment view:events`, Report EVENT audit feed, trace/requestId 표시

> **핵심 구현 포인트**
> - read-only Admin GET client: `monitor-bot/internal/reportadmin/client.go`
> - audit event parser: `monitor-bot/internal/monitor/assignment_audit.go`
> - assignment issue lifecycle와 digest: `monitor-bot/internal/monitor/assignment_ops.go`

> 자세한 흐름은 [Assignment Audit Flow](./docs/api-flows/assignment-audit.md)에서 확인할 수 있습니다.

### Common Response / Error Code Policy

Gateway에서 직접 반환하는 실패 응답은 `success=false`, `data=null`, `error`, `timestamp` 구조를 유지합니다. 에러 코드는 `[서비스 1자리][분류 1자리][상세 3자리]` 형식의 5자리 정수이며, Gateway 직접 발행 코드는 `GatewayErrorCode` enum에서 관리합니다.

> **🎬 동작 화면**
> - GIF 위치: `docs/assets/gifs/error-response-demo.gif`
> - 대체 이미지: `docs/assets/images/error-code-policy.png`
> - 촬영 범위: 인증 실패, 권한 부족, allowlist 차단, JSON Content-Type 위반 응답

> **핵심 구현 포인트**
> - 응답 모델과 error code enum: `src/main/kotlin/com/aandi/gateway/common/response/GatewayResponse.kt`
> - 응답 writer: `src/main/kotlin/com/aandi/gateway/common/response/GatewayResponseWriter.kt`
> - 계약 테스트: `src/test/kotlin/com/aandi/gateway/common/response/GatewayErrorCodeTests.kt`

> 자세한 정책은 [Error Response Policy](./docs/error-response-policy.md)에서 확인할 수 있습니다.

## 문서

- [문서 목차](./docs/README.md)
- [Architecture](./docs/architecture.md)
- [Observability](./docs/observability.md)
- [Discord Monitor Bot](./docs/discord-monitor-bot.md)
- [Error Response Policy](./docs/error-response-policy.md)
- [Test Results](./docs/test.md)
- [Resume Evidence](./docs/resume-evidence.md)
- [Demo Capture](./docs/demo-capture.md)

## 검증 요약

상세 실행 로그와 coverage 출력은 [Test Results](./docs/test.md)에 정리했습니다.

- `./gradlew clean test`: 통과
- `cd monitor-bot && go test ./...`: 통과
- `cd monitor-bot && go test ./... -cover`: 통과

## 이력서 연결 포인트

- MSA Gateway에서 route, 인증/인가, 요청 정책, 공통 응답 계약을 한 곳에서 적용했습니다.
- Gateway JVM과 분리된 Go sidecar로 Discord 운영 도구를 구성해 운영 조회 기능을 본 서버 프로세스와 분리했습니다.
- CloudWatch Logs, traceId, critical/general alert routing, assignment audit feed를 연결해 장애와 운영 이벤트를 Discord에서 추적할 수 있게 했습니다.

근거별 이력서 문장은 [Resume Evidence](./docs/resume-evidence.md)에서 확인할 수 있습니다.
