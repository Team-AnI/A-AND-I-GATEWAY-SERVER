# Gateway API Specification

기준일: 2026-02-15  
대상 레포: [Team-AnI/A-AND-I-GATEWAY-SERVER](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER)

## 1) 범위

- 본 문서는 Gateway의 현재 공개 계약을 정의한다.
- 하위 도메인 서비스 API 스펙은 각 서비스 레포에서 관리한다.

## 2) 인증 정책

- 기본 정책: 모든 요청은 Bearer Access Token 필요
- 예외(무인증):
  - `GET /actuator/health`
  - `GET /actuator/health/**`
  - `GET /actuator/info`

## 3) 요청 헤더 계약

클라이언트가 보내는 아래 헤더는 신뢰하지 않으며 Gateway에서 제거된다.

- `X-User-Id`
- `X-Roles`
- `X-Auth-Context`
- `X-Auth-Context-Cache`

## 4) Gateway -> Downstream 전달 헤더

인증 성공 요청에 대해 Gateway가 하위 서비스에 주입:

- `X-User-Id`: 인증 주체 식별자
- `X-Roles`: 권한 목록(csv)
- `X-Auth-Context`: 토큰 기반 컨텍스트 JSON
- `X-Auth-Context-Cache`: `HIT` 또는 `MISS`

## 5) 토큰 컨텍스트 캐시 계약

- 저장소: Redis
- 키 패턴: `cache:token:{subject}:{tokenHash}`
- TTL: 기본 24시간 (`TOKEN_CACHE_TTL`, default `24h`)
- 동작:
  - 캐시 HIT: Redis JSON 재사용
  - 캐시 MISS: 컨텍스트 생성 후 저장
  - Redis 장애: 캐시 우회(요청 실패로 확장하지 않음)

## 6) 응답 코드 정책 (Gateway 관점)

- `401 Unauthorized`: 토큰 누락/유효하지 않음
- `403 Forbidden`: 인증은 되었으나 접근 불가(정책 확장 시)
- `404 Not Found`: 유효 요청이지만 매칭 라우트 없음

## 7) 환경변수

- `AUTH_ISSUER_URI`
- `AUTH_JWK_SET_URI`
- `TOKEN_CACHE_TTL` (optional, default `24h`)
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`

## 8) 미정의/추가 예정

- 하위 서비스 라우트 실구성 (`spring.cloud.gateway.routes`)
- audience 검증 규칙
- 캐시 강제 무효화 API/이벤트
