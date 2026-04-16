# A-AND-I-REPORT-GATEWAY-SERVER

## API Response Contract

이 게이트웨이에서 발생하는 **모든 응답과 에러는 반드시 아래 공통 응답 형식**을 따라야 합니다.

### Success Response

```json
{
  "success": "SUCCESS",
  "data": {
    "...": "..."
  },
  "error": null,
  "timestamp": "2026-03-25T21:23:36.958558466+09:00"
}
```

### Failure Response

```json
{
  "success": "SUCCESS",
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

### Error Field Definition

- `code` (`Integer`): 에러 응답 코드
- `message` (`String`): 어떤 필드 또는 어떤 값에서 문제가 발생했는지 나타내는 개발자용 메시지
- `value` (`String`): `code`에 대응하는 에러 값
- `alert` (`String`): 클라이언트가 토스트 또는 다이얼로그로 사용자에게 보여줄 메시지

## Error Code Policy

게이트웨이에서 발생할 수 있는 모든 에러는 아래 에러 코드 체계를 따라야 합니다.

### 5.1 에러 코드 형식

| 항목 | 내용 |
|---|---|
| 형식 | 5자리 정수 |
| 구조 | `[서비스 1자리][분류 1자리][상세 3자리]` |
| 예시 | `21301` |

### 5.2 서비스 구분 코드

| 코드 | 서비스 |
|---|---|
| 1 | Gateway |
| 2 | Auth |
| 3 | User |
| 4 | Report |
| 5 | Judge |
| 6 | Blog |
| 9 | Common |

### 5.3 분류 코드

| 코드 | 의미 |
|---|---|
| 0 | 일반 |
| 1 | 인증 |
| 2 | 인가 |
| 3 | 검증 |
| 4 | 비즈니스 |
| 5 | 리소스 없음 |
| 6 | 중복 / 충돌 |
| 7 | 외부 시스템 |
| 8 | 시스템 내부 오류 |

## Gateway Error Codes

아래 표는 **현재 게이트웨이 서버가 직접 발행하는 에러코드**를 정리한 목록입니다.  
문서화 시에는 반드시 **현재 코드에 실제로 구현된 에러만** 사용해야 하며, 아직 구현되지 않은 raw 응답을 표준 응답처럼 서술하면 안 됩니다.

| Code | 분류 | 에러 상황 | HTTP | Value | Message | Alert |
|---:|---|---|---:|---|---|---|
| 10001 | 일반 | HTTPS 정책 위반 | 403 | HTTPS_REQUIRED | 이 엔드포인트는 HTTPS 연결만 허용합니다. | 보안 연결이 필요해요. 잠시 후 다시 시도해 주세요. |
| 10002 | 일반 | 허용되지 않은 Host | 403 | HOST_NOT_ALLOWED | 요청 Host가 게이트웨이 허용 호스트 정책에 포함되지 않았습니다. | 현재 주소에서는 요청을 처리할 수 없어요. 공식 도메인으로 다시 접속해 주세요. |
| 10003 | 일반 | 로그인 rate limit 초과 | 429 | LOGIN_RATE_LIMIT_EXCEEDED | 로그인 요청 횟수 제한을 초과했습니다. | 로그인 시도가 너무 많아요. 잠시 후 다시 시도해 주세요. |
| 10004 | 일반 | refresh rate limit 초과 | 429 | REFRESH_RATE_LIMIT_EXCEEDED | 토큰 재발급 요청 횟수 제한을 초과했습니다. | 요청이 너무 많아요. 잠시 후 다시 시도해 주세요. |
| 10005 | 일반 | logout rate limit 초과 | 429 | LOGOUT_RATE_LIMIT_EXCEEDED | 로그아웃 요청 횟수 제한을 초과했습니다. | 요청이 너무 많아요. 잠시 후 다시 시도해 주세요. |
| 11001 | 인증 | 인증 필요 또는 액세스 토큰 검증 실패 | 401 | AUTHENTICATION_FAILED | 인증이 필요하거나 액세스 토큰 검증에 실패했습니다. | 로그인 후 이용해주세요. |
| 11002 | 인증 | refresh token 사전 검증 실패 | 401 | REFRESH_TOKEN_INVALID | 리프레시 토큰이 유효하지 않거나 `REFRESH` 타입이 아닙니다. | 로그인이 만료되었습니다. |
| 11003 | 인증 | 내부 invalidation webhook 토큰 불일치 | 403 | INTERNAL_TOKEN_INVALID | 내부 이벤트 토큰이 없거나 설정값과 일치하지 않습니다. | 내부 요청 인증에 실패했어요. |
| 12001 | 인가 | 권한 부족 | 403 | ACCESS_DENIED | 인증된 사용자가 이 리소스에 접근할 권한이 없습니다. | 이 작업을 수행할 권한이 없어요. |
| 13001 | 검증 | 로그인 요청에서 `username` 또는 `password` 누락/공백 | 400 | LOGIN_REQUEST_BODY_INVALID | 로그인 요청 본문 검증에 실패했습니다. `username` 또는 `password` 값이 없거나 비어 있습니다. | 아이디와 비밀번호를 확인해 주세요. |
| 13002 | 검증 | refresh/logout 요청에서 `refreshToken` 누락/공백 | 400 | REFRESH_TOKEN_REQUIRED | 토큰 재발급 또는 로그아웃 요청에 `refreshToken` 값이 없거나 비어 있습니다. | 로그인이 만료되었습니다. |
| 13003 | 검증 | JSON Content-Type 강제 위반 | 415 | JSON_CONTENT_TYPE_REQUIRED | 요청 `Content-Type`은 `application/json` 또는 호환되는 `+json` 형식이어야 합니다. | 요청 형식이 올바르지 않아요. 다시 시도해 주세요. |
| 15001 | 리소스 없음 | 허용되지 않은 method/path | 404 | ENDPOINT_NOT_ALLOWLISTED | 요청 메서드와 경로가 게이트웨이 허용 목록에 없습니다. | 요청한 기능을 찾을 수 없어요. |
| 18801 | 시스템 내부 오류 | 게이트웨이 내부 예외 | 500 | INTERNAL_SERVER_ERROR | 게이트웨이 내부 처리 중 예기치 못한 오류가 발생했습니다. | 일시적인 오류가 발생했어요. 잠시 후 다시 시도해 주세요. |

### 코드 정의 원본

게이트웨이 에러코드의 실제 구현 원본은 아래 enum을 기준으로 관리합니다.

- `src/main/kotlin/com/aandi/gateway/common/response/GatewayResponse.kt`
