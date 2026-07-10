# Service Gateway Integration Guide

기준일: 2026-07-10

코드 기준: `application.yaml`, `SecurityConfig.kt`, `GatewayRequestPolicyFilter.kt`

이 문서는 Auth, Report, Post, Online Judge 서비스가 Gateway 뒤에서 구현해야 할 현재 런타임 계약을 정리한다. 라우트를 변경할 때는 이 문서만 수정하지 말고 위 세 코드와 테스트를 함께 변경한다.

## 1. 공통 요청 계약

### 외부 경계

- 외부 클라이언트는 서비스에 직접 접근하지 않고 Gateway를 통해 접근한다.
- 서비스는 내부망이나 보안 그룹으로 Gateway에서 오는 연결만 허용해야 한다.
- Gateway 인증은 기본적으로 활성화된다. `gateway.auth.enabled=false`인 환경에서는 모든 보안 라우트가 `permitAll`이 되므로 운영 서비스에서 사용하면 안 된다.

### 경로 전달과 rewrite

Gateway에는 전역 `StripPrefix`가 없다. 다음 OpenAPI 예외를 제외하면 외부 경로를 그대로 downstream에 전달한다.

| 외부 경로 | downstream 경로 |
| --- | --- |
| `/v2/auth/v3/api-docs` | `/v3/api-docs` |
| `/v2/auth/v3/api-docs/{segment...}` | `/v3/api-docs/{segment...}` |
| `/v2/report/v3/api-docs` | `/v3/api-docs` |
| `/v2/report/v3/api-docs/{segment...}` | `/v3/api-docs/{segment...}` |
| `/v2/post/v3/api-docs` | `/v3/api-docs` |
| `/v2/post/v3/api-docs/{segment...}` | `/v3/api-docs/{segment...}` |
| `/v2/online-judge/v3/api-docs` | `/v3/api-docs` |
| `/v2/online-judge/v3/api-docs/{segment...}` | `/v3/api-docs/{segment...}` |

Report OpenAPI 라우트는 일반 `/v2/report/**` 라우트보다 뒤에 선언되어 있지만 root `order=-2`, subpath `order=-1`로 먼저 평가된다. 이 우선순위를 제거하면 rewrite 없이 원래 경로가 전달되므로 route 변경 시 함께 검증해야 한다.

OpenAPI 라우트는 `GET`만 허용한다. 특히 Report의 광범위한 `/v2/report/**` mutation 정책이 OpenAPI 경로까지 확장되지 않도록 다른 method는 명시적으로 거부한다.

`/v1/report` 라우트의 `SetPath`/`RewritePath`는 같은 `/v1/report...` 경로로 다시 쓰므로, 서비스에 보이는 경로는 변하지 않는다.

예:

```text
GET /v2/report/articles?course=spring
 -> REPORT_SERVICE_URI/v2/report/articles?course=spring

POST /v2/online-judge/submissions
 -> ONLINE_JUDGE_SERVICE_URI/v2/online-judge/submissions
```

### 메서드·호스트·본문 정책

라우트의 `Method` predicate가 없더라도 모든 메서드가 허용되는 것은 아니다. `GatewayRequestPolicyFilter` 허용 목록에 없는 method/path 조합은 라우팅 전에 거부된다.

- `OPTIONS /**`는 정책 필터와 보안 필터에서 허용한다.
- `ENFORCE_HTTPS=true`면 TLS 연결이거나 `X-Forwarded-Proto: https`인 요청만 허용한다.
- `ALLOWED_HOSTS`가 비어 있지 않으면 허용된 host만 받는다. `ALLOW_PRIVATE_IP_HOST=true`면 loopback/private IP host도 허용한다.
- `POST`, `PUT`, `PATCH`는 기본적으로 `application/json` 또는 `+json` Content-Type이 필요하다.
- JSON Content-Type 검사 예외는 `/v1/me`, `/v1/posts...`, `/v2/me`, `/v2/post...`, `/v2/posts...`, `/v2/blogs...`, `/v2/lectures...` 중 코드에 명시된 root/item/image 경로다. 멀티파트 등을 받는 서비스는 해당 예외 경로인지 확인해야 한다.
- 요청 본문 크기는 기본 2 MiB로 제한되며 `MAX_REQUEST_BODY_SIZE`로 변경한다.

## 2. 인증과 인가

### Access Token

Gateway는 `Authorization: Bearer <token>`을 먼저 해석하고, Bearer token을 얻지 못하면 `Authenticate: Bearer <token>`을 사용한다. Access Token은 다음 조건을 모두 만족해야 한다.

- HS256 서명 검증, `exp`/`nbf`가 있으면 clock skew를 적용한 시간 검증
- 설정된 `issuer` 및 `audience`
- `token_type=ACCESS`
- UUID 형식의 `sub`
- `USER`, `ORGANIZER`, `ADMIN` 중 하나인 `role`
- 빈 값이 아닌 `jti`
- 현재 시간보다 허용 clock skew 이상 미래가 아닌 `iat`

권한 계층은 다음과 같다.

| `role` claim | Gateway authority | 이 문서 표기 |
| --- | --- | --- |
| `USER` | `ROLE_USER` | USER+ |
| `ORGANIZER` | `ROLE_ORGANIZER,ROLE_USER` | ORGANIZER+ |
| `ADMIN` | `ROLE_ADMIN,ROLE_ORGANIZER,ROLE_USER` | ADMIN |

`USER+`는 유효한 세 역할 모두, `ORGANIZER+`는 ORGANIZER와 ADMIN을 의미한다. 아래 표에서 별도로 표시하지 않은 downstream 도메인 소유권은 각 서비스가 다시 검증해야 한다.

### downstream identity header

Gateway는 클라이언트가 보낸 다음 헤더를 먼저 삭제한다.

- `X-User-Id`
- `X-Roles`
- `X-Auth-Context`
- `X-Auth-Context-Cache`

인증된 요청에는 Gateway가 다음 두 헤더를 새로 설정한다.

| 헤더 | 값 |
| --- | --- |
| `X-User-Id` | JWT `sub` UUID |
| `X-Roles` | Gateway authority를 comma로 연결한 값. 예: ADMIN은 `ROLE_ADMIN,ROLE_ORGANIZER,ROLE_USER` |

공개 endpoint에 token 없이 접근하면 두 헤더는 downstream에 존재하지 않는다. `X-Auth-Context*`는 현재 downstream identity 계약이 아니며 Gateway가 외부 입력을 삭제한다.

서비스는 위 헤더를 신뢰할 수 있는 네트워크 경계를 유지하고 다음을 구현해야 한다.

- `X-User-Id`를 UUID로 파싱하되, 클라이언트 직접 접근에서 받은 같은 이름의 헤더는 신뢰하지 않는다.
- `X-Roles`를 comma로 분리하고 `ROLE_` prefix를 포함한 값으로 비교한다.
- Gateway 인가만으로 완료할 수 없는 리소스 소유권, 강의 등록 여부, 게시물 협업자 권한은 downstream에서 검증한다.

## 3. Auth Service

Target: `${AUTH_SERVICE_URI:http://localhost:9000}`

일반 API 경로: rewrite 없음

| Gateway route | method | Gateway 인가 |
| --- | --- | --- |
| `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout` | POST | 공개 |
| `/activate`, `/v2/activate` | POST | 공개 |
| `/v2/auth/login`, `/v2/auth/refresh`, `/v2/auth/logout` | POST | 공개 |
| `/v2/ping`, `/v2/ping/**` | GET | 공개 |
| `/v1/me` | GET, POST, PATCH | USER+ |
| `/v1/me/password` | POST | USER+ |
| `/v2/auth/me` | GET, POST, PATCH | USER+ |
| `/v2/auth/me/password` | POST | USER+ |
| `/v2/me` | GET, PATCH | USER+ |
| `/v2/me/profile-image/upload-url` | POST | USER+ |
| `/v2/me/password` | PATCH | USER+ |
| `/v1/admin/ping`, `/v1/admin/users...` | 코드에 명시된 GET/POST/PATCH/DELETE | ADMIN |
| `/v2/auth/admin/ping`, `/v2/auth/admin/users...` | 코드에 명시된 GET/POST/PATCH/DELETE | ADMIN |
| `/v2/admin/ping`, `/v2/admin/users...` | 코드에 명시된 GET/POST/PATCH/DELETE | ADMIN |
| `/v1/users`, `/v1/users/**` | GET, POST, PUT, PATCH, DELETE | USER+ |
| `/v2/auth/users`, `/v2/auth/users/**` | GET, POST, PUT, PATCH, DELETE | USER+ |
| `/v2/users/lookup` | GET | ORGANIZER+ |
| `/v2/auth/v3/api-docs`, `/v2/auth/v3/api-docs/**` | GET | 공개, `/v3/api-docs...`로 rewrite |

관리자 사용자 route에는 다음 세부 패턴이 포함된다.

- v1/v2 auth-prefixed: 목록, 생성, sync, invite-mail, role 변경, 사용자 수정·삭제, password reset
- v2 native: 목록, 생성, invite-mail, `/{id}/password/reset`, `/{id}/role`, `/{id}` 수정·삭제

## 4. Report Service

Target: `${REPORT_SERVICE_URI:http://localhost:8081}`

일반 API 경로: rewrite 없음

| Gateway route | 허용 메서드/세부 패턴 | Gateway 인가 |
| --- | --- | --- |
| `/v1/report`, `/v1/report/**`, `/v2/report`, `/v2/report/**` | GET, POST, PUT, PATCH, DELETE 중 method/path allowlist와 일치하는 조합 | USER+ |
| `/v1/admin/courses...` | 강의 root GET/POST, 수정·삭제, week 생성, enrollment/assignment 조회·변경 | ADMIN |
| `/v2/admin/courses...` | v1 관리 기능 + assignment copy POST + submission-statuses GET | ADMIN |
| `/v1/courses...` | 강의·outline·week·assignment GET, assignment submission POST/GET/stream GET | USER+ |
| `/v2/courses...`, `/v2/assignments/{assignmentId}/course` | 강의·outline·week·assignment GET | USER+ |
| `/v2/post/admin/courses...` | v1과 동일한 관리 계열; Post가 아닌 Report로 전달 | ADMIN |
| `/v2/post/courses...` | v1과 동일한 사용자 계열; Post가 아닌 Report로 전달 | USER+ |
| `/v2/report/v3/api-docs`, `/v2/report/v3/api-docs/**` | GET | 공개, `/v3/api-docs...`로 rewrite |

`/v2/admin/courses/{courseSlug}/assignments/copy`는 POST만 허용한다. 같은 경로의 GET, PATCH, DELETE는 명시적으로 거부된다.

`/v2/post/courses`는 Report alias의 GET endpoint다. Post의 `/v2/post/{postId}` 패턴과 겹치더라도 PATCH와 DELETE는 Report로 전달하지 않고 명시적으로 거부한다.

## 5. Post Service

Target: `${POST_SERVICE_URI:http://localhost:8084}`

일반 API 경로: rewrite 없음

| Gateway route | method | Gateway 인가 |
| --- | --- | --- |
| `/v1/posts` | GET | 공개 |
| `/v1/posts/{postId}` | GET | 공개 |
| `/v1/posts` | POST | ORGANIZER+ |
| `/v1/posts/{postId}` | PATCH, DELETE | ORGANIZER+ |
| `/v1/posts/drafts`, `/v1/posts/drafts/**` | GET | ORGANIZER+ |
| `/v1/posts/images` | POST | ORGANIZER+ |
| `/v2/post`, `/v2/post/{postId}` | GET | 공개 |
| `/v2/post` | POST | ORGANIZER+ |
| `/v2/post/{postId}` | PATCH, DELETE | ORGANIZER+ |
| `/v2/post/drafts`, `/v2/post/drafts/**` | GET | ORGANIZER+ |
| `/v2/post/images`, `/v2/post/images/**` | GET/POST/PUT/PATCH/DELETE가 정책 allowlist에 존재 | root GET은 공개, 모든 POST와 root PATCH/DELETE는 ORGANIZER+, 나머지는 USER+ fallback |
| `/v2/posts`, `/v2/posts/{postId}` | GET | USER+ |
| `/v2/posts` POST, `/v2/posts/{postId}` PATCH/DELETE | 해당 method | ORGANIZER+ |
| `/v2/posts/me`, `/v2/posts/scheduled/me`, `/v2/posts/drafts`, `/v2/posts/drafts/me` | GET | ORGANIZER+ |
| `/v2/posts/{postId}/collaborators`, `/v2/posts/images` | POST | ORGANIZER+ |
| `/v2/blogs`, `/v2/blogs/{blogId}` | GET | 공개 |
| `/v2/blogs` POST, `/v2/blogs/{blogId}` PATCH/DELETE | 해당 method | ORGANIZER+ |
| `/v2/blogs/me`, `/v2/blogs/scheduled/me`, `/v2/blogs/drafts`, `/v2/blogs/drafts/me` | GET | ORGANIZER+ |
| `/v2/lectures`, `/v2/lectures/{lectureId}` | GET | USER+ |
| `/v2/lectures` POST, `/v2/lectures/{lectureId}` PATCH/DELETE | 해당 method | ORGANIZER+ |
| `/v2/lectures/me`, `/v2/lectures/scheduled/me`, `/v2/lectures/drafts`, `/v2/lectures/drafts/me` | GET | ORGANIZER+ |
| `/v2/post/v3/api-docs`, `/v2/post/v3/api-docs/**` | GET | 공개, `/v3/api-docs...`로 rewrite |

현대화 route의 item pattern은 `*`이므로 `/v2/posts/{id}/...` 형태의 임의 하위 경로를 포괄하지 않는다. 협업자와 drafts처럼 명시된 경로만 예외다.

## 6. Online Judge Service

Target: `${ONLINE_JUDGE_SERVICE_URI:https://localhost:8080}`

일반 API 경로: rewrite 없음

| Gateway route | method | Gateway 인가 |
| --- | --- | --- |
| `/v1/submissions` | POST | USER+ |
| `/v1/problems/{problemId}/submissions/me` | GET | USER+ |
| `/v1/submissions/{submissionId}` | GET | USER+ |
| `/v1/submissions/{submissionId}/stream` | GET | USER+ |
| `/v1/admin/submissions`, `/v1/admin/testcases` | GET | ADMIN |
| `/v2/online-judge/submissions` | POST | USER+ |
| `/v2/online-judge/problems/{problemId}/submissions/me` | GET | USER+ |
| `/v2/online-judge/submissions/{submissionId}` | GET | USER+ |
| `/v2/online-judge/submissions/{submissionId}/stream` | GET | USER+ |
| `/v2/online-judge/admin/submissions`, `/v2/online-judge/admin/testcases` | GET | ADMIN |
| `/v2/submissions` | POST | USER+ |
| `/v2/problems/{problemId}/submissions/me` | GET | USER+ |
| `/v2/submissions/{submissionId}`, `/v2/submissions/{submissionId}/stream` | GET | USER+ |
| `/v2/admin/submissions`, `/v2/admin/testcases` | GET | ADMIN |
| `/v2/online-judge/v3/api-docs`, `/v2/online-judge/v3/api-docs/**` | GET | 공개, `/v3/api-docs...`로 rewrite |

`/v2/online-judge/...` 계열과 `/v2/...` native alias는 둘 다 존재하며, 각각의 외부 경로가 변경 없이 서비스에 전달된다. submission stream 라우트의 response timeout은 3,600,000 ms이고, 나머지 라우트의 기본 response timeout은 10초다.

## 7. 서비스 연동 체크리스트

- [ ] 서비스가 Gateway에서 전달된 원본 API 경로를 그대로 구현했는지 확인한다.
- [ ] OpenAPI endpoint만 `/v3/api-docs...`로 받는지 확인한다.
- [ ] `X-User-Id` UUID와 comma-separated `X-Roles`를 파싱한다.
- [ ] Gateway 이외의 경로로 들어온 identity header를 신뢰하지 않는다.
- [ ] 서비스에서 리소스 소유권과 도메인 세부 인가를 다시 검증한다.
- [ ] 서비스 직접 접근을 내부망/보안 그룹으로 차단한다.
- [ ] `AUTH_SERVICE_URI`, `REPORT_SERVICE_URI`, `POST_SERVICE_URI`, `ONLINE_JUDGE_SERVICE_URI`가 실제 서비스 주소를 가리키는지 확인한다.
- [ ] route 변경 시 `application.yaml`, method/path allowlist, `SecurityConfig.kt`, 보안 테스트와 이 문서를 함께 변경한다.
