#!/usr/bin/env python3
import argparse
import json
import statistics
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


MIN_RECOMMENDED_RUNS = 5


def run_gh(args):
    result = subprocess.run(["gh", *args], check=True, text=True, capture_output=True)
    return json.loads(result.stdout)


def parse_time(value):
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def duration_seconds(started_at, completed_at):
    start = parse_time(started_at)
    end = parse_time(completed_at)
    if start is None or end is None:
        return None
    return int((end - start).total_seconds())


def median(values):
    numbers = [value for value in values if isinstance(value, (int, float))]
    return statistics.median(numbers) if numbers else None


def mean(values):
    numbers = [value for value in values if isinstance(value, (int, float))]
    return statistics.mean(numbers) if numbers else None


def min_max(values):
    numbers = [value for value in values if isinstance(value, (int, float))]
    if not numbers:
        return {"min": None, "max": None}
    return {"min": min(numbers), "max": max(numbers)}


def confidence(count):
    return "확인 완료" if count >= MIN_RECOMMENDED_RUNS else "측정 필요"


def resume_use(count, failure_rate):
    if count < MIN_RECOMMENDED_RUNS or failure_rate > 0:
        return "사용 비추천"
    return "사용 가능"


def list_runs(repo, workflow, branch, limit):
    return run_gh([
        "run",
        "list",
        "--repo",
        repo,
        "--workflow",
        workflow,
        "--branch",
        branch,
        "--limit",
        str(limit),
        "--json",
        "databaseId,conclusion,createdAt,updatedAt,displayTitle,event,headBranch,headSha,status,url",
    ])


def view_run(repo, run_id):
    return run_gh([
        "run",
        "view",
        str(run_id),
        "--repo",
        repo,
        "--json",
        "databaseId,conclusion,createdAt,updatedAt,event,headBranch,headSha,jobs,name,status,url",
    ])


def summarize_durations(items):
    durations = [item["durationSeconds"] for item in items if item.get("durationSeconds") is not None]
    count = len(durations)
    return {
        "count": count,
        "medianSeconds": median(durations),
        "averageSeconds": mean(durations),
        **min_max(durations),
        "confidence": confidence(count),
    }


def collect(repo, workflow, branch, limit):
    runs = []
    listed = list_runs(repo, workflow, branch, limit)
    for listed_run in listed:
        detail = view_run(repo, listed_run["databaseId"])
        run_duration = duration_seconds(detail.get("createdAt"), detail.get("updatedAt"))
        jobs = []
        for job in detail.get("jobs", []):
            steps = []
            for step in job.get("steps", []):
                steps.append({
                    "name": step.get("name"),
                    "conclusion": step.get("conclusion"),
                    "startedAt": step.get("startedAt"),
                    "completedAt": step.get("completedAt"),
                    "durationSeconds": duration_seconds(step.get("startedAt"), step.get("completedAt")),
                })
            jobs.append({
                "name": job.get("name"),
                "conclusion": job.get("conclusion"),
                "startedAt": job.get("startedAt"),
                "completedAt": job.get("completedAt"),
                "durationSeconds": duration_seconds(job.get("startedAt"), job.get("completedAt")),
                "steps": steps,
                "url": job.get("url"),
            })
        runs.append({
            "runId": detail.get("databaseId"),
            "url": detail.get("url"),
            "displayTitle": listed_run.get("displayTitle"),
            "event": detail.get("event"),
            "branch": detail.get("headBranch"),
            "headSha": detail.get("headSha"),
            "status": detail.get("status"),
            "conclusion": detail.get("conclusion"),
            "createdAt": detail.get("createdAt"),
            "updatedAt": detail.get("updatedAt"),
            "durationSeconds": run_duration,
            "jobs": jobs,
        })

    completed = [run for run in runs if run.get("status") == "completed"]
    successful = [run for run in completed if run.get("conclusion") == "success"]
    failure_count = len([run for run in completed if run.get("conclusion") != "success"])
    failure_rate = failure_count / len(completed) if completed else None

    job_items = {}
    step_items = {}
    for run in successful:
        for job in run["jobs"]:
            job_items.setdefault(job["name"], []).append({
                "runId": run["runId"],
                "durationSeconds": job["durationSeconds"],
            })
            for step in job["steps"]:
                key = step["name"]
                step_items.setdefault(key, []).append({
                    "runId": run["runId"],
                    "job": job["name"],
                    "durationSeconds": step["durationSeconds"],
                })

    run_count = len(successful)
    return {
        "schemaVersion": 1,
        "generatedAt": datetime.now(timezone.utc).isoformat(),
        "repo": repo,
        "workflow": workflow,
        "branch": branch,
        "limit": limit,
        "completedRunCount": len(completed),
        "successfulRunCount": run_count,
        "failureCount": failure_count,
        "failureRate": failure_rate,
        "confidence": confidence(run_count),
        "resumeUse": resume_use(run_count, failure_rate or 0),
        "summary": {
            "workflowDuration": summarize_durations(successful),
            "jobs": {name: summarize_durations(items) for name, items in sorted(job_items.items())},
            "steps": {name: summarize_durations(items) for name, items in sorted(step_items.items())},
        },
        "runs": runs,
    }


def fmt_seconds(value):
    if value is None:
        return "[측정 필요]"
    minutes, seconds = divmod(int(round(value)), 60)
    return f"{minutes}m {seconds}s" if minutes else f"{seconds}s"


def to_markdown(summary):
    lines = [
        f"# {summary['workflow']} Metrics",
        "",
        f"- Repo: `{summary['repo']}`",
        f"- Branch: `{summary['branch']}`",
        f"- Successful Runs: {summary['successfulRunCount']}",
        f"- Failure Rate: {summary['failureRate'] if summary['failureRate'] is not None else '[확인 필요]'}",
        f"- Confidence: {summary['confidence']}",
        f"- Resume Use: {summary['resumeUse']}",
        "",
        "## Workflow",
        "",
        "| Metric | Median | Average | Min | Max | Count |",
        "| --- | ---: | ---: | ---: | ---: | ---: |",
    ]
    workflow = summary["summary"]["workflowDuration"]
    lines.append(
        f"| Duration | {fmt_seconds(workflow['medianSeconds'])} | {fmt_seconds(workflow['averageSeconds'])} | "
        f"{fmt_seconds(workflow['min'])} | {fmt_seconds(workflow['max'])} | {workflow['count']} |"
    )
    lines.extend(["", "## Jobs", "", "| Job | Median | Average | Min | Max | Count |", "| --- | ---: | ---: | ---: | ---: | ---: |"])
    for name, item in summary["summary"]["jobs"].items():
        lines.append(
            f"| {name} | {fmt_seconds(item['medianSeconds'])} | {fmt_seconds(item['averageSeconds'])} | "
            f"{fmt_seconds(item['min'])} | {fmt_seconds(item['max'])} | {item['count']} |"
        )
    lines.extend(["", "## Steps", "", "| Step | Median | Average | Min | Max | Count |", "| --- | ---: | ---: | ---: | ---: | ---: |"])
    for name, item in summary["summary"]["steps"].items():
        lines.append(
            f"| {name} | {fmt_seconds(item['medianSeconds'])} | {fmt_seconds(item['averageSeconds'])} | "
            f"{fmt_seconds(item['min'])} | {fmt_seconds(item['max'])} | {item['count']} |"
        )
    lines.append("")
    return "\n".join(lines)


def write_outputs(summary, out_json, out_md):
    out_json.parent.mkdir(parents=True, exist_ok=True)
    out_json.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if out_md:
        out_md.parent.mkdir(parents=True, exist_ok=True)
        out_md.write_text(to_markdown(summary), encoding="utf-8")


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Collect GitHub Actions duration metrics via gh read-only metadata.")
    parser.add_argument("--repo", required=True)
    parser.add_argument("--workflow", required=True)
    parser.add_argument("--branch", required=True)
    parser.add_argument("--limit", type=int, default=20)
    parser.add_argument("--out-json", required=True, type=Path)
    parser.add_argument("--out-md", type=Path)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv or sys.argv[1:])
    summary = collect(args.repo, args.workflow, args.branch, args.limit)
    write_outputs(summary, args.out_json, args.out_md)
    print(args.out_json)
    return 0


if __name__ == "__main__":
    sys.exit(main())
