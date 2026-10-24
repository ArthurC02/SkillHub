from __future__ import annotations

import json
import re
from pathlib import Path, PurePosixPath
from typing import Any

from .common import shown_path
from .git_hooks import hooks_dir
from .sources import (
    discover_ci_markers,
    file_name,
    ignored_relatives,
    repository_files,
    test_candidates,
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
TASK_PREFIXES = (("lint", "lint"), ("typecheck", "types"), ("test", "tests"))
GO_MODULE = "go.mod"
TASKFILE_NAMES = ("Taskfile.yml", "Taskfile.yaml", "taskfile.yml", "taskfile.yaml")
TASKS_SECTION = re.compile(r"^tasks:[ \t]*$(.*?)(?=^\S|\Z)", re.MULTILINE | re.DOTALL)
TASK_NAME = re.compile(r"""^  ['"]?([A-Za-z0-9_.:-]+)['"]?:""", re.MULTILINE)
PYPROJECT_TABLE = re.compile(r"^\[tool\.([A-Za-z0-9_-]+)", re.MULTILINE)
DEPGUARD_SETTINGS = re.compile(r"^\s*depguard\s*:", re.MULTILINE)


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
    try:
        tables = set(PYPROJECT_TABLE.findall(path.read_text(encoding="utf-8")))
    except UnicodeDecodeError:
        return []
    return [
        gate(kind, tool, shown)
        for tool, kind in PYPROJECT_TOOLS.items()
        if tool in tables
    ]


def package_gates(path: Path, shown: str) -> list[dict[str, str]]:
    try:
        scripts = json.loads(path.read_text(encoding="utf-8")).get("scripts", {})
    except (json.JSONDecodeError, UnicodeDecodeError, AttributeError):
        return []
    if not isinstance(scripts, dict):
        return []
    return [
        gate(kind, f"script:{name}", shown)
        for name, kind in PACKAGE_SCRIPTS.items()
        if name in scripts
    ]


def taskfile_gates(path: Path, shown: str) -> list[dict[str, str]]:
    try:
        section = TASKS_SECTION.search(path.read_text(encoding="utf-8"))
    except UnicodeDecodeError:
        return []
    names = TASK_NAME.findall(section[1]) if section else []
    return [
        gate(kind, f"task:{name}", shown)
        for name in names
        for prefix, kind in TASK_PREFIXES
        if name.startswith(prefix)
    ]


def go_test_gates(candidates: list[str], tests: list[str]) -> list[dict[str, str]]:
    modules = [entry for entry in candidates if file_name(entry) == GO_MODULE]
    tests = [entry for entry in tests if entry.endswith("_test.go") and not in_fixture(entry)]
    return [
        gate("tests", "go-test", module)
        for module in modules
        if any(_inside(test, PurePosixPath(module).parent.as_posix()) for test in tests)
    ]


def _inside(path: str, directory: str) -> bool:
    return directory == "." or path.startswith(directory + "/")


def golangci_gates(path: Path, shown: str) -> list[dict[str, str]]:
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return []
    return [gate("architecture", "depguard", shown)] if DEPGUARD_SETTINGS.search(text) else []


GIT_HOOK_NAMES = {
    "applypatch-msg", "pre-applypatch", "post-applypatch", "pre-commit", "pre-merge-commit",
    "prepare-commit-msg", "commit-msg", "post-commit", "pre-rebase", "post-checkout", "post-merge",
    "pre-push", "pre-receive", "post-receive", "post-update", "push-to-checkout", "pre-auto-gc",
    "post-rewrite", "reference-transaction",
}


def shipped_hooks(files: list[str], installed: list[dict[str, Any]]) -> list[dict[str, Any]]:
    enabled = {entry["path"] for entry in installed}
    return [
        {**gate("hooks", f"git:{file_name(path)}", path), "enabled": False}
        for path in files
        if file_name(path) in GIT_HOOK_NAMES and path not in enabled
    ]


def installed_hooks(root: Path) -> list[dict[str, str]]:
    hooks = hooks_dir(root)
    if not hooks.is_dir():
        return []
    return [
        gate("hooks", f"git:{entry.name}", shown_path(entry, root))
        for entry in sorted(hooks.iterdir())
        if entry.is_file() and "." not in entry.name
    ]


FIXTURE_DIRECTORIES = {"fixtures", "__fixtures__", "testdata"}

CONTENT_READERS = {
    "pyproject.toml": pyproject_gates,
    "package.json": package_gates,
    ".golangci.yml": golangci_gates,
    ".golangci.yaml": golangci_gates,
    **dict.fromkeys(TASKFILE_NAMES, taskfile_gates),
}


def in_fixture(entry: str) -> bool:
    return any(part in FIXTURE_DIRECTORIES for part in entry.split("/"))


def candidate_files(files: list[str]) -> list[str]:
    return sorted(
        entry for entry in files
        if (marker_for(file_name(entry)) or file_name(entry) in CONTENT_READERS or file_name(entry) == GO_MODULE)
        and not in_fixture(entry)
    )


def configured_gates(root: Path, candidates: list[str]) -> list[dict[str, str]]:
    gates = []
    for shown in candidates:
        name = file_name(shown)
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
    files = repository_files(root)
    candidates, tests = candidate_files(files), test_candidates(files)
    ignored = ignored_relatives(root, candidates + tests)
    gates = configured_gates(root, [entry for entry in candidates if entry not in ignored])
    gates.extend(go_test_gates(
        [entry for entry in candidates if entry not in ignored], [entry for entry in tests if entry not in ignored]
    ))
    installed = installed_hooks(root)
    gates.extend(installed)
    gates.extend(shipped_hooks(files, installed))
    gates.extend(gate("ci", tool, path) for path, tool in discover_ci_markers(root))
    present = sorted({entry["kind"] for entry in gates})
    return {
        "format": FORMAT,
        "gates": gates,
        "kinds_present": present,
        "kinds_missing": [kind for kind in KINDS if kind not in present],
        "test_files": sum(entry not in ignored and not in_fixture(entry) for entry in tests),
        "standard": "configured" if STANDARD_KINDS & set(present) else "none",
    }
