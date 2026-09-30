from __future__ import annotations

import json
import os
import re
from pathlib import Path, PurePosixPath
from typing import Any

from .sources import (
    EXCLUDED_DIRECTORIES,
    discover_ci_tools,
    relative,
    test_locations,
    without_ignored,
)

FORMAT = "domain-memory-quality-gates/v1"
KINDS = ("lint", "types", "architecture", "tests", "hooks", "ci")
STANDARD_KINDS = {"lint", "types", "architecture"}

MARKER_FILES = {
    ".golangci.yml": ("lint", "golangci-lint"),
    ".golangci.yaml": ("lint", "golangci-lint"),
    "ruff.toml": ("lint", "ruff"),
    ".ruff.toml": ("lint", "ruff"),
    ".flake8": ("lint", "flake8"),
    ".pylintrc": ("lint", "pylint"),
    ".oxlintrc.json": ("lint", "oxlint"),
    "biome.json": ("lint", "biome"),
    ".rubocop.yml": ("lint", "rubocop"),
    "clippy.toml": ("lint", "clippy"),
    "checkstyle.xml": ("lint", "checkstyle"),
    ".shellcheckrc": ("lint", "shellcheck"),
    "tsconfig.json": ("types", "typescript"),
    "mypy.ini": ("types", "mypy"),
    "pyrightconfig.json": ("types", "pyright"),
    ".importlinter": ("architecture", "import-linter"),
    ".go-arch-lint.yml": ("architecture", "go-arch-lint"),
    "pytest.ini": ("tests", "pytest"),
    ".pre-commit-config.yaml": ("hooks", "pre-commit"),
    "lefthook.yml": ("hooks", "lefthook"),
}
MARKER_PREFIXES = {
    ".eslintrc": ("lint", "eslint"),
    "eslint.config.": ("lint", "eslint"),
    ".dependency-cruiser.": ("architecture", "dependency-cruiser"),
    "jest.config.": ("tests", "jest"),
    "vitest.config.": ("tests", "vitest"),
    "playwright.config.": ("tests", "playwright"),
}
PYPROJECT_TOOLS = {
    "ruff": "lint",
    "flake8": "lint",
    "pylint": "lint",
    "mypy": "types",
    "pyright": "types",
    "importlinter": "architecture",
    "pytest": "tests",
}
PACKAGE_SCRIPTS = {"lint": "lint", "typecheck": "types", "test": "tests"}
PYPROJECT_TABLE = re.compile(r"^\[tool\.([A-Za-z0-9_-]+)", re.MULTILINE)


def gate(kind: str, tool: str, path: str) -> dict[str, str]:
    return {"kind": kind, "tool": tool, "path": path}


def marker_for(name: str) -> tuple[str, str] | None:
    if name in MARKER_FILES:
        return MARKER_FILES[name]
    for prefix, marker in MARKER_PREFIXES.items():
        if name.startswith(prefix):
            return marker
    return None


def pyproject_gates(path: Path, shown: str) -> list[dict[str, str]]:
    tables = set(PYPROJECT_TABLE.findall(path.read_text(encoding="utf-8")))
    return [
        gate(kind, tool, shown)
        for tool, kind in PYPROJECT_TOOLS.items()
        if tool in tables
    ]


def package_gates(path: Path, shown: str) -> list[dict[str, str]]:
    try:
        scripts = json.loads(path.read_text(encoding="utf-8")).get("scripts", {})
    except (json.JSONDecodeError, AttributeError):
        return []
    return [
        gate(kind, f"script:{name}", shown)
        for name, kind in PACKAGE_SCRIPTS.items()
        if name in scripts
    ]


FIXTURE_DIRECTORIES = {"fixtures", "__fixtures__", "testdata"}
NOT_THIS_REPOSITORYS_GATES = EXCLUDED_DIRECTORIES | FIXTURE_DIRECTORIES

CONTENT_READERS = {"pyproject.toml": pyproject_gates, "package.json": package_gates}


def candidate_files(root: Path) -> list[str]:
    found = []
    for directory, children, files in os.walk(root):
        children[:] = [name for name in children if name not in NOT_THIS_REPOSITORYS_GATES]
        found.extend(
            relative(root, Path(directory) / name)
            for name in files
            if marker_for(name) or name in CONTENT_READERS
        )
    return without_ignored(root, sorted(found))


def in_fixture(entry: str) -> bool:
    return any(part in FIXTURE_DIRECTORIES for part in PurePosixPath(entry).parts)


def configured_gates(root: Path) -> list[dict[str, str]]:
    gates = []
    for shown in candidate_files(root):
        name = Path(shown).name
        marker = marker_for(name)
        if marker:
            gates.append(gate(*marker, shown))
        if name in CONTENT_READERS:
            gates.extend(CONTENT_READERS[name](root / shown, shown))
    return gates


def quality_gates(repo_root: Path) -> dict[str, Any]:
    root = repo_root.resolve()
    if not root.is_dir():
        raise ValueError(f"repository root is not a directory: {repo_root}")
    gates = configured_gates(root)
    gates.extend(gate("ci", tool, "") for tool in discover_ci_tools(root))
    present = sorted({entry["kind"] for entry in gates})
    return {
        "format": FORMAT,
        "gates": gates,
        "kinds_present": present,
        "kinds_missing": [kind for kind in KINDS if kind not in present],
        "test_files": sum(not in_fixture(entry) for entry in test_locations(root)),
        "standard": "configured" if STANDARD_KINDS & set(present) else "none",
    }
