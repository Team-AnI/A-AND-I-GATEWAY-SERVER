#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import statistics
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


MIN_RESUME_RUNS = 3
SUCCESS_CONCLUSIONS = {"success"}
RUN_LIST_FIELDS = ",".join(
    [
        "databaseId",
        "conclusion",
        "status",
        "createdAt",
        "updatedAt",
        "displayTitle",
        "event",
        "headBranch",
        "headSha",
        "workflowName",
    ]
)

SECRET_PATTERNS = [
    re.compile(r"gh[opsru]_[A-Za-z0-9_]{20,}"),
    re.compile(r"AKIA[0-9A-Z]{16}"),
    re.compile(r"(?i)(discord[_-]?(bot[_-]?)?token)[=:][^\s]+"),
    re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----", re.DOTALL),
]


@dataclass(frozen=True)
class MetricSpec:
    key: str
    label: str
    aliases: tuple[str, ...]
    resume_candidate: bool = True


CI_STEP_METRICS = (
    MetricSpec("gradle_test", "./gradlew test", ("Run tests",)),
    MetricSpec(
        "monitor_bot_go_test",
        "monitor-bot go test",
        ("Run monitor-bot tests", "Test monitor-bot"),
    ),
    MetricSpec("jar_build", "./gradlew build 또는 build -x test", ("Build JAR",)),
    MetricSpec("k6_install", "k6 install", ("Install k6",)),
    MetricSpec("performance_asset_validation", "performance asset validation", ("Validate performance assets",)),
)

CD_STEP_METRICS = (
    MetricSpec("gradle_test", "./gradlew test", ("Run tests",)),
    MetricSpec("monitor_bot_go_test", "monitor-bot go test", ("Run monitor-bot tests", "Test monitor-bot")),
    MetricSpec("jar_build", "./gradlew build", ("Build JAR",)),
    MetricSpec("gateway_docker_build_push", "Gateway Docker build/push", ("Build and push Docker image",)),
    MetricSpec(
        "monitor_bot_docker_build_push",
        "monitor-bot Docker build/push",
        ("Build and push monitor-bot Docker image",),
    ),
    MetricSpec("deploy_step_metadata", "Deploy step 조회 전용", ("Deploy to EC2 via SSH",), False),
)

FORBIDDEN_OPERATIONS = [
    "workflow_dispatch",
    "tag push",
    "aws cli",
    "ssh",
    "docker push",
    "production url access",
]


class GhError(RuntimeError):
    pass


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Collect read-only GitHub Actions duration metrics for resume evidence.",
    )
    parser.add_argument("--repo", required=True, help="GitHub repository, for example Team-AnI/A-AND-I-GATEWAY-SERVER")
    parser.add_argument("--workflow", default="CI", help="CI workflow name or file name. Default: CI")
    parser.add_argument("--branch", default=None, help="Branch filter for the CI workflow")
    parser.add_argument("--limit", type=int, default=20, help="Recent CI run count to inspect. Default: 20")
    parser.add_argument("--cd-workflow", default="CD", help="CD workflow name or file name. Default: CD")
    parser.add_argument("--cd-branch", default=None, help="Optional branch filter for the CD workflow")
    parser.add_argument("--cd-limit", type=int, default=None, help="Recent CD run count to inspect. Default: --limit")
    parser.add_argument("--skip-cd", action="store_true", help="Skip CD workflow metadata collection")
    parser.add_argument("--out-json", required=True, help="Output JSON path")
    parser.add_argument("--out-md", required=True, help="Output Markdown path")
    return parser.parse_args()


def redact(value: str) -> str:
    redacted = value
    for pattern in SECRET_PATTERNS:
        redacted = pattern.sub("[REDACTED]", redacted)
    return redacted


def run_gh_json(args: list[str]) -> Any:
    command = ["gh", *args]
    completed = subprocess.run(
        command,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if completed.returncode != 0:
        stderr = redact(completed.stderr.strip())
        raise GhError(f"gh command failed: {' '.join(command)}\n{stderr}")
    try:
        return json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise GhError(f"gh command returned invalid JSON: {' '.join(command)}") from exc


def parse_timestamp(value: str | None) -> datetime | None:
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def duration_seconds(start: str | None, end: str | None) -> float | None:
    started_at = parse_timestamp(start)
    completed_at = parse_timestamp(end)
    if started_at is None or completed_at is None:
        return None
    seconds = (completed_at - started_at).total_seconds()
    if seconds < 0:
        return None
    return seconds


def normalize_name(value: str) -> str:
    return " ".join(value.strip().lower().split())


def find_step(jobs: list[dict[str, Any]], aliases: tuple[str, ...]) -> dict[str, Any] | None:
    normalized_aliases = {normalize_name(alias) for alias in aliases}
    for job in jobs:
        for step in job.get("steps", []):
            if normalize_name(str(step.get("name", ""))) in normalized_aliases:
                return step
    return None


def collect_run_list(repo: str, workflow: str, branch: str | None, limit: int) -> list[dict[str, Any]]:
    args = [
        "run",
        "list",
        "--repo",
        repo,
        "--workflow",
        workflow,
        "--limit",
        str(limit),
        "--json",
        RUN_LIST_FIELDS,
    ]
    if branch:
        args.extend(["--branch", branch])
    data = run_gh_json(args)
    if not isinstance(data, list):
        raise GhError("gh run list returned a non-list response")
    return data


def collect_jobs(repo: str, run_id: int) -> list[dict[str, Any]]:
    data = run_gh_json(["run", "view", str(run_id), "--repo", repo, "--json", "jobs"])
    jobs = data.get("jobs", [])
    if not isinstance(jobs, list):
        raise GhError(f"gh run view returned invalid jobs for run {run_id}")
    return jobs


def numeric_stats(values: list[float]) -> dict[str, float | None]:
    if not values:
        return {
            "averageSeconds": None,
            "medianSeconds": None,
            "minSeconds": None,
            "maxSeconds": None,
        }
    return {
        "averageSeconds": sum(values) / len(values),
        "medianSeconds": statistics.median(values),
        "minSeconds": min(values),
        "maxSeconds": max(values),
    }


def confidence_for_count(count: int) -> str:
    if count == 0:
        return "확인 필요"
    if count < MIN_RESUME_RUNS:
        return "측정 필요"
    return "확인 완료"


def resume_use_for_count(count: int, resume_candidate: bool) -> str:
    if count < MIN_RESUME_RUNS:
        return "사용 비추천"
    if not resume_candidate:
        return "참고용"
    return "사용 가능"


def metric_summary(
    *,
    key: str,
    label: str,
    values: list[float],
    conclusions: list[str | None],
    expected_count: int,
    resume_candidate: bool,
) -> dict[str, Any]:
    failure_count = sum(1 for conclusion in conclusions if conclusion != "success")
    denominator = len(conclusions)
    stats = numeric_stats(values)
    return {
        "key": key,
        "label": label,
        "count": len(values),
        "expectedCount": expected_count,
        "missingCount": max(expected_count - len(values), 0),
        "failureCount": failure_count,
        "failureRate": (failure_count / denominator) if denominator else None,
        "confidence": confidence_for_count(len(values)),
        "resumeUse": resume_use_for_count(len(values), resume_candidate),
        **stats,
    }


def sanitize_run_title(value: str | None) -> str:
    if not value:
        return ""
    return redact(value)


def collect_workflow(
    *,
    repo: str,
    workflow: str,
    branch: str | None,
    limit: int,
    kind: str,
    step_metrics: tuple[MetricSpec, ...],
) -> dict[str, Any]:
    runs = collect_run_list(repo, workflow, branch, limit)
    run_records: list[dict[str, Any]] = []
    run_durations: list[float] = []
    run_conclusions: list[str | None] = []

    step_values: dict[str, list[float]] = {metric.key: [] for metric in step_metrics}
    step_conclusions: dict[str, list[str | None]] = {metric.key: [] for metric in step_metrics}

    for run in runs:
        run_id = int(run["databaseId"])
        jobs = collect_jobs(repo, run_id)
        run_duration = duration_seconds(run.get("createdAt"), run.get("updatedAt"))
        if run_duration is not None:
            run_durations.append(run_duration)
        run_conclusions.append(run.get("conclusion"))

        steps: dict[str, Any] = {}
        for metric in step_metrics:
            step = find_step(jobs, metric.aliases)
            if not step:
                steps[metric.key] = {
                    "label": metric.label,
                    "name": None,
                    "durationSeconds": None,
                    "conclusion": None,
                    "confidence": "확인 필요",
                }
                continue
            step_duration = duration_seconds(step.get("startedAt"), step.get("completedAt"))
            conclusion = step.get("conclusion")
            if step_duration is not None:
                step_values[metric.key].append(step_duration)
            step_conclusions[metric.key].append(conclusion)
            steps[metric.key] = {
                "label": metric.label,
                "name": step.get("name"),
                "durationSeconds": step_duration,
                "conclusion": conclusion,
            }

        run_records.append(
            {
                "runId": run_id,
                "workflowName": run.get("workflowName"),
                "displayTitle": sanitize_run_title(run.get("displayTitle")),
                "event": run.get("event"),
                "headBranch": run.get("headBranch"),
                "headSha": run.get("headSha"),
                "status": run.get("status"),
                "conclusion": run.get("conclusion"),
                "createdAt": run.get("createdAt"),
                "updatedAt": run.get("updatedAt"),
                "durationSeconds": run_duration,
                "steps": steps,
            }
        )

    metrics: dict[str, Any] = {
        "overall": metric_summary(
            key="overall",
            label=f"{kind.upper()} 전체 시간",
            values=run_durations,
            conclusions=run_conclusions,
            expected_count=len(runs),
            resume_candidate=True,
        )
    }
    for metric in step_metrics:
        metrics[metric.key] = metric_summary(
            key=metric.key,
            label=metric.label,
            values=step_values[metric.key],
            conclusions=step_conclusions[metric.key],
            expected_count=len(runs),
            resume_candidate=metric.resume_candidate,
        )

    return {
        "workflow": workflow,
        "branch": branch,
        "limit": limit,
        "runsAnalyzed": len(runs),
        "metrics": metrics,
        "runs": run_records,
    }


def format_duration(seconds: float | None) -> str:
    if seconds is None:
        return "[확인 필요]"
    if seconds < 60:
        return f"{seconds:.1f}s"
    minutes = int(seconds // 60)
    remaining = seconds - minutes * 60
    return f"{minutes}m {remaining:.0f}s"


def format_rate(value: float | None) -> str:
    if value is None:
        return "[확인 필요]"
    return f"{value * 100:.2f}%"


def metric_row(workflow_label: str, metric: dict[str, Any]) -> str:
    return " | ".join(
        [
            workflow_label,
            metric["label"],
            str(metric["count"]),
            str(metric["missingCount"]),
            format_duration(metric["averageSeconds"]),
            format_duration(metric["medianSeconds"]),
            format_duration(metric["minSeconds"]),
            format_duration(metric["maxSeconds"]),
            format_rate(metric["failureRate"]),
            metric["confidence"],
            metric["resumeUse"],
        ]
    )


def markdown_table(workflow_label: str, workflow: dict[str, Any]) -> list[str]:
    lines = [
        "| Workflow | Metric | Runs | Missing | Avg | Median | Min | Max | Failure rate | 신뢰도 | 사용 여부 |",
        "| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :--- |",
    ]
    for metric in workflow["metrics"].values():
        lines.append("| " + metric_row(workflow_label, metric) + " |")
    return lines


def build_resume_sentence(ci: dict[str, Any] | None) -> str:
    if not ci:
        return "GitHub Actions에서 Gateway 테스트, Monitor Bot 테스트, JAR 빌드, k6 asset 검증을 자동화 [확인 필요]"
    required_keys = [
        "overall",
        "gradle_test",
        "monitor_bot_go_test",
        "jar_build",
        "k6_install",
        "performance_asset_validation",
    ]
    metrics = ci["metrics"]
    if all(metrics[key]["count"] >= MIN_RESUME_RUNS for key in required_keys):
        median = format_duration(metrics["overall"]["medianSeconds"])
        return (
            "GitHub Actions에서 Gateway 테스트, Monitor Bot 테스트, JAR 빌드, "
            f"k6 asset 검증을 자동화하고 CI median {median} 기준으로 검증 시간을 관리"
        )
    return "GitHub Actions에서 Gateway 테스트, Monitor Bot 테스트, JAR 빌드, k6 asset 검증을 자동화"


def build_markdown(summary: dict[str, Any]) -> str:
    ci = summary["workflows"].get("ci")
    cd = summary["workflows"].get("cd")
    lines = [
        "# Gateway CI/CD Metrics",
        "",
        "> GitHub Actions run metadata를 읽기 전용으로 조회해 계산한 값입니다.",
        "> workflow_dispatch, tag push, AWS CLI, SSH, docker push, 운영 URL 접근은 수행하지 않습니다.",
        "",
        "## Scope",
        "",
        f"- Repository: `{summary['repository']}`",
        f"- Generated at: `{summary['generatedAt']}`",
        "- Source commands: `gh run list`, `gh run view --json jobs`",
        "- Secrets, AWS host, EC2 host, SSH key, token 값은 저장하지 않습니다.",
        "- 3회 미만의 수치는 이력서 사용 비추천으로 표시합니다.",
        "",
    ]

    if ci:
        lines.extend(
            [
                "## CI",
                "",
                f"- Workflow: `{ci['workflow']}`",
                f"- Branch: `{ci['branch'] or '[all]'}`",
                f"- Runs analyzed: `{ci['runsAnalyzed']}`",
                "",
                *markdown_table("CI", ci),
                "",
            ]
        )
    if cd:
        lines.extend(
            [
                "## CD",
                "",
                f"- Workflow: `{cd['workflow']}`",
                f"- Branch: `{cd['branch'] or '[all]'}`",
                f"- Runs analyzed: `{cd['runsAnalyzed']}`",
                "- Deploy step은 과거 run metadata의 step duration만 조회했습니다. 실행, SSH 접속, AWS 명령은 수행하지 않았습니다.",
                "",
                *markdown_table("CD", cd),
                "",
            ]
        )

    lines.extend(
        [
            "## Resume Sentence Candidate",
            "",
            f"- {build_resume_sentence(ci)}",
            "",
            "## Notes",
            "",
            "- GitHub Actions UI와 GitHub CLI가 제공하는 step startedAt/completedAt 기준입니다.",
            "- CD 전체 시간은 GitHub run createdAt/updatedAt metadata 기준입니다. 오래 열린 run이 있으면 평균보다 중앙값과 step별 시간을 우선 확인합니다.",
            "- Docker build/push와 deploy 시간은 과거 CD run metadata로만 계산합니다.",
            "- 운영 최대 처리량, 운영 배포 안정성, 운영 트래픽 처리량으로 해석하지 않습니다.",
            "",
        ]
    )
    return "\n".join(lines)


def build_summary(args: argparse.Namespace) -> dict[str, Any]:
    generated_at = datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
    workflows: dict[str, Any] = {
        "ci": collect_workflow(
            repo=args.repo,
            workflow=args.workflow,
            branch=args.branch,
            limit=args.limit,
            kind="ci",
            step_metrics=CI_STEP_METRICS,
        )
    }
    if not args.skip_cd:
        workflows["cd"] = collect_workflow(
            repo=args.repo,
            workflow=args.cd_workflow,
            branch=args.cd_branch,
            limit=args.cd_limit or args.limit,
            kind="cd",
            step_metrics=CD_STEP_METRICS,
        )
    return {
        "schemaVersion": 1,
        "generatedAt": generated_at,
        "repository": args.repo,
        "source": {
            "readOnly": True,
            "commands": ["gh run list", "gh run view --json jobs"],
            "forbiddenOperations": FORBIDDEN_OPERATIONS,
        },
        "minimumResumeRuns": MIN_RESUME_RUNS,
        "workflows": workflows,
        "resumeSentenceCandidate": build_resume_sentence(workflows.get("ci")),
    }


def write_text(path: str, content: str) -> None:
    output = Path(path)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(content, encoding="utf-8")


def main() -> int:
    args = parse_args()
    if args.limit <= 0:
        raise SystemExit("--limit must be positive")
    if args.cd_limit is not None and args.cd_limit <= 0:
        raise SystemExit("--cd-limit must be positive")

    try:
        summary = build_summary(args)
    except GhError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    write_text(args.out_json, json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    write_text(args.out_md, build_markdown(summary))
    print(f"Wrote {args.out_json}")
    print(f"Wrote {args.out_md}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
