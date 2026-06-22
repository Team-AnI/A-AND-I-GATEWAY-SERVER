#!/usr/bin/env python3
import argparse
import html
import json
import statistics
import sys
from pathlib import Path


DEFAULT_OUTPUT = Path("docs/assets/performance/gateway-k6-overhead.svg")
DATA_GLOB = "docs/performance/data/*-gateway-local-check.json"

PRIMARY = "#0F172A"
BLUE = "#2563EB"
BLUE_HOVER = "#1D4ED8"
BLUE_BG = "#DBEAFE"
SKY_BG = "#E0F2FE"
SURFACE = "#FFFFFF"
NEUTRAL = "#F8FAFC"
MUTED = "#F1F5F9"
TEXT_MUTED = "#475569"
TEXT_SUBTLE = "#64748B"
BORDER = "#E2E8F0"
WARNING = "#F59E0B"
WARNING_BG = "#FEF3C7"
SLATE_BAR = "#64748B"


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
        values.append(require_number(
            pair.get("additional", {}).get("p95Ms"),
            f"pairs[{index}].additional.p95Ms",
        ))
    return statistics.median(values)


def extract_metrics(data):
    median = data.get("median", {})
    direct_p95 = require_number(median.get("directP95Ms"), "median.directP95Ms")
    gateway_p95 = require_number(median.get("gatewayP95Ms"), "median.gatewayP95Ms")
    additional_p95 = require_number(
        median.get("gatewayAdditionalP95Ms"),
        "median.gatewayAdditionalP95Ms",
    )
    for name, value in (
        ("directP95Ms", direct_p95),
        ("gatewayP95Ms", gateway_p95),
        ("gatewayAdditionalP95Ms", additional_p95),
    ):
        require(value >= 0, f"{name} must be non-negative")

    pair_median = median_pair_additional_p95(data)
    require(
        abs(pair_median - additional_p95) < 1e-9,
        "Additional P95 must match pair median",
    )
    return {
        "directP95Ms": direct_p95,
        "gatewayP95Ms": gateway_p95,
        "gatewayAdditionalP95Ms": additional_p95,
        "httpFailedRate": require_number(
            median.get("httpFailedRate"),
            "median.httpFailedRate",
        ),
        "checkRate": require_number(median.get("checkRate"), "median.checkRate"),
    }


def svg_text(x, y, text, size=16, weight=400, color=PRIMARY, anchor="start", mono=False):
    family = (
        "'JetBrains Mono','SFMono-Regular',Consolas,monospace"
        if mono
        else "'Noto Sans CJK KR',Inter,Pretendard,system-ui,sans-serif"
    )
    return (
        f'<text x="{x}" y="{y}" fill="{color}" font-size="{size}" '
        f'font-family="{family}" font-weight="{weight}" '
        f'text-anchor="{anchor}">{html.escape(text)}</text>'
    )


def rect(x, y, width, height, fill=SURFACE, stroke=BORDER, radius=16):
    return (
        f'<rect x="{x}" y="{y}" width="{width}" height="{height}" '
        f'rx="{radius}" fill="{fill}" stroke="{stroke}"/>'
    )


def pill(x, y, width, text, fill=WARNING_BG, color=WARNING):
    return "\n".join([
        f'<rect x="{x}" y="{y}" width="{width}" height="30" '
        f'rx="15" fill="{fill}"/>',
        svg_text(x + width / 2, y + 20, text, 12, 800, color, "middle"),
    ])


def render_svg(data):
    metrics = extract_metrics(data)
    direct = metrics["directP95Ms"]
    gateway = metrics["gatewayP95Ms"]
    additional = metrics["gatewayAdditionalP95Ms"]

    max_value = max(direct, gateway, 1)
    bar_x = 270
    bar_width = 742
    direct_width = direct / max_value * bar_width
    gateway_width = gateway / max_value * bar_width

    return "\n".join([
        '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="550" '
        'viewBox="0 0 1200 550" role="img" aria-labelledby="title desc">',
        '<title id="title">Gateway 경유 추가 지연</title>',
        '<desc id="desc">Direct P95와 Gateway P95를 비교하고 추가 지연, '
        '실패율과 check 성공률을 보여주는 로컬 k6 회귀 결과</desc>',
        f'<rect width="1200" height="550" fill="{NEUTRAL}"/>',
        f'<rect x="0" y="0" width="1200" height="8" fill="{BLUE}"/>',
        svg_text(48, 48, "K6 · LOCAL REGRESSION", 12, 800, BLUE),
        svg_text(48, 94, "Gateway 경유 추가 지연", 34, 800, PRIMARY),
        svg_text(
            48,
            126,
            "Mock delay 50 ms · payload 1 KB · 5 VUs · 3회 중앙값",
            15,
            500,
            TEXT_MUTED,
        ),
        rect(48, 166, 1104, 228),
        svg_text(80, 216, "Direct P95", 16, 750, PRIMARY),
        svg_text(1116, 216, format_ms(direct), 18, 800, PRIMARY, "end"),
        f'<rect x="{bar_x}" y="200" width="{bar_width}" height="18" '
        f'rx="9" fill="{MUTED}"/>',
        f'<rect x="{bar_x}" y="200" width="{direct_width:.3f}" height="18" '
        f'rx="9" fill="{SLATE_BAR}"/>',
        svg_text(80, 298, "Gateway P95", 16, 750, PRIMARY),
        svg_text(1116, 298, format_ms(gateway), 18, 800, PRIMARY, "end"),
        f'<rect x="{bar_x}" y="282" width="{bar_width}" height="18" '
        f'rx="9" fill="{MUTED}"/>',
        f'<rect x="{bar_x}" y="282" width="{gateway_width:.3f}" height="18" '
        f'rx="9" fill="{BLUE}"/>',
        pill(930, 326, 186, f"+{additional:.3f} ms overhead"),
        f'<rect x="48" y="424" width="1104" height="78" '
        f'rx="14" fill="{SKY_BG}"/>',
        svg_text(
            76,
            455,
            "HTTP 실패율 "
            + format_percent(metrics["httpFailedRate"])
            + " · Check 성공률 "
            + format_percent(metrics["checkRate"]),
            15,
            750,
            BLUE_HOVER,
        ),
        svg_text(
            76,
            484,
            "Gateway 정책·라우팅·로깅 계층의 회귀를 확인하는 로컬 기준입니다.",
            13.5,
            500,
            TEXT_MUTED,
        ),
        "</svg>",
        "",
    ])


def assert_svg_safe(svg):
    lowered = svg.lower()
    require("<script" not in lowered, "SVG must not include script")
    require("foreignobject" not in lowered, "SVG must not include foreignObject")
    require(
        "href=" not in lowered and "xlink:" not in lowered,
        "SVG must not include external references",
    )
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
    parser = argparse.ArgumentParser(
        description="Generate deterministic Gateway k6 SVG assets."
    )
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
