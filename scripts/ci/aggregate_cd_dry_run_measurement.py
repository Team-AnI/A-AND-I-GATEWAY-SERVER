#!/usr/bin/env python3
import argparse
import json
import statistics
from datetime import datetime, timezone
from pathlib import Path


SIDES = ("before", "after")
SCOPE = "cd-dry-run-same-scope"


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
        if payload.get("scope") == SCOPE and payload.get("side") in SIDES:
            results[payload["side"]] = payload
    return results


def success_runs(runs):
    return [run for run in runs if run.get("status") == "success"]


def failure_count(runs):
    return sum(1 for run in runs if run.get("status") == "failure")


def skipped_count(runs):
    return sum(1 for run in runs if run.get("status") not in {"success", "failure"})


def durations(runs):
    values = []
    for run in success_runs(runs):
        value = run.get("duration_seconds")
        if value is not None:
            values.append(float(value))
    return values


def median_seconds(runs):
    values = durations(runs)
    return round(statistics.median(values), 3) if values else None


def average_seconds(runs):
    values = durations(runs)
    return round(sum(values) / len(values), 3) if values else None


def improvement(before_median, after_median):
    if before_median is None or after_median is None or before_median <= 0:
        return None
    return round(((before_median - after_median) / before_median) * 100, 2)


def side_summary(payload):
    runs = payload.get("runs", []) if payload else []
    return {
        "runs": runs,
        "success_count": len(success_runs(runs)),
        "failure_count": failure_count(runs),
        "cancelled_or_skipped_count": skipped_count(runs),
        "median_seconds": median_seconds(runs),
        "average_seconds": average_seconds(runs),
        "gradle_cache_configured": payload.get("gradle_cache_configured") if payload else None,
        "gradle_cache_hit_observed": payload.get("gradle_cache_hit_observed", "확인 필요") if payload else "확인 필요",
        "go_cache_configured": payload.get("go_cache_configured") if payload else None,
        "go_cache_hit_observed": payload.get("go_cache_hit_observed", "확인 필요") if payload else "확인 필요",
        "buildkit_cache_configured": payload.get("buildkit_cache_configured") if payload else None,
        "buildkit_cache_hit_observed": payload.get("buildkit_cache_hit_observed", "확인 필요") if payload else "확인 필요",
    }


def bool_from_results(results, key):
    for payload in results.values():
        if payload.get(key) is True:
            return True
    return False


def main():
    args = parse_args()
    results = load_results(args.input_dir)
    missing = [side for side in SIDES if side not in results]
    status = "completed" if not missing else "blocked_incomplete_measurement"

    before = side_summary(results.get("before"))
    after = side_summary(results.get("after"))
    improvement_percent = improvement(before["median_seconds"], after["median_seconds"])
    min_success = 5

    if status != "completed":
        resume_usage = "측정 실패"
        rejection_reason = "Missing before/after measurement result artifacts."
    elif before["success_count"] < min_success or after["success_count"] < min_success:
        resume_usage = "측정 부족"
        rejection_reason = "Official measurement does not have enough successful before/after samples."
    elif before["failure_count"] or after["failure_count"]:
        resume_usage = "측정 실패"
        rejection_reason = "At least one before/after iteration failed."
    elif improvement_percent is None or improvement_percent <= 0:
        resume_usage = "개선 없음"
        rejection_reason = "Median after time did not improve over median before time."
    else:
        resume_usage = "사용 가능"
        rejection_reason = ""

    safety = {
        "production_deploy_executed": bool_from_results(results, "production_deploy_executed"),
        "tag_push_executed": False,
        "docker_push_executed": bool_from_results(results, "docker_push_executed"),
        "aws_credentials_used": False,
        "ecr_login_or_push_executed": False,
        "ec2_ssh_executed": False,
        "aws_ecr_ssh_executed": bool_from_results(results, "aws_ecr_ssh_executed"),
        "production_url_accessed": False,
    }

    output = {
        "repository": "Team-AnI/A-AND-I-GATEWAY-SERVER",
        "scope": SCOPE,
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
        "missing_measurement_results": missing,
        "production_deploy_executed": safety["production_deploy_executed"],
        "docker_push_executed": safety["docker_push_executed"],
        "aws_ecr_ssh_executed": safety["aws_ecr_ssh_executed"],
        "safety": safety,
        "before": before,
        "after": after,
        "improvement_percent": improvement_percent,
        "resume_usage": resume_usage,
        "rejection_reason": rejection_reason,
    }

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8") as handle:
        json.dump(output, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


if __name__ == "__main__":
    main()
