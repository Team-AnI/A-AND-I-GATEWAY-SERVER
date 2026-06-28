#!/usr/bin/env python3
import argparse
import json
import sys
from pathlib import Path


MIN_RECOMMENDED_RUNS = 5


CI_METRICS = [
    ("CI 전체 시간", ("workflow", None), ("workflow", None)),
    ("Backend test", ("steps", "Run tests"), ("jobs", "backend-test")),
    ("Monitor Bot test", ("steps", "Run monitor-bot tests"), ("jobs", "monitor-bot-test")),
    ("Performance asset validation", ("steps", "Validate performance assets"), ("jobs", "performance-assets")),
    ("Build JAR", ("steps", "Build JAR"), ("jobs", "build-jar")),
]

CD_METRICS = [
    ("Gateway image dry-run build", ("steps", "Build and push Docker image"), ("jobs", "gateway-image-build-dry-run")),
    ("Monitor Bot image dry-run build", ("steps", "Build and push monitor-bot Docker image"), ("jobs", "monitor-bot-image-build-dry-run")),
    ("CD dry-run 전체 시간", ("workflow", None), ("workflow", None)),
]


def load(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def value(summary, selector):
    section, name = selector
    if section == "workflow":
        return summary["summary"]["workflowDuration"]
    return summary["summary"].get(section, {}).get(name)


def median(summary, selector):
    item = value(summary, selector)
    if not item:
        return None
    return item.get("medianSeconds")


def count(summary, selector):
    item = value(summary, selector)
    if not item:
        return 0
    return item.get("count") or 0


def improvement(before, after):
    if before in (None, 0) or after is None:
        return None
    return ((before - after) / before) * 100


def confidence(before_count, after_count, before_value, after_value):
    if before_value is None or after_value is None:
        return "확인 필요"
    if before_count < MIN_RECOMMENDED_RUNS or after_count < MIN_RECOMMENDED_RUNS:
        return "측정 필요"
    return "확인 완료"


def resume_use(confidence_value, failure_rate_before, failure_rate_after, conditional=False):
    if confidence_value != "확인 완료":
        return "사용 비추천"
    if (failure_rate_before or 0) > 0 or (failure_rate_after or 0) > 0:
        return "사용 비추천"
    return "조건부 사용" if conditional else "사용 가능"


def row(label, before_summary, after_summary, before_selector, after_selector, conditional=False):
    before_median = median(before_summary, before_selector)
    after_median = median(after_summary, after_selector)
    before_count = count(before_summary, before_selector)
    after_count = count(after_summary, after_selector)
    confidence_value = confidence(before_count, after_count, before_median, after_median)
    return {
        "item": label,
        "beforeMedianSeconds": before_median,
        "afterMedianSeconds": after_median,
        "improvementPercent": improvement(before_median, after_median),
        "beforeRunCount": before_count,
        "afterRunCount": after_count,
        "confidence": confidence_value,
        "resumeUse": resume_use(
            confidence_value,
            before_summary.get("failureRate"),
            after_summary.get("failureRate"),
            conditional=conditional,
        ),
    }


def compare(before_ci, after_ci, before_cd=None, after_cd=None):
    rows = [row(label, before_ci, after_ci, before_sel, after_sel) for label, before_sel, after_sel in CI_METRICS]
    if before_cd and after_cd:
        rows.extend([
            row(label, before_cd, after_cd, before_sel, after_sel, conditional=True)
            for label, before_sel, after_sel in CD_METRICS
        ])

    confirmed = [item for item in rows if item["confidence"] == "확인 완료"]
    return {
        "schemaVersion": 1,
        "resultType": "gateway-cicd-before-after",
        "confidence": "확인 완료" if confirmed else "측정 필요",
        "resumeUse": "사용 가능" if confirmed else "사용 비추천",
        "before": {
            "ci": source_summary(before_ci),
            "cd": source_summary(before_cd) if before_cd else None,
        },
        "after": {
            "ci": source_summary(after_ci),
            "cd": source_summary(after_cd) if after_cd else None,
        },
        "metrics": rows,
        "safety": {
            "productionDeployExecuted": False,
            "awsEcrSshExecuted": False,
            "dockerPushExecuted": False,
            "metadataReadOnly": True,
        },
    }


def source_summary(summary):
    if not summary:
        return None
    return {
        "workflow": summary.get("workflow"),
        "branch": summary.get("branch"),
        "successfulRunCount": summary.get("successfulRunCount"),
        "failureRate": summary.get("failureRate"),
    }


def fmt(value):
    if value is None:
        return "[확인 필요]"
    minutes, seconds = divmod(int(round(value)), 60)
    return f"{minutes}m {seconds}s" if minutes else f"{seconds}s"


def fmt_pct(value):
    if value is None:
        return "[확인 필요]"
    return f"{value:.1f}%"


def to_markdown(summary):
    lines = [
        "# Gateway CI/CD Optimization",
        "",
        "> GitHub Actions metadata 기준입니다. 운영 배포 시간이나 운영 트래픽 처리량이 아닙니다.",
        "",
        "## Before/After",
        "",
        "| 항목 | Before median | After median | 개선율 | 신뢰도 | 사용 여부 |",
        "| :--- | ---: | ---: | ---: | :--- | :--- |",
    ]
    for item in summary["metrics"]:
        lines.append(
            f"| {item['item']} | {fmt(item['beforeMedianSeconds'])} | {fmt(item['afterMedianSeconds'])} | "
            f"{fmt_pct(item['improvementPercent'])} | {item['confidence']} | {item['resumeUse']} |"
        )
    lines.extend([
        "",
        "## Safety",
        "",
        "- Production deploy executed: false",
        "- AWS/ECR/SSH executed: false",
        "- Docker push executed: false",
        "- GitHub Actions metadata read-only 조회",
        "",
        "## Resume Sentence Candidates",
        "",
    ])
    ci_total = next((item for item in summary["metrics"] if item["item"] == "CI 전체 시간"), None)
    if ci_total and ci_total["confidence"] == "확인 완료":
        lines.append(
            "GitHub Actions CI를 Backend test, Monitor Bot test, Performance asset validation, Build job으로 분리해 "
            f"PR 검증 시간을 median {fmt(ci_total['beforeMedianSeconds'])} → {fmt(ci_total['afterMedianSeconds'])}로 "
            f"{fmt_pct(ci_total['improvementPercent'])} 단축"
        )
    else:
        lines.append("[측정 필요] GitHub Actions CI 병렬화 before/after median 표본이 부족해 이력서 사용 비추천")

    cd_total = next((item for item in summary["metrics"] if item["item"] == "CD dry-run 전체 시간"), None)
    if cd_total and cd_total["confidence"] == "확인 완료":
        lines.append(
            "Gateway와 Monitor Bot Docker image build를 병렬화하고 BuildKit cache를 적용해 "
            f"CD dry-run image build 시간을 median {fmt(cd_total['beforeMedianSeconds'])} → {fmt(cd_total['afterMedianSeconds'])}로 "
            f"{fmt_pct(cd_total['improvementPercent'])} 단축"
        )
    else:
        lines.append("[측정 필요] CD dry-run image build before/after median 표본이 부족해 이력서 사용 비추천")
    lines.append("")
    return "\n".join(lines)


def write_outputs(summary, out_json, out_md):
    out_json.parent.mkdir(parents=True, exist_ok=True)
    out_json.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    out_md.parent.mkdir(parents=True, exist_ok=True)
    out_md.write_text(to_markdown(summary), encoding="utf-8")


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Compare CI/CD before and after GitHub Actions metrics.")
    parser.add_argument("--before", required=True, type=Path, help="Before CI metrics JSON")
    parser.add_argument("--after", required=True, type=Path, help="After CI metrics JSON")
    parser.add_argument("--before-cd", type=Path, help="Before CD metrics JSON")
    parser.add_argument("--after-cd", type=Path, help="After CD dry-run metrics JSON")
    parser.add_argument("--out-json", required=True, type=Path)
    parser.add_argument("--out-md", required=True, type=Path)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv or sys.argv[1:])
    summary = compare(
        load(args.before),
        load(args.after),
        load(args.before_cd) if args.before_cd else None,
        load(args.after_cd) if args.after_cd else None,
    )
    write_outputs(summary, args.out_json, args.out_md)
    print(args.out_json)
    return 0


if __name__ == "__main__":
    sys.exit(main())
