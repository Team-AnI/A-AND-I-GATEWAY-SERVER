# PR Cleanup Plan

## Kept

- #44: final Gateway CI/CD same-scope remeasurement PR. It contains the completed 5-run source of truth at `docs/metrics/gateway-cicd-remeasure.json`, the measurement workflow, updated CI workflow, and final docs.

## Closed as superseded

- #43: superseded by #44 because it carried incomplete or blocked CI/CD before/after measurement artifacts, including old `gateway-cicd-before-after` style metrics and separate CD dry-run docs.
- #42: superseded by #44 because it was an earlier CI parallelization timing draft and did not contain the completed same-scope official measurement.
- #39: superseded by #44 because it was an earlier GitHub Actions metrics collector draft and its summary artifacts were replaced by the completed #44 measurement policy and JSON.

Each closed PR received the superseded comment that points to #44 as the completed 5-run remeasurement PR and states that no production deploy, AWS, ECR, SSH, or docker push was executed.

## Left Open

- #18: left open because it is a refresh response security PR and is unrelated to Gateway CI/CD remeasurement.
- #40: left open because it is an observability/error contract metrics PR, not a superseded CI/CD measurement PR.
- #41: left open because it is a performance overhead scenario PR, not a superseded CI/CD measurement PR.

Open PR diff review confirmed these retained PRs do not duplicate the #44 CI/CD remeasurement artifact set.

## Artifact Cleanup

- `docs/metrics/gateway-cicd-remeasure.json` remains the only Gateway CI/CD remeasurement source of truth in #44.
- No old blocked, attempt, or PR #43 measurement JSON exists in the #44 branch, so no archive move was required.
- No `docs/metrics/archive/` directory is required in #44 because there are no stale Gateway CI/CD measurement JSON files to preserve outside the completed source of truth.
- If an archived measurement artifact is added later, mark it as superseded by `docs/metrics/gateway-cicd-remeasure.json`.

## Legacy Files Not Carried Forward

The following files existed in earlier CI/CD timing branches and are intentionally not carried forward because they are replaced by `docs/metrics/gateway-cicd-remeasure.json` and the same-scope audit docs:

- `docs/metrics/gateway-ci-before.json`
- `docs/metrics/gateway-ci-after.json`
- `docs/metrics/gateway-cd-before.json`
- `docs/metrics/gateway-cd-dry-run-after.json`
- `docs/metrics/gateway-cicd-before-after.json`
- `docs/metrics/gateway-cicd-before-after.example.json`
- `scripts/ci/collect_github_actions_metrics.py`
- `scripts/ci/compare_cicd_before_after.py`

Do not reintroduce those files as active evidence. If they are needed for historical context later, put them under an archive path and mark them superseded by the completed remeasurement JSON.

## Notes

- Closed PRs were not merged.
- No branch deletion was performed.
- #44 contains the completed metrics source of truth.
- No production deploy, AWS, ECR, SSH, docker push, or operating workflow was executed for this cleanup.
- README was not regenerated; only small additive references should be made when linking these docs.
