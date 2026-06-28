import unittest

from performance.aggregate.summarize_gateway_overhead import summarize_results


def result(target, pair_index, p95, scenario="payload-overhead", dimension=1024):
    return {
        "schemaVersion": 1,
        "testName": f"gateway-payload-overhead-{target}",
        "config": {
            "scenario": scenario,
            "overheadTarget": target,
            "pairIndex": pair_index,
            "payloadBytes": dimension,
            "mockDelayMs": 0,
            "routePath": "/v2/blogs",
            "duration": "10s",
            "vus": 1,
            "k6Version": "v1.7.1",
            "commitSha": "abc123",
        },
        "metrics": {
            "http_req_duration": {
                "med": p95 - 2,
                "p90": p95 - 1,
                "p95": p95,
                "p99": p95 + 1,
            },
            "http_reqs": {"rate": 10.0, "count": 100},
            "http_req_failed": {"rate": 0.0},
            "checks": {"rate": 1.0},
            "iterations": {"count": 100},
            "dropped_iterations": {"count": 0},
        },
        "thresholdFailures": [],
    }


class SummarizeGatewayOverheadTest(unittest.TestCase):
    def test_marks_three_pairs_as_resume_ready(self):
        results = []
        for index, direct_p95, gateway_p95 in ((1, 10, 15), (2, 11, 20), (3, 12, 19)):
            results.append(result("direct", index, direct_p95))
            results.append(result("gateway", index, gateway_p95))

        summary = summarize_results(results, {"commitSha": "abc123"})

        group = summary["groups"][0]
        self.assertEqual(group["acceptedPairCount"], 3)
        self.assertEqual(group["confidence"], "확인 완료")
        self.assertEqual(group["resumeUse"], "사용 가능")
        self.assertEqual(group["median"]["gatewayAdditionalP95Ms"], 7)

    def test_marks_under_three_pairs_as_measurement_needed(self):
        results = [
            result("direct", 1, 10),
            result("gateway", 1, 15),
            result("direct", 2, 11),
            result("gateway", 2, 17),
        ]

        summary = summarize_results(results, {"commitSha": "abc123"})

        group = summary["groups"][0]
        self.assertEqual(group["acceptedPairCount"], 2)
        self.assertEqual(group["confidence"], "측정 필요")
        self.assertEqual(group["resumeUse"], "사용 비추천")
        self.assertEqual(summary["resumeUse"], "사용 비추천")


if __name__ == "__main__":
    unittest.main()
