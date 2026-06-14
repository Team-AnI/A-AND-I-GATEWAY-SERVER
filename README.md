# A-AND-I-GATEWAY-SERVER

## 1. 프로젝트 요약

A&I 서비스의 외부 API 진입점을 단일화하는 Kotlin/Spring Cloud Gateway 서버입니다. Gateway는 Auth, Report, Blog, Online Judge 앞단에서 라우팅, 인증/인가, 요청 정책, 공통 실패 응답을 먼저 처리합니다.

운영 관찰가능성을 위해 Gateway JVM과 분리된 Go 기반 Discord Monitor Bot도 함께 둡니다. Monitor Bot은 Discord HTTP Interactions sidecar로 동작하며 CloudWatch Logs, 서비스 health, assignment audit 이벤트를 `/ops` 명령으로 조회합니다.

핵심 목표는 두 가지입니다.

- 클라이언트가 여러 downstream API를 직접 신경 쓰지 않도록 Gateway에서 공통 정책을 적용합니다.
- 운영자가 Discord에서 장애 신호와 traceId를 빠르게 확인하고 CloudWatch Logs로 원인 후보를 좁힐 수 있게 합니다.

## 2. 왜 만들었나

A&I 서비스가 여러 서버로 나뉘면서 클라이언트 진입점, 인증 정책, 실패 응답 형식, 운영 로그 확인 방식이 흩어질 수 있었습니다. 이 저장소는 그 문제를 Gateway와 Monitor Bot으로 나누어 해결합니다.

- Gateway는 route allowlist, Host/HTTPS 정책, JSON Content-Type 정책, JWT role 정책을 일관되게 적용합니다.
- Gateway가 직접 반환하는 오류는 `success/data/error/timestamp` 구조와 5자리 error code를 따릅니다.
- Monitor Bot은 운영자가 Discord에서 dashboard, logs, alert, assignment, help 흐름을 조회할 수 있게 합니다.
- assignment 변경 주체와 시점은 WEB Admin snapshot에서 추측하지 않고 Report V2 EVENT 로그를 source of truth로 사용합니다.

## 3. 주요 구현 범위와 기여 영역

이 README는 공유 레포의 구현 범위를 기준으로 Gateway 공통 정책과 운영 조회 흐름을 설명합니다.

- Spring Cloud Gateway route와 보안 정책으로 허용되지 않은 method/path를 downstream 전달 전에 차단합니다.
- Gateway 실패 응답은 공통 응답 모델과 `GatewayErrorCode` enum으로 관리합니다.
- 요청별 trace header를 구조화 로그에 연결해 CloudWatch Logs 조회 기준으로 사용합니다.
- Discord Monitor Bot은 Gateway JVM과 분리된 Go sidecar로 동작하며, `/ops` command family 중심의 운영 UX를 제공합니다.
- general/critical alert route, role mention 제한, trace drilldown 버튼, assignment audit feed는 read-only 운영 정책 안에서 동작합니다.

## 4. 한눈에 보는 구조

![Gateway Architecture](./docs/assets/diagrams/gateway-architecture.png)

```text
Client
  |
  v
Spring Cloud Gateway
  |-- Auth Service
  |-- Report Service
  |-- Blog/Post Service
  |-- Online Judge Service
  |
  `-- structured logs with traceId/requestId
        |
        v
   CloudWatch Logs <---- Go Discord Monitor Bot <---- Discord /ops commands
```

| 구성요소 | 역할 |
| :--- | :--- |
| Spring Cloud Gateway | 외부 요청 라우팅, 요청 정책 검사, 공통 실패 응답 반환 |
| Downstream APIs | Auth, Report, Blog/Post, Online Judge 도메인 기능 처리 |
| Redis | 인증 컨텍스트 캐시와 인증 요청 rate limit 기준 저장 |
| CloudWatch Logs | Gateway와 downstream의 structured log 조회 대상 |
| Discord Monitor Bot | 운영 dashboard, log query, alert routing, assignment audit 조회 |
| Discord Channel | general 운영 알림과 critical 장애 알림 확인 |

라우팅 설정은 [application.yaml](./src/main/resources/application.yaml)에 정의되어 있고, 서비스 연동 원칙은 [SERVICE_GATEWAY_INTEGRATION.md](./docs/SERVICE_GATEWAY_INTEGRATION.md)에 정리되어 있습니다.

## 5. 핵심 기능과 동작 증거

### 5.1 Gateway Routing and Policy

Gateway는 등록된 method/path만 통과시키고, 정책 위반 요청은 downstream으로 전달하지 않습니다. Host/HTTPS/JSON Content-Type 정책도 Gateway 단계에서 검사합니다.

동작 근거:

- 구현: [GatewayRequestPolicyFilter.kt](./src/main/kotlin/com/aandi/gateway/security/GatewayRequestPolicyFilter.kt)
- 보안 정책: [SecurityConfig.kt](./src/main/kotlin/com/aandi/gateway/security/SecurityConfig.kt)
- 테스트: [SecurityConfigTests.kt](./src/test/kotlin/com/aandi/gateway/security/SecurityConfigTests.kt)

### 5.2 Common Response and Error Code

Gateway가 직접 반환하는 실패 응답은 공통 응답 구조를 유지합니다. error code는 5자리 정수로 관리하고, Gateway 직접 발행 코드는 enum에 모읍니다.

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": 15001,
    "message": "요청 메서드와 경로가 게이트웨이 허용 목록에 없습니다.",
    "value": "ENDPOINT_NOT_ALLOWLISTED",
    "alert": "요청한 기능을 찾을 수 없어요."
  },
  "timestamp": "2026-06-04T17:08:18.537129+09:00"
}
```

동작 근거:

- 구현: [GatewayResponse.kt](./src/main/kotlin/com/aandi/gateway/common/response/GatewayResponse.kt)
- 정책 문서: [GATEWAY_ERROR_CODES.md](./docs/GATEWAY_ERROR_CODES.md)
- 테스트: [GatewayErrorCodeTests.kt](./src/test/kotlin/com/aandi/gateway/common/response/GatewayErrorCodeTests.kt)

### 5.3 Trace Logging

Gateway는 요청마다 traceId와 requestId를 재사용하거나 생성해 downstream 요청과 structured log에 연결합니다. 이 값은 Monitor Bot의 trace drilldown과 CloudWatch Logs 조회 기준이 됩니다.

![Logging and Alert Pipeline](./docs/assets/diagrams/log-to-alert-pipeline.png)

동작 근거:

- 구현: [RequestResponseLoggingFilter.kt](./src/main/kotlin/com/aandi/gateway/logging/RequestResponseLoggingFilter.kt)
- 테스트: [ApiLoggingContractTests.kt](./src/test/kotlin/com/aandi/gateway/logging/ApiLoggingContractTests.kt)
- CloudWatch query: [queries.go](./monitor-bot/internal/cloudwatch/queries.go)

### 5.4 Discord Monitor Bot

Discord Monitor Bot은 별도 Go 컨테이너로 실행되는 read-only 운영 도구입니다. 기본 UX는 `/ops` 아래 5개 command family로 제한합니다.

![Discord Command Map](./docs/assets/diagrams/discord-command-map.png)

- 운영 계약: bot never creates/updates/deletes/publishes assignments.
- `/ops dashboard`: 서비스 상태와 dashboard watch
- `/ops logs`: 오류, critical, slow, security, trace, assignment EVENT 조회
- `/ops alert`: general/critical 채널, role mention, on/off/status/test 설정
- `/ops assignment`: assignment 상태, 진단, audit 이벤트 조회
- `/ops help`: 운영 명령 도움말

동작 근거:

- command schema: [commands.go](./monitor-bot/internal/discord/commands.go)
- interaction signature 검증: [signature.go](./monitor-bot/internal/discord/signature.go)
- button fallback 처리: [interactions.go](./monitor-bot/internal/discord/interactions.go)
- 운영 문서: [monitor-bot/README.md](./monitor-bot/README.md), [discord-monitor-bot.md](./docs/discord-monitor-bot.md)

### 5.5 Alert Routing and Assignment Audit

Monitor Bot은 structured V2 log의 severity와 error code를 기준으로 general alert와 critical alert를 나눕니다. CRITICAL server alerts만 critical route와 configured role mention을 사용할 수 있고, assignment audit 이벤트는 general route로만 보냅니다.

아래 이미지는 실제 운영 로그가 아니라 README 설명용 mock 이미지입니다.

![Discord Critical Alert Mock](./docs/assets/images/discord-critical-alert.png)

assignment audit feed는 Report V2 EVENT logs를 기준으로 `ASSIGNMENT_CREATED`, `ASSIGNMENT_UPDATED`, `ASSIGNMENT_DELETED`, `ASSIGNMENT_PUBLISHED`, `ASSIGNMENT_UNPUBLISHED` 이벤트를 조회합니다.

동작 근거:

- alert routing: [alerts.go](./monitor-bot/internal/monitor/alerts.go)
- assignment audit: [assignment_audit.go](./monitor-bot/internal/monitor/assignment_audit.go)
- alert 테스트: [alerts_test.go](./monitor-bot/internal/monitor/alerts_test.go)
- audit 테스트: [assignment_audit_test.go](./monitor-bot/internal/monitor/assignment_audit_test.go)

## 6. 기술적 고민과 해결

| 고민 | 해결 |
| :--- | :--- |
| 서비스별 라우팅과 인증 정책이 흩어지는 문제 | Gateway에서 route allowlist와 role 정책을 먼저 적용하고, 서비스는 도메인 상세 인가에 집중하게 했습니다. |
| downstream 장애와 Gateway 오류를 같은 방식으로 추적하기 어려운 문제 | traceId/requestId를 structured log에 남기고 Monitor Bot의 trace query로 연결했습니다. |
| 운영 알림이 한 채널에 섞이는 문제 | general/critical route를 분리하고 CRITICAL server alert에만 role mention을 허용했습니다. |
| Discord 운영 도구가 본 서버 프로세스와 강하게 결합될 위험 | Monitor Bot을 Go HTTP Interactions sidecar로 분리했습니다. |
| assignment 변경 주체를 현재 snapshot에서 추측할 위험 | Report V2 EVENT 로그를 audit source로 사용하고 WEB Admin snapshot은 현재 상태 조회에만 사용했습니다. |
| 운영 출력에 민감정보가 섞일 위험 | Discord 출력 필드를 allowlist로 제한하고 민감 필드는 sanitize 처리했습니다. |

## 7. 운영 보안 기준

- Gateway 뒤의 서비스는 외부에 직접 노출하지 않고 Gateway를 통해 접근합니다.
- Gateway는 인증된 사용자 식별 정보를 downstream용 헤더로 다시 구성하며, 클라이언트가 보낸 동일 목적의 헤더를 그대로 신뢰하지 않습니다.
- Monitor Bot은 assignment 생성/수정/삭제/공개/비공개 command를 제공하지 않습니다.
- Discord 출력에는 인증값, 요청 본문, 전체 응답 데이터, 비공개 사용자 데이터가 노출되지 않도록 제한합니다.
- `@everyone`, `@here` 같은 전체 멘션은 허용하지 않고, configured role mention은 CRITICAL server alert에서만 사용합니다.
- 운영 환경 파일은 git에 커밋하지 않으며, 환경변수 예시는 [.env.example](./.env.example)만 참고합니다.

관련 구현:

- 출력 sanitize: [sanitize.go](./monitor-bot/internal/security/sanitize.go)
- sanitize 테스트: [sanitize_test.go](./monitor-bot/internal/security/sanitize_test.go)
- 운영 환경 가이드: [ENV_SETUP.md](./docs/ENV_SETUP.md)

## 8. 테스트와 검증

이 저장소의 기본 검증 명령은 다음과 같습니다.

```bash
./gradlew test
cd monitor-bot && go test ./...
```

검증 범위:

- Gateway route/security policy 테스트
- Gateway 공통 응답과 error code 계약 테스트
- trace logging 계약 테스트
- Discord command schema와 interaction 처리 테스트
- CloudWatch query builder 입력 검증 테스트
- alert routing, role mention 제한, assignment audit feed 테스트

CI 기준은 [ci.yml](./.github/workflows/ci.yml)에 정의되어 있습니다.

## 9. 실행 방법

Gateway 로컬 실행:

```bash
./gradlew bootRun
```

Gateway와 Redis를 Docker Compose로 실행:

```bash
docker compose up -d redis gateway
```

health 확인:

```bash
curl -i http://localhost:8080/actuator/health
```

Monitor Bot 테스트:

```bash
cd monitor-bot
go test ./...
```

실제 Discord 연동 실행은 필요한 환경변수를 설정한 뒤 진행합니다. 자세한 값과 운영 주의사항은 [monitor-bot/README.md](./monitor-bot/README.md)를 기준으로 확인합니다.

## 10. 기술 스택

| 영역 | 사용 기술 |
| :--- | :--- |
| Gateway | Kotlin 2.2, Java 21, Spring Boot 4, Spring Cloud Gateway WebFlux |
| Security | Spring Security, OAuth2 Resource Server, JWT role policy |
| Reactive / Cache | WebFlux, Reactor, Redis reactive |
| Observability | structured logging, traceId/requestId, CloudWatch Logs |
| Monitor Bot | Go 1.24, Discord HTTP Interactions, AWS SDK for CloudWatch Logs |
| Infra / CI | Docker, Docker Compose, nginx, GitHub Actions |

버전 근거:

- Gateway build: [build.gradle.kts](./build.gradle.kts)
- Monitor Bot module: [go.mod](./monitor-bot/go.mod)

## 11. 관련 문서

- [Discord Monitor Bot Operator Guide](./docs/discord-monitor-bot.md)
- [Monitor Bot README](./monitor-bot/README.md)
- [Gateway Error Contract Mapping](./docs/GATEWAY_ERROR_CODES.md)
- [Service Gateway Integration Guide](./docs/SERVICE_GATEWAY_INTEGRATION.md)
- [EC2 Environment Setup](./docs/ENV_SETUP.md)
- [Report Copy Routing](./docs/report-copy-routing.md)
- [Log Retention](./docs/log-retention.md)
