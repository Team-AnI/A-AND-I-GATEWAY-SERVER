# A&I Gateway Server

> Auth, Assignment & Report, Blog, Online Judge로 나뉜 A&I 백엔드의 공통 진입점입니다.

클라이언트는 서비스마다 다른 주소와 인증 규칙을 알 필요 없이 Gateway 한 곳으로 요청합니다.

Gateway는 요청을 전달하기 전에 공통 정책과 권한을 확인하고, 응답 뒤에는 같은 요청을 끝까지 따라갈 수 있는 로그를 남깁니다.

## A&I 서비스 전체 구조

![A&I 서비스 전체 구조](./docs/assets/diagrams/aandi-platform-architecture.png)

외부 요청은 Nginx와 Gateway를 거쳐 각 서비스로 전달됩니다.

과제와 채점 데이터는 서비스끼리 직접 호출하지 않고 SNS/SQS 이벤트로 맞춥니다.

## Gateway와 운영 조회 구조

![Gateway와 Discord Monitor Bot 운영 구조](./docs/assets/diagrams/gateway-architecture.png)

Gateway는 라우팅, JWT 권한 검사, allowlist, 공통 오류 응답과 구조화 로그를 담당합니다.

인증 요청 제한은 Gateway 인스턴스 안에서 처리하고, Redis는 token context cache에 사용합니다.

Discord Monitor Bot은 Gateway JVM과 분리된 Go sidecar입니다.

CloudWatch Logs와 WEB Admin GET API를 읽기 전용으로 조회하며 서비스 데이터를 바꾸는 명령은 제공하지 않습니다.

## 요청 처리 흐름

```mermaid
sequenceDiagram
    autonumber
    actor Client
    participant Gateway as A&I Gateway
    participant Policy as Request Policy
    participant Security as JWT / Role Check
    participant Service as Downstream Service
    participant Logs as CloudWatch Logs

    Client->>Gateway: API 요청
    Gateway->>Policy: HTTPS · Host · Method · Path · Content-Type 확인
    Policy-->>Gateway: 정책 검사 결과

    alt 요청 정책 위반
        Gateway-->>Client: 공통 오류 응답
    else 정책 통과
        Gateway->>Security: JWT와 역할 확인
        Security-->>Gateway: 인증·권한 검사 결과

        alt 인증 또는 권한 실패
            Gateway-->>Client: 401 또는 403 응답
        else 인증 성공
            Gateway->>Service: traceId · requestId와 함께 전달
            Service-->>Gateway: 서비스 응답
            Gateway->>Logs: 경로 · 상태 코드 · 지연시간 · 오류 코드 기록
            Gateway-->>Client: 공통 응답 반환
        end
    end
```

허용되지 않은 요청은 downstream까지 보내지 않고 Gateway에서 종료합니다.

`traceId`와 `requestId`는 서비스 요청과 로그에 함께 남아 Gateway와 각 서비스의 기록을 하나의 요청으로 이어 줍니다.

## 장애 확인 흐름

```mermaid
sequenceDiagram
    autonumber
    participant Gateway
    participant Service as Downstream Service
    participant Logs as CloudWatch Logs
    participant Bot as Discord Monitor Bot
    actor Operator as 운영자

    Gateway->>Logs: 요청·응답 구조화 로그
    Service->>Logs: 서비스 로그
    Bot->>Logs: 상태·오류·지연 요청 조회
    Logs-->>Bot: 조회 결과
    Bot-->>Operator: Discord 대시보드 또는 장애 알림

    Operator->>Bot: /ops logs 또는 trace 조회
    Bot->>Logs: traceId와 조회 조건 전달
    Logs-->>Bot: Gateway·서비스 관련 로그
    Bot-->>Operator: 확인에 필요한 결과 요약
```

![Discord Command Map](./docs/assets/diagrams/discord-command-map.png)

`/ops dashboard`, `/ops logs`, `/ops alert`, `/ops assignment`, `/ops help` 다섯 가지 명령 묶음으로 운영 상태를 확인합니다.

![Discord Alert Example](./docs/assets/images/discord-critical-alert.png)

동일한 원인의 장애는 cooldown 동안 반복 전송하지 않습니다.

CRITICAL 서버 장애만 전용 채널과 허용된 운영자 역할 mention을 사용합니다.

## k6 부하 테스트

![Gateway k6 overhead](./docs/assets/performance/gateway-k6-overhead.svg)

동일한 Mock Downstream에서 Direct P95는 `56.959 ms`, Gateway P95는 `65.357 ms`였고 추가 지연은 `8.399 ms`였습니다.

3회 측정의 HTTP 실패율은 `0.00%`, check 성공률은 `100.00%`였습니다.

이 결과는 정책·라우팅·로깅 계층의 회귀를 확인하기 위한 로컬 기준이며 운영 환경의 최대 처리량을 뜻하지 않습니다.

## 테스트

[![CI](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/actions/workflows/ci.yml)

CI에서 다음 항목을 모두 확인합니다.

- `./gradlew test` — Gateway 테스트
- `cd monitor-bot && go test ./...` — Monitor Bot 테스트
- `./gradlew bootJar` — 실행 JAR 빌드
- performance Python unit tests
- k6 시나리오와 생성 asset drift 검사

## 실행

```bash
docker compose up -d redis gateway
curl -i http://localhost:8080/actuator/health
```

```bash
./gradlew test
cd monitor-bot && go test ./...
```

## 기술 스택

| 영역 | 기술 |
| :--- | :--- |
| Gateway | Kotlin 2.2, Java 21, Spring Boot 4, Spring Cloud Gateway WebFlux |
| Security | Spring Security, OAuth2 Resource Server, JWT role policy |
| Cache | Redis Reactive |
| Observability | Structured logging, traceId/requestId, CloudWatch Logs |
| Monitor Bot | Go 1.24, Discord HTTP Interactions, AWS SDK |
| Infra | Docker, Docker Compose, Nginx, GitHub Actions |
| Performance | k6 |

## 참고 문서

- [Gateway 오류 계약](./docs/GATEWAY_ERROR_CODES.md)
- [서비스 연동 원칙](./docs/SERVICE_GATEWAY_INTEGRATION.md)
- [성능 측정 환경과 전체 결과](./docs/PERFORMANCE.md)
- [CI/CD optimization metrics](docs/cicd-optimization.md)
- [CI/CD measurement audit](docs/cicd-measurement-audit.md)
- [Resume metrics](docs/resume-metrics.md)
- [Discord Monitor Bot 실행과 운영](./monitor-bot/README.md)
