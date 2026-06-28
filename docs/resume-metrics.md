# Gateway Resume Metrics

> 이 문서는 운영 지표가 아니라 이력서에 사용할 수 있는 검증 근거를 정리합니다.
>
> 로컬 측정은 반드시 "로컬 Mock Downstream 기준"이라고 표현하며, 운영 최대 처리량이나 운영 트래픽 처리량으로 쓰지 않습니다.

| 영역 | 수치 | 신뢰도 | 근거 파일 | 이력서 문장 | 사용 여부 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 기존 Direct vs Gateway baseline | Direct P95 `56.959ms`, Gateway P95 `65.357ms`, 추가 P95 `8.399ms`, HTTP 실패율 `0.00%`, check 성공률 `100.00%` | 확인 완료 | [docs/PERFORMANCE.md](./PERFORMANCE.md), [2026-06-20-gateway-local-check.json](./performance/data/2026-06-20-gateway-local-check.json) | 로컬 Mock Downstream 기준 Direct/Gateway P95 차이를 측정해 Gateway 라우팅·정책·로깅 계층의 회귀 기준을 관리 | 사용 가능 |
| payload별 Gateway overhead | `[측정 필요]` 1KB/64KB/1MB별 Direct/Gateway P50/P90/P95/P99, 추가 P95/P99 | 측정 필요 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | `[측정 필요]` 로컬 Mock Downstream 기준 payload 1KB/64KB/1MB별 Gateway P95/P99와 추가 지연을 측정해 라우팅·정책·로깅 계층 회귀 기준을 관리 | 사용 비추천 |
| route 유형별 Gateway overhead | `[측정 필요]` public route, protected route, JWT filter 경유 route 추가 지연 | 측정 필요 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | `[측정 필요]` Mock Downstream 기반 k6 시나리오로 Gateway 라우팅·인증·오류 계약의 성능 회귀를 검증 | 사용 비추천 |
| 구조화 로깅 on/off overhead | `[측정 필요]` logging enabled/disabled P95/P99 추가 지연 | 측정 필요 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | `[측정 필요]` 구조화 로깅 on/off 비교로 Gateway 요청 추적 기능의 지연 비용을 로컬 기준으로 검증 | 사용 비추천 |
| rate limit/error contract 성능 | `[측정 필요]` 기대 차단 수, 실제 차단 수, downstream failure 오류 계약 유지 여부 | 측정 필요 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | `[측정 필요]` Mock Downstream 기반 k6 시나리오로 Gateway rate limit과 downstream failure 오류 계약을 로컬에서 검증 | 사용 비추천 |

## 사용 기준

- 확인 완료: 코드, 테스트, CI 결과, k6 결과, 문서로 확인 가능
- 측정 필요: 측정 방법은 있지만 아직 실행 결과 없음
- 확인 필요: 레포만 보고 판단 불가
- 사용 비추천: 운영 영향 위험, 측정 부족, 또는 과장 가능성이 있음

## PR 3 상태

2026-06-28 기준 추가 overhead runner와 집계 스크립트는 추가했지만 실제 측정은 실행하지 않았습니다.

- Docker daemon이 실행 중이 아니어서 로컬 Docker Compose Mock Downstream을 시작할 수 없었습니다.
- 설치된 k6는 `k6 v2.0.0 (commit/devel, go1.26.3, darwin/arm64)`이고, 이 레포의 성능 검증 기준은 공식 `v1.7.1`입니다.
- 따라서 PR 3 신규 수치는 모두 `[측정 필요]`이며 이력서 사용은 비추천합니다.
