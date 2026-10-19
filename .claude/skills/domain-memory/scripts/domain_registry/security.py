from __future__ import annotations

import os
import re
import subprocess
from dataclasses import dataclass, field
from pathlib import Path

from .git import run_git

PATTERNS = {
    "private-key": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    "github-token": re.compile(r"\bgh[pousr]_[A-Za-z0-9_]{20,}\b"),
    "gitlab-token": re.compile(r"\bglpat-[A-Za-z0-9_-]{20,}\b"),
    "aws-access-key": re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b"),
    "slack-token": re.compile(r"\bxox[abprs]-[A-Za-z0-9-]{10,}\b"),
    "generic-secret": re.compile(
        r"(?i)\b(?:api[_-]?key|secret|password|token)\s*[:=]\s*"
        r"(?:['\"][A-Za-z0-9_\-]{16,}['\"]|[A-Za-z0-9_\-]{16,}(?![A-Za-z0-9_\-.(\[]))"
    ),
}
UNSCANNED_DIRECTORIES = {".git", "node_modules", ".venv"}
MAX_SCAN_FILES = 50_000
MAX_SCAN_BYTES = 512 * 1024 * 1024
MAX_FILE_BYTES = 1_048_576
BINARY_PROBE_BYTES = 8192
BINARY_PROBE_BYTES = 8192
MAX_REPORTED_SKIPS = 1_000


RESOURCE_LIMIT = "resource-limit"


@dataclass
class Scan:
    root: Path
    findings: list[dict[str, str | int]] = field(default_factory=list)
    skipped: list[dict[str, str]] = field(default_factory=list)
    skipped_count: int = 0
    scanned_files: int = 0
    scanned_bytes: int = 0

    def skip(self, path: Path, reason: str) -> None:
        self.skipped_count += 1
        if len(self.skipped) < MAX_REPORTED_SKIPS:
            self.skipped.append({"path": path.relative_to(self.root).as_posix(), "reason": reason})

    def admission(self, path: Path) -> str | None:
        size = path.stat().st_size
        if size > MAX_FILE_BYTES:
            return "file-too-large"
        if self.scanned_files + 1 > MAX_SCAN_FILES or self.scanned_bytes + size > MAX_SCAN_BYTES:
            return RESOURCE_LIMIT
        self.scanned_files += 1
        self.scanned_bytes += size
        return None

    def read(self, path: Path) -> None:
        shown = path.relative_to(self.root).as_posix()
        with path.open("rb") as source:
            if b"\0" in source.read(BINARY_PROBE_BYTES):
                return
            source.seek(0)
            for number, raw in enumerate(source, 1):
                line = raw.decode("utf-8", errors="ignore")
                self.findings.extend(
                    {"path": shown, "line": number, "kind": kind}
                    for kind, pattern in PATTERNS.items()
                    if pattern.search(line)
                )


def _scannable_files(root: Path, scan: Scan):
    listed = _files_known_to_git(root)
    if listed is None:
        yield from _walked_files(root, scan)
        return
    for entry in listed:
        path = root / entry
        if any(part in UNSCANNED_DIRECTORIES for part in Path(entry).parts):
            continue
        if path.is_symlink():
            scan.skip(path, "symlink")
        elif path.is_file():
            yield path


def _files_known_to_git(root: Path) -> list[str] | None:
    try:
        result = run_git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    return [entry for entry in result.stdout.split("\0") if entry]


def _walked_files(root: Path, scan: Scan):
    for directory, children, files in os.walk(root):
        here = Path(directory)
        kept = []
        for name in children:
            if name in UNSCANNED_DIRECTORIES:
                continue
            if (here / name).is_symlink():
                scan.skip(here / name, "symlink")
            else:
                kept.append(name)
        children[:] = kept
        for name in files:
            path = here / name
            if name in UNSCANNED_DIRECTORIES:
                continue
            if path.is_symlink():
                scan.skip(path, "symlink")
            elif path.is_file():
                yield path


def scan_report(root: Path) -> dict[str, object]:
    root = root.resolve()
    scan = Scan(root)
    for path in _scannable_files(root, scan):
        try:
            refused = scan.admission(path)
            if refused is None:
                scan.read(path)
        except OSError:
            refused = "unreadable"
        if refused:
            scan.skip(path, refused)
        if refused == RESOURCE_LIMIT:
            break
    return {
        "status": "incomplete" if scan.skipped_count else "complete",
        "findings": scan.findings,
        "skipped": scan.skipped,
        "skipped_count": scan.skipped_count,
        "scanned_files": scan.scanned_files,
        "scanned_bytes": scan.scanned_bytes,
    }

