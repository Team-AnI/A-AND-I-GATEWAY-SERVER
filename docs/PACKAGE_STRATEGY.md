# Package Separation Strategy

기준일: 2026-02-15

## 목표

- Gateway 역할(인증 검증/라우팅/공통 필터)에 맞게 패키지를 분리한다.
- 도메인 기능과 인프라 구현을 분리해 변경 영향 범위를 줄인다.

## 현재 패키지 구조

```text
com.aandi.gateway
├── common
│   └── HeaderNames.kt
├── config
│   └── RedisConfig.kt
├── security
│   ├── SecurityConfig.kt
│   └── AuthenticatedPrincipalHeaderFilter.kt
└── cache
    ├── TokenContextCacheService.kt
    └── TokenContextHeaderFilter.kt
```

## 분리 원칙

1. `common`
- 전역 상수/공용 유틸만 둔다.
- 비즈니스 로직을 넣지 않는다.

2. `security`
- 인증/인가 정책, 보안 필터만 둔다.
- 데이터 저장/조회 로직은 넣지 않는다.

3. `cache`
- 캐시 키 정책, TTL, 캐시 조회/갱신 로직을 둔다.
- 보안 정책 결정은 하지 않는다.

4. `config`
- Bean 등록과 환경 설정만 둔다.
- 런타임 정책 분기는 서비스/필터로 이동한다.

## 의존 방향

- `security` -> `common`
- `cache` -> `common`, `config`(Spring Bean 주입)
- `config`는 다른 기능 패키지에 의존하지 않는다.

## 다음 확장 시 권장

- 하위 서비스 라우팅이 늘어나면 `routing` 패키지 추가
- 외부 인증 서버 연동(인증 컨텍스트 조회)이 생기면 `integration.auth` 패키지 추가
- 운영용 endpoint/health 확장이 필요하면 `management` 패키지 추가
