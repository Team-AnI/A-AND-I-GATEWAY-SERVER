#!/usr/bin/env python3
import argparse
import json
import sys


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("metrics_path")
    parser.add_argument("--allow-pending", action="store_true")
    return parser.parse_args()


def safety_value(payload, top_level_key, fallback_keys):
    if top_level_key in payload:
        return payload[top_level_key]

    safety = payload.get("safety", {})
    for key in fallback_keys:
        if key in safety:
            return safety[key]
    return None


def validate_safety(payload, errors):
    safety_checks = [
        ("production_deploy_executed", ["production_deploy_executed"]),
        ("docker_push_executed", ["docker_push_executed", "ecr_login_or_push_executed"]),
        ("aws_ecr_ssh_executed", ["aws_ecr_ssh_executed", "aws_credentials_used", "ecr_login_or_push_executed", "ec2_ssh_executed"]),
    ]

    for top_level_key, fallback_keys in safety_checks:
        value = safety_value(payload, top_level_key, fallback_keys)
        if value is None:
            errors.append(f"missing safety flag: {top_level_key}")
        elif value is not False:
            errors.append(f"safety flag must be false: {top_level_key}")


def metric_is_usable(metric):
    return (
        metric.get("resume_usage") == "사용 가능"
        and metric.get("before_success_count", 0) >= 5
        and metric.get("after_success_count", 0) >= 5
        and metric.get("before_failure_count", 0) == 0
        and metric.get("after_failure_count", 0) == 0
        and metric.get("improvement_percent") is not None
        and metric.get("improvement_percent") > 0
    )


def main():
    args = parse_args()
    with open(args.metrics_path, encoding="utf-8") as handle:
        payload = json.load(handle)

    errors = []
    validate_safety(payload, errors)

    status = payload.get("measurement_status")
    if status != "completed":
        if args.allow_pending:
            if errors:
                for error in errors:
                    print(f"ERROR: {error}", file=sys.stderr)
                return 1
            print(f"pending measurement accepted: {status}")
            return 0
        errors.append("measurement_status must be completed")

    metrics = payload.get("metrics")
    if not isinstance(metrics, list):
        errors.append("metrics must be a list")
        metrics = []

    usable_metrics = [metric for metric in metrics if metric.get("resume_usage") == "사용 가능"]
    if not usable_metrics:
        errors.append('at least one metric must have resume_usage == "사용 가능"')

    valid_usable_metrics = [metric for metric in usable_metrics if metric_is_usable(metric)]
    if not valid_usable_metrics:
        errors.append("at least one usable metric must have >=5 before/after successes, zero failures, and positive improvement_percent")

    for metric in usable_metrics:
        if metric.get("before_success_count", 0) < 5:
            errors.append(f"{metric.get('name')} before_success_count must be >= 5")
        if metric.get("after_success_count", 0) < 5:
            errors.append(f"{metric.get('name')} after_success_count must be >= 5")
        if metric.get("before_failure_count", 0) != 0:
            errors.append(f"{metric.get('name')} before_failure_count must be 0")
        if metric.get("after_failure_count", 0) != 0:
            errors.append(f"{metric.get('name')} after_failure_count must be 0")
        improvement = metric.get("improvement_percent")
        if improvement is None or improvement <= 0:
            errors.append(f"{metric.get('name')} improvement_percent must be > 0")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("gateway CI/CD metrics validation passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
