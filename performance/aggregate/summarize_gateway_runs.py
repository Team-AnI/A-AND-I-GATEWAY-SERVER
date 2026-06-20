#!/usr/bin/env python3
import argparse
import json
import statistics
import sys
from pathlib import Path


PROJECT = "A-AND-I-GATEWAY-SERVER"
RESULT_TYPE = "local-regression-check"
EXPECTED_K6_VERSION = "v1.7.1"
EXPECTED_REPEATS = 3
EXPECTED_RUN_ORDER = "alternating"
EXPECTED_PAIR_ORDERS = {
    1: "direct-then-gateway",
    2: "gateway-then-direct",
    3: "direct-then-gateway",
}
EXPECTED_CONFIG = {
    "vus": 5,
    "duration": "1m",
    "sleepSeconds": 0.1,
    "payloadBytes": 1024,
    "mockDelayMs": 50,
    "mockStatus": 200,
    "routePath": "/v2/blogs",
    "warmupCompleted": True,
}
MAX_HTTP_FAILED_RATE = 0.01
MIN_CHECK_RATE = 0.99


class ValidationError(ValueError):
    pass


def load_json(path):
    with Path(path).open("r", encoding="utf-8") as handle:
        return json.load(handle)


def is_number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def require(condition, message):
    if not condition:
        raise ValidationError(message)


def require_number(value, path):
    require(is_number(value), f"missing numeric metric: {path}")
    return value


def metric(result, metric_name, field):
    return result.get("metrics", {}).get(metric_name, {}).get(field)


def duration_seconds(value):
    if value.endswith("m"):
        return int(value[:-1]) * 60
    if value.endswith("s"):
        return int(value[:-1])
    raise ValidationError(f"unsupported duration: {value}")


def median(values):
    return statistics.median(values)


def min_max(values):
    return {
        "min": min(values),
        "max": max(values),
    }


def pair_label(pair_order):
    return {
        "direct-then-gateway": "Direct -> Gateway",
        "gateway-then-direct": "Gateway -> Direct",
    }.get(pair_order, pair_order)


def validate_metric_block(pair, side):
    block = pair.get(side, {})
    for key in ("p50Ms", "p95Ms", "p99Ms", "rps", "requestCount", "httpFailedRate", "checkRate"):
        require_number(block.get(key), f"{side}.{key}")
    require(block["requestCount"] > 0, f"{side}.requestCount must be positive")
    require(block["httpFailedRate"] < MAX_HTTP_FAILED_RATE, f"{side}.httpFailedRate must be below 1%")
    require(block["checkRate"] > MIN_CHECK_RATE, f"{side}.checkRate must be above 99%")


def validate_additional_block(pair):
    block = pair.get("gatewayAdditionalLatencyMs")
    require(isinstance(block, dict), "missing gatewayAdditionalLatencyMs")
    for key in ("p50", "p95", "p99"):
        require_number(block.get(key), f"gatewayAdditionalLatencyMs.{key}")


def comparison_config(pair):
    config = pair.get("config", {})
    direct = config.get("direct", {})
    gateway = config.get("gateway", {})
    require(isinstance(direct, dict), "missing config.direct")
    require(isinstance(gateway, dict), "missing config.gateway")
    return direct, gateway


def validate_comparison(pair):
    require(pair.get("comparable") is True, "comparison must be comparable")
    validate_metric_block(pair, "direct")
    validate_metric_block(pair, "gateway")
    validate_additional_block(pair)

    direct_config, gateway_config = comparison_config(pair)
    for key in EXPECTED_CONFIG:
        require(direct_config.get(key) == gateway_config.get(key), f"direct/gateway config mismatch: {key}")
        require(direct_config.get(key) == EXPECTED_CONFIG[key], f"unexpected {key}: {direct_config.get(key)}")

    require(direct_config.get("commitSha") == gateway_config.get("commitSha"), "direct/gateway SHA mismatch")
    require(direct_config.get("commitSha") not in (None, "", "unknown"), "commitSha must be recorded")
    require(direct_config.get("k6Version") == gateway_config.get("k6Version"), "direct/gateway k6Version mismatch")
    require(direct_config.get("k6Version") == EXPECTED_K6_VERSION, "k6Version must be v1.7.1")
    require(direct_config.get("gitDirty") is False and gateway_config.get("gitDirty") is False, "gitDirty must be false")
    require(direct_config.get("runRepeat") == EXPECTED_REPEATS, "runRepeat must be 3")
    require(direct_config.get("runOrder") == EXPECTED_RUN_ORDER, "runOrder must be alternating")
    require(direct_config.get("pairIndex") == gateway_config.get("pairIndex"), "direct/gateway pairIndex mismatch")
    require(direct_config.get("pairOrder") == gateway_config.get("pairOrder"), "direct/gateway pairOrder mismatch")
    return direct_config


def validate_comparisons(comparisons):
    require(len(comparisons) == EXPECTED_REPEATS, "exactly 3 comparison files are required")
    configs = []
    for pair in comparisons:
        configs.append(validate_comparison(pair))

    by_index = {config["pairIndex"]: (pair, config) for pair, config in zip(comparisons, configs)}
    require(set(by_index) == {1, 2, 3}, "pairIndex values must be 1, 2, 3")

    baseline = configs[0]
    stable_fields = (
        "commitSha",
        "k6Version",
        "gitDirty",
        "vus",
        "duration",
        "sleepSeconds",
        "payloadBytes",
        "mockDelayMs",
        "mockStatus",
        "routePath",
        "runRepeat",
        "runOrder",
        "warmupCompleted",
    )
    for config in configs[1:]:
        for field in stable_fields:
            require(config.get(field) == baseline.get(field), f"comparison config mismatch: {field}")

    ordered = []
    for pair_index in (1, 2, 3):
        pair, config = by_index[pair_index]
        expected_order = EXPECTED_PAIR_ORDERS[pair_index]
        require(config.get("pairOrder") == expected_order, f"pair {pair_index} order must be {expected_order}")
        ordered.append((pair, config))
    return ordered


def contract_passed(result):
    return (
        metric(result, "http_req_failed", "rate") == 0
        and metric(result, "checks", "rate") == 1
        and result.get("thresholdFailures", []) == []
    )


def build_contracts(contract_result):
    require(contract_result is not None, "missing gateway error contract result")
    require(contract_result.get("testName") == "gateway-error-contract", "unexpected contract result")
    require(contract_passed(contract_result), "gateway error contract did not pass")
    return {
        "401": True,
        "403": True,
        "404": True,
        "429": True,
        "502": bool(contract_result.get("config", {}).get("expectReport502")),
    }


def build_rate_limit(rate_limit_result):
    require(rate_limit_result is not None, "missing gateway rate limit result")
    require(rate_limit_result.get("testName") == "gateway-rate-limit", "unexpected rate limit result")
    require(contract_passed(rate_limit_result), "gateway rate limit contract did not pass")
    allowed = require_number(metric(rate_limit_result, "rate_limit_allowed_responses", "count"), "rateLimit.allowed")
    rejected = require_number(metric(rate_limit_result, "rate_limit_rejected_responses", "count"), "rateLimit.rejected")
    request_count = require_number(metric(rate_limit_result, "http_reqs", "count"), "rateLimit.requestCount")
    expected_allowed = rate_limit_result.get("config", {}).get("expectedLoginRateLimitPerMinute")
    expected_rejected = rate_limit_result.get("config", {}).get("expectedRejectedResponses")
    require(allowed == expected_allowed, f"allowed rate limit count mismatch: {allowed}")
    require(rejected == expected_rejected, f"rejected rate limit count mismatch: {rejected}")
    unexpected = request_count - allowed - rejected
    require(unexpected == 0, f"unexpected rate limit responses: {unexpected}")
    return {
        "allowed": int(allowed),
        "rejected": int(rejected),
        "unexpected": int(unexpected),
    }


def build_pair(pair, config):
    direct = pair["direct"]
    gateway = pair["gateway"]
    additional = pair["gatewayAdditionalLatencyMs"]
    return {
        "pairIndex": config["pairIndex"],
        "order": pair_label(config["pairOrder"]),
        "direct": {
            "p50Ms": direct["p50Ms"],
            "p95Ms": direct["p95Ms"],
            "p99Ms": direct["p99Ms"],
            "rps": direct["rps"],
            "requestCount": direct["requestCount"],
            "httpFailedRate": direct["httpFailedRate"],
            "checkRate": direct["checkRate"],
        },
        "gateway": {
            "p50Ms": gateway["p50Ms"],
            "p95Ms": gateway["p95Ms"],
            "p99Ms": gateway["p99Ms"],
            "rps": gateway["rps"],
            "requestCount": gateway["requestCount"],
            "httpFailedRate": gateway["httpFailedRate"],
            "checkRate": gateway["checkRate"],
        },
        "additional": {
            "p50Ms": additional["p50"],
            "p95Ms": additional["p95"],
            "p99Ms": additional["p99"],
        },
        "httpFailedRate": max(direct["httpFailedRate"], gateway["httpFailedRate"]),
        "checkRate": min(direct["checkRate"], gateway["checkRate"]),
    }


def summarize(comparisons, contract_result, rate_limit_result):
    ordered = validate_comparisons(comparisons)
    pairs = [build_pair(pair, config) for pair, config in ordered]
    baseline = ordered[0][1]

    direct_p50 = [pair["direct"]["p50Ms"] for pair in pairs]
    direct_p95 = [pair["direct"]["p95Ms"] for pair in pairs]
    direct_p99 = [pair["direct"]["p99Ms"] for pair in pairs]
    gateway_p50 = [pair["gateway"]["p50Ms"] for pair in pairs]
    gateway_p95 = [pair["gateway"]["p95Ms"] for pair in pairs]
    gateway_p99 = [pair["gateway"]["p99Ms"] for pair in pairs]
    additional_p50 = [pair["additional"]["p50Ms"] for pair in pairs]
    additional_p95 = [pair["additional"]["p95Ms"] for pair in pairs]
    additional_p99 = [pair["additional"]["p99Ms"] for pair in pairs]
    direct_rps = [pair["direct"]["rps"] for pair in pairs]
    gateway_rps = [pair["gateway"]["rps"] for pair in pairs]

    return {
        "schemaVersion": 1,
        "project": PROJECT,
        "resultType": RESULT_TYPE,
        "measurementTargetSha": baseline["commitSha"],
        "k6Version": baseline["k6Version"],
        "acceptedPairCount": len(pairs),
        "discardedPairs": [],
        "load": {
            "vus": baseline["vus"],
            "durationSeconds": duration_seconds(baseline["duration"]),
            "repeats": baseline["runRepeat"],
            "payloadBytes": baseline["payloadBytes"],
            "mockDelayMs": baseline["mockDelayMs"],
            "sleepSeconds": baseline["sleepSeconds"],
        },
        "median": {
            "directP50Ms": median(direct_p50),
            "directP95Ms": median(direct_p95),
            "directP99Ms": median(direct_p99),
            "gatewayP50Ms": median(gateway_p50),
            "gatewayP95Ms": median(gateway_p95),
            "gatewayP99Ms": median(gateway_p99),
            "gatewayAdditionalP50Ms": median(additional_p50),
            "gatewayAdditionalP95Ms": median(additional_p95),
            "gatewayAdditionalP99Ms": median(additional_p99),
            "directRps": median(direct_rps),
            "gatewayRps": median(gateway_rps),
            "httpFailedRate": median([pair["httpFailedRate"] for pair in pairs]),
            "checkRate": median([pair["checkRate"] for pair in pairs]),
        },
        "range": {
            "directP95Ms": min_max(direct_p95),
            "gatewayP95Ms": min_max(gateway_p95),
            "gatewayAdditionalP95Ms": min_max(additional_p95),
            "directRps": min_max(direct_rps),
            "gatewayRps": min_max(gateway_rps),
        },
        "pairs": pairs,
        "contracts": build_contracts(contract_result),
        "rateLimit": build_rate_limit(rate_limit_result),
        "limitations": [
            "local mock downstream",
            "not a throughput claim",
            "not a performance improvement claim",
        ],
    }


def run_id_from_comparisons(comparisons):
    config = comparisons[0].get("config", {}).get("direct", {})
    run_id = config.get("runId")
    require(run_id, "runId must be recorded")
    return run_id


def default_output_path(run_id):
    date_part = run_id[:8]
    formatted = f"{date_part[:4]}-{date_part[4:6]}-{date_part[6:8]}"
    return Path("docs/performance/data") / f"{formatted}-gateway-local-check.json"


def infer_contract_path(comparison_paths, run_id):
    path = Path(comparison_paths[0]).parent / f"gateway-error-contract-{run_id}-rcontract.json"
    require(path.exists(), f"missing inferred contract result: {path}")
    return path


def infer_rate_limit_path(comparison_paths, run_id):
    path = Path(comparison_paths[0]).parent / f"gateway-rate-limit-{run_id}-rrate-limit.json"
    require(path.exists(), f"missing inferred rate limit result: {path}")
    return path


def dump_summary(summary):
    return json.dumps(summary, ensure_ascii=False, indent=2) + "\n"


def write_summary(summary, path):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(dump_summary(summary), encoding="utf-8")


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Summarize accepted local Gateway k6 comparison pairs.")
    parser.add_argument("comparisons", nargs=3, type=Path)
    parser.add_argument("--contract-result", type=Path)
    parser.add_argument("--rate-limit-result", type=Path)
    parser.add_argument("--output", type=Path)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv or sys.argv[1:])
    comparisons = [load_json(path) for path in args.comparisons]
    run_id = run_id_from_comparisons(comparisons)
    contract_path = args.contract_result or infer_contract_path(args.comparisons, run_id)
    rate_limit_path = args.rate_limit_result or infer_rate_limit_path(args.comparisons, run_id)
    output_path = args.output or default_output_path(run_id)

    summary = summarize(comparisons, load_json(contract_path), load_json(rate_limit_path))
    write_summary(summary, output_path)
    print(output_path)


if __name__ == "__main__":
    try:
        main()
    except ValidationError as error:
        print(f"error: {error}", file=sys.stderr)
        sys.exit(2)
