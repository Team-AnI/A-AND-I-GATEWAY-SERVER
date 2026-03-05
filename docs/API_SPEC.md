# Gateway API Specification

기준일: 2026-03-05  
대상 레포: `A-AND-I-GATEWAY-SERVER`  
소스 오브 트루스:
- `src/main/resources/application.yaml`
- `src/main/kotlin/com/aandi/gateway/security/SecurityConfig.kt`
- `src/main/kotlin/com/aandi/gateway/security/GatewayRequestPolicyFilter.kt`
- `src/main/kotlin/com/aandi/gateway/security/AuthRequestValidationFilter.kt`
- `src/main/kotlin/com/aandi/gateway/security/AuthRateLimitFilter.kt`
- `src/main/kotlin/com/aandi/gateway/cache/InvalidationWebhookController.kt`

## 1. 문서 범위

- 본 문서는 **Gateway가 실제로 외부에 허용하는 모든 API 경로**와 Gateway 레벨의 Request/Response 계약을 정의한다.
- 도메인 비즈니스 payload(예: post/course/report 상세 DTO)는 하위 서비스 계약을 따른다.
- Gateway에서 body 스키마를 직접 검증하는 API는 본 문서에 명시한다.

## 2. 공통 Request 계약

### 2.1 기본

- Base Path: `/` (v1 + legacy v2 경로 공존)
- Content-Type:
  - `POST|PUT|PATCH`는 기본적으로 `application/json` 또는 `+json` 필요
  - 예외(멀티파트 허용): `/v1/me`, `/v1/posts`, `/v1/posts/images`, `/v2/post`, `/v2/post/images/**`

### 2.2 인증 헤더

- 인증 필요 API는 `Authorization: Bearer <access-token>` 필요
- JWT 검증:
  - `iss` = `security.jwt.issuer`
  - `aud` = `security.jwt.audience`
  - `token_type` = `ACCESS`
  - `sub` UUID 형식
  - `role` in `USER | ORGANIZER | ADMIN`
  - `jti` 존재

### 2.3 헤더 정리/주입

- 클라이언트가 보낸 아래 헤더는 제거됨:
  - `X-User-Id`
  - `X-Roles`
  - `X-Auth-Context`
  - `X-Auth-Context-Cache`
- 인증 성공 시 Gateway가 하위 서비스로 주입:
  - `X-User-Id: <jwt.sub>`
  - `X-Roles: <authorities csv>`

## 3. 공통 Response 계약 (Gateway 생성 응답)

| 상황 | HTTP | Body |
|---|---|---|
| 인증 실패 (Spring Security) | 401 | `{"message":"Unauthorized"}` |
| 인가 실패 (Spring Security) | 403 | `{"message":"Forbidden"}` |
| 허용되지 않은 method/path (allowlist) | 404 | empty |
| JSON Content-Type 정책 위반 | 415 | empty |
| auth rate limit 초과 | 429 | empty |
| auth 요청 body 검증 실패 | 400/401 | empty |
| 내부 무효화 토큰 불일치 | 403 | empty |

참고:
- 하위 서비스로 전달된 요청의 성공/실패 body는 Gateway가 변환하지 않고 passthrough 한다.

## 4. Gateway가 직접 검증하는 Request Body

### 4.1 로그인

- 대상:
  - `POST /v1/auth/login`
  - `POST /v2/auth/login`
- 필수 필드:
  - `username: string (blank 불가)`
  - `password: string (blank 불가)`
- 실패 응답:
  - 누락/blank -> `400`

### 4.2 토큰 갱신/로그아웃

- 대상:
  - `POST /v1/auth/refresh`
  - `POST /v2/auth/refresh`
  - `POST /v1/auth/logout`
  - `POST /v2/auth/logout`
- 필수 필드:
  - `refreshToken: string (blank 불가)`
- 추가 검증(`prevalidateRefreshTokenType=true`):
  - refresh JWT 서명 검증 + `token_type == REFRESH`
- 실패 응답:
  - 누락/blank -> `400`
  - 타입/검증 실패 -> `401`

### 4.3 내부 캐시 무효화

- 대상:
  - `POST /internal/v1/cache/invalidation`
- 필수 헤더:
  - `X-Internal-Token`
- Request Body:
```json
{
  "eventType": "LOGOUT",
  "subject": "user-subject"
}
```
- `eventType` enum:
  - `LOGOUT`
  - `ROLE_CHANGED`
- Success Response (`202 Accepted`):
```json
{
  "invalidatedKeys": 3
}
```
- 실패:
  - 내부 토큰 불일치 -> `403`

## 5. 인증/권한 정책

### 5.1 Public

- `OPTIONS /**`
- `POST /v1/auth/**`
- `POST /v2/auth/login`
- `POST /v2/auth/refresh`
- `POST /activate`
- `POST /internal/v1/cache/invalidation` (내부 토큰은 별도 검증)
- `GET /api/ping/**`
- `GET /`, `GET /index.html`
- `GET /v3/api-docs/**`
- `GET /v2/*/v3/api-docs`, `GET /v2/*/v3/api-docs/**`
- `GET /swagger-ui.html`, `GET /swagger-ui/**`
- `GET /v2/docs`, `GET /v2/docs/**`
- `GET /v2/swagger-ui/index.html`, `GET /v2/swagger-ui/**`
- `GET /actuator/health`, `GET /actuator/health/**`, `GET /actuator/info`
- Blog 조회:
  - `GET /v1/posts`
  - `GET /v1/posts/*`
  - `GET /v2/post`
  - `GET /v2/post/*`

### 5.2 USER|ORGANIZER|ADMIN

- `GET /v1/me`, `POST /v1/me`, `PATCH /v1/me`
- `GET /v2/auth/me`
- `GET /v1/courses`, `GET /v1/courses/**`
- `GET /v2/post/courses`, `GET /v2/post/courses/**`
- `GET /v1/report`, `GET|POST|PUT|DELETE /v1/report/**`

### 5.3 ADMIN

- `/v1/admin/**`
- `/v2/auth/admin/**`
- `/v2/post/admin/courses`, `/v2/post/admin/courses/**`

### 5.4 ORGANIZER|ADMIN

- `POST /v1/posts`, `POST /v2/post`
- `PATCH /v1/posts/*`, `PATCH /v2/post/*`
- `DELETE /v1/posts/*`, `DELETE /v2/post/*`
- `GET /v1/posts/drafts`, `GET /v2/post/drafts`
- `POST /v1/posts/images`, `POST /v2/post/images`

## 6. 라우팅 매핑 (요약)

### 6.1 Auth 서비스

- v1 경로: `/v1/auth/*`, `/v1/me*`, `/v1/admin/users*`, `/v1/admin/invite-mail`, `/v1/admin/ping`
- v2 alias:
  - `/v2/auth/login -> /v1/auth/login`
  - `/v2/auth/refresh -> /v1/auth/refresh`
  - `/v2/auth/logout -> /v1/auth/logout`
  - `/v2/auth/me -> /v1/me`
  - `/v2/auth/admin/** -> /v1/admin/**`
  - `/activate -> /activate`

### 6.2 Report 서비스

- `/v1/report*`
- `/v2/report*` (`/api/report` 경유)
- Courses/Admin Courses:
  - `/v1/admin/courses*`
  - `/v1/courses*`
  - `/v2/post/admin/courses* -> /v1/admin/courses*`
  - `/v2/post/courses* -> /v1/courses*`

### 6.3 Post(Blog) 서비스

- `/v1/posts*`
- `/v2/post* -> /v1/posts*`
- `/v2/post/images* -> /v1/posts/images*`

## 7. 전체 API 카탈로그 (코드 기준)

요청/응답 표기 규칙:
- Request:
  - `GW 검증` = Gateway가 body/헤더를 직접 검증
  - `Pass-through` = body 검증 없이 하위 서비스로 전달
- Response:
  - `Pass-through` = 하위 서비스 응답 원문 전달
  - `GW 생성` = Gateway가 직접 생성

### 7.1 Public/Infra

| Method | Path | Request | Response |
|---|---|---|---|
| GET | `/` | 없음 | Pass-through (swagger-ui index) |
| GET | `/index.html` | 없음 | Pass-through |
| GET | `/api/ping/**` | 없음 | Pass-through |
| GET | `/v3/api-docs/**` | 없음 | Pass-through |
| GET | `/swagger-ui.html` | 없음 | Pass-through |
| GET | `/swagger-ui/**` | 없음 | Pass-through |
| GET | `/v2/docs` | 없음 | Pass-through |
| GET | `/v2/docs/**` | 없음 | Pass-through |
| GET | `/v2/swagger-ui/index.html` | 없음 | Pass-through |
| GET | `/v2/swagger-ui/**` | 없음 | Pass-through |
| GET | `/v2/post/v3/api-docs` | 없음 | Pass-through |
| GET | `/v2/post/v3/api-docs/**` | 없음 | Pass-through |
| GET | `/v2/report/v3/api-docs` | 없음 | Pass-through |
| GET | `/v2/report/v3/api-docs/**` | 없음 | Pass-through |
| GET | `/v2/auth/v3/api-docs` | 없음 | Pass-through |
| GET | `/v2/auth/v3/api-docs/**` | 없음 | Pass-through |
| GET | `/actuator/health` | 없음 | Pass-through |
| GET | `/actuator/health/**` | 없음 | Pass-through |
| GET | `/actuator/info` | 없음 | Pass-through |
| POST | `/internal/v1/cache/invalidation` | GW 검증 (`X-Internal-Token`, `eventType`, `subject`) | `202 {"invalidatedKeys":number}` or `403` |

### 7.2 Auth/User/Admin (v1)

| Method | Path | Request | Response |
|---|---|---|---|
| POST | `/v1/auth/login` | GW 검증 (`username`, `password`) | Pass-through (`400` GW 가능) |
| POST | `/v1/auth/refresh` | GW 검증 (`refreshToken`) | Pass-through (`400/401` GW 가능) |
| POST | `/v1/auth/logout` | GW 검증 (`refreshToken`) | Pass-through (`400/401` GW 가능) |
| POST | `/activate` | Pass-through | Pass-through |
| GET | `/v1/me` | Pass-through (Bearer 필요) | Pass-through |
| POST | `/v1/me` | Pass-through (Bearer 필요) | Pass-through |
| PATCH | `/v1/me` | Pass-through (Bearer 필요) | Pass-through |
| POST | `/v1/me/password` | Pass-through (Bearer 필요) | Pass-through |
| GET | `/v1/admin/ping` | Pass-through (ADMIN) | Pass-through |
| GET | `/v1/admin/users` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/users` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/invite-mail` | Pass-through (ADMIN) | Pass-through |
| PATCH | `/v1/admin/users/role` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/users/{id}/reset-password` | Pass-through (ADMIN) | Pass-through |
| DELETE | `/v1/admin/users` | Pass-through (ADMIN) | Pass-through |
| DELETE | `/v1/admin/users/{id}` | Pass-through (ADMIN) | Pass-through |

### 7.3 Auth/Admin Alias (v2)

| Method | Path | Rewrite | Request | Response |
|---|---|---|---|---|
| POST | `/v2/auth/login` | `/v1/auth/login` | GW 검증 (`username`, `password`) | Pass-through (`400` GW 가능) |
| POST | `/v2/auth/refresh` | `/v1/auth/refresh` | GW 검증 (`refreshToken`) | Pass-through (`400/401` GW 가능) |
| POST | `/v2/auth/logout` | `/v1/auth/logout` | GW 검증 (`refreshToken`) | Pass-through (`400/401` GW 가능) |
| GET | `/v2/auth/me` | `/v1/me` | Pass-through (Bearer) | Pass-through |
| GET | `/v2/auth/admin/ping` | `/v1/admin/ping` | Pass-through (ADMIN) | Pass-through |
| GET | `/v2/auth/admin/users` | `/v1/admin/users` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/auth/admin/users` | `/v1/admin/users` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/auth/admin/invite-mail` | `/v1/admin/invite-mail` | Pass-through (ADMIN) | Pass-through |
| PATCH | `/v2/auth/admin/users/role` | `/v1/admin/users/role` | Pass-through (ADMIN) | Pass-through |
| DELETE | `/v2/auth/admin/users` | `/v1/admin/users` | Pass-through (ADMIN) | Pass-through |
| DELETE | `/v2/auth/admin/users/{id}` | `/v1/admin/users/{id}` | Pass-through (ADMIN) | Pass-through |

### 7.4 Report (v1 + v2)

| Method | Path | Rewrite | Request | Response |
|---|---|---|---|---|
| GET | `/v1/report` | `/api/report` | Pass-through (Bearer) | Pass-through |
| POST | `/v1/report` | `/api/report` | Pass-through (Bearer) | Pass-through |
| GET | `/v1/report/allReport` | `/api/report/allReport` | Pass-through (Bearer) | Pass-through |
| GET | `/v1/report/{id}` | `/api/report/{id}` | Pass-through (Bearer) | Pass-through |
| PUT | `/v1/report/{id}` | `/api/report/{id}` | Pass-through (Bearer) | Pass-through |
| DELETE | `/v1/report/{id}` | `/api/report/{id}` | Pass-through (Bearer) | Pass-through |
| GET | `/v2/report` | `/api/report` | Pass-through (Bearer) | Pass-through |
| POST | `/v2/report` | `/api/report` | Pass-through (Bearer) | Pass-through |
| GET | `/v2/report/allReport` | `/allReport` (stripPrefix) | Pass-through | Pass-through |
| GET | `/v2/report/{id}` | `/{id}` (stripPrefix) | Pass-through | Pass-through |
| PUT | `/v2/report/{id}` | `/{id}` (stripPrefix) | Pass-through | Pass-through |
| DELETE | `/v2/report/{id}` | `/{id}` (stripPrefix) | Pass-through | Pass-through |

### 7.5 Courses/Admin Courses (v1)

| Method | Path | Request | Response |
|---|---|---|---|
| POST | `/v1/admin/courses` | Pass-through (ADMIN) | Pass-through |
| DELETE | `/v1/admin/courses/{courseSlug}` | Pass-through (ADMIN) | Pass-through |
| PATCH | `/v1/admin/courses/{courseSlug}` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/courses/{courseSlug}/weeks` | Pass-through (ADMIN) | Pass-through |
| GET | `/v1/admin/courses/{courseSlug}/enrollments` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/courses/{courseSlug}/enrollments` | Pass-through (ADMIN) | Pass-through |
| PATCH | `/v1/admin/courses/{courseSlug}/enrollments/{userId}` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/courses/{courseSlug}/assignments` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/courses/{courseSlug}/assignments/{assignmentId}/publish` | Pass-through (ADMIN) | Pass-through |
| GET | `/v1/admin/courses/{courseSlug}/assignments/{assignmentId}/deliveries` | Pass-through (ADMIN) | Pass-through |
| POST | `/v1/admin/courses/{courseSlug}/assignments/{assignmentId}/deliveries` | Pass-through (ADMIN) | Pass-through |
| GET | `/v1/courses` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v1/courses/{courseSlug}` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v1/courses/{courseSlug}/weeks` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v1/courses/{courseSlug}/weeks/{weekNo}/assignments` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v1/courses/{courseSlug}/assignments` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v1/courses/{courseSlug}/assignments/{assignmentId}` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v1/courses/assignments/{assignmentId}/course` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |

### 7.6 Courses/Admin Courses Alias (v2/post prefix)

| Method | Path | Rewrite | Request | Response |
|---|---|---|---|---|
| POST | `/v2/post/admin/courses` | `/v1/admin/courses` | Pass-through (ADMIN) | Pass-through |
| DELETE | `/v2/post/admin/courses/{courseSlug}` | `/v1/admin/courses/{courseSlug}` | Pass-through (ADMIN) | Pass-through |
| PATCH | `/v2/post/admin/courses/{courseSlug}` | `/v1/admin/courses/{courseSlug}` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/post/admin/courses/{courseSlug}/weeks` | `/v1/admin/courses/{courseSlug}/weeks` | Pass-through (ADMIN) | Pass-through |
| GET | `/v2/post/admin/courses/{courseSlug}/enrollments` | `/v1/admin/courses/{courseSlug}/enrollments` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/post/admin/courses/{courseSlug}/enrollments` | `/v1/admin/courses/{courseSlug}/enrollments` | Pass-through (ADMIN) | Pass-through |
| PATCH | `/v2/post/admin/courses/{courseSlug}/enrollments/{userId}` | `/v1/admin/courses/{courseSlug}/enrollments/{userId}` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/post/admin/courses/{courseSlug}/assignments` | `/v1/admin/courses/{courseSlug}/assignments` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/post/admin/courses/{courseSlug}/assignments/{assignmentId}/publish` | `/v1/admin/courses/{courseSlug}/assignments/{assignmentId}/publish` | Pass-through (ADMIN) | Pass-through |
| GET | `/v2/post/admin/courses/{courseSlug}/assignments/{assignmentId}/deliveries` | `/v1/admin/courses/{courseSlug}/assignments/{assignmentId}/deliveries` | Pass-through (ADMIN) | Pass-through |
| POST | `/v2/post/admin/courses/{courseSlug}/assignments/{assignmentId}/deliveries` | `/v1/admin/courses/{courseSlug}/assignments/{assignmentId}/deliveries` | Pass-through (ADMIN) | Pass-through |
| GET | `/v2/post/courses` | `/v1/courses` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v2/post/courses/{courseSlug}` | `/v1/courses/{courseSlug}` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v2/post/courses/{courseSlug}/weeks` | `/v1/courses/{courseSlug}/weeks` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v2/post/courses/{courseSlug}/weeks/{weekNo}/assignments` | `/v1/courses/{courseSlug}/weeks/{weekNo}/assignments` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v2/post/courses/{courseSlug}/assignments` | `/v1/courses/{courseSlug}/assignments` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v2/post/courses/{courseSlug}/assignments/{assignmentId}` | `/v1/courses/{courseSlug}/assignments/{assignmentId}` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |
| GET | `/v2/post/courses/assignments/{assignmentId}/course` | `/v1/courses/assignments/{assignmentId}/course` | Pass-through (USER/ORGANIZER/ADMIN) | Pass-through |

### 7.7 Blog/Post

| Method | Path | Rewrite | Request | Response |
|---|---|---|---|---|
| GET | `/v1/posts` | - | Pass-through (Public) | Pass-through |
| GET | `/v1/posts/drafts` | - | Pass-through (ORGANIZER/ADMIN) | Pass-through |
| POST | `/v1/posts` | - | Pass-through (ORGANIZER/ADMIN, multipart 허용) | Pass-through |
| GET | `/v1/posts/{postId}` | - | Pass-through (Public) | Pass-through |
| PATCH | `/v1/posts/{postId}` | - | Pass-through (ORGANIZER/ADMIN) | Pass-through |
| DELETE | `/v1/posts/{postId}` | - | Pass-through (ORGANIZER/ADMIN) | Pass-through |
| POST | `/v1/posts/images` | - | Pass-through (ORGANIZER/ADMIN, multipart 허용) | Pass-through |
| GET | `/v2/post` | `/v1/posts` | Pass-through (Public) | Pass-through |
| GET | `/v2/post/drafts` | `/v1/posts/drafts` | Pass-through (ORGANIZER/ADMIN) | Pass-through |
| POST | `/v2/post` | `/v1/posts` | Pass-through (ORGANIZER/ADMIN, multipart 허용) | Pass-through |
| GET | `/v2/post/{postId}` | `/v1/posts/{postId}` | Pass-through (Public) | Pass-through |
| PATCH | `/v2/post/{postId}` | `/v1/posts/{postId}` | Pass-through (ORGANIZER/ADMIN) | Pass-through |
| DELETE | `/v2/post/{postId}` | `/v1/posts/{postId}` | Pass-through (ORGANIZER/ADMIN) | Pass-through |
| POST | `/v2/post/images` | `/v1/posts/images` | Pass-through (ORGANIZER/ADMIN, multipart 허용) | Pass-through |

## 8. 코드 기준 비노출 경로 (주의)

아래는 하위 서비스 OpenAPI에 존재할 수 있으나, 현재 Gateway allowlist/route 기준으로는 **외부 비노출** 상태다.

- `POST /v1/posts/{postId}/collaborators`
- `GET /v1/posts/me`
- `GET /v1/posts/drafts/me`

필요 시 `GatewayRequestPolicyFilter` allowlist + `application.yaml` route를 함께 추가해야 한다.

## 9. 환경변수

- `AUTH_SERVICE_URI`
- `REPORT_SERVICE_URI`
- `POST_SERVICE_URI`
- `AUTH_ISSUER_URI`
- `AUTH_AUDIENCE`
- `AUTH_JWT_SECRET`
- `AUTH_JWT_CLOCK_SKEW_SECONDS`
- `INTERNAL_EVENT_TOKEN`
- `CORS_ALLOWED_ORIGIN_PATTERNS`
- `ENFORCE_HTTPS`
- `ALLOWED_HOSTS`
- `ALLOW_PRIVATE_IP_HOST`
- `ENFORCE_METHOD_PATH_ALLOWLIST`
- `ENFORCE_JSON_CONTENT_TYPE`
- `PREVALIDATE_REFRESH_TOKEN_TYPE`
- `AUTH_RATE_LIMIT_ENABLED`
- `AUTH_LOGIN_RATE_LIMIT_PER_MINUTE`
- `AUTH_REFRESH_RATE_LIMIT_PER_MINUTE`
- `AUTH_LOGOUT_RATE_LIMIT_PER_MINUTE`
- `MAX_REQUEST_BODY_SIZE`
- `MAX_REQUEST_HEADER_SIZE`
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`
