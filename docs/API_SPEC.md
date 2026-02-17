# Gateway API Specification

기준일: 2026-02-15  
대상 레포: [Team-AnI/A-AND-I-GATEWAY-SERVER](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER)

## 1) 범위

- 본 문서는 Gateway의 현재 공개 계약을 정의한다.
- 하위 도메인 서비스 API 스펙은 각 서비스 레포에서 관리한다.

## 1-1) API 버전/경로 규칙

- 버전 prefix: `/v2`
- 기능 순서: `/v2/{feature}/{resource}`
- 기능 분류:
  - `report`, `user`, `admin`, `post`: 외부 요청 라우팅 대상
  - `cache`: Gateway 내부 캐시 기능 (외부 API 미노출)
- 현재 상태:
  - Gateway는 기능 prefix 기준으로 하위 서비스 라우팅
  - `cache`는 Gateway 내부 처리 전용 (외부 API 미노출)

## 1-2) 라우팅 매핑 (현재)

1. `/v2/report/**` -> `REPORT_SERVICE_URI` (default `http://localhost:8081`)
2. `/v2/user/**` -> `USER_SERVICE_URI` (default `http://localhost:8082`)
3. `/v2/admin/**` -> `ADMIN_SERVICE_URI` (default `http://localhost:8083`)
4. `/v2/post/**` -> `POST_SERVICE_URI` (default `http://localhost:8084`)
5. `/v2/post/images/**` -> `POST_SERVICE_URI` (default `http://localhost:8084`)
6. Path 변환 규칙:
  - 기본 `StripPrefix=2`
  - `/v2/report/articles` -> `/articles`
  - post 라우트는 리라이트 적용:
    - `/v2/post` -> `/api/v1/posts`
    - `/v2/post/{postId}` -> `/api/v1/posts/{postId}`
    - `/v2/post/images` -> `/api/v1/images`

## 2) 인증 정책

- 기본 정책: 모든 요청은 Bearer Access Token 필요
- JWT 검증: `issuer` + `audience` 모두 검증
- 예외(무인증):
  - `GET /actuator/health`
  - `GET /actuator/health/**`
  - `GET /actuator/info`
  - `POST /internal/v1/cache/invalidation` (내부 토큰 헤더 검증)
- 인가 책임 분리:
  - Gateway는 토큰 인증/전달만 담당
  - `/v2/admin/**` 상세 권한(`ROLE_ADMIN` 등)은 admin 서비스에서 검증

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

## 5) 토큰 컨텍스트 캐시 계약

- 저장소: Redis
- 키 전략(확정):
  - 데이터 키: `cache:token:{subject}:{tokenHash}`
  - 인덱스 키: `cache:token-index:{subject}`
- TTL: 기본 24시간 (`TOKEN_CACHE_TTL`, default `24h`)
- 동작:
  - 캐시 HIT: 내부 캐시 재사용
  - 캐시 MISS: 컨텍스트 생성 후 저장
  - MISS 저장 시 subject 인덱스에 데이터 키를 함께 등록
  - Redis 장애: 캐시 우회(요청 실패로 확장하지 않음)
  - 외부 API로 조회/삭제를 제공하지 않음
- 강제 무효화 트리거(확정):
  - 로그아웃 이벤트 수신 시 해당 subject 인덱스 기준 일괄 삭제
  - 권한 변경 이벤트 수신 시 해당 subject 인덱스 기준 일괄 삭제
  - 내부 웹훅: `POST /internal/v1/cache/invalidation`
    - 헤더: `X-Internal-Token`
    - 바디: `{"eventType":"LOGOUT|ROLE_CHANGED","subject":"<user-subject>"}`
    - 접근 제어: Nginx에서 사설 CIDR만 허용 + 외부 deny
- TTL 운영 기준(확정):
  - 기본값 24시간 유지
  - 권한 변경 반영이 민감한 환경은 `TOKEN_CACHE_TTL` 단축 운영

## 6) 응답 코드 정책 (Gateway 관점)

- `401 Unauthorized`: 토큰 누락/유효하지 않음
- `403 Forbidden`: 인증은 되었으나 접근 불가(정책 확장 시)
- `404 Not Found`: 유효 요청이지만 매칭 라우트 없음

## 7) 환경변수

- `AUTH_ISSUER_URI`
- `AUTH_JWK_SET_URI`
- `AUTH_AUDIENCE` (default `aandi-gateway`)
- `INTERNAL_EVENT_TOKEN`
- `TOKEN_CACHE_TTL` (optional, default `24h`)
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`
- `REPORT_SERVICE_URI`
- `USER_SERVICE_URI`
- `ADMIN_SERVICE_URI`
- `POST_SERVICE_URI`
- `MANAGEMENT_SERVER_PORT` (default `9090`)
- `MANAGEMENT_SERVER_ADDRESS` (default `127.0.0.1`)

## 8) 미정의/추가 예정

- 무효화 이벤트를 메시지 브로커(SQS/SNS/Kafka)로 전환할지 여부

## 9) 호환성 정책

- 구버전(`v1`) 경로는 신규 구현하지 않는다.
- 신규 라우팅/엔드포인트는 `/v2/{feature}/{resource}` 규칙을 따른다.
