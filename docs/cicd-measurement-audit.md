# Gateway CI/CD Measurement Audit

## Gateway CI/CD Remeasurement Audit

Source of truth: [`docs/metrics/gateway-cicd-remeasure.json`](./metrics/gateway-cicd-remeasure.json)

Current status: `completed`

The official remeasurement completed with `measurement_profile=official` on run `28313731044` for candidate commit `46e44f0bfa04542347ed8ed74421d65690d9ff7c`. Each metric has 5 successful before samples and 5 successful after samples. Median seconds are the primary values used below.

Superseded attempts are preserved in the JSON and are not official measurements:

- `9a4ffd5`: dispatch failed because the workflow parser rejected job-level `runner.temp`.
- `28313639595` / `b7c1cf0`: workflow failed before writing measurement artifacts because the baseline checkout on `main` did not contain `scripts/ci/install-k6.sh`.

### PR #43 Measurement Risk

The previous PR #43 measurement is treated as unsuitable for resume wording or final performance claims. Its CI total worsened, and the Build JAR comparison could have compared a same-runner warm path with a separate cold runner path. That scope mismatch makes the old Build JAR number invalid for a same-scope improvement claim.

PR #44 also had a blocked attempt before this completed run. That blocked state was not official measurement data because it did not produce a completed 5-run before/after batch.

### Workflow Dispatch Unblock

The measurement workflow became dispatchable after removing the invalid job-level `runner.temp` expression and making the k6 install path available when jobs checked out the baseline ref. The completed run uses `.github/workflows/measure-gateway-cicd.yml` through `workflow_dispatch` with:

- `baseline_ref=main`
- `candidate_ref=develop-gateway-cicd-same-scope-remeasure`
- `iterations=5`
- `measurement_profile=official`

### Same-Scope Basis

The same-scope CI basis is:

- Gateway backend test
- Gateway JAR build on the same runner after backend test
- Monitor Bot `go test ./...`
- performance asset validation
- k6 inspect validation

The same-scope CI critical path improved from median `10.317s` to `9.973s` (`3.33%`) and is marked `사용 가능`.

### Full-Gate Basis

The full-gate basis is same-scope CI plus artifact upload/download and summary verification. It is not interchangeable with same-scope CI and must not be used to claim a narrower job-level improvement.

Full-gate median moved from `10.767s` to `10.133s` (`5.89%`), but the JSON marks it `참고 전용` because it is not a same-scope resume metric. Do not describe this as PR validation overall time reduction.

### Build JAR Cold Runner Issue

The corrected comparison removes the separate cold-runner `build-jar` path from the improvement claim. The candidate path measures `./gradlew test bootJar --build-cache --no-daemon` on the same runner, matching the test and JAR build scope.

Build JAR same-scope median improved from `10.317s` to `9.973s` (`3.33%`) with 5/5 successful before and after samples, so this metric is marked `사용 가능`.

### Monitor Bot Test Result

Monitor Bot test was measured with the same command on both sides:

```bash
cd monitor-bot && go test ./...
```

The candidate job used `actions/setup-go` cache and the completed JSON recorded Go cache hit evidence as `true`. Median moved from `0.308s` to `0.31s` (`-0.65%`), so this metric is `개선 없음` and must not be used as a resume sentence.

### CD Dry-Run Scope

CD dry-run is not a production deploy measurement. It is only a safe validation path for build and dry-run image construction with no AWS credentials, ECR login, docker push, EC2 SSH, or production URL access.

Legacy deploy and dry-run are different scopes and must not be directly compared. A production deploy path includes remote infrastructure steps that are intentionally absent from dry-run.

### Full Path vs Image-Only

CD dry-run full path means backend test and JAR build, Monitor Bot test, Gateway image build, and Monitor Bot image build with no deploy step.

Image-only metrics isolate Docker image construction:

- Gateway image build only uses the prebuilt Gateway JAR and builds the Gateway image without push.
- Monitor Bot image build only builds the Monitor Bot image without push.
- Image build warm cache is intended to exercise the BuildKit cache path without push.

Measured dry-run and image-only medians were:

| Metric | Before median | After median | 개선율 | Resume usage |
| :--- | ---: | ---: | ---: | :--- |
| CD dry-run full path | 12.053s | 12.053s | 0.0% | 개선 없음 |
| Gateway image build only | 0.902s | 1.635s | -81.26% | 개선 없음 |
| Monitor Bot image build only | 0.313s | 1.042s | -232.91% | 개선 없음 |
| Image build warm cache | 5.264s | 5.332s | -1.29% | 개선 없음 |

BuildKit cache was configured and hit evidence is `true` for the CD dry-run and image-only rows. The explicit warm-cache metric also recorded hit evidence as `true`, but its median worsened from `5.264s` to `5.332s`; this completed batch does not support a cold/warm BuildKit speedup claim. Because none of those medians improved, no cache-hit-based speedup claim is allowed.

### Measurement Profile

The measurement workflow uses `measurement_profile=official` for the 5-run gate. Cancelled, skipped, failed, blocked, and superseded attempts are not included as successful samples.

### Resume Usage

Usable resume candidates:

| Metric | Before median | After median | 개선율 | Reason |
| :--- | ---: | ---: | ---: | :--- |
| ci_same_scope_total | 10.317s | 9.973s | 3.33% | Same-scope critical path, 5/5 success, zero failures |
| backend_test | 9.766s | 9.307s | 4.7% | Same command scope, 5/5 success, zero failures |
| performance_assets | 0.807s | 0.709s | 12.14% | Same validation and k6 inspect scope, 5/5 success, zero failures |
| build_jar_same_scope | 10.317s | 9.973s | 3.33% | Same-runner test plus bootJar scope, 5/5 success, zero failures |

Do not use as resume claims:

- `ci_full_gate_total`: `참고 전용`; do not call it PR validation overall time reduction.
- `monitor_bot_test`: median worsened from `0.308s` to `0.31s`.
- `cd_dry_run_full_path`: no median improvement and not a production deploy.
- `gateway_image_build_only`: median worsened from `0.902s` to `1.635s`.
- `monitor_bot_image_build_only`: median worsened from `0.313s` to `1.042s`.
- `image_build_warm_cache`: median worsened from `5.264s` to `5.332s`.
