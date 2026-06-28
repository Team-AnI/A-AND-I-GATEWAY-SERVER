# Gateway CI/CD Optimization

Source of truth: [`docs/metrics/gateway-cicd-remeasure.json`](./metrics/gateway-cicd-remeasure.json)

The official remeasurement is incomplete: `blocked_workflow_not_on_default_branch`.

No timing value or improvement percentage is available because the measurement workflow was not dispatchable from GitHub Actions at the time of the official attempt.

| 항목 | Before median | After median | 개선율 | Run 수 | 신뢰도 | 사용 여부 |
| :--- | ---: | ---: | ---: | :--- | :--- | :--- |
| CI same-scope total | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| CI full-gate total | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Backend test | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Monitor Bot test | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Performance assets | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Build JAR same-scope | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| CD dry-run full path | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Gateway image build only | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Monitor Bot image build only | N/A | N/A | N/A | 0/0 | low | 측정 실패 |
| Image build warm cache | N/A | N/A | N/A | 0/0 | low | 측정 실패 |

## Cache Evidence

| 항목 | Go cache configured | Go cache hit observed | BuildKit cache configured | BuildKit cache hit observed |
| :--- | :--- | :--- | :--- | :--- |
| CI same-scope total | true | 확인 필요 | false | 확인 필요 |
| CI full-gate total | true | 확인 필요 | false | 확인 필요 |
| Backend test | false | 확인 필요 | false | 확인 필요 |
| Monitor Bot test | true | 확인 필요 | false | 확인 필요 |
| Performance assets | false | 확인 필요 | false | 확인 필요 |
| Build JAR same-scope | false | 확인 필요 | false | 확인 필요 |
| CD dry-run full path | true | 확인 필요 | true | 확인 필요 |
| Gateway image build only | false | 확인 필요 | true | 확인 필요 |
| Monitor Bot image build only | false | 확인 필요 | true | 확인 필요 |
| Image build warm cache | false | 확인 필요 | true | 확인 필요 |

Cache hit evidence was not collected. Do not claim cache-hit-based speed improvement from this measurement file.
