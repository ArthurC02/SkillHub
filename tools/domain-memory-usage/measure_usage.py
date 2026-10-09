"""Report how often each Domain Memory command fails in local agent transcripts."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

INVOCATION = re.compile(r"registry_tools\.py[\"']?\s+([a-z][a-z0-9-]*)")
COMMAND_LIST = re.compile(r"\{([a-z0-9,-]+)\}")
FAILURE_LINE = re.compile(
    r"^(ERROR: .*|registry_tools\.py [a-z0-9-]+: error: .*|[A-Za-z]*Error: .*)$", re.MULTILINE
)
TRACEBACK = "Traceback (most recent call last)"
HOST_REFUSALS = (
    "Permission to use",
    "Permission for this action was denied",
    "<tool_use_error>",
    "The user doesn't want to proceed",
)
REDACTIONS = (
    (re.compile(r"(?:[A-Za-z]:)?[\w.~-]*(?:[\\/][\w.~-]+)+[\\/]?"), "<path>"),
    (re.compile(r"'[^']*'|\"[^\"]*\""), "<text>"),
    (re.compile(r"\b[0-9a-f]{7,64}\b"), "<hex>"),
    (re.compile(r"\d+"), "<n>"),
)
SIGNATURE_LIMIT = 140
DEFAULT_PLUGIN = Path(".claude/skills/domain-memory/scripts/registry_tools.py")


@dataclass
class CommandUsage:
    calls: int = 0
    failures: int = 0
    signatures: Counter = field(default_factory=Counter)


@dataclass
class Usage:
    commands: dict[str, CommandUsage] = field(default_factory=lambda: defaultdict(CommandUsage))
    compound_calls: int = 0
    unanswered_calls: int = 0
    host_refusals: int = 0


def signature(text: str) -> str:
    for pattern, placeholder in REDACTIONS:
        text = pattern.sub(placeholder, text)
    return text.strip()[:SIGNATURE_LIMIT]


def result_text(result: dict[str, Any]) -> str:
    content = result.get("content")
    if isinstance(content, list):
        content = "\n".join(
            part.get("text", "") for part in content if isinstance(part, dict)
        )
    return content if isinstance(content, str) else ""


def refused_by_host(result: dict[str, Any]) -> bool:
    return result_text(result).lstrip().startswith(HOST_REFUSALS)


def failure_of(result: dict[str, Any]) -> str | None:
    text = result_text(result)
    lines = FAILURE_LINE.findall(text)
    if lines:
        return signature(lines[-1])
    if TRACEBACK in text:
        return "Traceback"
    if result.get("is_error"):
        first = next((line for line in text.splitlines() if line.strip()), "")
        return signature(first) or "error"
    return None


def _blocks(entry: dict[str, Any]) -> list[dict[str, Any]]:
    message = entry.get("message")
    content = message.get("content") if isinstance(message, dict) else None
    if not isinstance(content, list):
        return []
    return [block for block in content if isinstance(block, dict)]


def _entries(transcript: Path):
    for line in transcript.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(entry, dict):
            yield entry


def _calls_and_results(
    projects_root: Path, since: str | None
) -> tuple[dict[str, str], dict[str, dict[str, Any]]]:
    calls: dict[str, str] = {}
    results: dict[str, dict[str, Any]] = {}
    for transcript in sorted(projects_root.rglob("*.jsonl")):
        for entry in _entries(transcript):
            recent = since is None or str(entry.get("timestamp", "")) >= since
            for block in _blocks(entry):
                if block.get("type") == "tool_result":
                    results[str(block.get("tool_use_id"))] = block
                elif block.get("type") == "tool_use" and recent:
                    command = (block.get("input") or {}).get("command")
                    if isinstance(command, str):
                        calls.setdefault(str(block.get("id")), command)
    return calls, results


def scan(projects_root: Path, known: set[str], since: str | None = None) -> Usage:
    calls, results = _calls_and_results(projects_root, since)
    usage = Usage()
    for call_id, command in calls.items():
        names = [name for name in INVOCATION.findall(command) if name in known]
        if not names:
            continue
        if len(names) > 1:
            usage.compound_calls += 1
            for name in names:
                usage.commands[name].calls += 1
            continue
        result = results.get(call_id)
        if result is None:
            usage.unanswered_calls += 1
            usage.commands[names[0]].calls += 1
            continue
        if refused_by_host(result):
            usage.host_refusals += 1
            continue
        record = usage.commands[names[0]]
        record.calls += 1
        failure = failure_of(result)
        if failure:
            record.failures += 1
            record.signatures[failure] += 1
    return usage


def report(usage: Usage, top: int) -> dict[str, Any]:
    rows = sorted(
        usage.commands.items(), key=lambda item: (-item[1].failures, -item[1].calls, item[0])
    )
    return {
        "calls": sum(record.calls for _, record in rows),
        "failures": sum(record.failures for _, record in rows),
        "compound_calls": usage.compound_calls,
        "unanswered_calls": usage.unanswered_calls,
        "host_refusals": usage.host_refusals,
        "commands": [
            {
                "command": name,
                "calls": record.calls,
                "failures": record.failures,
                "top_failures": [
                    {"count": count, "signature": text}
                    for text, count in record.signatures.most_common(top)
                ],
            }
            for name, record in rows
        ],
    }


def render(summary: dict[str, Any]) -> str:
    lines = [
        f"{summary['calls']} calls, {summary['failures']} failed; "
        f"{summary['compound_calls']} ran several commands at once and "
        f"{summary['unanswered_calls']} have no result, so neither is judged; "
        f"{summary['host_refusals']} were refused by the host before they ran",
        "",
        f"{'command':<28}{'calls':>7}{'failed':>8}",
    ]
    for row in summary["commands"]:
        lines.append(f"{row['command']:<28}{row['calls']:>7}{row['failures']:>8}")
        lines.extend(f"    {item['count']:>4}  {item['signature']}" for item in row["top_failures"])
    return "\n".join(lines)


def plugin_commands(plugin: Path) -> set[str]:
    shown = subprocess.run(
        [sys.executable, str(plugin), "--help"], capture_output=True, text=True, check=True
    )
    found = COMMAND_LIST.search(shown.stdout)
    if found is None:
        raise SystemExit(f"{plugin} --help lists no commands")
    return set(found.group(1).split(","))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--projects-root", type=Path, default=Path.home() / ".claude" / "projects")
    parser.add_argument("--plugin", type=Path, default=DEFAULT_PLUGIN)
    parser.add_argument("--since", help="ISO-8601 timestamp; earlier calls are left out")
    parser.add_argument("--top", type=int, default=3)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args(argv)
    usage = scan(args.projects_root, plugin_commands(args.plugin), args.since)
    summary = report(usage, args.top)
    sys.stdout.write((json.dumps(summary, indent=2) if args.json else render(summary)) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
