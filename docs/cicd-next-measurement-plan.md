# Gateway CI/CD Next Measurement Plan

This document tracks the metrics that are not resume-safe yet and the evidence needed before they can be used. Do not treat recent workflow duration as before/after improvement evidence unless the scope is identical.

## Guardrails

- README must not be regenerated.
- Production deploy, ECR push, EC2 SSH, and production URL checks are not executed for measurement unless explicitly approved.
- Same-scope before/after comparison is required for improvement wording.
- Median is the primary statistic; average is reference-only.
- Each resume-safe metric needs source command or file, run date or commit, success count, failure count, and rejection criteria.
- Cache hit evidence alone is not a speedup claim.

## Current Exclusions

| Metric candidate | Current reason excluded | Evidence needed |
| :--- | :--- | :--- |
| Production deploy reduction | No same-scope before/after deploy path measurement | Same CD workflow, same deploy step, before/after samples, deploy safety notes |
| Docker build/push reduction | Recent step duration exists, but no same-scope before/after comparison | Split build and push timing with same image context, tags, runner type, and network path |
| Docker layer cache speedup | BuildKit cache hit was observed, but measured image medians worsened | Repeated cold/warm cache measurement where after median improves |
| CD Dry Run vs production CD | Dry-run excludes ECR push, AWS credentials, EC2 SSH, and production deploy | Keep dry-run wording separate from production deploy wording |
| Monitor Bot test speedup | Go cache hit exists, but median moved from `0.308s` to `0.31s` | New same-command batch with after median lower than before |
| MTTR reduction | No incident timeline with detection, alert, ack, query, and resolution timestamps | Incident or drill log schema and repeated event records |
| High availability or large-scale traffic | No SLO, uptime, failover, or traffic capacity evidence | Separate SLO/traffic measurement plan |

## Phase 1: Reference Baseline Collection

Purpose: make recent workflow observations reproducible without claiming improvement.

Tasks:

- Use `scripts/ci/collect_recent_workflow_durations.py` to collect recent successful workflow run durations from `gh`.
- Collect workflow wall-clock, job duration, and selected step duration.
- Keep output marked as reference-only.
- Do not compare different workflows as before/after.

Command:

```bash
python3 scripts/ci/collect_recent_workflow_durations.py --limit 10 --output /tmp/aandi-recent-workflow-durations.json
```

Optional cache log parsing:

```bash
python3 scripts/ci/collect_recent_workflow_durations.py --limit 10 --workflow CI --include-cache-log --output /tmp/aandi-ci-cache-reference.json
```

Expected output:

- CI recent run duration table
- CD recent run duration table
- CD step medians for test, Docker build/push, and deploy
- Cache hit observations only where logs expose them

Resume usage:

- Not resume-safe as improvement evidence.
- Can be used to decide which next same-scope measurement is worth running.

Verification performed:

- `python3 scripts/ci/collect_recent_workflow_durations.py --limit 10 --output /tmp/aandi-recent-workflow-durations-full.json`
- JSON validation with `python3 -m json.tool`
- Collected reference medians matched the documented recent run reference: CI `46.0s`, CD `261.5s`, CD Dry Run `158.5s`, Measure Gateway CI/CD Same Scope `181.0s`.

## Phase 2: CD Dry-Run Same-Scope Measurement

Purpose: create a safe validation-path metric without production deployment.

Tasks:

- Keep build/test/image dry-run scope separate from production CD.
- Measure before/after with 5 or more successful samples per side.
- Record `production_deploy_executed=false`, `docker_push_executed=false`, and `aws_ecr_ssh_executed=false`.
- Reject the metric if after median does not improve.

Resume wording allowed only if successful:

- Use “CD dry-run validation path” wording.
- Do not use “deployment time reduction.”

## Phase 3: Docker Build Cache Remeasurement

Purpose: test whether BuildKit cache can produce a real same-scope improvement.

Tasks:

- Separate Gateway image build-only and Monitor Bot image build-only.
- Measure cold and warm cache paths explicitly.
- Keep `push=false` until a separate push measurement is approved.
- Require BuildKit hit evidence and improved median.

Rejection rule:

- If the median worsens or is unchanged, keep only “BuildKit cache configured/hit observed” as supporting evidence.

## Phase 4: Docker Push and Production Deploy Measurement

Purpose: measure real deploy path only after the low-risk phases are complete.

Required approval before execution:

- ECR push with temporary tags
- Cleanup policy for temporary tags
- EC2 SSH deploy or production URL checks
- Release/tag strategy

Resume wording allowed only if successful:

- Use exact scope, such as “ECR image push step median” or “CD deploy SSH step median.”
- Do not combine build, push, and deploy unless the same combined path is measured.

## Phase 5: Ops / MTTR Evidence

Purpose: support operational response metrics without inventing incident outcomes.

Needed event fields:

- alert_detected_at
- discord_notified_at
- operator_ack_at
- first_trace_query_at
- resolved_at
- trace_id or incident_id

Candidate metrics:

- alert detection-to-notification latency
- `/ops logs` query response time
- duplicate alert suppression count
- incident drill response timeline

Resume wording allowed only after repeated records exist:

- Use measured drill or incident scope.
- Do not claim MTTR reduction without before/after incident evidence.
