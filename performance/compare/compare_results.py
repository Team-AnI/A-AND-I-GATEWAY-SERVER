#!/usr/bin/env python3
import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


EXPECTED_SCHEMA_VERSION = 1
DIRECT_TEST_NAME = "direct-upstream"
GATEWAY_TEST_NAME = "gateway-public-route"
MAX_HTTP_FAILED_RATE = 0.01
MIN_CHECK_RATE = 0.99

REQUIRED_MATCH_FIELDS = [
    "commitSha",
    "executor",
    "vus",
    "duration",
    "sleepSeconds",
    "p95ThresholdMs",
    "payloadBytes",
    "mockDelayMs",
    "mockStatus",
    "routePath",
    "runRepeat",
    "pairIndex",
    "pairOrder",
    "warmupCompleted",
    "k6Version",
]

REQUIRED_METRICS = [
    ("http_req_duration", "med", "P50"),
    ("http_req_duration", "p95", "P95"),
    ("http_req_duration", "p99", "P99"),
    ("http_reqs", "rate", "request rate"),
    ("http_reqs", "count", "request count"),
    ("http_req_failed", "rate", "HTTP failure rate"),
    ("checks", "rate", "Check rate"),
]


def load_result(path):
    with Path(path).open("r", encoding="utf-8") as handle:
        return json.load(handle)


def metric(result, metric_name, field):
    return result.get("metrics", {}).get(metric_name, {}).get(field)


def is_number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def fmt(value, suffix=""):
    if value is None:
        return "n/a"
    if isinstance(value, (int, float)):
        return f"{value:.3f}{suffix}"
    return f"{value}{suffix}"


def validation_error(message, field=None, direct=None, gateway=None):
    error = {"message": message}
    if field is not None:
        error["field"] = field
    if direct is not None:
        error["direct"] = direct
    if gateway is not None:
        error["gateway"] = gateway
    return error


def validate_result_shape(result, expected_name, label):
    errors = []
    if result.get("schemaVersion") != EXPECTED_SCHEMA_VERSION:
        errors.append(validation_error(f"{label} schemaVersion mismatch", "schemaVersion", result.get("schemaVersion")))
    if result.get("testName") != expected_name:
        errors.append(validation_error(f"{label} testName must be {expected_name}", "testName", result.get("testName")))
    return errors


def validate_required_metrics(result, label):
    errors = []
    for metric_name, field, display_name in REQUIRED_METRICS:
        value = metric(result, metric_name, field)
        if not is_number(value):
            errors.append(validation_error(f"{label} missing numeric {display_name}", f"{metric_name}.{field}", value))

    if is_number(metric(result, "http_reqs", "count")) and metric(result, "http_reqs", "count") <= 0:
        errors.append(validation_error(f"{label} request count is zero", "http_reqs.count", metric(result, "http_reqs", "count")))

    dropped = metric(result, "dropped_iterations", "count")
    if is_number(dropped) and dropped > 0:
        errors.append(validation_error(f"{label} has dropped iterations", "dropped_iterations.count", dropped))

    failed_rate = metric(result, "http_req_failed", "rate")
    if is_number(failed_rate) and failed_rate >= MAX_HTTP_FAILED_RATE:
        errors.append(validation_error(f"{label} HTTP failure rate exceeds limit", "http_req_failed.rate", failed_rate))

    check_rate = metric(result, "checks", "rate")
    if is_number(check_rate) and check_rate <= MIN_CHECK_RATE:
        errors.append(validation_error(f"{label} check rate is below limit", "checks.rate", check_rate))

    threshold_failures = result.get("thresholdFailures", [])
    if threshold_failures:
        errors.append(validation_error(f"{label} threshold failures exist", "thresholdFailures", threshold_failures))

    return errors


def compare_config(direct, gateway):
    direct_config = direct.get("config", {})
    gateway_config = gateway.get("config", {})
    errors = []
    for field in REQUIRED_MATCH_FIELDS:
        if direct_config.get(field) != gateway_config.get(field):
            errors.append(
                validation_error(
                    "Direct and Gateway config mismatch",
                    field,
                    direct_config.get(field),
                    gateway_config.get(field),
                )
            )

    if direct_config.get("commitSha") == "unknown" or gateway_config.get("commitSha") == "unknown":
        errors.append(validation_error("commitSha must not be unknown", "commitSha"))

    if direct_config.get("gitDirty") is not False or gateway_config.get("gitDirty") is not False:
        errors.append(
            validation_error(
                "gitDirty must be false",
                "gitDirty",
                direct_config.get("gitDirty"),
                gateway_config.get("gitDirty"),
            )
        )

    if direct_config.get("k6Version") in (None, "", "unknown") or gateway_config.get("k6Version") in (None, "", "unknown"):
        errors.append(validation_error("k6Version must be recorded", "k6Version"))

    return errors


def validate(direct, gateway):
    errors = []
    errors.extend(validate_result_shape(direct, DIRECT_TEST_NAME, "direct"))
    errors.extend(validate_result_shape(gateway, GATEWAY_TEST_NAME, "gateway"))
    errors.extend(validate_required_metrics(direct, "direct"))
    errors.extend(validate_required_metrics(gateway, "gateway"))
    errors.extend(compare_config(direct, gateway))
    return errors


def build_comparison(direct, gateway, errors):
    direct_p50 = metric(direct, "http_req_duration", "med")
    direct_p95 = metric(direct, "http_req_duration", "p95")
    direct_p99 = metric(direct, "http_req_duration", "p99")
    gateway_p50 = metric(gateway, "http_req_duration", "med")
    gateway_p95 = metric(gateway, "http_req_duration", "p95")
    gateway_p99 = metric(gateway, "http_req_duration", "p99")

    comparable = len(errors) == 0
    return {
        "schemaVersion": EXPECTED_SCHEMA_VERSION,
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "comparable": comparable,
        "errors": errors,
        "direct": {
            "p50Ms": direct_p50,
            "p95Ms": direct_p95,
            "p99Ms": direct_p99,
            "rps": metric(direct, "http_reqs", "rate"),
            "requestCount": metric(direct, "http_reqs", "count"),
            "httpFailedRate": metric(direct, "http_req_failed", "rate"),
            "checkRate": metric(direct, "checks", "rate"),
        },
        "gateway": {
            "p50Ms": gateway_p50,
            "p95Ms": gateway_p95,
            "p99Ms": gateway_p99,
            "rps": metric(gateway, "http_reqs", "rate"),
            "requestCount": metric(gateway, "http_reqs", "count"),
            "httpFailedRate": metric(gateway, "http_req_failed", "rate"),
            "checkRate": metric(gateway, "checks", "rate"),
        },
        "gatewayAdditionalLatencyMs": None if not comparable else {
            "p50": gateway_p50 - direct_p50,
            "p95": gateway_p95 - direct_p95,
            "p99": gateway_p99 - direct_p99,
        },
        "config": {
            "direct": direct.get("config", {}),
            "gateway": gateway.get("config", {}),
        },
    }


def to_markdown(comparison):
    lines = [
        "# Gateway Additional Latency Comparison",
        "",
        f"- Generated At: {comparison['generatedAt']}",
        f"- Comparable: {'yes' if comparison['comparable'] else 'no'}",
        "",
    ]

    if not comparison["comparable"]:
        lines.extend([
            "## Comparison Blocked",
            "",
            "Direct and Gateway runs did not satisfy comparison requirements.",
            "",
            "| Field | Message | Direct | Gateway |",
            "| --- | --- | --- | --- |",
        ])
        for error in comparison["errors"]:
            lines.append(
                f"| {error.get('field', '')} | {error['message']} | {error.get('direct', '')} | {error.get('gateway', '')} |"
            )
        lines.append("")
        return "\n".join(lines)

    direct = comparison["direct"]
    gateway = comparison["gateway"]
    additional = comparison["gatewayAdditionalLatencyMs"]

    lines.extend([
        "| Metric | Direct | Gateway | Gateway Additional Latency |",
        "| --- | ---: | ---: | ---: |",
        f"| P50 | {fmt(direct['p50Ms'], ' ms')} | {fmt(gateway['p50Ms'], ' ms')} | {fmt(additional['p50'], ' ms')} |",
        f"| P95 | {fmt(direct['p95Ms'], ' ms')} | {fmt(gateway['p95Ms'], ' ms')} | {fmt(additional['p95'], ' ms')} |",
        f"| P99 | {fmt(direct['p99Ms'], ' ms')} | {fmt(gateway['p99Ms'], ' ms')} | {fmt(additional['p99'], ' ms')} |",
        f"| RPS | {fmt(direct['rps'])} | {fmt(gateway['rps'])} | n/a |",
        f"| Request count | {fmt(direct['requestCount'])} | {fmt(gateway['requestCount'])} | n/a |",
        f"| HTTP failed rate | {fmt(direct['httpFailedRate'])} | {fmt(gateway['httpFailedRate'])} | n/a |",
        f"| Check rate | {fmt(direct['checkRate'])} | {fmt(gateway['checkRate'])} | n/a |",
        "",
        "This is a Gateway additional-latency comparison, not a performance-improvement claim.",
        "",
    ])
    return "\n".join(lines)


def write_outputs(comparison, output_dir):
    run_id = comparison["config"]["gateway"].get("runId") or datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    run_index = comparison["config"]["gateway"].get("runIndex") or "1"
    output_dir.mkdir(parents=True, exist_ok=True)
    json_path = output_dir / f"comparison-{run_id}-r{run_index}.json"
    markdown_path = output_dir / f"comparison-{run_id}-r{run_index}.md"

    json_path.write_text(json.dumps(comparison, ensure_ascii=False, indent=2), encoding="utf-8")
    markdown_path.write_text(to_markdown(comparison), encoding="utf-8")
    return json_path, markdown_path


def main():
    parser = argparse.ArgumentParser(description="Compare k6 direct-upstream and gateway route results.")
    parser.add_argument("--direct", required=True, help="Path to direct-upstream JSON summary")
    parser.add_argument("--gateway", required=True, help="Path to gateway-public-route JSON summary")
    parser.add_argument("--output-dir", default="performance/results", help="Directory for comparison output")
    args = parser.parse_args()

    direct = load_result(args.direct)
    gateway = load_result(args.gateway)
    errors = validate(direct, gateway)
    comparison = build_comparison(direct, gateway, errors)
    json_path, markdown_path = write_outputs(comparison, Path(args.output_dir))

    print(f"Wrote {json_path}")
    print(f"Wrote {markdown_path}")
    if errors:
        for error in errors:
            print(f"Comparison blocked: {error['message']} ({error.get('field', 'n/a')})", file=sys.stderr)
        raise SystemExit(2)


if __name__ == "__main__":
    main()
