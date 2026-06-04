# Error Response Policy

> 메인 README로 돌아가기: [README](../README.md)

본 프로젝트는 Gateway가 직접 반환하는 실패 응답을 공통 응답 구조와 5자리 error code 체계로 관리합니다. 정책 원본은 `src/main/kotlin/com/aandi/gateway/common/response/GatewayResponse.kt`와 `GatewayResponseWriter.kt`입니다.

## Common Response

### Success

```json
{
  "success": true,
  "data": {
    "...": "..."
  },
  "error": null,
  "timestamp": "2026-03-25T21:23:36.958558466+09:00"
}
```

### Failure

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": 13001,
    "message": "로그인 요청 본문 검증에 실패했습니다. `username` 또는 `password` 값이 없거나 비어 있습니다.",
    "value": "LOGIN_REQUEST_BODY_INVALID",
    "alert": "아이디와 비밀번호를 확인해 주세요."
  },
  "timestamp": "2026-03-25T21:23:36.958558466+09:00"
}
```

## Error Field

| 필드 | 타입 | 용도 |
| :--- | :--- | :--- |
| `code` | Integer | 서비스/분류/상세를 담은 5자리 에러 코드 |
| `message` | String | 개발자와 운영자가 원인을 파악하는 메시지 |
| `value` | String | 코드와 1:1 대응되는 영문 대문자 스네이크 케이스 |
| `alert` | String | 프론트엔드가 사용자에게 보여줄 수 있는 문구 |

## 5자리 Error Code

형식은 `[서비스 1자리][분류 1자리][상세 3자리]`입니다.

| 자리 | 의미 | 예시 |
| :--- | :--- | :--- |
| 서비스 | Gateway/Auth/User/Report/Judge/Blog/Common | `1` = Gateway |
| 분류 | 일반/인증/인가/검증/비즈니스/리소스 없음/충돌/외부 시스템/내부 오류 | `3` = 검증 |
| 상세 | 세부 오류 번호 | `001` |

서비스 코드:

| 코드 | 서비스 |
| :--- | :--- |
| 1 | Gateway |
| 2 | Auth |
| 3 | User |
| 4 | Report |
| 5 | Judge |
| 6 | Blog |
| 9 | Common |

분류 코드:

| 코드 | 의미 |
| :--- | :--- |
| 0 | 일반 |
| 1 | 인증 |
| 2 | 인가 |
| 3 | 검증 |
| 4 | 비즈니스 |
| 5 | 리소스 없음 |
| 6 | 중복 / 충돌 |
| 7 | 외부 시스템 |
| 8 | 시스템 내부 오류 |

## Gateway 직접 발행 코드

| Code | HTTP | Value | 상황 | Severity |
| ---: | ---: | :--- | :--- | :--- |
| 10001 | 403 | HTTPS_REQUIRED | HTTPS 정책 위반 | LOW |
| 10002 | 403 | HOST_NOT_ALLOWED | 허용되지 않은 Host | HIGH |
| 10003 | 429 | LOGIN_RATE_LIMIT_EXCEEDED | 로그인 rate limit 초과 | MEDIUM |
| 10004 | 429 | REFRESH_RATE_LIMIT_EXCEEDED | refresh rate limit 초과 | MEDIUM |
| 10005 | 429 | LOGOUT_RATE_LIMIT_EXCEEDED | logout rate limit 초과 | MEDIUM |
| 11001 | 401 | AUTHENTICATION_FAILED | 인증 필요 또는 access token 검증 실패 | LOW |
| 11002 | 401 | REFRESH_TOKEN_INVALID | refresh token 사전 검증 실패 | LOW |
| 11003 | 403 | INTERNAL_TOKEN_INVALID | 내부 invalidation webhook token 불일치 | HIGH |
| 12001 | 403 | ACCESS_DENIED | 권한 부족 | LOW |
| 13001 | 400 | LOGIN_REQUEST_BODY_INVALID | 로그인 요청 body 검증 실패 | LOW |
| 13002 | 400 | REFRESH_TOKEN_REQUIRED | refresh/logout 요청에 refreshToken 누락 | LOW |
| 13003 | 415 | JSON_CONTENT_TYPE_REQUIRED | JSON Content-Type 정책 위반 | LOW |
| 15001 | 404 | ENDPOINT_NOT_ALLOWLISTED | method/path allowlist 위반 | MEDIUM |
| 17801 | 502 | DOWNSTREAM_SERVICE_UNAVAILABLE | downstream service unavailable | HIGH |
| 18801 | 500 | INTERNAL_SERVER_ERROR | Gateway 내부 예외 | CRITICAL |

## HTTP Status Mapping

Gateway filter나 Spring Security exception handler는 `GatewayResponseWriter.writeError`로 HTTP status와 공통 실패 payload를 함께 씁니다.

| HTTP | 대표 코드 | 발생 위치 |
| ---: | :--- | :--- |
| 400 | 13001, 13002 | `AuthRequestValidationFilter` |
| 401 | 11001, 11002 | `SecurityConfig`, `AuthRequestValidationFilter` |
| 403 | 10001, 10002, 11003, 12001 | `GatewayRequestPolicyFilter`, `InvalidationWebhookController`, `SecurityConfig` |
| 404 | 15001 | `GatewayRequestPolicyFilter` |
| 415 | 13003 | `GatewayRequestPolicyFilter` |
| 429 | 10003, 10004, 10005 | `AuthRateLimitFilter` |
| 500 | 18801 | `GlobalExceptionHandler` |
| 502 | 17801 | downstream unavailable contract |

## 프론트/운영자 관점

- 프론트엔드는 `alert`를 사용자 표시 문구로 사용하고, `value`와 `code`를 조건 분기에 사용할 수 있습니다.
- 운영자는 `code` 첫 자리를 보고 서비스 영역을, 두 번째 자리를 보고 장애 분류를 빠르게 파악합니다.
- monitor-bot은 structured log의 `response.error.code`를 읽어 service/category/severity를 판단하고 critical/general alert route를 결정합니다.

## 로컬 응답 예시

민감정보 없는 로컬 Gateway 실행에서 allowlist 차단 응답을 screenshot으로 남겼습니다.

![Gateway error response example](./assets/images/error-code-policy.png)

## 검증

`src/test/kotlin/com/aandi/gateway/common/response/GatewayErrorCodeTests.kt`는 다음 계약을 검증합니다.

- Gateway error code가 5자리인지
- Gateway 직접 발행 코드는 서비스 코드 `1`로 시작하는지
- 실패 응답이 `success=false`, `data=null`, `error.code/value/alert`를 유지하는지
- 응답 payload가 내부 metadata인 `status`, `service`, `category`, `severity`를 노출하지 않는지
