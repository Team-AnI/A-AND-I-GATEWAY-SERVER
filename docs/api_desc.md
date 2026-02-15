# API_DESC

기준일: 2026-02-15

## 경로 네이밍 규칙

- 기본 형식: `/v2/{feature}/{resource}`
- 규칙 의도:
  - `v2`로 버전 고정 (`api` prefix 미사용)
  - 버전 뒤에 기능(feature)을 먼저 배치
  - 기능 안에서 리소스(resource)를 세분화

## feature 정의

- `cache`: 토큰 컨텍스트 캐시 관련 내부 기능
- (향후) `auth`, `routing`, `admin` 등 기능 단위로 확장

## 현재 API 목록

- 현재 외부 공개 `v2` API는 없음
- 캐시 기능은 Gateway 내부 필터에서만 사용
- 라우팅 규칙:
  - `/v2/report/**` -> `REPORT_SERVICE_URI`
  - `/v2/user/**` -> `USER_SERVICE_URI`
  - `/v2/admin/**` -> `ADMIN_SERVICE_URI`
- 기본 Path 변환:
  - 각 라우트 `StripPrefix=2` 기본값 사용
  - 필요 시 `*_STRIP_PREFIX` 환경변수로 서비스별 조정

## 호환성 정책

- 구버전(`v1`) 경로는 신규 구현하지 않는다.
- 신규 API는 동일 규칙(`/v2/{feature}/{resource}`)을 따른다.
