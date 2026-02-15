# Service Gateway Integration Guide

기준일: 2026-02-15  
대상: Gateway 뒤에서 동작하는 각 서비스 레포(`report`, `user`, `admin` 등)

## 1) 통신 원칙

- 외부 클라이언트는 서비스로 직접 접근하지 않고 Gateway를 통해서만 접근한다.
- 서비스는 가능하면 private subnet 또는 보안그룹으로 내부 접근만 허용한다.
- Gateway가 전달하는 사용자 식별 헤더를 기준으로 요청 주체를 해석한다.

## 2) Gateway 라우팅 규칙

- `/v2/report/**` -> report 서비스
- `/v2/user/**` -> user 서비스
- `/v2/admin/**` -> admin 서비스
- 기본 `StripPrefix=2`이므로 서비스는 `/...` 기준 경로를 구현한다.

예시:
- 외부 요청: `GET /v2/report/articles`
- report 서비스 수신 경로: `GET /articles`

## 3) 서비스에서 신뢰할 헤더

- `X-User-Id`
- `X-Roles`

주의:
- 서비스는 외부에서 직접 들어온 요청의 동일 헤더를 신뢰하면 안 된다.
- 보안그룹/Nginx 정책으로 서비스 직접 노출을 차단해야 한다.

## 4) 인증/인가 분리

- Gateway 책임:
  - Access Token 유효성 검증(issuer + audience)
  - 라우팅
  - 공통 사용자 헤더 전달
- 서비스 책임:
  - 도메인 상세 인가 정책(`ROLE_ADMIN` 필요 여부 등)
  - 리소스 소유권 검사

## 5) 서비스 구현 체크리스트

- [ ] 서비스 외부 공개 포트 차단(내부망/게이트웨이만 허용)
- [ ] 라우트 경로를 `StripPrefix=2` 전제에 맞춰 구현
- [ ] `X-User-Id`, `X-Roles` 파싱 로직 추가
- [ ] 관리자 권한/소유권 인가 로직은 서비스 내부에서 처리
- [ ] 서비스 Swagger/OpenAPI는 각 서비스 레포에서 자체 관리

## 6) 운영 연동 체크리스트

- [ ] Gateway `REPORT_SERVICE_URI/USER_SERVICE_URI/ADMIN_SERVICE_URI`가 실제 서비스 주소를 가리키는지 확인
- [ ] Gateway와 서비스 간 보안그룹 통신 정책 확인
- [ ] 장애 시 우회 경로(직접 서비스 접근)가 열려 있지 않은지 점검
