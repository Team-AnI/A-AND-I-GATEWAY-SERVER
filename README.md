# A&I Gateway Server

> A&I 서비스의 외부 API 진입점이자, Gateway 로그와 운영 이벤트를 Discord Monitor Bot으로 조회할 수 있게 만든 운영 관측 서버입니다.

## 1. 프로젝트 요약

| 항목 | 내용 |
| :--- | :--- |
| 프로젝트 성격 | A&I 서비스의 외부 API 진입점과 운영 관측 도구 |
| 핵심 역할 | 라우팅, 인증/인가, 공통 실패 응답, trace logging, Discord 운영 조회 |
| 주요 기술 | Kotlin, Spring Cloud Gateway WebFlux, Redis Reactive, Go, CloudWatch Logs |
| 운영 도구 | Discord HTTP Interactions 기반 read-only Monitor Bot |
| 검증 | `./gradlew test`, `cd monitor-bot && go test ./...` |

Gateway는 Auth, User, Report WEB, Blog/Post, Online Judge 앞단에서 요청 정책을 먼저 적용합니다. Discord Monitor Bot은 Gateway JVM과 분리된 Go sidecar로 동작하며, 운영자가 Discord에서 로그와 운영 이벤트를 read-only로 조회할 수 있게 합니다.

## 2. 왜 만들었나

A&I 서비스는 Auth, User, Report WEB, Blog/Post, Online Judge처럼 여러 서버로 나뉘어 있습니다. 클라이언트가 각 서버의 주소와 응답 규약을 직접 신경 쓰면 API 사용 흐름이 복잡해지고, 장애가 발생했을 때 어느 서버의 문제인지 추적하기도 어렵습니다.

Gateway는 외부 요청의 진입점을 하나로 모으고, 인증/인가, allowlist, rate limit, 공통 실패 응답, 구조화 로그를 담당합니다.

여기에 Discord Monitor Bot을 분리된 Go sidecar로 두어 운영자가 서버에 직접 접속하지 않아도 CloudWatch Logs와 WEB Admin GET API를 read-only로 조회할 수 있게 했습니다. 핵심은 로그 문자열을 사람이 뒤지는 방식이 아니라, `traceId`, `errorCode`, `statusCode`, `latencyMs`, `service.domainCode` 같은 구조화 필드로 문제를 추적하는 것입니다.

## 3. 내 역할과 핵심 기여

이 README는 기능을 과장하지 않고, 실제 코드와 운영 문서로 확인할 수 있는 Gateway와 Discord Monitor Bot의 구현 범위를 중심으로 정리합니다.

- Gateway 앞단에서 route allowlist, Host/HTTPS 정책, JSON Content-Type 정책, JWT role 정책을 적용하는 흐름을 문서화했습니다.
- Gateway 실패 응답을 공통 응답 모델과 5자리 error code 기준으로 설명했습니다.
- traceId/requestId 기반 구조화 로그가 CloudWatch Logs와 Discord Monitor Bot 조회로 이어지는 흐름을 정리했습니다.
- Discord Monitor Bot이 Gateway JVM과 분리된 Go sidecar이며, `/ops` command family 중심으로 동작한다는 운영 모델을 설명했습니다.
- general/critical alert route, role mention 제한, trace drilldown, assignment audit feed가 read-only guard 안에서 동작한다는 기준을 명확히 했습니다.

## 4. 한눈에 보는 구조

![Gateway Architecture](./docs/assets/diagrams/gateway-architecture.png)

| 구성요소 | 역할 |
| :--- | :--- |
| Client/Admin | Web, Swagger UI를 통해 Gateway에 요청 |
| Nginx/HTTPS | TLS 종료와 `/discord/interactions` 프록시 처리 |
| A&I Gateway Server | 라우팅, 인증/인가, rate limit, allowlist, 공통 응답, 구조화 로그 처리 |
| Redis | rate limit과 운영 상태 관리에 필요한 상태 저장 |
| CloudWatch Logs | v2 structured log 조회 대상 |
| Discord Monitor Bot | Discord HTTP Interactions 기반 read-only 운영 조회 |
| Discord Channels | 일반 알림과 critical 알림 라우팅 |

라우팅 설정은 [application.yaml](./src/main/resources/application.yaml)에 정의되어 있고, 서비스 연동 원칙은 [SERVICE_GATEWAY_INTEGRATION.md](./docs/SERVICE_GATEWAY_INTEGRATION.md)에 정리되어 있습니다.

## 5. 핵심 기능과 동작 증거

### 5.1 Gateway Routing & Request Policy

Gateway는 외부 요청을 Auth, User, Report WEB, Blog/Post, Online Judge 등 downstream service로 라우팅합니다. 라우팅 전후로 인증/인가, allowlist, rate limit, 공통 실패 응답, 요청/응답 로그를 처리합니다.

대표 구현 근거:

| 파일 | 역할 |
| :--- | :--- |
| [GatewayRequestPolicyFilter.kt](./src/main/kotlin/com/aandi/gateway/security/GatewayRequestPolicyFilter.kt) | Gateway 요청 정책 검사 |
| [SecurityConfig.kt](./src/main/kotlin/com/aandi/gateway/security/SecurityConfig.kt) | OAuth2 Resource Server와 보안 설정 |
| [GatewayResponse.kt](./src/main/kotlin/com/aandi/gateway/common/response/GatewayResponse.kt) | 공통 응답 규약 |
| [RequestResponseLoggingFilter.kt](./src/main/kotlin/com/aandi/gateway/logging/RequestResponseLoggingFilter.kt) | 요청/응답 구조화 로그 |

### 5.2 Structured Logging

![Logging and Alert Pipeline](./docs/assets/diagrams/log-to-alert-pipeline.png)

v2 API 로그는 단순 message 문자열이 아니라 `logType`, `service.domainCode`, `http.statusCode`, `response.error.code`, `trace.traceId`, `latencyMs` 같은 구조화 필드를 기준으로 남깁니다. Discord Monitor Bot은 이 필드를 기준으로 오류, critical, slow, security, event 로그를 조회합니다.

현재 Monitor Bot의 V2 로그 자동 조회/알림 대상은 gateway, auth, report/web, blog/post입니다. Online Judge는 Gateway 라우팅 대상에 포함되지만, V2 로그 연동이 확인되기 전까지 자동 알림 대상으로 설명하지 않습니다.

관련 근거:

- 로그 계약 테스트: [ApiLoggingContractTests.kt](./src/test/kotlin/com/aandi/gateway/logging/ApiLoggingContractTests.kt)
- CloudWatch query builder: [queries.go](./monitor-bot/internal/cloudwatch/queries.go)

### 5.3 Discord Monitor Bot

![Discord Command Map](./docs/assets/diagrams/discord-command-map.png)

Discord Monitor Bot은 Gateway JVM과 분리된 Go HTTP Interactions sidecar입니다. 운영자는 5개 command family만 기억하고, 세부 조회 조건은 option으로 드릴다운합니다.

| Command | 역할 |
| :--- | :--- |
| `/ops dashboard` | 서비스 상태와 dashboard watch 조회 |
| `/ops logs` | 오류, slow, security, trace 로그 조회 |
| `/ops alert` | general/critical 채널과 role 설정 |
| `/ops assignment` | 과제 상태, 진단, 이벤트, 제출 현황 조회 |
| `/ops help` | 상황별 운영 명령 검색 |

Monitor Bot은 read-only 운영 도구입니다. 운영 계약은 `bot never creates/updates/deletes/publishes assignments`입니다. 과제 생성, 수정, 삭제, 공개/비공개 명령은 제공하지 않습니다.

관련 근거:

- command schema: [commands.go](./monitor-bot/internal/discord/commands.go)
- interaction signature 검증: [signature.go](./monitor-bot/internal/discord/signature.go)
- 운영 문서: [monitor-bot/README.md](./monitor-bot/README.md), [docs/discord-monitor-bot.md](./docs/discord-monitor-bot.md)

### 5.4 Discord Alert Routing

![Discord Alert Example](./docs/assets/images/discord-critical-alert.png)

위 이미지는 민감정보와 trace 값을 마스킹한 Discord 알림 예시입니다. 일반 오류와 audit 이벤트는 general route로 보내고, 서버 장애로 분류한 CRITICAL server alerts만 별도 채널과 role mention을 사용합니다.

Trace 상세 조회는 public channel을 로그 결과로 채우지 않도록 ephemeral follow-up으로 처리합니다.

assignment audit feed는 Report V2 EVENT logs를 기준으로 `ASSIGNMENT_CREATED`, `ASSIGNMENT_UPDATED`, `ASSIGNMENT_DELETED`, `ASSIGNMENT_PUBLISHED`, `ASSIGNMENT_UNPUBLISHED` 이벤트를 조회합니다.

관련 근거:

- alert routing: [alerts.go](./monitor-bot/internal/monitor/alerts.go)
- assignment audit: [assignment_audit.go](./monitor-bot/internal/monitor/assignment_audit.go)
- alert 테스트: [alerts_test.go](./monitor-bot/internal/monitor/alerts_test.go)
- audit 테스트: [assignment_audit_test.go](./monitor-bot/internal/monitor/assignment_audit_test.go)

## 6. 기술적 고민과 해결

| 고민 | 해결 |
| :--- | :--- |
| 여러 서버로 나뉜 API 진입점 관리 | Spring Cloud Gateway로 외부 진입점을 통합하고 downstream service 라우팅을 분리 |
| 서버별 응답 형식과 오류 코드 차이 | 공통 응답 규약과 5자리 error code 체계를 Gateway README에 문서화 |
| 장애 원인 추적 시 로그 문자열 검색에 의존 | `traceId`, `errorCode`, `statusCode`, `latencyMs`, `service.domainCode` 중심의 structured log로 전환 |
| 운영자가 서버에 직접 접속해야 하는 문제 | Discord Monitor Bot을 Go sidecar로 분리하고 CloudWatch Logs를 read-only로 조회 |
| critical 알림 남발 위험 | 일반 route와 critical route를 분리하고 role mention은 configured role에만 허용 |
| 운영 명령이 데이터 변경으로 이어질 위험 | Monitor Bot에서 과제 생성·수정·삭제·공개/비공개 command를 제공하지 않는 read-only guard 적용 |

## 7. 테스트와 검증

| 항목 | 명령 |
| :--- | :--- |
| Gateway 테스트 | `./gradlew test` |
| Monitor Bot 테스트 | `cd monitor-bot && go test ./...` |
| README 링크 검증 | README 내 상대 경로가 실제 파일로 존재하는지 확인 |

검증 범위:

- Gateway route/security policy 테스트
- Gateway 공통 응답과 error code 계약 테스트
- trace logging 계약 테스트
- Discord command schema와 interaction 처리 테스트
- CloudWatch query builder 입력 검증 테스트
- alert routing, role mention 제한, assignment audit feed 테스트

CI 기준은 [ci.yml](./.github/workflows/ci.yml)에 정의되어 있습니다.

## 8. 실행 방법

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

## 9. 기술 스택

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

## 10. 관련 문서

| 문서 | 설명 |
| :--- | :--- |
| [Discord Monitor Bot 운영 문서](./docs/discord-monitor-bot.md) | Bot 명령, 알림 라우팅, read-only 운영 기준 |
| [Monitor Bot README](./monitor-bot/README.md) | Go sidecar 실행과 테스트 방법 |
| [Gateway Error Contract Mapping](./docs/GATEWAY_ERROR_CODES.md) | Gateway 공통 실패 응답과 error code 기준 |
| [Service Gateway Integration Guide](./docs/SERVICE_GATEWAY_INTEGRATION.md) | Gateway 뒤 서비스 연동 원칙 |
| [EC2 Environment Setup](./docs/ENV_SETUP.md) | 운영 환경 변수와 배포 환경 설정 |
| [개발 로그: “API가 이상한 것 같은데요?”를 듣기 전에 서버가 먼저 알려주게 만들기](https://velog.io/@stdiodh/%EC%82%BD%EC%A7%88-API%EA%B0%80-%EC%9D%B4%EC%83%81%ED%95%9C-%EA%B2%83-%EA%B0%99%EC%9D%80%EB%8D%B0%EC%9A%94%EB%A5%BC-%EB%93%A3%EA%B8%B0-%EC%A0%84%EC%97%90-%EC%84%9C%EB%B2%84%EA%B0%80-%EB%A8%BC%EC%A0%80-%EC%95%8C%EB%A0%A4%EC%A3%BC%EA%B2%8C-%EB%A7%8C%EB%93%A4%EA%B8%B0) | 구조화 로그와 Discord 운영 알림을 만든 과정 |
