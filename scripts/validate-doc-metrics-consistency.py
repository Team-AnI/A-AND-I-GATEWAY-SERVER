#!/usr/bin/env python3
import argparse
import json
import re
import sys
from pathlib import Path


LABELS = {
    "ci_same_scope_total": "CI same-scope total",
    "ci_full_gate_total": "CI full-gate total",
    "backend_test": "Backend test",
    "monitor_bot_test": "Monitor Bot test",
    "performance_assets": "Performance assets",
    "build_jar_same_scope": "Build JAR same-scope",
    "cd_dry_run_full_path": "CD dry-run full path",
    "gateway_image_build_only": "Gateway image build only",
    "monitor_bot_image_build_only": "Monitor Bot image build only",
    "image_build_warm_cache": "Image build warm cache",
}


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--metrics", required=True)
    parser.add_argument("--docs", nargs="+", required=True)
    return parser.parse_args()


def seconds(value):
    return f"{value}s"


def percent(value):
    return f"{value}%"


def read_docs(paths):
    return {Path(path).name: Path(path).read_text(encoding="utf-8") for path in paths}


def expected_metric_row(metric):
    label = LABELS[metric["name"]]
    runs = f"{metric['before_success_count']}/{metric['after_success_count']}"
    return (
        f"| {label} | {seconds(metric['before_median_seconds'])} | "
        f"{seconds(metric['after_median_seconds'])} | "
        f"{percent(metric['improvement_percent'])} | {runs} | "
        f"{metric['confidence']} | {metric['resume_usage']} |"
    )


def section_after_heading(text, heading):
    pattern = rf"^## {re.escape(heading)}\n(?P<body>.*?)(?=^## |\Z)"
    match = re.search(pattern, text, flags=re.MULTILINE | re.DOTALL)
    return match.group("body") if match else ""


def validate_optimization(metrics, docs, errors):
    text = docs.get("cicd-optimization.md", "")
    if not text:
        errors.append("missing docs/cicd-optimization.md in --docs")
        return

    for metric in metrics:
        row = expected_metric_row(metric)
        if row not in text:
            errors.append(f"missing or stale optimization row: {row}")


def validate_resume(metrics, docs, errors):
    text = docs.get("resume-metrics.md", "")
    if not text:
        errors.append("missing docs/resume-metrics.md in --docs")
        return

    candidates = section_after_heading(text, "Resume Sentence Candidates")
    if not candidates:
        errors.append("resume metrics document is missing Resume Sentence Candidates section")
        return

    for metric in metrics:
        metric_tokens = [
            seconds(metric["before_median_seconds"]),
            seconds(metric["after_median_seconds"]),
            percent(metric["improvement_percent"]),
        ]
        if metric["resume_usage"] == "사용 가능":
            missing = [token for token in metric_tokens if token not in candidates]
            if missing:
                errors.append(f"usable metric {metric['name']} is missing candidate values: {', '.join(missing)}")
        else:
            label = LABELS[metric["name"]]
            if label in candidates or metric["name"] in candidates:
                errors.append(f"non-usable metric appears in resume candidates: {metric['name']}")
            rejected_prefix = f"| {metric['name']} | {metric['resume_usage']} |"
            if rejected_prefix not in text:
                errors.append(f"non-usable metric is missing rejected row: {metric['name']}")

    forbidden_patterns = [
        r"CI/CD\s*전체.*단축",
        r"운영\s*배포.*단축",
        r"legacy\s*CD.*dry-run.*단축",
        r"dry-run.*legacy\s*CD.*단축",
    ]
    for pattern in forbidden_patterns:
        if re.search(pattern, text, flags=re.IGNORECASE):
            errors.append(f"forbidden resume wording matched: {pattern}")


def validate_audit(payload, metrics, docs, errors):
    text = docs.get("cicd-measurement-audit.md", "")
    if not text:
        errors.append("missing docs/cicd-measurement-audit.md in --docs")
        return

    run = payload.get("measurement_run", {})
    required_tokens = [
        "Current status: `completed`",
        str(run.get("run_id")),
        run.get("candidate_commit", ""),
        "PR #43",
        "workflow_dispatch",
        "same-scope",
        "full-gate",
        "BuildKit",
    ]
    for attempt in payload.get("superseded_measurement_attempts", []):
        if "run_id" in attempt:
            required_tokens.append(str(attempt["run_id"]))
        required_tokens.append(attempt.get("candidate_commit", ""))

    for metric in metrics:
        required_tokens.extend(
            [
                metric["name"],
                seconds(metric["before_median_seconds"]),
                seconds(metric["after_median_seconds"]),
                percent(metric["improvement_percent"]),
                metric["resume_usage"],
            ]
        )

    for token in [token for token in required_tokens if token]:
        if token not in text:
            errors.append(f"audit document is missing token: {token}")


def main():
    args = parse_args()
    payload = json.loads(Path(args.metrics).read_text(encoding="utf-8"))
    docs = read_docs(args.docs)
    metrics = payload.get("metrics", [])

    errors = []
    if payload.get("measurement_status") != "completed":
        errors.append("metrics JSON must be completed before validating docs")

    validate_optimization(metrics, docs, errors)
    validate_resume(metrics, docs, errors)
    validate_audit(payload, metrics, docs, errors)

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("gateway CI/CD docs metrics consistency validation passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
