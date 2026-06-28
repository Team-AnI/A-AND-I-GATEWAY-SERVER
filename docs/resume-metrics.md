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
