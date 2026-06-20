import copy
import unittest

from performance.aggregate import summarize_gateway_runs


def base_config(pair_index):
    return {
        "commitSha": "abcdef123456",
        "gitDirty": False,
        "k6Version": "v1.7.1",
        "executor": "constant-vus",
        "vus": 5,
        "duration": "1m",
        "sleepSeconds": 0.1,
        "payloadBytes": 1024,
        "mockDelayMs": 50,
        "mockStatus": 200,
        "routePath": "/v2/blogs",
        "runId": "20260620T154632Z",
        "runIndex": str(pair_index),
        "runRepeat": 3,
        "runOrder": "alternating",
        "pairOrder": {
            1: "direct-then-gateway",
            2: "gateway-then-direct",
            3: "direct-then-gateway",
        }[pair_index],
        "pairIndex": pair_index,
        "warmupCompleted": True,
    }


def comparison(pair_index, direct_p95, gateway_p95):
    direct_config = base_config(pair_index)
    gateway_config = copy.deepcopy(direct_config)
    return {
        "schemaVersion": 1,
        "comparable": True,
        "errors": [],
        "direct": {
            "p50Ms": direct_p95 - 10,
            "p95Ms": direct_p95,
            "p99Ms": direct_p95 + 5,
            "rps": 30 + pair_index,
            "requestCount": 1000 + pair_index,
            "httpFailedRate": 0,
            "checkRate": 1,
        },
        "gateway": {
            "p50Ms": gateway_p95 - 10,
            "p95Ms": gateway_p95,
            "p99Ms": gateway_p95 + 5,
            "rps": 29 + pair_index,
            "requestCount": 900 + pair_index,
            "httpFailedRate": 0,
            "checkRate": 1,
        },
        "gatewayAdditionalLatencyMs": {
            "p50": gateway_p95 - direct_p95,
            "p95": gateway_p95 - direct_p95,
            "p99": gateway_p95 - direct_p95,
        },
        "config": {
            "direct": direct_config,
            "gateway": gateway_config,
        },
    }


def contract_result():
    return {
        "testName": "gateway-error-contract",
        "thresholdFailures": [],
        "config": {
            "expectReport502": True,
        },
        "metrics": {
            "http_req_failed": {"rate": 0},
            "checks": {"rate": 1},
        },
    }


def rate_limit_result():
    return {
        "testName": "gateway-rate-limit",
        "thresholdFailures": [],
        "config": {
            "expectedLoginRateLimitPerMinute": 10,
            "expectedRejectedResponses": 2,
        },
        "metrics": {
            "http_req_failed": {"rate": 0},
            "checks": {"rate": 1},
            "http_reqs": {"count": 12},
            "rate_limit_allowed_responses": {"count": 10},
            "rate_limit_rejected_responses": {"count": 2},
        },
    }


def base_comparisons():
    return [
        comparison(1, 50, 55),
        comparison(2, 60, 80),
        comparison(3, 70, 72),
    ]


class SummarizeGatewayRunsTests(unittest.TestCase):
    def summarize(self, comparisons=None):
        return summarize_gateway_runs.summarize(
            comparisons or base_comparisons(),
            contract_result(),
            rate_limit_result(),
        )

    def assert_rejected(self, comparisons):
        with self.assertRaises(summarize_gateway_runs.ValidationError):
            self.summarize(comparisons)

    def test_normal_three_pair_median(self):
        summary = self.summarize()
        self.assertEqual(3, summary["acceptedPairCount"])
        self.assertEqual(60, summary["median"]["directP95Ms"])
        self.assertEqual(72, summary["median"]["gatewayP95Ms"])
        self.assertEqual(5, summary["median"]["gatewayAdditionalP95Ms"])

    def test_comparable_false_pair_rejected(self):
        comparisons = base_comparisons()
        comparisons[0]["comparable"] = False
        self.assert_rejected(comparisons)

    def test_sha_mismatch_rejected(self):
        comparisons = base_comparisons()
        comparisons[2]["config"]["direct"]["commitSha"] = "different"
        comparisons[2]["config"]["gateway"]["commitSha"] = "different"
        self.assert_rejected(comparisons)

    def test_k6_version_mismatch_rejected(self):
        comparisons = base_comparisons()
        comparisons[1]["config"]["direct"]["k6Version"] = "v2.0.0"
        comparisons[1]["config"]["gateway"]["k6Version"] = "v2.0.0"
        self.assert_rejected(comparisons)

    def test_git_dirty_rejected(self):
        comparisons = base_comparisons()
        comparisons[0]["config"]["direct"]["gitDirty"] = True
        self.assert_rejected(comparisons)

    def test_missing_pair_rejected(self):
        self.assert_rejected(base_comparisons()[:2])

    def test_pair_order_error_rejected(self):
        comparisons = base_comparisons()
        comparisons[1]["config"]["direct"]["pairOrder"] = "direct-then-gateway"
        comparisons[1]["config"]["gateway"]["pairOrder"] = "direct-then-gateway"
        self.assert_rejected(comparisons)

    def test_metric_missing_rejected(self):
        comparisons = base_comparisons()
        del comparisons[0]["direct"]["p95Ms"]
        self.assert_rejected(comparisons)

    def test_additional_p95_uses_pair_median(self):
        summary = self.summarize()
        self.assertEqual(5, summary["median"]["gatewayAdditionalP95Ms"])
        self.assertNotEqual(
            summary["median"]["gatewayP95Ms"] - summary["median"]["directP95Ms"],
            summary["median"]["gatewayAdditionalP95Ms"],
        )

    def test_same_input_generates_same_json(self):
        summary = self.summarize()
        first = summarize_gateway_runs.dump_summary(summary)
        second = summarize_gateway_runs.dump_summary(summary)
        self.assertEqual(first, second)


if __name__ == "__main__":
    unittest.main()
