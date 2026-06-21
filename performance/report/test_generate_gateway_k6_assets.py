import copy
import tempfile
import unittest
from pathlib import Path

from performance.report import generate_gateway_k6_assets


def aggregate():
    return {
        "median": {
            "directP95Ms": 56.95855,
            "gatewayP95Ms": 65.35735,
            "gatewayAdditionalP95Ms": 8.3988,
            "httpFailedRate": 0,
            "checkRate": 1,
        },
        "pairs": [
            {"additional": {"p95Ms": 12.1385}},
            {"additional": {"p95Ms": 8.3988}},
            {"additional": {"p95Ms": 5.0104}},
        ],
    }


class GenerateGatewayK6AssetsTests(unittest.TestCase):
    def test_missing_required_metric_rejected(self):
        data = aggregate()
        del data["median"]["directP95Ms"]
        with self.assertRaises(generate_gateway_k6_assets.AssetError):
            generate_gateway_k6_assets.render_svg(data)

    def test_negative_p95_rejected(self):
        data = aggregate()
        data["median"]["gatewayP95Ms"] = -1
        with self.assertRaises(generate_gateway_k6_assets.AssetError):
            generate_gateway_k6_assets.render_svg(data)

    def test_additional_p95_must_match_pair_median(self):
        data = aggregate()
        data["median"]["gatewayAdditionalP95Ms"] = 10
        with self.assertRaises(generate_gateway_k6_assets.AssetError):
            generate_gateway_k6_assets.render_svg(data)

    def test_svg_contains_measurements(self):
        svg = generate_gateway_k6_assets.render_svg(aggregate())
        self.assertIn("56.959 ms", svg)
        self.assertIn("65.357 ms", svg)
        self.assertIn("+8.399 ms", svg)

    def test_svg_has_no_external_references_or_script(self):
        svg = generate_gateway_k6_assets.render_svg(aggregate())
        lowered = svg.lower()
        self.assertNotIn("<script", lowered)
        self.assertNotIn("foreignobject", lowered)
        self.assertNotIn("href=", lowered)
        self.assertNotIn("url(", lowered)
        generate_gateway_k6_assets.assert_svg_safe(svg)

    def test_deterministic_generation(self):
        data = aggregate()
        first = generate_gateway_k6_assets.render_svg(data)
        second = generate_gateway_k6_assets.render_svg(copy.deepcopy(data))
        self.assertEqual(first, second)

    def test_check_detects_drift(self):
        data = aggregate()
        svg = generate_gateway_k6_assets.render_svg(data)
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "gateway-k6-overhead.svg"
            path.write_text(svg.replace("Gateway Local", "Changed"), encoding="utf-8")
            with self.assertRaises(generate_gateway_k6_assets.AssetError):
                generate_gateway_k6_assets.check_svg(svg, path)


if __name__ == "__main__":
    unittest.main()
