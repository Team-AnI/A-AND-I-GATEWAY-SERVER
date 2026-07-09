#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import re
import statistics
import subprocess
import sys
from pathlib import Path


DEFAULT_WORKFLOWS = [
    "CI",
    "CD",
    "CD Dry Run",
    "Measure Gateway CI/CD Same Scope",
]

ANSI_PATTERN = re.compile(r"\x1b\[[0-9;]*m")


def parse_args():
    parser = argparse.ArgumentParser(
        description="Collect recent successful GitHub Actions workflow durations as reference-only evidence."
    )
    parser.add_argument("--repo", help="GitHub repository in owner/name form. Defaults to gh repo view.")
    parser.add_argument("--workflow", action="append", dest="workflows", help="Workflow name to collect. Can be repeated.")
    parser.add_argument("--limit", type=int, default=10, help="Successful runs per workflow.")
    parser.add_argument("--include-cache-log", action="store_true", help="Parse run logs for Gradle, Go, and k6 cache hits.")
    parser.add_argument("--output", help="Write JSON output to this path instead of stdout.")
    return parser.parse_args()


def gh_json(args):
    output = subprocess.check_output(["gh", *args], text=True)
    return json.loads(output) if output.strip() else None


def gh_text(args):
    return subprocess.check_output(["gh", *args], text=True, stderr=subprocess.STDOUT)


def detect_repo(repo_arg):
    if repo_arg:
        return repo_arg
    return gh_text(["repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"]).strip()


def parse_time(value):
    if not value:
        return None
    if value.endswith("Z"):
        value = f"{value[:-1]}+00:00"
    return dt.datetime.fromisoformat(value)


def duration_seconds(start, end):
    parsed_start = parse_time(start)
    parsed_end = parse_time(end)
    if not parsed_start or not parsed_end:
        return None
    return round((parsed_end - parsed_start).total_seconds(), 3)


def summary(values):
    clean = [value for value in values if value is not None]
    if not clean:
        return {"count": 0}
    return {
        "count": len(clean),
        "average_seconds": round(statistics.mean(clean), 3),
        "median_seconds": round(statistics.median(clean), 3),
        "min_seconds": round(min(clean), 3),
        "max_seconds": round(max(clean), 3),
    }


def collect_runs(repo, workflow, limit):
    return gh_json(
        [
            "run",
            "list",
            "--repo",
            repo,
            "--workflow",
            workflow,
            "--status",
            "success",
            "--limit",
            str(limit),
            "--json",
            "databaseId,workflowName,displayTitle,event,headBranch,headSha,createdAt,updatedAt,url",
        ]
    )


def collect_jobs(repo, run_id):
    payload = gh_json(["api", f"repos/{repo}/actions/runs/{run_id}/jobs?per_page=100"])
    return payload.get("jobs", [])


def cache_value(log_text, name):
    match = re.search(rf"{re.escape(name)}=([^\s\"\r\n]*)", log_text)
    if not match or not match.group(1):
        return "확인 필요"
    return match.group(1)


def collect_cache_hits(repo, run_id):
    try:
        log_text = gh_text(["run", "view", str(run_id), "--repo", repo, "--log"])
    except subprocess.CalledProcessError as error:
        return {"error": error.output.strip() or str(error)}

    log_text = ANSI_PATTERN.sub("", log_text)
    if "Cache hit for: setup-java-Linux-x64-gradle-" in log_text or "Cache restored from key: setup-java-Linux-x64-gradle-" in log_text:
        gradle = "true"
    elif "gradle cache is not found" in log_text or "Cache not found for input keys: setup-java-Linux-x64-gradle-" in log_text:
        gradle = "false"
    else:
        gradle = "확인 필요"

    return {
        "gradle_cache_hit": gradle,
        "go_cache_hit": cache_value(log_text, "setup-go-cache-hit"),
        "k6_cache_hit": cache_value(log_text, "k6-cache-hit"),
    }


def collect_workflow(repo, workflow, limit, include_cache_log):
    runs = collect_runs(repo, workflow, limit)
    run_results = []
    job_durations = {}
    step_durations = {}
    cache_results = []

    for run in runs:
        run_duration = duration_seconds(run.get("createdAt"), run.get("updatedAt"))
        run_result = {
            "run_id": run.get("databaseId"),
            "workflow": run.get("workflowName"),
            "display_title": run.get("displayTitle"),
            "event": run.get("event"),
            "head_branch": run.get("headBranch"),
            "head_sha": run.get("headSha"),
            "created_at": run.get("createdAt"),
            "updated_at": run.get("updatedAt"),
            "duration_seconds": run_duration,
            "url": run.get("url"),
        }

        jobs = collect_jobs(repo, run["databaseId"])
        run_result["jobs"] = []
        for job in jobs:
            job_duration = duration_seconds(job.get("started_at"), job.get("completed_at"))
            job_durations.setdefault(job["name"], []).append(job_duration)
            job_result = {
                "name": job.get("name"),
                "status": job.get("status"),
                "conclusion": job.get("conclusion"),
                "started_at": job.get("started_at"),
                "completed_at": job.get("completed_at"),
                "duration_seconds": job_duration,
            }
            steps = []
            for step in job.get("steps", []):
                step_duration = duration_seconds(step.get("started_at"), step.get("completed_at"))
                if step_duration is not None and step.get("conclusion") == "success":
                    step_durations.setdefault(step["name"], []).append(step_duration)
                steps.append(
                    {
                        "name": step.get("name"),
                        "status": step.get("status"),
                        "conclusion": step.get("conclusion"),
                        "started_at": step.get("started_at"),
                        "completed_at": step.get("completed_at"),
                        "duration_seconds": step_duration,
                    }
                )
            job_result["steps"] = steps
            run_result["jobs"].append(job_result)

        if include_cache_log:
            cache_observation = collect_cache_hits(repo, run["databaseId"])
            cache_observation["run_id"] = run["databaseId"]
            cache_results.append(cache_observation)

        run_results.append(run_result)

    return {
        "workflow": workflow,
        "resume_usage": "참고용",
        "comparison_policy": "Do not use as before/after evidence unless compared against the same workflow scope.",
        "run_summary": summary([run.get("duration_seconds") for run in run_results]),
        "job_summary": {name: summary(values) for name, values in sorted(job_durations.items())},
        "step_summary": {name: summary(values) for name, values in sorted(step_durations.items())},
        "cache_observations": cache_results,
        "runs": run_results,
    }


def main():
    args = parse_args()
    repo = detect_repo(args.repo)
    workflows = args.workflows or DEFAULT_WORKFLOWS
    payload = {
        "repository": repo,
        "collected_at": dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z"),
        "limit": args.limit,
        "resume_usage": "참고용",
        "workflows": [collect_workflow(repo, workflow, args.limit, args.include_cache_log) for workflow in workflows],
    }

    output = json.dumps(payload, ensure_ascii=False, indent=2)
    if args.output:
        Path(args.output).write_text(output + "\n", encoding="utf-8")
    else:
        print(output)
    return 0


if __name__ == "__main__":
    sys.exit(main())
