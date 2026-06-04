# Report Assignment Copy Routing

## API

Report PR #50 adds the admin assignment copy API:

```text
POST /v2/admin/courses/{targetCourseSlug}/assignments/copy
```

Gateway route id:

```text
report-service-v2-admin-assignment-copy
```

The route forwards the original path to `REPORT_SERVICE_URI` without `RewritePath` or `SetPath`.

## Why An Explicit Allowlist Entry Is Needed

`application.yaml` already has a broad `REPORT_SERVICE_URI` route for `/v2/admin/courses/**`, but that is not enough when `GatewayRequestPolicyFilter` has `enforceMethodPathAllowlist=true`.

If `POST /v2/admin/courses/{courseSlug}/assignments/copy` is absent from `allowRules`, Gateway rejects the request with 404 before it reaches Report. This change adds only the POST v2 copy path. It does not open GET/PATCH/DELETE copy routes and does not add a legacy `/v2/post/admin/courses/**` alias.

## Security

`SecurityConfig` already protects `/v2/admin/courses` and `/v2/admin/courses/**` with ADMIN role.

Expected behavior:

- No token or expired token: 401
- USER/ORGANIZER role: 403
- ADMIN role: request passes Gateway policy and is forwarded to Report

## V2 Log Check

Report logs are expected in:

```text
/a-and-i/prod/report
```

Useful V2 fields:

- `service.name=report-service`
- `service.domainCode=4`
- `logType=API` or `API_ERROR`
- `trace.traceId`
- `http.method=POST`
- `http.path=/v2/admin/courses/<targetCourseSlug>/assignments/copy`
- `http.route=/v2/admin/courses/{targetCourseSlug}/assignments/copy`

## Smoke Test

```bash
curl -i -X POST "https://api.example.com/v2/admin/courses/<targetCourseSlug>/assignments/copy" \
  -H "Authorization: Bearer <ADMIN_ACCESS_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "sourceAssignmentId": "<SOURCE_ASSIGNMENT_UUID>",
    "targetWeekNo": 1,
    "targetOrderInWeek": 2,
    "targetStartAt": "2026-05-12T09:00:00+09:00",
    "targetEndAt": "2026-05-19T08:59:59+09:00"
  }'
```

Response guide:

- 2xx: 정상
- 401: 토큰 없음 또는 만료
- 403: 관리자 권한 없음
- 404: Gateway allowlist 또는 Report controller path 문제
- 409: 중복 방어 정상 가능성
- 502/504: Gateway에서 Report 서버 연결 문제

## Deployment Note

This change does not deploy Gateway, restart EC2 containers, or create a new tag. The currently running Gateway v2.0.13 container is unaffected until a separately approved manual deployment happens.
