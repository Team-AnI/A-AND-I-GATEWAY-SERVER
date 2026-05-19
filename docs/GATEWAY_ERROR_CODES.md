# Gateway Error Contract Mapping

이 문서는 현재 `aandi-gateway-server`에서 실제로 발생하는 에러 상황을 표준 에러 통신 규약으로 매핑한 정의다.

## Rules

- 이 게이트웨이에서 발생하는 모든 에러 응답은 반드시 공통 응답 형식을 따라야 한다.
- `message`와 `alert`는 한국어로 작성한다.
- `value`는 영문 대문자 스네이크 케이스를 유지한다.
- 아래 표의 각 항목은 실제 게이트웨이 에러 상황에 대한 공식 매핑이다.

## Mapping Table

| Code | 에러 상황 | HTTP | Value | Message | Alert |
|---:|---|---:|---|---|---|
| 10001 | HTTPS 정책 위반 | 403 | HTTPS_REQUIRED | 이 엔드포인트는 HTTPS 연결만 허용합니다. | 보안 연결이 필요해요. 잠시 후 다시 시도해 주세요. |
| 10002 | 허용되지 않은 Host | 403 | HOST_NOT_ALLOWED | 요청 Host가 게이트웨이 허용 호스트 정책에 포함되지 않았습니다. | 현재 주소에서는 요청을 처리할 수 없어요. 공식 도메인으로 다시 접속해 주세요. |
| 10003 | 로그인 rate limit 초과 | 429 | LOGIN_RATE_LIMIT_EXCEEDED | 로그인 요청 횟수 제한을 초과했습니다. | 로그인 시도가 너무 많아요. 잠시 후 다시 시도해 주세요. |
| 10004 | refresh rate limit 초과 | 429 | REFRESH_RATE_LIMIT_EXCEEDED | 토큰 재발급 요청 횟수 제한을 초과했습니다. | 요청이 너무 많아요. 잠시 후 다시 시도해 주세요. |
| 10005 | logout rate limit 초과 | 429 | LOGOUT_RATE_LIMIT_EXCEEDED | 로그아웃 요청 횟수 제한을 초과했습니다. | 요청이 너무 많아요. 잠시 후 다시 시도해 주세요. |
| 11001 | 인증 필요 또는 액세스 토큰 검증 실패 | 401 | AUTHENTICATION_FAILED | 인증이 필요하거나 액세스 토큰 검증에 실패했습니다. | 로그인 후 이용해주세요. |
| 11002 | refresh token 사전 검증 실패 | 401 | REFRESH_TOKEN_INVALID | 리프레시 토큰이 유효하지 않거나 `REFRESH` 타입이 아닙니다. | 로그인이 만료되었습니다. |
| 11003 | 내부 invalidation webhook 토큰 불일치 | 403 | INTERNAL_TOKEN_INVALID | 내부 이벤트 토큰이 없거나 설정값과 일치하지 않습니다. | 내부 요청 인증에 실패했어요. |
| 12001 | 권한 부족 | 403 | ACCESS_DENIED | 인증된 사용자가 이 리소스에 접근할 권한이 없습니다. | 이 작업을 수행할 권한이 없어요. |
| 13001 | 로그인 요청에서 `username` 또는 `password` 누락/공백 | 400 | LOGIN_REQUEST_BODY_INVALID | 로그인 요청 본문 검증에 실패했습니다. `username` 또는 `password` 값이 없거나 비어 있습니다. | 아이디와 비밀번호를 확인해 주세요. |
| 13002 | refresh/logout 요청에서 `refreshToken` 누락/공백 | 400 | REFRESH_TOKEN_REQUIRED | 토큰 재발급 또는 로그아웃 요청에 `refreshToken` 값이 없거나 비어 있습니다. | 로그인이 만료되었습니다. |
| 13003 | JSON Content-Type 강제 위반 | 415 | JSON_CONTENT_TYPE_REQUIRED | 요청 `Content-Type`은 `application/json` 또는 호환되는 `+json` 형식이어야 합니다. | 요청 형식이 올바르지 않아요. 다시 시도해 주세요. |
| 15001 | 허용되지 않은 method/path | 404 | ENDPOINT_NOT_ALLOWLISTED | 요청 메서드와 경로가 게이트웨이 허용 목록에 없습니다. | 요청한 기능을 찾을 수 없어요. |

## Source Mapping

- `SecurityConfig.kt`
  - 인증 실패
  - 인가 실패
- `GatewayRequestPolicyFilter.kt`
  - HTTPS 정책 위반
  - Host 정책 위반
  - 허용되지 않은 method/path
  - JSON Content-Type 강제 위반
- `AuthRequestValidationFilter.kt`
  - 로그인 요청 body 검증 실패
  - refreshToken 누락
  - refresh token 사전 검증 실패
- `AuthRateLimitFilter.kt`
  - login / refresh / logout rate limit 초과
- `InvalidationWebhookController.kt`
  - 내부 webhook 토큰 불일치
