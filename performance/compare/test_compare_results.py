import copy
import tempfile
import unittest
from pathlib import Path

from performance.compare import compare_results


def base_result(test_name):
    return {
        "schemaVersion": 1,
        "testName": test_name,
        "thresholdFailures": [],
        "config": {
            "commitSha": "abcdef123456",
            "gitDirty": False,
            "executor": "constant-vus",
            "vus": 1,
            "duration": "3s",
            "sleepSeconds": 0.1,
            "p95ThresholdMs": None,
            "payloadBytes": 128,
            "mockDelayMs": 0,
            "mockStatus": 200,
            "routePath": "/v2/blogs",
            "runRepeat": 1,
            "pairIndex": 1,
            "pairOrder": "direct-then-gateway",
            "warmupCompleted": True,
            "k6Version": "1.4.1",
        },
        "metrics": {
            "http_req_duration": {
                "med": 1.0,
                "p95": 2.0,
                "p99": 3.0,
            },
            "http_reqs": {
                "rate": 10.0,
                "count": 30,
            },
            "http_req_failed": {
                "rate": 0.0,
            },
            "checks": {
                "rate": 1.0,
            },
            "dropped_iterations": {
                "count": 0,
            },
        },
    }


class CompareResultsTests(unittest.TestCase):
    def setUp(self):
        self.direct = base_result("direct-upstream")
        self.gateway = base_result("gateway-public-route")
        self.gateway["metrics"]["http_req_duration"] = {"med": 2.0, "p95": 3.0, "p99": 4.0}

    def assert_not_comparable(self, direct=None, gateway=None):
        errors = compare_results.validate(direct or self.direct, gateway or self.gateway)
        self.assertTrue(errors)
        return errors

    def test_normal_comparison(self):
        errors = compare_results.validate(self.direct, self.gateway)
        self.assertEqual([], errors)
        comparison = compare_results.build_comparison(self.direct, self.gateway, errors)
        self.assertTrue(comparison["comparable"])
        self.assertEqual(1.0, comparison["gatewayAdditionalLatencyMs"]["p50"])

    def test_config_mismatch(self):
        self.gateway["config"]["duration"] = "5s"
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "duration" for error in errors))

    def test_sleep_seconds_mismatch(self):
        self.gateway["config"]["sleepSeconds"] = 0.2
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "sleepSeconds" for error in errors))

    def test_p95_threshold_mismatch(self):
        self.direct["config"]["p95ThresholdMs"] = 100
        self.gateway["config"]["p95ThresholdMs"] = 200
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "p95ThresholdMs" for error in errors))

    def test_wrong_test_name(self):
        self.gateway["testName"] = "direct-upstream"
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "testName" for error in errors))

    def test_missing_metric(self):
        del self.direct["metrics"]["http_reqs"]["rate"]
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "http_reqs.rate" for error in errors))

    def test_null_p95(self):
        self.direct["metrics"]["http_req_duration"]["p95"] = None
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "http_req_duration.p95" for error in errors))

    def test_unknown_commit_sha(self):
        self.direct["config"]["commitSha"] = "unknown"
        self.gateway["config"]["commitSha"] = "unknown"
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "commitSha" for error in errors))

    def test_unknown_k6_version(self):
        self.direct["config"]["k6Version"] = "unknown"
        self.gateway["config"]["k6Version"] = "unknown"
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "k6Version" for error in errors))

    def test_both_dirty_results_are_rejected(self):
        self.direct["config"]["gitDirty"] = True
        self.gateway["config"]["gitDirty"] = True
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("message") == "gitDirty must be false" for error in errors))

    def test_one_dirty_result_is_rejected(self):
        self.gateway["config"]["gitDirty"] = True
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("message") == "gitDirty must be false" for error in errors))

    def test_clean_results_are_allowed(self):
        self.direct["config"]["gitDirty"] = False
        self.gateway["config"]["gitDirty"] = False
        errors = compare_results.validate(self.direct, self.gateway)
        self.assertEqual([], errors)

    def test_check_failure(self):
        self.gateway["metrics"]["checks"]["rate"] = 0.5
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "checks.rate" for error in errors))

    def test_http_failure_rate_exceeded(self):
        self.direct["metrics"]["http_req_failed"]["rate"] = 0.02
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "http_req_failed.rate" for error in errors))

    def test_dropped_iterations_exist(self):
        self.gateway["metrics"]["dropped_iterations"]["count"] = 1
        errors = self.assert_not_comparable()
        self.assertTrue(any(error.get("field") == "dropped_iterations.count" for error in errors))

    def test_comparison_failure_writes_output_and_exits_two(self):
        self.gateway["config"]["duration"] = "5s"
        errors = compare_results.validate(self.direct, self.gateway)
        comparison = compare_results.build_comparison(self.direct, self.gateway, errors)
        with tempfile.TemporaryDirectory() as temp_dir:
            json_path, markdown_path = compare_results.write_outputs(comparison, Path(temp_dir))
            self.assertTrue(json_path.exists())
            self.assertTrue(markdown_path.exists())
            self.assertFalse(comparison["comparable"])


if __name__ == "__main__":
    unittest.main()
