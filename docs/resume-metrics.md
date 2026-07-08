# Resume Metrics

## Gateway CI/CD

Source of truth: [`docs/metrics/gateway-cicd-remeasure.json`](./metrics/gateway-cicd-remeasure.json)

Current status: `completed`

Only metrics with `resume_usage == "사용 가능"` in the completed JSON are listed as sentence candidates.

## Resume Sentence Candidates

- 같은 검증 범위 기준으로 GitHub Actions CI를 backend-test-and-jar, monitor-bot-test, performance-assets job으로 분리해 same-scope CI critical path를 median 10.317s에서 9.973s로 3.33% 단축
- GitHub Actions에서 Gateway backend test median을 9.766s에서 9.307s로 4.7% 단축
- GitHub Actions에서 performance asset validation과 k6 inspect 검증 median을 0.807s에서 0.709s로 12.14% 단축
- 같은 runner에서 Gateway test와 bootJar를 통합해 Build JAR same-scope median을 10.317s에서 9.973s로 3.33% 단축

## Rejected Gateway CI/CD Metrics

Monitor Bot test는 median이 `0.308s`에서 `0.31s`로 악화되어 단독 resume 문장으로 쓰지 않는다. CD image build only 계열도 모두 개선이 없어 resume 문장 후보에서 제외한다.

| Metric | Resume usage | Reason |
| :--- | :--- | :--- |
| ci_full_gate_total | 참고 전용 | Full-gate includes artifact upload/download and summary verification, so do not describe it as PR validation overall time reduction. |
| monitor_bot_test | 개선 없음 | `cd monitor-bot && go test ./...` median moved from 0.308s to 0.31s (-0.65%) even though Go cache hit evidence was true. |
| cd_dry_run_full_path | 개선 없음 | Dry-run full path median stayed 12.053s to 12.053s (0.0%); this is not production deploy timing. |
| gateway_image_build_only | 개선 없음 | 운영 배포가 아닌 CD dry-run image build 단계에서 median moved from 0.902s to 1.635s (-81.26%). |
| monitor_bot_image_build_only | 개선 없음 | 운영 배포가 아닌 CD dry-run image build 단계에서 median moved from 0.313s to 1.042s (-232.91%). |
| image_build_warm_cache | 개선 없음 | BuildKit cache hit evidence was true, but median moved from 5.264s to 5.332s (-1.29%), so no cache-hit speedup claim is allowed. |

## 확인필요 처리 결과

The checks below were reviewed on `2026-07-08 KST`. Values from recent workflow history are reference-only unless they match the completed same-scope before/after measurement.

| 항목 | 처리 | 근거 | 이력서 사용 |
| :--- | :--- | :--- | :--- |
| Recent Gradle cache | 확인됨, 참고용 | Recent CI logs showed `setup-java` Gradle cache hit on 9/10 checked runs. | 단독 개선 claim 제외 |
| Recent Go cache | 확인됨, 참고용 | Recent CI logs showed `setup-go-cache-hit=true` on 9/10 checked runs. | Monitor Bot test median이 악화되어 단독 claim 제외 |
| Recent k6 cache | 확인됨, 참고용 | Recent CI logs showed `k6-cache-hit=true` on 9/10 checked runs. | `performance_assets` same-scope metric의 보조 근거로만 사용 |
| Docker BuildKit cache | 확인됨, 이력서 제외 | Completed JSON recorded BuildKit cache hit evidence as `true` for dry-run/image rows. | image build median이 악화되어 speedup claim 제외 |
| Docker build/push duration | 참고용, 이력서 제외 | Recent CD median: Gateway build/push `24.5s`, Monitor Bot build/push `38.0s`. | before/after same-scope 비교 없음 |
| Production deploy duration | 참고용, 이력서 제외 | Recent CD `Deploy to EC2 via SSH` median `44.5s`. | before/after same-scope 비교 없음 |
| CD Dry Run duration | 참고용, 이력서 제외 | Recent CD Dry Run workflow median `158.5s`. | production CD와 scope가 달라 비교 금지 |

## 남은 [확인 필요]

Current docs have no unresolved resume-safe CI/CD duration metric. New claims for production deploy reduction, Docker push reduction, or Docker layer cache speedup require a new same-scope before/after measurement and must stay out of resume bullets until then.
