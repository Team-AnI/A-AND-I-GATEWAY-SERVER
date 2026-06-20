# Gateway k6 Result Template

## Run Metadata

- Commit SHA:
- Date:
- Operator:
- Environment:
- Gateway URL:
- Mock Downstream URL:
- Public route:
- Protected route:
- Mock delay:
- Payload bytes:
- VUs:
- Duration:
- Repeat count:

## Existing Load Test State

- Existing performance files before this run:
- Existing Mock Server or WireMock:
- Notes:

## Separation Method

- Gateway downstream URI overrides:
- Mock Downstream container/image:
- Real Auth/User/Report/Blog/Online Judge traffic avoided:

## Direct Result

| Metric | Value |
| --- | ---: |
| P50 |  |
| P95 |  |
| P99 |  |
| RPS |  |
| HTTP failed rate |  |

## Gateway Result

| Metric | Value |
| --- | ---: |
| P50 |  |
| P95 |  |
| P99 |  |
| RPS |  |
| HTTP failed rate |  |

## Gateway Additional Latency

| Metric | Value |
| --- | ---: |
| P50 additional latency |  |
| P95 additional latency |  |
| P99 additional latency |  |

## Error Contract Result

| Check | Result | Notes |
| --- | --- | --- |
| 401 unauthenticated request |  |  |
| 403 insufficient role |  |  |
| 404 non-allowlisted route |  |  |
| Downstream 5xx common contract |  |  |
| 401/403 not classified as internal error |  |  |
| Content-Type and required fields |  |  |
| Authorization header not exposed |  |  |

## Rate Limit Result

| Check | Result | Notes |
| --- | --- | --- |
| Local-only environment |  |  |
| Actual key policy |  |  |
| 2xx responses before limit |  |  |
| 429 responses after limit |  |  |
| Retry-After or related headers |  |  |
| 429 excluded from unintended failure metric |  |  |
| Key cleanup |  | Current implementation uses in-memory auth rate limit keys. |

## Commands Executed

```bash

```

## Not Run

| Item | Reason |
| --- | --- |
|  |  |

## README Check

```bash
git diff -- README.md
```

Result:
