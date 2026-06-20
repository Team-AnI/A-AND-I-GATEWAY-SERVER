#!/usr/bin/env python3
import argparse
import json
from datetime import datetime, timezone
from pathlib import Path


REQUIRED_MATCH_FIELDS = [
    "commitSha",
    "vus",
    "duration",
    "payloadBytes",
    "mockDelayMs",
    "mockStatus",
    "routePath",
    "runRepeat",
    "runOrder",
]


def load_result(path):
    with Path(path).open("r", encoding="utf-8") as handle:
        return json.load(handle)


def metric(result, metric_name, field):
    return result.get("metrics", {}).get(metric_name, {}).get(field)


def fmt(value, suffix=""):
    if value is None:
        return "n/a"
    if isinstance(value, (int, float)):
        return f"{value:.3f}{suffix}"
    return f"{value}{suffix}"


def compare_config(direct, gateway):
    direct_config = direct.get("config", {})
    gateway_config = gateway.get("config", {})
    mismatches = []
    for field in REQUIRED_MATCH_FIELDS:
        if direct_config.get(field) != gateway_config.get(field):
            mismatches.append(
                {
                    "field": field,
                    "direct": direct_config.get(field),
                    "gateway": gateway_config.get(field),
                }
            )
    return mismatches


def build_comparison(direct, gateway, mismatches):
    direct_p50 = metric(direct, "http_req_duration", "med")
    direct_p95 = metric(direct, "http_req_duration", "p95")
    direct_p99 = metric(direct, "http_req_duration", "p99")
    gateway_p50 = metric(gateway, "http_req_duration", "med")
    gateway_p95 = metric(gateway, "http_req_duration", "p95")
    gateway_p99 = metric(gateway, "http_req_duration", "p99")

    comparable = len(mismatches) == 0
    return {
        "schemaVersion": 1,
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "comparable": comparable,
        "mismatches": mismatches,
        "direct": {
            "p50Ms": direct_p50,
            "p95Ms": direct_p95,
            "p99Ms": direct_p99,
            "rps": metric(direct, "http_reqs", "rate"),
            "httpFailedRate": metric(direct, "http_req_failed", "rate"),
        },
        "gateway": {
            "p50Ms": gateway_p50,
            "p95Ms": gateway_p95,
            "p99Ms": gateway_p99,
            "rps": metric(gateway, "http_reqs", "rate"),
            "httpFailedRate": metric(gateway, "http_req_failed", "rate"),
        },
        "gatewayAdditionalLatencyMs": None if not comparable else {
            "p50": None if direct_p50 is None or gateway_p50 is None else gateway_p50 - direct_p50,
            "p95": None if direct_p95 is None or gateway_p95 is None else gateway_p95 - direct_p95,
            "p99": None if direct_p99 is None or gateway_p99 is None else gateway_p99 - direct_p99,
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
            "Direct and Gateway runs did not use identical comparison settings.",
            "",
            "| Field | Direct | Gateway |",
            "| --- | --- | --- |",
        ])
        for mismatch in comparison["mismatches"]:
            lines.append(
                f"| {mismatch['field']} | {mismatch['direct']} | {mismatch['gateway']} |"
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
        f"| HTTP failed rate | {fmt(direct['httpFailedRate'])} | {fmt(gateway['httpFailedRate'])} | n/a |",
        "",
        "This is a Gateway additional-latency comparison, not a performance-improvement claim.",
        "",
    ])
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Compare k6 direct-upstream and gateway route results.")
    parser.add_argument("--direct", required=True, help="Path to direct-upstream JSON summary")
    parser.add_argument("--gateway", required=True, help="Path to gateway-public-route JSON summary")
    parser.add_argument("--output-dir", default="performance/results", help="Directory for comparison output")
    args = parser.parse_args()

    direct = load_result(args.direct)
    gateway = load_result(args.gateway)
    mismatches = compare_config(direct, gateway)
    comparison = build_comparison(direct, gateway, mismatches)

    run_id = gateway.get("config", {}).get("runId") or datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    run_index = gateway.get("config", {}).get("runIndex") or "1"
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    json_path = output_dir / f"comparison-{run_id}-r{run_index}.json"
    markdown_path = output_dir / f"comparison-{run_id}-r{run_index}.md"

    json_path.write_text(json.dumps(comparison, ensure_ascii=False, indent=2), encoding="utf-8")
    markdown_path.write_text(to_markdown(comparison), encoding="utf-8")

    print(f"Wrote {json_path}")
    print(f"Wrote {markdown_path}")
    if not comparison["comparable"]:
        print("Comparison blocked: run settings differ.")


if __name__ == "__main__":
    main()
