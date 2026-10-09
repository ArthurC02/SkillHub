from __future__ import annotations

import re
from pathlib import Path
from typing import cast

PATTERNS = {
    "private-key": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    "github-token": re.compile(r"\bgh[pousr]_[A-Za-z0-9_]{20,}\b"),
    "generic-secret": re.compile(
        r"(?i)\b(?:api[_-]?key|secret|password|token)\s*[:=]\s*['\"]?[A-Za-z0-9_\-]{16,}"
    ),
}
UNSCANNED_DIRECTORIES = {".git", "node_modules", ".venv"}
MAX_SCAN_FILES = 50_000
MAX_SCAN_BYTES = 512 * 1024 * 1024
MAX_FILE_BYTES = 1_048_576
MAX_REPORTED_SKIPS = 1_000


def scan_report(root: Path) -> dict[str, object]:  # noqa: C901
    root = root.resolve()
    findings: list[dict[str, str | int]] = []
    skipped: list[dict[str, str]] = []
    skipped_count = 0
    scanned_files = 0
    scanned_bytes = 0

    def record_skip(path: Path, reason: str) -> None:
        nonlocal skipped_count
        skipped_count += 1
        if len(skipped) < MAX_REPORTED_SKIPS:
            skipped.append(
                {"path": path.relative_to(root).as_posix(), "reason": reason}
            )

    for path in root.rglob("*"):
        if UNSCANNED_DIRECTORIES.intersection(path.relative_to(root).parts):
            continue
        if path.is_symlink():
            record_skip(path, "symlink")
            continue
        if not path.is_file():
            continue
        try:
            size = path.stat().st_size
            if size > MAX_FILE_BYTES:
                record_skip(path, "file-too-large")
                continue
            if (
                scanned_files + 1 > MAX_SCAN_FILES
                or scanned_bytes + size > MAX_SCAN_BYTES
            ):
                record_skip(path, "resource-limit")
                break
            scanned_files += 1
            scanned_bytes += size
            source = path.open(encoding="utf-8", errors="ignore")
        except OSError:
            record_skip(path, "unreadable")
            continue
        shown = path.relative_to(root).as_posix()
        with source:
            for number, line in enumerate(source, 1):
                findings.extend(
                    {"path": shown, "line": number, "kind": kind}
                    for kind, pattern in PATTERNS.items()
                    if pattern.search(line)
                )
    return {
        "status": "incomplete" if skipped_count else "complete",
        "findings": findings,
        "skipped": skipped,
        "skipped_count": skipped_count,
        "scanned_files": scanned_files,
        "scanned_bytes": scanned_bytes,
    }


def scan(root: Path) -> list[dict[str, str | int]]:
    return cast(list[dict[str, str | int]], scan_report(root)["findings"])
