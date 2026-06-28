#!/usr/bin/env python3
import argparse
import json
import statistics
import sys
from datetime import datetime, timezone
from pathlib import Path


PROJECT = "A-AND-I-GATEWAY-SERVER"
RESULT_TYPE = "gateway-local-overhead"
EXPECTED_REPEATS = 3
LOCAL_REGRESSION_NOTE = "운영 최대 처리량이 아니라 로컬 Mock Downstream 기반 회귀 검증 기준"
EXPECTED_GROUPS = (
    ("payload-overhead", 1024),
    ("payload-overhead", 65536),
    ("payload-overhead", 1048576),
    ("route-overhead", "public"),
    ("route-overhead", "protected"),
    ("logging-overhead", "enabled"),
    ("logging-overhead", "disabled"),
)


class ValidationError(ValueError):
    pass


def is_number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def metric(result, metric_name, field):
    return result.get("metrics", {}).get(metric_name, {}).get(field)


def fmt(value, suffix=""):
    if value is None:
        return "[측정 필요]"
    if isinstance(value, (int, float)):
        return f"{value:.3f}{suffix}"
    return f"{value}{suffix}"


def median(values):
    numeric = [value for value in values if is_number(value)]
    if not numeric:
        return None
    return statistics.median(numeric)


def min_max(values):
    numeric = [value for value in values if is_number(value)]
    if not numeric:
        return {"min": None, "max": None}
    return {"min": min(numeric), "max": max(numeric)}


def load_results(input_dir, run_prefix):
    results = []
    for path in sorted(Path(input_dir).glob("*.json")):
        try:
            result = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            continue
        if not isinstance(result, dict) or "testName" not in result:
            continue
        config = result.get("config", {})
        if run_prefix and not str(config.get("runId", "")).startswith(run_prefix):
            continue
        result["_path"] = str(path)
        results.append(result)
    return results


def metric_block(result):
    return {
        "p50Ms": metric(result, "http_req_duration", "med"),
        "p90Ms": metric(result, "http_req_duration", "p90"),
        "p95Ms": metric(result, "http_req_duration", "p95"),
        "p99Ms": metric(result, "http_req_duration", "p99"),
        "throughputRps": metric(result, "http_reqs", "rate"),
        "requestCount": metric(result, "http_reqs", "count"),
        "httpFailedRate": metric(result, "http_req_failed", "rate"),
        "checkRate": metric(result, "checks", "rate"),
        "iterations": metric(result, "iterations", "count"),
        "droppedIterations": metric(result, "dropped_iterations", "count"),
    }


def group_key(result):
    config = result.get("config", {})
    scenario = config.get("scenario")
    if scenario == "payload-overhead":
        return ("payload-overhead", config.get("payloadBytes"))
    if scenario == "route-overhead":
        return ("route-overhead", config.get("routeKind"))
    if scenario == "logging-overhead":
        return ("logging-overhead", config.get("loggingMode"))
    return None


def result_side(result):
    config = result.get("config", {})
    return config.get("overheadTarget") or config.get("target")


def build_pair(pair_index, direct, gateway):
    direct_block = metric_block(direct)
    gateway_block = metric_block(gateway)
    return {
        "pairIndex": pair_index,
        "direct": direct_block,
        "gateway": gateway_block,
        "gatewayAdditionalLatencyMs": {
            "p50": gateway_block["p50Ms"] - direct_block["p50Ms"],
            "p90": gateway_block["p90Ms"] - direct_block["p90Ms"],
            "p95": gateway_block["p95Ms"] - direct_block["p95Ms"],
            "p99": gateway_block["p99Ms"] - direct_block["p99Ms"],
        },
        "httpFailedRate": max(direct_block["httpFailedRate"], gateway_block["httpFailedRate"]),
        "checkRate": min(direct_block["checkRate"], gateway_block["checkRate"]),
        "droppedIterations": max(direct_block["droppedIterations"] or 0, gateway_block["droppedIterations"] or 0),
    }


def validate_pair_inputs(direct, gateway):
    for label, result in (("direct", direct), ("gateway", gateway)):
        for metric_name, field in (
            ("http_req_duration", "med"),
            ("http_req_duration", "p90"),
            ("http_req_duration", "p95"),
            ("http_req_duration", "p99"),
            ("http_reqs", "rate"),
            ("http_reqs", "count"),
            ("http_req_failed", "rate"),
            ("checks", "rate"),
        ):
            value = metric(result, metric_name, field)
            if not is_number(value):
                raise ValidationError(f"{label} missing numeric metric {metric_name}.{field}")
        if result.get("thresholdFailures"):
            raise ValidationError(f"{label} threshold failures: {result['thresholdFailures']}")
    direct_config = direct.get("config", {})
    gateway_config = gateway.get("config", {})
    for field in ("payloadBytes", "mockDelayMs", "routePath", "duration", "vus", "k6Version", "commitSha"):
        if direct_config.get(field) != gateway_config.get(field):
            raise ValidationError(f"direct/gateway config mismatch: {field}")


def summarize_group(key, entries):
    by_pair = {}
    discarded = []
    for result in entries:
        config = result.get("config", {})
        pair_index = config.get("pairIndex")
        side = result_side(result)
        by_pair.setdefault(pair_index, {})[side] = result

    pairs = []
    for pair_index in sorted(by_pair, key=lambda value: str(value)):
        pair = by_pair[pair_index]
        direct = pair.get("direct")
        gateway = pair.get("gateway")
        if not direct or not gateway:
            discarded.append({"pairIndex": pair_index, "reason": "missing direct or gateway result"})
            continue
        try:
            validate_pair_inputs(direct, gateway)
            pairs.append(build_pair(pair_index, direct, gateway))
        except ValidationError as error:
            discarded.append({"pairIndex": pair_index, "reason": str(error)})

    confidence = "확인 완료" if len(pairs) >= EXPECTED_REPEATS else "측정 필요"
    resume_use = "사용 가능" if len(pairs) >= EXPECTED_REPEATS else "사용 비추천"
    first_config = entries[0].get("config", {}) if entries else {}
    return {
        "scenario": key[0],
        "dimension": key[1],
        "confidence": confidence,
        "resumeUse": resume_use,
        "acceptedPairCount": len(pairs),
        "requiredPairCount": EXPECTED_REPEATS,
        "load": {
            "vus": first_config.get("vus"),
            "duration": first_config.get("duration"),
            "payloadBytes": first_config.get("payloadBytes"),
            "mockDelayMs": first_config.get("mockDelayMs"),
            "publicRoutePath": first_config.get("routePath"),
        },
        "target": {
            "gatewayBaseUrl": first_config.get("gatewayBaseUrl"),
            "upstreamBaseUrl": first_config.get("upstreamBaseUrl"),
            "downstreamUrl": first_config.get("downstreamUrl"),
        },
        "median": {
            "directP50Ms": median([pair["direct"]["p50Ms"] for pair in pairs]),
            "directP90Ms": median([pair["direct"]["p90Ms"] for pair in pairs]),
            "directP95Ms": median([pair["direct"]["p95Ms"] for pair in pairs]),
            "directP99Ms": median([pair["direct"]["p99Ms"] for pair in pairs]),
            "gatewayP50Ms": median([pair["gateway"]["p50Ms"] for pair in pairs]),
            "gatewayP90Ms": median([pair["gateway"]["p90Ms"] for pair in pairs]),
            "gatewayP95Ms": median([pair["gateway"]["p95Ms"] for pair in pairs]),
            "gatewayP99Ms": median([pair["gateway"]["p99Ms"] for pair in pairs]),
            "gatewayAdditionalP50Ms": median([pair["gatewayAdditionalLatencyMs"]["p50"] for pair in pairs]),
            "gatewayAdditionalP90Ms": median([pair["gatewayAdditionalLatencyMs"]["p90"] for pair in pairs]),
            "gatewayAdditionalP95Ms": median([pair["gatewayAdditionalLatencyMs"]["p95"] for pair in pairs]),
            "gatewayAdditionalP99Ms": median([pair["gatewayAdditionalLatencyMs"]["p99"] for pair in pairs]),
            "httpFailedRate": median([pair["httpFailedRate"] for pair in pairs]),
            "checkRate": median([pair["checkRate"] for pair in pairs]),
            "directThroughputRps": median([pair["direct"]["throughputRps"] for pair in pairs]),
            "gatewayThroughputRps": median([pair["gateway"]["throughputRps"] for pair in pairs]),
            "directIterations": median([pair["direct"]["iterations"] for pair in pairs]),
            "gatewayIterations": median([pair["gateway"]["iterations"] for pair in pairs]),
            "droppedIterations": median([pair["droppedIterations"] for pair in pairs]),
        },
        "range": {
            "gatewayAdditionalP95Ms": min_max([pair["gatewayAdditionalLatencyMs"]["p95"] for pair in pairs]),
            "gatewayAdditionalP99Ms": min_max([pair["gatewayAdditionalLatencyMs"]["p99"] for pair in pairs]),
        },
        "pairs": pairs,
        "discardedPairs": discarded,
    }


def build_rate_limit(result):
    if result is None:
        return {
            "confidence": "측정 필요",
            "resumeUse": "사용 비추천",
            "expectedAllowed": None,
            "expectedRejected": None,
            "actualAllowed": None,
            "actualRejected": None,
            "checksRate": None,
            "httpFailedRate": None,
        }
    config = result.get("config", {})
    return {
        "confidence": "확인 완료" if result.get("thresholdFailures", []) == [] else "측정 필요",
        "resumeUse": "사용 가능" if result.get("thresholdFailures", []) == [] else "사용 비추천",
        "expectedAllowed": config.get("expectedLoginRateLimitPerMinute"),
        "expectedRejected": config.get("expectedRejectedResponses"),
        "actualAllowed": metric(result, "rate_limit_allowed_responses", "count"),
        "actualRejected": metric(result, "rate_limit_rejected_responses", "count"),
        "checksRate": metric(result, "checks", "rate"),
        "httpFailedRate": metric(result, "http_req_failed", "rate"),
        "thresholdFailures": result.get("thresholdFailures", []),
    }


def build_error_contract(result):
    if result is None:
        return {
            "confidence": "측정 필요",
            "resumeUse": "사용 비추천",
            "status": "[측정 필요]",
            "checksRate": None,
            "httpFailedRate": None,
        }
    passed = result.get("thresholdFailures", []) == [] and metric(result, "checks", "rate") == 1
    return {
        "confidence": "확인 완료" if passed else "측정 필요",
        "resumeUse": "사용 가능" if passed else "사용 비추천",
        "status": "passed" if passed else "needs review",
        "checksRate": metric(result, "checks", "rate"),
        "httpFailedRate": metric(result, "http_req_failed", "rate"),
        "expectReport502": result.get("config", {}).get("expectReport502"),
        "thresholdFailures": result.get("thresholdFailures", []),
    }


def summarize_results(results, environment, generated_at=None, blocking_reasons=None):
    generated_at = generated_at or datetime.now(timezone.utc).isoformat()
    blocking_reasons = blocking_reasons or []
    grouped = {}
    rate_limit = None
    error_contract = None
    for result in results:
        name = result.get("testName")
        if name == "gateway-rate-limit":
            rate_limit = result
        elif name == "gateway-error-contract":
            error_contract = result
        key = group_key(result)
        if key:
            grouped.setdefault(key, []).append(result)

    group_summaries = [summarize_group(key, entries) for key, entries in sorted(grouped.items())]
    accepted_pair_count = sum(group["acceptedPairCount"] for group in group_summaries)
    missing_groups = [
        {"scenario": scenario, "dimension": dimension}
        for scenario, dimension in EXPECTED_GROUPS
        if (scenario, dimension) not in grouped
    ]
    measurement_ready = (
        not missing_groups
        and bool(group_summaries)
        and all(group["acceptedPairCount"] >= EXPECTED_REPEATS for group in group_summaries)
    )
    confidence = "확인 완료" if measurement_ready else "측정 필요"
    resume_use = "사용 가능" if measurement_ready else "사용 비추천"

    return {
        "schemaVersion": 1,
        "project": PROJECT,
        "resultType": RESULT_TYPE,
        "generatedAt": generated_at,
        "measurementStatus": confidence,
        "confidence": confidence,
        "resumeUse": resume_use,
        "acceptedPairCount": accepted_pair_count,
        "requiredPairCountPerDimension": EXPECTED_REPEATS,
        "localRegressionNote": LOCAL_REGRESSION_NOTE,
        "safety": {
            "productionAccess": False,
            "awsAccess": False,
            "discordAccess": False,
            "allowedTargets": ["localhost", "127.0.0.1", "mock-upstream"],
            "blockedTargets": ["aandiclub.com", "api.aandiclub.com", "EC2 public IPv4", "prod", "production"],
        },
        "environment": environment,
        "plannedMeasurement": environment.get("plannedMeasurement", {}),
        "blockingReasons": blocking_reasons,
        "groups": group_summaries,
        "missingGroups": missing_groups,
        "rateLimit": build_rate_limit(rate_limit),
        "errorContract": build_error_contract(error_contract),
        "limitations": [
            "local mock downstream only",
            "not a maximum throughput claim",
            "not production traffic",
            "requires at least 3 direct/gateway pairs before resume use",
        ],
    }


def empty_summary(environment, generated_at, blocking_reasons):
    return summarize_results([], environment, generated_at=generated_at, blocking_reasons=blocking_reasons)


def scenario_label(group):
    labels = {
        "payload-overhead": f"payload {group['dimension']} bytes",
        "route-overhead": f"{group['dimension']} route",
        "logging-overhead": f"logging {group['dimension']}",
    }
    return labels.get(group["scenario"], group["scenario"])


def to_markdown(summary):
    env = summary["environment"]
    planned = summary.get("plannedMeasurement", {})
    target_urls = planned.get("targetUrls", {})
    lines = [
        "# Gateway Local Overhead Measurement",
        "",
        f"> {LOCAL_REGRESSION_NOTE}",
        "",
        f"- Generated At: {summary['generatedAt']}",
        f"- Measurement Status: {summary['measurementStatus']}",
        f"- Resume Use: {summary['resumeUse']}",
        f"- Commit SHA: `{env.get('commitSha', 'unknown')}`",
        f"- k6: {env.get('k6Version', 'unknown')}",
        f"- JVM: {env.get('jvmVersion', 'unknown')}",
        f"- Docker: {env.get('dockerVersion', 'unknown')}",
        f"- Planned VUs: {planned.get('vus', '[측정 필요]')}",
        f"- Planned Duration: {planned.get('duration', '[측정 필요]')}",
        f"- Planned Payload Bytes: {planned.get('payloadBytes', '[측정 필요]')}",
        f"- BASE_URL: {target_urls.get('BASE_URL', '[측정 필요]')}",
        f"- UPSTREAM_BASE_URL: {target_urls.get('UPSTREAM_BASE_URL', '[측정 필요]')}",
        f"- DOWNSTREAM_URL: {target_urls.get('DOWNSTREAM_URL', '[측정 필요]')}",
        f"- Target URL Policy: localhost/127.0.0.1/mock-upstream only",
        "",
    ]

    if summary.get("blockingReasons"):
        lines.extend(["## Measurement Blocked", ""])
        for reason in summary["blockingReasons"]:
            lines.append(f"- {reason}")
        lines.append("")

    if summary.get("missingGroups"):
        lines.extend(["## Missing Required Dimensions", ""])
        for missing in summary["missingGroups"]:
            lines.append(f"- {missing['scenario']} / {missing['dimension']}")
        lines.append("")

    lines.extend([
        "## Direct vs Gateway Overhead",
        "",
        "| Scenario | Pairs | Direct P95 | Gateway P95 | Additional P95 | Additional P99 | HTTP failed | Checks | Throughput Direct/Gateway | Resume Use |",
        "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |",
    ])
    if not summary["groups"]:
        lines.append("| [측정 필요] | 0 | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | [측정 필요] | 사용 비추천 |")
    for group in summary["groups"]:
        median_block = group["median"]
        lines.append(
            f"| {scenario_label(group)} | {group['acceptedPairCount']} | "
            f"{fmt(median_block['directP95Ms'], ' ms')} | {fmt(median_block['gatewayP95Ms'], ' ms')} | "
            f"{fmt(median_block['gatewayAdditionalP95Ms'], ' ms')} | {fmt(median_block['gatewayAdditionalP99Ms'], ' ms')} | "
            f"{fmt(median_block['httpFailedRate'])} | {fmt(median_block['checkRate'])} | "
            f"{fmt(median_block['directThroughputRps'])}/{fmt(median_block['gatewayThroughputRps'])} | {group['resumeUse']} |"
        )

    rate_limit = summary["rateLimit"]
    error_contract = summary["errorContract"]
    lines.extend([
        "",
        "## Rate Limit and Error Contract",
        "",
        "| Scenario | Expected | Actual | Checks | HTTP failed | Resume Use |",
        "| --- | --- | --- | ---: | ---: | --- |",
        f"| Rate limit | allow {fmt(rate_limit['expectedAllowed'])}, reject {fmt(rate_limit['expectedRejected'])} | allow {fmt(rate_limit['actualAllowed'])}, reject {fmt(rate_limit['actualRejected'])} | {fmt(rate_limit['checksRate'])} | {fmt(rate_limit['httpFailedRate'])} | {rate_limit['resumeUse']} |",
        f"| Downstream failure contract | 502 maintained | {error_contract['status']} | {fmt(error_contract['checksRate'])} | {fmt(error_contract['httpFailedRate'])} | {error_contract['resumeUse']} |",
        "",
        "## Resume Sentence Candidates",
        "",
        "- 확인된 경우: 로컬 Mock Downstream 기준 payload 1KB/64KB/1MB별 Gateway P95/P99와 추가 지연을 측정해 라우팅·정책·로깅 계층 회귀 기준을 관리",
        "- 확인된 경우: 구조화 로깅 on/off 비교로 Gateway 요청 추적 기능의 지연 비용을 로컬 기준으로 검증",
        "- 측정 부족 시: [측정 필요] Mock Downstream 기반 k6 시나리오로 Gateway 라우팅·인증·오류 계약의 성능 회귀를 검증",
        "",
    ])
    return "\n".join(lines)


def write_json(summary, path):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_markdown(summary, path):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(to_markdown(summary), encoding="utf-8")


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Summarize local-only Gateway overhead k6 results.")
    parser.add_argument("--input", default="performance/results", type=Path)
    parser.add_argument("--run-prefix", default="")
    parser.add_argument("--out-json", required=True, type=Path)
    parser.add_argument("--out-md", required=True, type=Path)
    parser.add_argument("--docs-json", type=Path)
    parser.add_argument("--allow-empty", action="store_true")
    parser.add_argument("--measurement-date", default="")
    parser.add_argument("--blocking-reason", action="append", default=[])
    parser.add_argument("--commit-sha", default="unknown")
    parser.add_argument("--k6-version", default="unknown")
    parser.add_argument("--jvm-version", default="unknown")
    parser.add_argument("--docker-version", default="unknown")
    parser.add_argument("--planned-vus", default="1")
    parser.add_argument("--planned-duration", default="10s")
    parser.add_argument("--planned-payload-bytes", default="1024,65536,1048576")
    parser.add_argument("--planned-base-url", default="http://localhost:8080")
    parser.add_argument("--planned-upstream-url", default="http://localhost:18080")
    parser.add_argument("--planned-downstream-url", default="http://localhost:18080")
    return parser.parse_args(argv)


def generated_at_from_date(value):
    if not value:
        return None
    return f"{value}T00:00:00+00:00"


def main(argv=None):
    args = parse_args(argv or sys.argv[1:])
    environment = {
        "commitSha": args.commit_sha,
        "k6Version": args.k6_version,
        "jvmVersion": args.jvm_version,
        "dockerVersion": args.docker_version,
        "plannedMeasurement": {
            "vus": args.planned_vus,
            "duration": args.planned_duration,
            "payloadBytes": [
                int(value.strip())
                for value in args.planned_payload_bytes.split(",")
                if value.strip()
            ],
            "targetUrls": {
                "BASE_URL": args.planned_base_url,
                "UPSTREAM_BASE_URL": args.planned_upstream_url,
                "DOWNSTREAM_URL": args.planned_downstream_url,
            },
        },
    }
    results = load_results(args.input, args.run_prefix)
    if not results and not args.allow_empty:
        print("error: no overhead results found; pass --allow-empty to generate a measurement-needed artifact", file=sys.stderr)
        return 2

    summary = summarize_results(
        results,
        environment,
        generated_at=generated_at_from_date(args.measurement_date),
        blocking_reasons=args.blocking_reason,
    )
    write_json(summary, args.out_json)
    write_markdown(summary, args.out_md)
    if args.docs_json:
        write_json(summary, args.docs_json)
    print(args.out_json)
    if args.docs_json:
        print(args.docs_json)
    return 0


if __name__ == "__main__":
    sys.exit(main())
