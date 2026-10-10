from __future__ import annotations

import re
from dataclasses import dataclass, field
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
        with path.open(encoding="utf-8", errors="ignore") as source:
            for number, line in enumerate(source, 1):
                self.findings.extend(
                    {"path": shown, "line": number, "kind": kind}
                    for kind, pattern in PATTERNS.items()
                    if pattern.search(line)
                )


def _scannable_files(root: Path, scan: Scan):
    for path in root.rglob("*"):
        if UNSCANNED_DIRECTORIES.intersection(path.relative_to(root).parts):
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


def scan(root: Path) -> list[dict[str, str | int]]:
    return cast(list[dict[str, str | int]], scan_report(root)["findings"])
