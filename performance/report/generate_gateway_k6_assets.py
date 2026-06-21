#!/usr/bin/env python3
import argparse
import html
import json
import statistics
import sys
from pathlib import Path


DEFAULT_OUTPUT = Path("docs/assets/performance/gateway-k6-overhead.svg")
DATA_GLOB = "docs/performance/data/*-gateway-local-check.json"


class AssetError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise AssetError(message)


def is_number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def require_number(value, name):
    require(is_number(value), f"missing numeric metric: {name}")
    return value


def load_json(path):
    with Path(path).open("r", encoding="utf-8") as handle:
        return json.load(handle)


def default_input_path():
    matches = sorted(Path(".").glob(DATA_GLOB))
    require(matches, f"no aggregate JSON found: {DATA_GLOB}")
    return matches[-1]


def format_ms(value):
    return f"{value:.3f} ms"


def format_percent(value):
    return f"{value * 100:.2f}%"


def median_pair_additional_p95(data):
    pairs = data.get("pairs", [])
    require(len(pairs) == 3, "aggregate must contain 3 pairs")
    values = []
    for index, pair in enumerate(pairs, start=1):
        values.append(require_number(pair.get("additional", {}).get("p95Ms"), f"pairs[{index}].additional.p95Ms"))
    return statistics.median(values)


def extract_metrics(data):
    median = data.get("median", {})
    direct_p95 = require_number(median.get("directP95Ms"), "median.directP95Ms")
    gateway_p95 = require_number(median.get("gatewayP95Ms"), "median.gatewayP95Ms")
    additional_p95 = require_number(median.get("gatewayAdditionalP95Ms"), "median.gatewayAdditionalP95Ms")
    for name, value in (
        ("directP95Ms", direct_p95),
        ("gatewayP95Ms", gateway_p95),
        ("gatewayAdditionalP95Ms", additional_p95),
    ):
        require(value >= 0, f"{name} must be non-negative")

    pair_median = median_pair_additional_p95(data)
    require(abs(pair_median - additional_p95) < 1e-9, "Additional P95 must match pair median")
    return {
        "directP95Ms": direct_p95,
        "gatewayP95Ms": gateway_p95,
        "gatewayAdditionalP95Ms": additional_p95,
        "httpFailedRate": require_number(median.get("httpFailedRate"), "median.httpFailedRate"),
        "checkRate": require_number(median.get("checkRate"), "median.checkRate"),
    }


def svg_text(x, y, text, size=16, weight=400, color="#17212b", anchor="start"):
    return (
        f'<text x="{x}" y="{y}" fill="{color}" font-size="{size}" '
        f'font-family="Inter, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif" '
        f'font-weight="{weight}" text-anchor="{anchor}">{html.escape(text)}</text>'
    )


def render_svg(data):
    metrics = extract_metrics(data)
    direct = metrics["directP95Ms"]
    gateway = metrics["gatewayP95Ms"]
    additional = metrics["gatewayAdditionalP95Ms"]
    max_value = max(direct, gateway, 1)
    bar_x = 230
    bar_width = 520
    direct_width = direct / max_value * bar_width
    gateway_width = gateway / max_value * bar_width
    direct_label = format_ms(direct)
    gateway_label = format_ms(gateway)
    additional_label = f"+{additional:.3f} ms"

    return "\n".join([
        '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="320" viewBox="0 0 900 320" role="img" aria-labelledby="title desc">',
        '<title id="title">Gateway Local Routing Overhead</title>',
        '<desc id="desc">Mock delay 50 ms, payload 1 KB, 5 VUs, median of 3 pairs</desc>',
        '<rect width="900" height="320" fill="#f7f8f4"/>',
        '<rect x="0" y="0" width="900" height="320" fill="none" stroke="#d8ded2"/>',
        svg_text(48, 54, "Gateway Local Routing Overhead", 28, 700, "#102027"),
        svg_text(48, 82, "Mock delay 50 ms · payload 1 KB · 5 VUs · median of 3 pairs", 15, 500, "#50615d"),
        svg_text(68, 148, "Direct P95", 17, 650, "#102027"),
        f'<line x1="{bar_x}" y1="142" x2="{bar_x + direct_width:.3f}" y2="142" stroke="#2f7d6d" stroke-width="18" stroke-linecap="round"/>',
        svg_text(780, 148, direct_label, 17, 700, "#102027"),
        svg_text(68, 204, "Gateway P95", 17, 650, "#102027"),
        f'<line x1="{bar_x}" y1="198" x2="{bar_x + gateway_width:.3f}" y2="198" stroke="#c95d41" stroke-width="18" stroke-linecap="round"/>',
        svg_text(780, 204, gateway_label, 17, 700, "#102027"),
        f'<line x1="{bar_x + direct_width:.3f}" y1="178" x2="{bar_x + gateway_width:.3f}" y2="178" stroke="#9d6b2f" stroke-width="2" stroke-dasharray="4 5"/>',
        f'<circle cx="{bar_x + direct_width:.3f}" cy="178" r="4" fill="#9d6b2f"/>',
        f'<circle cx="{bar_x + gateway_width:.3f}" cy="178" r="4" fill="#9d6b2f"/>',
        svg_text(min(bar_x + gateway_width + 18, 820), 183, additional_label, 16, 700, "#7a4b16"),
        svg_text(48, 276, "Local regression evidence; not production capacity", 14, 500, "#50615d"),
        '</svg>',
        '',
    ])


def assert_svg_safe(svg):
    lowered = svg.lower()
    require("<script" not in lowered, "SVG must not include script")
    require("foreignobject" not in lowered, "SVG must not include foreignObject")
    require("href=" not in lowered and "xlink:" not in lowered, "SVG must not include external references")
    require("url(" not in lowered, "SVG must not include URL references")


def write_svg(svg, output_path):
    assert_svg_safe(svg)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(svg, encoding="utf-8")


def check_svg(svg, output_path):
    require(output_path.exists(), f"SVG does not exist: {output_path}")
    current = output_path.read_text(encoding="utf-8")
    require(current == svg, f"SVG drift detected: {output_path}")


def check_readme(data, readme_path):
    metrics = extract_metrics(data)
    readme = readme_path.read_text(encoding="utf-8")
    expected = [
        format_ms(metrics["directP95Ms"]),
        format_ms(metrics["gatewayP95Ms"]),
        format_ms(metrics["gatewayAdditionalP95Ms"]),
        format_percent(metrics["httpFailedRate"]),
        format_percent(metrics["checkRate"]),
    ]
    missing = [value for value in expected if value not in readme]
    require(not missing, f"README is missing aggregate values: {', '.join(missing)}")


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Generate deterministic Gateway k6 SVG assets.")
    parser.add_argument("aggregate", nargs="?", type=Path)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--check-readme", type=Path)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv or sys.argv[1:])
    aggregate_path = args.aggregate or default_input_path()
    data = load_json(aggregate_path)
    svg = render_svg(data)
    assert_svg_safe(svg)
    if args.check:
        check_svg(svg, args.output)
    else:
        write_svg(svg, args.output)
    if args.check_readme:
        check_readme(data, args.check_readme)
    print(args.output)


if __name__ == "__main__":
    try:
        main()
    except AssetError as error:
        print(f"error: {error}", file=sys.stderr)
        sys.exit(2)
