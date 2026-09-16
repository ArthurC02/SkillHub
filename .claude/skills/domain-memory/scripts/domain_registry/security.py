from __future__ import annotations

import re
from pathlib import Path


PATTERNS = {
    "private-key": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    "github-token": re.compile(r"\bgh[pousr]_[A-Za-z0-9_]{20,}\b"),
    "generic-secret": re.compile(r"(?i)\b(?:api[_-]?key|secret|password|token)\s*[:=]\s*['\"]?[A-Za-z0-9_\-]{16,}"),
}


def scan(root: Path) -> list[dict[str, str | int]]:
    findings = []
    for path in root.rglob("*"):
        if not path.is_file() or any(part in {".git", "node_modules", ".venv"} for part in path.relative_to(root).parts):
            continue
        if path.stat().st_size > 1_048_576:
            continue
        for number, line in enumerate(path.read_text(encoding="utf-8", errors="ignore").splitlines(), 1):
            for kind, pattern in PATTERNS.items():
                if pattern.search(line):
                    findings.append({"path": path.relative_to(root).as_posix(), "line": number, "kind": kind})
    return findings
