#!/usr/bin/env python3
import argparse
import json
import statistics
from datetime import datetime, timezone
from pathlib import Path


EXPECTED_SCOPES = [
    "full-gate-ci",
    "backend-test",
    "monitor-bot-test",
    "performance-assets",
    "build-jar-same-scope",
    "cd-dry-run-full-path",
    "gateway-image-build-only",
    "monitor-bot-image-build-only",
    "image-build-warm-cache",
]
SIDES = ("before", "after")
SYNTHETIC_SAME_SCOPE_COMPONENTS = [
    "build-jar-same-scope",
    "monitor-bot-test",
    "performance-assets",
]

SCOPE_TO_METRIC = {
    "full-gate-ci": "ci_full_gate_total",
    "backend-test": "backend_test",
    "monitor-bot-test": "monitor_bot_test",
    "performance-assets": "performance_assets",
    "build-jar-same-scope": "build_jar_same_scope",
    "cd-dry-run-full-path": "cd_dry_run_full_path",
    "gateway-image-build-only": "gateway_image_build_only",
    "monitor-bot-image-build-only": "monitor_bot_image_build_only",
    "image-build-warm-cache": "image_build_warm_cache",
}

METRIC_ORDER = [
    "ci_same_scope_total",
    "ci_full_gate_total",
    "backend_test",
    "monitor_bot_test",
    "performance_assets",
    "build_jar_same_scope",
    "cd_dry_run_full_path",
    "gateway_image_build_only",
    "monitor_bot_image_build_only",
    "image_build_warm_cache",
]

METRIC_SCOPES = {
    "ci_same_scope_total": "parallel critical path of backend test/JAR, monitor-bot go test ./..., and performance asset validation",
    "ci_full_gate_total": "same-scope CI plus full-gate artifact upload/download checks",
    "backend_test": "./gradlew test --build-cache --no-daemon",
    "monitor_bot_test": "cd monitor-bot && go test ./...",
    "performance_assets": "performance asset validation plus unchanged k6 inspect set",
    "build_jar_same_scope": "./gradlew test bootJar --build-cache --no-daemon on the same runner",
    "cd_dry_run_full_path": "backend test/JAR, monitor-bot test, Gateway image build, and Monitor Bot image build with no deploy path",
    "gateway_image_build_only": "prebuilt Gateway JAR followed by Gateway Docker image build only with no push",
    "monitor_bot_image_build_only": "Monitor Bot Docker image build only with no push",
    "image_build_warm_cache": "Gateway and Monitor Bot image builds through the warm BuildKit cache path with no push",
}

SAME_SCOPE_METRICS = {
    "ci_same_scope_total",
    "backend_test",
    "monitor_bot_test",
    "performance_assets",
    "build_jar_same_scope",
    "cd_dry_run_full_path",
    "gateway_image_build_only",
    "monitor_bot_image_build_only",
    "image_build_warm_cache",
}


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-dir", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--baseline-ref", required=True)
    parser.add_argument("--candidate-ref", required=True)
    parser.add_argument("--iterations", required=True, type=int)
    parser.add_argument("--measurement-profile", required=True)
    return parser.parse_args()


def load_results(input_dir):
    results = {}
    root = Path(input_dir)
    if not root.exists():
        return results

    for path in sorted(root.rglob("*.json")):
        with path.open(encoding="utf-8") as handle:
            payload = json.load(handle)
        scope = payload.get("scope")
        side = payload.get("side")
        if scope in EXPECTED_SCOPES and side in SIDES:
            results[(scope, side)] = payload
    return results


def success_runs(runs):
    return [run for run in runs if run.get("status") == "success"]


def failure_count(runs):
    return sum(1 for run in runs if run.get("status") == "failure")


def skipped_count(runs):
    return sum(1 for run in runs if run.get("status") not in {"success", "failure"})


def run_duration(run):
    value = run.get("duration_seconds")
    return float(value) if value is not None else None


def median_seconds(runs):
    values = [run_duration(run) for run in success_runs(runs)]
    values = [value for value in values if value is not None]
    return round(statistics.median(values), 3) if values else None


def average_seconds(runs):
    values = [run_duration(run) for run in success_runs(runs)]
    values = [value for value in values if value is not None]
    return round(sum(values) / len(values), 3) if values else None


def improvement(before_median, after_median):
    if before_median is None or after_median is None or before_median <= 0:
        return None
    return round(((before_median - after_median) / before_median) * 100, 2)


def result_runs(results, scope, side):
    payload = results.get((scope, side))
    return payload.get("runs", []) if payload else []


def bool_from_payloads(results, scopes, key):
    for scope in scopes:
        for side in SIDES:
            payload = results.get((scope, side))
            if payload and payload.get(key) is True:
                return True
    return False


def first_cache_value(results, scopes, key):
    for scope in scopes:
        payload = results.get((scope, "after"))
        if payload and payload.get(key):
            return payload[key]
    return "확인 필요"


def synthetic_same_scope_runs(results, side, iterations):
    runs = []
    component_runs = {
        scope: {run.get("iteration"): run for run in result_runs(results, scope, side)}
        for scope in SYNTHETIC_SAME_SCOPE_COMPONENTS
    }

    for iteration in range(1, iterations + 1):
        selected = [runs_by_iteration.get(iteration) for runs_by_iteration in component_runs.values()]
        if any(run is None for run in selected):
            continue

        durations = [run_duration(run) for run in selected if run_duration(run) is not None]
        status = "success" if all(run.get("status") == "success" for run in selected) else "failure"
        runs.append({
            "iteration": iteration,
            "status": status,
            "duration_seconds": round(max(durations), 3) if durations else None,
            "component_statuses": {
                scope: component_runs[scope][iteration].get("status")
                for scope in SYNTHETIC_SAME_SCOPE_COMPONENTS
            },
        })
    return runs


def build_metric(name, before_runs, after_runs, cache_scopes, results, status, min_success):
    before_median = median_seconds(before_runs)
    after_median = median_seconds(after_runs)
    before_failures = failure_count(before_runs)
    after_failures = failure_count(after_runs)
    improvement_percent = improvement(before_median, after_median)

    metric = {
        "name": name,
        "scope": METRIC_SCOPES[name],
        "same_scope": name in SAME_SCOPE_METRICS,
        "before_runs": before_runs,
        "after_runs": after_runs,
        "before_success_count": len(success_runs(before_runs)),
        "after_success_count": len(success_runs(after_runs)),
        "before_failure_count": before_failures,
        "after_failure_count": after_failures,
        "before_cancelled_or_skipped_count": skipped_count(before_runs),
        "after_cancelled_or_skipped_count": skipped_count(after_runs),
        "before_median_seconds": before_median,
        "after_median_seconds": after_median,
        "before_average_seconds": average_seconds(before_runs),
        "after_average_seconds": average_seconds(after_runs),
        "improvement_percent": improvement_percent,
        "outliers": [],
        "go_cache_configured": bool_from_payloads(results, cache_scopes, "go_cache_configured"),
        "go_cache_hit_observed": first_cache_value(results, cache_scopes, "go_cache_hit_observed"),
        "k6_cache_configured": bool_from_payloads(results, cache_scopes, "k6_cache_configured"),
        "k6_cache_hit_observed": first_cache_value(results, cache_scopes, "k6_cache_hit_observed"),
        "buildkit_cache_configured": bool_from_payloads(results, cache_scopes, "buildkit_cache_configured"),
        "buildkit_cache_hit_observed": first_cache_value(results, cache_scopes, "buildkit_cache_hit_observed"),
        "cache_evidence": "",
        "confidence": "medium",
        "resume_sentence_candidate": "",
    }

    usable = (
        status == "completed"
        and metric["before_success_count"] >= min_success
        and metric["after_success_count"] >= min_success
        and before_failures == 0
        and after_failures == 0
        and improvement_percent is not None
        and improvement_percent > 0
        and metric["same_scope"]
    )

    if usable:
        metric["resume_usage"] = "사용 가능"
        metric["rejection_reason"] = ""
    elif status != "completed":
        metric["resume_usage"] = "측정 실패"
        metric["rejection_reason"] = "Official measurement did not complete for all required scopes."
    elif metric["before_success_count"] < min_success or metric["after_success_count"] < min_success:
        metric["resume_usage"] = "측정 부족"
        metric["rejection_reason"] = "Official measurement does not have enough successful before/after samples."
    elif before_failures or after_failures:
        metric["resume_usage"] = "측정 실패"
        metric["rejection_reason"] = "At least one before/after iteration failed."
    elif improvement_percent is None or improvement_percent <= 0:
        metric["resume_usage"] = "개선 없음"
        metric["rejection_reason"] = "Median after time did not improve over median before time."
    else:
        metric["resume_usage"] = "참고 전용"
        metric["rejection_reason"] = "Metric is not a same-scope resume metric."

    return metric


def main():
    args = parse_args()
    results = load_results(args.input_dir)
    expected_keys = [(scope, side) for scope in EXPECTED_SCOPES for side in SIDES]
    missing = [
        {"scope": scope, "side": side}
        for scope, side in expected_keys
        if (scope, side) not in results
    ]
    status = "completed" if not missing else "blocked_incomplete_measurement"
    min_success = 5

    metrics_by_name = {
        "ci_same_scope_total": build_metric(
            "ci_same_scope_total",
            synthetic_same_scope_runs(results, "before", args.iterations),
            synthetic_same_scope_runs(results, "after", args.iterations),
            SYNTHETIC_SAME_SCOPE_COMPONENTS,
            results,
            status,
            min_success,
        )
    }

    for scope, name in SCOPE_TO_METRIC.items():
        metrics_by_name[name] = build_metric(
            name,
            result_runs(results, scope, "before"),
            result_runs(results, scope, "after"),
            [scope],
            results,
            status,
            min_success,
        )

    safety = {
        "production_deploy_executed": bool_from_payloads(results, EXPECTED_SCOPES, "production_deploy_executed"),
        "tag_push_executed": False,
        "docker_push_executed": bool_from_payloads(results, EXPECTED_SCOPES, "docker_push_executed"),
        "aws_credentials_used": False,
        "ecr_login_or_push_executed": False,
        "ec2_ssh_executed": False,
        "aws_ecr_ssh_executed": bool_from_payloads(results, EXPECTED_SCOPES, "aws_ecr_ssh_executed"),
        "production_url_accessed": False,
    }

    output = {
        "repository": "Team-AnI/A-AND-I-GATEWAY-SERVER",
        "base_ref": args.baseline_ref,
        "baseline_ref": args.baseline_ref,
        "candidate_ref": args.candidate_ref,
        "measurement_status": status,
        "measurement_profile": args.measurement_profile,
        "measured_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
        "official_measurement_policy": {
            "rerun_for_better_numbers_forbidden": True,
            "official_batch_count": args.iterations if status == "completed" else 0,
            "iterations_per_side": args.iterations,
            "median_is_primary": True,
            "average_is_reference_only": True,
        },
        "blocking_reason": "Missing before/after measurement result artifacts." if missing else "",
        "missing_measurement_results": missing,
        "production_deploy_executed": safety["production_deploy_executed"],
        "docker_push_executed": safety["docker_push_executed"],
        "aws_ecr_ssh_executed": safety["aws_ecr_ssh_executed"],
        "safety": safety,
        "metrics": [metrics_by_name[name] for name in METRIC_ORDER],
    }

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8") as handle:
        json.dump(output, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


if __name__ == "__main__":
    main()
