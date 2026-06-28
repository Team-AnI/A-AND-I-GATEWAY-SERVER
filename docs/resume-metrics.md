# Resume Metrics

## Gateway CI/CD

Source of truth: [`docs/metrics/gateway-cicd-remeasure.json`](./metrics/gateway-cicd-remeasure.json)

Current status: `blocked_workflow_not_on_default_branch`

No Gateway CI/CD sentence candidate is available. The official measurement workflow could not be dispatched because it was not present on the default branch `main`, and the JSON contains no completed before/after samples.

Do not use the Gateway CI/CD metrics in resume wording until a completed official measurement file records:

- at least 5 successful before samples and 5 successful after samples
- zero failures for the claimed metric
- matching scope
- median improvement
- clear dry-run or image-only wording for CD-related metrics
- cache hit evidence before making any cache-hit-based claim

Rejected Gateway CI/CD metrics:

| Metric | Resume usage | Reason |
| :--- | :--- | :--- |
| ci_same_scope_total | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| ci_full_gate_total | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| backend_test | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| monitor_bot_test | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| performance_assets | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| build_jar_same_scope | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| cd_dry_run_full_path | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| gateway_image_build_only | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| monitor_bot_image_build_only | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
| image_build_warm_cache | 측정 실패 | Official measurement workflow could not be dispatched because it is not on the default branch. |
