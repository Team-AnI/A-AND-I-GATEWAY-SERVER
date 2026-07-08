# Gateway CI/CD Optimization

Source of truth: [`docs/metrics/gateway-cicd-remeasure.json`](./metrics/gateway-cicd-remeasure.json)

The official remeasurement is completed with `measurement_profile=official`.

- Measurement run: `28313731044`
- Candidate commit: `46e44f0bfa04542347ed8ed74421d65690d9ff7c`
- Measured at: `2026-06-28T06:27:34Z`
- Primary statistic: median seconds from 5 successful before runs and 5 successful after runs

| 항목 | Before median | After median | 개선율 | Run 수 | 신뢰도 | 사용 여부 |
| :--- | ---: | ---: | ---: | :--- | :--- | :--- |
| CI same-scope total | 10.317s | 9.973s | 3.33% | 5/5 | medium | 사용 가능 |
| CI full-gate total | 10.767s | 10.133s | 5.89% | 5/5 | medium | 참고 전용 |
| Backend test | 9.766s | 9.307s | 4.7% | 5/5 | medium | 사용 가능 |
| Monitor Bot test | 0.308s | 0.31s | -0.65% | 5/5 | medium | 개선 없음 |
| Performance assets | 0.807s | 0.709s | 12.14% | 5/5 | medium | 사용 가능 |
| Build JAR same-scope | 10.317s | 9.973s | 3.33% | 5/5 | medium | 사용 가능 |
| CD dry-run full path | 12.053s | 12.053s | 0.0% | 5/5 | medium | 개선 없음 |
| Gateway image build only | 0.902s | 1.635s | -81.26% | 5/5 | medium | 개선 없음 |
| Monitor Bot image build only | 0.313s | 1.042s | -232.91% | 5/5 | medium | 개선 없음 |
| Image build warm cache | 5.264s | 5.332s | -1.29% | 5/5 | medium | 개선 없음 |

## Cache Evidence

| 항목 | Go cache configured | Go cache hit observed | k6 cache configured | k6 cache hit observed | BuildKit cache configured | BuildKit cache hit observed |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| CI same-scope total | true | 확인 필요 | true | 확인 필요 | false | 확인 필요 |
| CI full-gate total | true | true | true | true | false | 확인 필요 |
| Backend test | false | 확인 필요 | false | 확인 필요 | false | 확인 필요 |
| Monitor Bot test | true | true | false | 확인 필요 | false | 확인 필요 |
| Performance assets | false | 확인 필요 | true | true | false | 확인 필요 |
| Build JAR same-scope | false | 확인 필요 | false | 확인 필요 | false | 확인 필요 |
| CD dry-run full path | true | true | false | 확인 필요 | true | true |
| Gateway image build only | false | 확인 필요 | false | 확인 필요 | true | true |
| Monitor Bot image build only | false | 확인 필요 | false | 확인 필요 | true | true |
| Image build warm cache | false | 확인 필요 | false | 확인 필요 | true | true |

Cache hit evidence is recorded from the completed JSON. Cache hits are not treated as speedup claims unless the matching median improved. The CD and image rows are dry-run or build-only paths with no production deploy, ECR push, docker push, or SSH step.

## Recent Run Reference

Recent successful workflow runs were checked on `2026-07-08 KST` with `gh run list`, `gh api repos/:owner/:repo/actions/runs/<run-id>/jobs`, and `gh run view <run-id> --log`.

| Workflow | Recent successful runs | Average duration | Median duration | Usage |
| :--- | ---: | ---: | ---: | :--- |
| CI | 10 | 58.9s | 46.0s | 참고용 |
| CD | 10 | 297.2s | 261.5s | 참고용 |
| CD Dry Run | 10 | 157.6s | 158.5s | 참고용, production deploy 아님 |
| Measure Gateway CI/CD Same Scope | 1 | 181.0s | 181.0s | measurement workflow wall-clock only |

Recent CI logs showed Gradle, Go, and k6 cache hits on 9 of the 10 checked runs. The 9 runs with all three cache hits had median workflow wall-clock `40.0s`; the remaining run was `118.0s`. These recent values are branch-mixed observations and are not used as before/after improvement metrics.
