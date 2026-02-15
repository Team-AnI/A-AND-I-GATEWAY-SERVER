# A-AND-I Gateway Server
> **한 줄 소개**: JWT 검증, 라우팅, 인증 컨텍스트 전달을 담당하는 WebFlux 기반 API Gateway

## 1. 프로젝트 개요 (Overview)
- **개발 기간**: 2026.02 ~ 진행 중
- **개발 인원**: 백엔드 1명
- **프로젝트 목적**: 서비스 분리 환경에서 인증 경계와 요청 라우팅을 중앙화하여 운영 복잡도 감소
- **GitHub**: https://github.com/stdiodh/A-AND-I-GATEWAY-SERVER.git

## 2. 사용 기술 및 선정 이유 (Tech Stack & Decision)

| Category | Tech Stack | Version | Decision Reason (Why?) |
| --- | --- | --- | --- |
| **Language** | Kotlin | 2.2.21 | 필터/보안/캐시 로직을 간결하게 유지하고 타입 안정성 확보 |
| **Framework** | Spring Boot + Spring Cloud Gateway + WebFlux | Boot 4.0.2 / Cloud 2025.1.0 | 고동시성 논블로킹 라우팅과 게이트웨이 표준 기능 활용 |
| **Security** | OAuth2 Resource Server (JWT) | - | 인증 서버 발급 토큰의 issuer/audience를 게이트웨이에서 1차 검증 |
| **Cache** | Redis Reactive | - | 토큰 컨텍스트 캐싱으로 반복 요청의 컨텍스트 생성 비용 절감 |
| **Infra** | Docker Compose, Nginx, Actuator | - | 운영 관측성과 배포 일관성 확보 |

## 3. 시스템 아키텍처 (System Architecture)
```mermaid
graph LR
  Client --> GW[Gateway(WebFlux)]
  GW --> Report[Report Service]
  GW --> User[User Service]
  GW --> Admin[Admin Service]
  GW --> Redis[(Redis Cache)]
  Auth[Auth Server] --> GW
```

- **설계 특징**:
- 토큰 기반 인증을 게이트웨이에서 선검증하여 다운스트림 서비스 보안 중복 제거
- `X-Auth-Context` 컨텍스트를 캐시 기반으로 생성하되, 스푸핑 헤더는 필터에서 제거
- 로그아웃/권한변경 이벤트 시 subject 인덱스 기반 일괄 무효화 설계

## 4. 핵심 기능 (Key Features)
- **JWT 보안 검증**: `issuer` + `required audience` 동시 검증으로 잘못된 토큰 조기 차단
- **서비스 라우팅**: `/v2/report`, `/v2/user`, `/v2/admin` 경로 기반 라우팅 및 prefix 제거
- **인증 컨텍스트 캐시**: 토큰 해시 키 + subject 인덱스로 조회/무효화 성능 확보
- **장애 격리**: Redis 오류 시에도 fallback 경로로 요청 흐름 유지

## 5. 트러블 슈팅 및 성능 개선 (Troubleshooting & Refactoring)
### 5-1. 인증 컨텍스트 위조 헤더 방지
- **문제(Problem)**: 클라이언트가 `X-Auth-Context`를 임의 주입하면 내부 서비스가 신뢰할 가능성 존재
- **원인(Cause)**: 헤더 정화 없이 전달 시 게이트웨이 경계를 우회한 권한 오해석 가능
- **해결(Solution)**:
  1. `TokenContextHeaderFilter`에서 인증 여부와 무관하게 스푸핑 헤더 제거
  2. 인증 성공 시에만 서버 측 생성 컨텍스트를 주입
  3. 단위 테스트로 인증/비인증 모두 헤더 제거 동작 검증
- **결과(Result)**: 클라이언트 주도 컨텍스트 오염 경로 제거. 인증 경계 신뢰도 향상

### 5-2. 토큰 컨텍스트 재생성 오버헤드 완화
- **문제(Problem)**: 모든 요청마다 JWT 기반 컨텍스트를 재계산하면 라우팅 단계 CPU 사용량 증가
- **해결(Solution)**:
  1. 토큰 해시 기반 캐시 키와 subject 인덱스 키로 Redis 캐싱
  2. 로그아웃/권한변경 웹훅으로 관련 키 묶음 무효화
  3. Redis 장애 시 fallback JSON 생성으로 기능 가용성 유지
- **결과(Result)**: 반복 요청의 컨텍스트 생성 비용 절감(추정). 캐시 미스/장애 상황에서도 요청 처리 지속

## 6. 프로젝트 회고 (Retrospective)
- **배운 점**: 게이트웨이의 핵심 가치는 라우팅보다 인증 경계 명확화와 실패 시 동작 정의에 있음
- **아쉬운 점 & 향후 계획**: 캐시 히트율과 라우팅 지연을 Actuator/메트릭 기반으로 시계열 추적해 튜닝 자동화 예정
