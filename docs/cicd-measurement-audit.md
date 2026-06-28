# Gateway CI/CD Measurement Audit

## Gateway CI/CD Remeasurement Audit

Source of truth: [`docs/metrics/gateway-cicd-remeasure.json`](./metrics/gateway-cicd-remeasure.json)

Current status: `blocked_workflow_not_on_default_branch`

The official remeasurement is incomplete. GitHub Actions did not expose the new `workflow_dispatch` workflow because `.github/workflows/measure-gateway-cicd.yml` was not present on the default branch `main` when dispatch was attempted. No official before/after run set was collected, so this document does not report any timing value or improvement percentage.

### PR #43 Measurement Risk

The previous PR #43 measurement is treated as unsuitable for a resume or final performance claim because the compared scopes were not proven equivalent. In particular, the Build JAR path could have compared a same-runner warm path with a separate cold runner path.

### Same-Scope Basis

The intended same-scope CI basis is:

- Gateway backend test
- Gateway JAR build on the same runner after backend test
- Monitor Bot `go test ./...`
- performance asset validation
- k6 inspect validation

Because the official batch did not run, every same-scope metric remains `N/A` and `측정 실패`.

### Full-Gate Basis

The intended full-gate basis is same-scope CI plus artifact upload/download and summary verification. It is not interchangeable with same-scope CI and must not be used to claim a narrower job-level improvement.

The JSON has no completed full-gate samples, so full-gate results are also `N/A`.

### Build JAR Cold Runner Issue

The target workflow structure removes the separate cold-runner `build-jar` job from the main comparison path by running backend test and JAR build on the same runner. This is a structural correction only. The JSON does not contain completed timing samples proving an improvement.

### Monitor Bot Test Result

Monitor Bot test is configured as an independent job with Go cache enabled and `monitor-bot/go.sum` as the dependency path. The official measurement did not run, so the measured result is unavailable.

Go cache hit evidence remains `확인 필요`; no cache-hit-based speed claim is allowed.

### CD Dry-Run Scope

CD dry-run is not a production deploy measurement. It is only a safe validation path for build and dry-run image construction with no AWS credentials, ECR login, docker push, EC2 SSH, or production URL access.

Legacy deploy and dry-run are different scopes and must not be directly compared. A production deploy path includes remote infrastructure steps that are intentionally absent from dry-run.

### Full Path vs Image-Only

CD dry-run full path means backend test and JAR build, Monitor Bot test, Gateway image build, and Monitor Bot image build with no deploy step.

Image-only metrics isolate Docker image construction:

- Gateway image build only uses the prebuilt Gateway JAR and builds the Gateway image without push.
- Monitor Bot image build only builds the Monitor Bot image without push.
- Image build warm cache is intended to exercise the BuildKit cache path without push.

BuildKit cache hit evidence remains `확인 필요`; no cache-hit-based speed claim is allowed.

### Measurement Profile

The measurement workflow uses `measurement_profile=official` for the 5-run gate. The official dispatch was blocked before the updated profile produced samples. Therefore, cache impact is not measured.

### Resume Usage

No Gateway CI/CD metric is currently usable for resume wording.

Do not use:

- same-scope CI timing
- full-gate timing
- backend test timing
- Monitor Bot test timing
- performance asset timing
- JAR build timing
- CD dry-run timing
- Gateway image-only timing
- Monitor Bot image-only timing
- image build warm-cache timing

All entries in the JSON are marked `측정 실패` because the official measurement workflow could not be dispatched.
