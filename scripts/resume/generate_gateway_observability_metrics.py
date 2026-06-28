#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


KOTLIN_TEST = re.compile(r"@Test\b")
GO_TEST = re.compile(r"^func Test[A-Za-z0-9_]+\(", re.MULTILINE)

EVIDENCE = {
    "traceIdRequestIdTests": {
        "label": "traceId/requestId tests",
        "files": [
            "src/test/kotlin/com/aandi/gateway/observability/TraceIdRequestIdTest.kt",
        ],
    },
    "errorContractTests": {
        "label": "Gateway error contract tests",
        "files": [
            "src/test/kotlin/com/aandi/gateway/errorcontract/GatewayErrorContractTest.kt",
        ],
    },
    "logRedactionTests": {
        "label": "structured log redaction tests",
        "files": [
            "src/test/kotlin/com/aandi/gateway/logging/StructuredLogRedactionTest.kt",
        ],
    },
    "existingLoggingContractTests": {
        "label": "existing Gateway logging contract tests",
        "files": [
            "src/test/kotlin/com/aandi/gateway/logging/ApiLoggingContractTests.kt",
        ],
    },
    "monitorBotMockTests": {
        "label": "Monitor Bot mock tests",
        "files": [
            "monitor-bot/internal/discord/observability_handlers_test.go",
            "monitor-bot/internal/discord/interactions_test.go",
            "monitor-bot/internal/discord/commands_test.go",
            "monitor-bot/internal/cloudwatch/queries_test.go",
            "monitor-bot/internal/monitor/alerts_test.go",
            "monitor-bot/internal/monitor/dashboard_test.go",
        ],
    },
}

SAFETY_FLAGS = {
    "actualAwsCloudWatchAccess": False,
    "actualDiscordApiAccess": False,
    "productionUrlAccess": False,
    "productionSecretStored": False,
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate Gateway observability resume metrics.")
    parser.add_argument("--out-json", required=True)
    parser.add_argument("--out-md", required=True)
    return parser.parse_args()


def count_tests(path: Path) -> int:
    if not path.exists():
        return 0
    text = path.read_text(encoding="utf-8")
    if path.suffix == ".go":
        return len(GO_TEST.findall(text))
    return len(KOTLIN_TEST.findall(text))


def build_summary() -> dict[str, Any]:
    generated_at = datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
    categories = {}
    for key, item in EVIDENCE.items():
        files = item["files"]
        file_counts = [
            {
                "path": file,
                "testCount": count_tests(Path(file)),
            }
            for file in files
        ]
        categories[key] = {
            "label": item["label"],
            "testCount": sum(file_count["testCount"] for file_count in file_counts),
            "files": file_counts,
        }

    gateway_observability_tests = (
        categories["traceIdRequestIdTests"]["testCount"]
        + categories["errorContractTests"]["testCount"]
        + categories["logRedactionTests"]["testCount"]
        + categories["existingLoggingContractTests"]["testCount"]
    )
    return {
        "schemaVersion": 1,
        "generatedAt": generated_at,
        "repository": "Team-AnI/A-AND-I-GATEWAY-SERVER",
        "scope": "Gateway observability, error contract, redaction, and Monitor Bot mock tests",
        "safety": SAFETY_FLAGS,
        "metrics": {
            "traceIdRequestIdTestCount": categories["traceIdRequestIdTests"]["testCount"],
            "errorContractTestCount": categories["errorContractTests"]["testCount"],
            "logRedactionTestCount": categories["logRedactionTests"]["testCount"],
            "gatewayObservabilityTestCount": gateway_observability_tests,
            "monitorBotMockTestCount": categories["monitorBotMockTests"]["testCount"],
        },
        "categories": categories,
        "resumeSentenceCandidates": [
            "traceId/requestId 기반 구조화 로그와 공통 오류 응답 계약을 테스트로 검증해 요청 추적성과 장애 분석 흐름을 관리",
            "Mock CloudWatch/Discord 기반 Monitor Bot 테스트로 장애 알림 cooldown과 중복 mention 억제 동작을 검증",
        ],
    }


def render_markdown(summary: dict[str, Any]) -> str:
    metrics = summary["metrics"]
    lines = [
        "# Gateway Observability Metrics",
        "",
        "> 운영 CloudWatch, Discord, 운영 URL을 호출하지 않고 로컬 테스트와 mock/fake 기반으로 검증한 수치입니다.",
        "",
        "## Safety",
        "",
        "| 항목 | 값 |",
        "| :--- | :--- |",
    ]
    for key, value in summary["safety"].items():
        lines.append(f"| {key} | `{str(value).lower()}` |")
    lines.extend(
        [
            "",
            "## Metrics",
            "",
            "| 영역 | 테스트 수 | 근거 파일 |",
            "| :--- | ---: | :--- |",
        ]
    )
    for category in summary["categories"].values():
        files = "<br>".join(f"`{item['path']}` ({item['testCount']})" for item in category["files"])
        lines.append(f"| {category['label']} | {category['testCount']} | {files} |")
    lines.extend(
        [
            "",
            "## Summary",
            "",
            f"- traceId/requestId 테스트 수: `{metrics['traceIdRequestIdTestCount']}`",
            f"- 오류 계약 테스트 수: `{metrics['errorContractTestCount']}`",
            f"- 로그 redaction 테스트 수: `{metrics['logRedactionTestCount']}`",
            f"- Gateway 관측 가능성 관련 테스트 수: `{metrics['gatewayObservabilityTestCount']}`",
            f"- Monitor Bot mock 테스트 수: `{metrics['monitorBotMockTestCount']}`",
            "- 실제 AWS/Discord 접근 여부: `false`",
            "- 운영 URL 접근 여부: `false`",
            "",
            "## Verified Coverage",
            "",
            "- Gateway 응답과 downstream 전달 header의 `X-Trace-Id`, `X-Request-Id`를 로컬 mock downstream으로 검증합니다.",
            "- 구조화 로그의 `path`, `method`, `status`, `latencyMs`, `traceId`, `requestId`와 민감정보 redaction을 captured log output으로 검증합니다.",
            "- 401, 403, 404, 415, 429, 502, 500 오류 응답 envelope와 trace/request header를 검증합니다.",
            "- Monitor Bot dashboard/logs/alert/help handler는 fake CloudWatch, fake Ops controller, local httptest health server로 검증합니다.",
            "- 기존 Monitor Bot alert 테스트가 critical 반복 이벤트의 cooldown과 role mention 중복 억제를 검증합니다.",
            "",
            "## Resume Sentence Candidates",
            "",
            "- traceId/requestId 기반 구조화 로그와 공통 오류 응답 계약을 테스트로 검증해 요청 추적성과 장애 분석 흐름을 관리",
            "- Mock CloudWatch/Discord 기반 Monitor Bot 테스트로 장애 알림 cooldown과 중복 mention 억제 동작을 검증",
            "",
        ]
    )
    return "\n".join(lines)


def write_text(path: str, content: str) -> None:
    output = Path(path)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(content, encoding="utf-8")


def main() -> int:
    args = parse_args()
    summary = build_summary()
    write_text(args.out_json, json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    write_text(args.out_md, render_markdown(summary))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
