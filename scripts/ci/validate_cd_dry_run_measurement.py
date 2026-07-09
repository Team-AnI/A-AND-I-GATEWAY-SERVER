#!/usr/bin/env python3
import argparse
import json
import sys


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("metrics_path")
    parser.add_argument("--allow-pending", action="store_true")
    return parser.parse_args()


def safety_value(payload, key):
    if key in payload:
        return payload[key]
    return payload.get("safety", {}).get(key)


def main():
    args = parse_args()
    with open(args.metrics_path, encoding="utf-8") as handle:
        payload = json.load(handle)

    errors = []
    for key in ("production_deploy_executed", "docker_push_executed", "aws_ecr_ssh_executed"):
        if safety_value(payload, key) is not False:
            errors.append(f"{key} must be false")

    status = payload.get("measurement_status")
    if status != "completed":
        if not args.allow_pending:
            errors.append("measurement_status must be completed")
    if payload.get("scope") != "cd-dry-run-same-scope":
        errors.append("scope must be cd-dry-run-same-scope")

    before = payload.get("before", {})
    after = payload.get("after", {})
    if status == "completed":
        if before.get("success_count", 0) < 5:
            errors.append("before.success_count must be >= 5")
        if after.get("success_count", 0) < 5:
            errors.append("after.success_count must be >= 5")
        if before.get("failure_count", 0) != 0:
            errors.append("before.failure_count must be 0")
        if after.get("failure_count", 0) != 0:
            errors.append("after.failure_count must be 0")

    resume_usage = payload.get("resume_usage")
    improvement = payload.get("improvement_percent")
    if resume_usage == "사용 가능":
        if improvement is None or improvement <= 0:
            errors.append("usable metric must have positive improvement_percent")
        if before.get("success_count", 0) < 5 or after.get("success_count", 0) < 5:
            errors.append("usable metric must have enough successful samples")
    elif resume_usage not in {"개선 없음", "측정 부족", "측정 실패", "참고 전용"}:
        errors.append("resume_usage has unexpected value")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("CD dry-run measurement validation passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
