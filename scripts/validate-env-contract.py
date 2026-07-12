#!/usr/bin/env python3
"""Validate that runtime environment keys are documented in .env.example."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ENV_KEY = re.compile(r"[A-Z][A-Z0-9_]*")
GATEWAY_REFERENCE = re.compile(r"\$\{([A-Z][A-Z0-9_]*)")
MONITOR_CALL = re.compile(r"\benv(?:Bool|Int|First)?\((.*?)\)", re.DOTALL)
QUOTED_KEY = re.compile(r'"([A-Z][A-Z0-9_]*)"')
DOCUMENTED_KEY = re.compile(r"^\s*#?\s*([A-Z][A-Z0-9_]*)=", re.MULTILINE)

# Spring Boot relaxed binding maps this environment variable to gateway.auth.enabled
# without an application.yaml placeholder.
SPRING_RELAXED_BINDING_KEYS = {"GATEWAY_AUTH_ENABLED"}


def read(relative_path: str) -> str:
    return (ROOT / relative_path).read_text(encoding="utf-8")


def gateway_keys() -> set[str]:
    return set(GATEWAY_REFERENCE.findall(read("src/main/resources/application.yaml")))


def monitor_keys() -> set[str]:
    source = read("monitor-bot/internal/config/config.go")
    keys: set[str] = set()
    for arguments in MONITOR_CALL.findall(source):
        keys.update(QUOTED_KEY.findall(arguments))
    return keys


def documented_keys() -> list[str]:
    return DOCUMENTED_KEY.findall(read(".env.example"))


def main() -> int:
    documented = documented_keys()
    duplicates = sorted({key for key in documented if documented.count(key) > 1})
    runtime = gateway_keys() | monitor_keys() | SPRING_RELAXED_BINDING_KEYS
    documented_set = set(documented)
    missing = sorted(runtime - documented_set)
    extra = sorted(documented_set - runtime)

    print(
        "env contract: "
        f"gateway={len(gateway_keys())} "
        f"monitor={len(monitor_keys())} "
        f"documented={len(documented_set)}"
    )
    if duplicates:
        print(f"duplicate .env.example keys: {', '.join(duplicates)}", file=sys.stderr)
    if missing:
        print(f"undocumented runtime keys: {', '.join(missing)}", file=sys.stderr)
    if extra:
        print(f"documented keys without runtime consumers: {', '.join(extra)}", file=sys.stderr)
    return 1 if duplicates or missing or extra else 0


if __name__ == "__main__":
    raise SystemExit(main())
