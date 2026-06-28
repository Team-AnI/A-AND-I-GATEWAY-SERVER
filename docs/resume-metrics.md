# Gateway Resume Metrics

> 이 문서는 운영 지표가 아니라 이력서에 사용할 수 있는 검증 근거를 정리합니다.
>
> 로컬 측정은 반드시 "로컬 Mock Downstream 기준"이라고 표현하며, 운영 최대 처리량이나 운영 트래픽 처리량으로 쓰지 않습니다.

| 영역 | 수치 | 신뢰도 | 근거 파일 | 이력서 문장 | 사용 여부 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 기존 Direct vs Gateway baseline | Direct P95 `56.959ms`, Gateway P95 `65.357ms`, 추가 P95 `8.399ms`, HTTP 실패율 `0.00%`, check 성공률 `100.00%` | 확인 완료 | [docs/PERFORMANCE.md](./PERFORMANCE.md), [2026-06-20-gateway-local-check.json](./performance/data/2026-06-20-gateway-local-check.json) | 로컬 Mock Downstream 기준 Direct/Gateway P95 차이를 측정해 Gateway 라우팅·정책·로깅 계층의 회귀 기준을 관리 | 사용 가능 |
| payload별 Gateway overhead | payload 1KB 추가 P95 `10.906ms`, 64KB 추가 P95 `8.301ms`, 1MB 추가 P95 `8.565ms`; 각 3회 반복, HTTP 실패율 `0.00%`, check 성공률 `100.00%` | 확인 완료 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | 로컬 Mock Downstream 기준 payload 1KB/64KB/1MB별 Gateway P95/P99와 추가 지연을 측정해 라우팅·정책·로깅 계층 회귀 기준을 관리 | 사용 가능 |
| route 유형별 Gateway overhead | public route 추가 P95 `6.639ms`, protected route 추가 P95 `9.853ms`; 각 3회 반복, HTTP 실패율 `0.00%`, check 성공률 `100.00%` | 확인 완료 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | 로컬 Mock Downstream 기준 public/protected route의 Gateway 추가 지연을 측정해 라우팅·인증 계층 회귀 기준을 관리 | 사용 가능 |
| 구조화 로깅 on/off overhead | logging enabled 추가 P95 `10.850ms`, logging disabled 추가 P95 `9.512ms`; 각 3회 반복 | 확인 완료 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | 구조화 로깅 on/off 비교로 Gateway 요청 추적 기능의 지연 비용을 로컬 기준으로 검증 | 사용 가능 |
| rate limit/error contract 성능 | rate limit 12건 중 예상대로 10건 허용/2건 차단, downstream failure 오류 계약 check 성공률 `100.00%` | 확인 완료 | [2026-06-28-gateway-local-overhead.json](./performance/data/2026-06-28-gateway-local-overhead.json) | 로컬 Mock Downstream 기반 k6 시나리오로 Gateway rate limit과 downstream failure 오류 계약을 검증 | 사용 가능 |

## 사용 기준

- 확인 완료: 코드, 테스트, CI 결과, k6 결과, 문서로 확인 가능
- 측정 필요: 측정 방법은 있지만 아직 실행 결과 없음
- 확인 필요: 레포만 보고 판단 불가
- 사용 비추천: 운영 영향 위험, 측정 부족, 또는 과장 가능성이 있음

## PR 3 상태

2026-06-28 기준 추가 overhead runner를 로컬 Docker Compose Mock Downstream에서 실행했고 7개 측정 그룹이 각각 3회 direct/gateway pair를 확보했습니다.

- `measurementStatus`: `확인 완료`
- `acceptedPairCount`: `21`
- `missingGroups`: `[]`
- `blockingReasons`: `[]`
- 운영 최대 처리량이 아니라 로컬 Mock Downstream 기반 회귀 검증 기준입니다.
