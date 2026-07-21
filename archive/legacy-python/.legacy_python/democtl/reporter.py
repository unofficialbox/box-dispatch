from __future__ import annotations

import json
from typing import Any

from .models import CommandReport, PhaseStatus


def print_report(report: CommandReport, as_json: bool, command: str | None = None) -> str:
    payload = report.to_dict()
    if command:
        payload["command"] = command

    if as_json:
        rendered = json.dumps(payload, indent=2)
    else:
        status = payload["status"]
        header = f"\n{payload['command'].upper()} ({payload['scenario'] or 'default'})"
        lines = [header, "=" * len(header)]
        if payload["dryRun"]:
            lines.append("Mode: dry-run")
        if payload["confirmRequired"]:
            lines.append("Confirmation required: yes")
        if payload["nextCommand"]:
            lines.append(f"Next command: {payload['nextCommand']}")
        if payload["warnings"]:
            lines.append("Warnings:")
            lines.extend(f"  - {item}" for item in payload["warnings"])
        lines.append("\nPhases:")
        for phase in payload["phases"]:
            lines.append(
                f"  - {phase['name']}: {phase['status']} ({phase['duration_ms']} ms)"
            )
            if phase["error"]:
                lines.append(f"      error: {phase['error']}")
        if payload["manual"]:
            lines.append("\nManual items:")
            lines.extend(f"  - {item}" for item in payload["manual"])
        lines.append(f"\nOverall status: {status}")
        rendered = "\n".join(lines)
    print(rendered)
    return rendered

