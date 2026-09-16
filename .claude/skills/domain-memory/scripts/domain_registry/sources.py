import json
import hashlib
from datetime import datetime, timezone
import fnmatch
import subprocess
from pathlib import Path
from typing import Any

from .audit import append as append_audit
from .common import completed_identifier


EXCLUDED_DIRECTORIES = {".git", ".hg", ".svn", "node_modules", ".venv", "vendor", "dist", "build", "generated"}


def git_ignored(root: Path, candidates: list[Path]) -> set[Path]:
    if not candidates or not (root / ".git").exists():
        return set()
    by_relative = {relative(root, path): path for path in candidates}
    try:
        result = subprocess.run(
            ["git", "-C", str(root), "check-ignore", "-z", "--stdin"],
            input=chr(0).join(by_relative),
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return set()
    if result.returncode not in (0, 1):
        return set()
    return {by_relative[entry] for entry in result.stdout.split(chr(0)) if entry in by_relative}


def relative(root: Path, path: Path) -> str:
    return path.relative_to(root).as_posix()


def existing_directories(root: Path, candidates: tuple[str, ...]) -> list[str]:
    return [candidate for candidate in candidates if (root / candidate).is_dir()]


def instruction_files(root: Path) -> list[str]:
    files = []
    for path in root.rglob("AGENTS.md"):
        if not any(part in EXCLUDED_DIRECTORIES for part in path.parts):
            files.append(relative(root, path))
    for path in root.rglob("CLAUDE.md"):
        if not any(part in EXCLUDED_DIRECTORIES for part in path.parts):
            files.append(relative(root, path))
    return sorted(set(files))


def boundary_files(root: Path) -> list[str]:
    files = []
    for base in ("apps", "src", "services", "packages"):
        directory = root / base
        if not directory.is_dir():
            continue
        for path in directory.rglob("doc.go"):
            if not any(part in EXCLUDED_DIRECTORIES for part in path.parts):
                files.append(relative(root, path))
    return sorted(files)


def test_locations(root: Path) -> list[str]:
    locations = []
    for candidate in ("tests", "test", "e2e", "integration"):
        directory = root / candidate
        if not directory.is_dir():
            continue
        found = False
        for path in directory.rglob("*"):
            if any(part in EXCLUDED_DIRECTORIES for part in path.parts):
                continue
            if path.is_file() and (path.name.endswith("_test.go") or ".test." in path.name or ".spec." in path.name):
                found = True
                break
        if found:
            locations.append(candidate)
    return locations


def discover_sources(root: Path) -> dict[str, Any]:
    root = root.resolve()
    groups = [
        {"kind": "repository_instructions", "authority": "instructions to coding agents, not a statement of the business domain",
         "caution": "These files exist to steer agents working in this repository. They are rewritten whenever the workflow changes, and a Domain Memory that rests on one loses its evidence when it is. Select them only for a domain fact no other source states, and record the gap.", "paths": instruction_files(root)},
        {"kind": "decisions", "authority": "current approved decisions when their status says so", "paths": existing_directories(root, ("docs/adr", "adr", "docs/architecture", "architecture"))},
        {"kind": "requirements", "authority": "intended behavior and acceptance criteria", "paths": existing_directories(root, ("docs/plans", "docs/spec", "docs/requirements", "spec", "requirements"))},
        {"kind": "contracts", "authority": "inter-process promises and compatibility", "paths": existing_directories(root, ("contracts", "openapi", "asyncapi"))},
        {"kind": "bounded_contexts", "authority": "candidate technical boundaries requiring domain review", "paths": boundary_files(root)},
        {"kind": "implementation", "authority": "current executable behavior", "paths": existing_directories(root, ("apps", "src", "services", "packages"))},
        {"kind": "tests", "authority": "evidence that a specific behavior is checked", "paths": test_locations(root)},
    ]
    return {
        "format": "domain-source-map/v1",
        "repo_root": ".",
        "source_order": [group["kind"] for group in groups],
        "source_groups": groups,
        "missing_groups": [group["kind"] for group in groups if not group["paths"]],
        "selection_status": "discovered",
        "note": "The map locates candidate sources. It does not establish a domain fact or source authority by itself.",
    }


def policy_matches(path: str, include: list[str], exclude: list[str]) -> bool:
    included = any(pattern == "**" or fnmatch.fnmatchcase(path, pattern) for pattern in include)
    excluded = any(fnmatch.fnmatchcase(path, pattern) for pattern in exclude)
    return included and not excluded


def source_files(root: Path, selected_paths: list[Path], policy: dict[str, Any] | None = None) -> list[Path]:
    include = policy.get("source_policy", {}).get("include", ["**"]) if policy else ["**"]
    exclude = policy.get("source_policy", {}).get("exclude", []) if policy else []
    files: set[Path] = set()
    for selected in selected_paths:
        resolved = selected.resolve()
        candidates = [resolved] if resolved.is_file() else resolved.rglob("*")
        for candidate in candidates:
            if candidate.is_file() and not any(part in EXCLUDED_DIRECTORIES for part in candidate.parts):
                relative_path = relative(root, candidate)
                if policy_matches(relative_path, include, exclude):
                    files.add(candidate)
    return sorted(files - git_ignored(root, sorted(files)))


def source_policy_report(root: Path, selected_paths: list[Path], policy: dict[str, Any]) -> dict[str, Any]:
    files = source_files(root, selected_paths, policy)
    limits = policy["limits"]
    total_bytes = sum(path.stat().st_size for path in files)
    oversized = [relative(root, path) for path in files if path.stat().st_size > limits["max_file_bytes"]]
    errors = []
    if len(files) > limits["max_file_count"]:
        errors.append(f"selected sources hold {len(files)} files, over max_file_count {limits['max_file_count']}")
    if total_bytes > limits["max_total_bytes"]:
        errors.append(f"selected sources hold {total_bytes} bytes, over max_total_bytes {limits['max_total_bytes']}")
    if oversized:
        named = ", ".join(oversized[:5])
        remainder = "" if len(oversized) <= 5 else f", and {len(oversized) - 5} more"
        errors.append(f"over max_file_bytes {limits['max_file_bytes']}: {named}{remainder}")
    return {"files": len(files), "total_bytes": total_bytes, "oversized_files": oversized, "errors": errors}


def owned_source_files(root: Path, selected: list[str], policy: dict[str, Any] | None = None) -> dict[str, list[Path]]:
    by_source = {path: source_files(root, [root / path], policy) for path in selected}
    owner = {}
    for path, files in by_source.items():
        for file in files:
            if len(path) > len(owner.get(file, "")):
                owner[file] = path
    return {path: [file for file in files if owner[file] == path] for path, files in by_source.items()}


def source_snapshot(root: Path, path: str, files: list[Path]) -> dict[str, Any]:
    digest = hashlib.sha256()
    for file in files:
        digest.update(relative(root, file).encode("utf-8"))
        digest.update(b"\0")
        digest.update(file.read_bytes())
        digest.update(b"\0")
    return {"path": path, "file_count": len(files), "digest": f"sha256:{digest.hexdigest()}"}


def source_snapshots(root: Path, selected: list[str], policy: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    owned = owned_source_files(root, selected, policy)
    return [source_snapshot(root, path, owned[path]) for path in selected]


def source_kinds(source_map: dict[str, Any]) -> dict[str, str]:
    kind_of = {}
    for group in source_map.get("source_groups", []):
        for path in group["paths"]:
            kind_of[path] = group["kind"]
    return {path: kind_of.get(path, "unclassified") for path in source_map.get("selected_paths", [])}


def source_kind_for(source_map: dict[str, Any], cited_path: str) -> str:
    kinds = source_map.get("source_kinds") or source_kinds(source_map)
    owner = ""
    for path in kinds:
        if (cited_path == path or cited_path.startswith(path + "/")) and len(path) > len(owner):
            owner = path
    return kinds.get(owner, "unclassified")


def selected_source_map(root: Path, selected_paths: list[Path], policy: dict[str, Any] | None = None) -> dict[str, Any]:
    root = root.resolve()
    selected = []
    for path in selected_paths:
        resolved = path.resolve()
        try:
            selected.append(relative(root, resolved))
        except ValueError as error:
            raise ValueError(f"source path is outside repository: {path}") from error
        if not resolved.exists():
            raise ValueError(f"source path does not exist: {path}")
    if not selected:
        raise ValueError("at least one source path is required")
    source_map = discover_sources(root)
    source_map["selection_status"] = "agent-asserted"
    source_map["selected_paths"] = sorted(set(selected))
    if policy:
        report = source_policy_report(root, [root / path for path in source_map["selected_paths"]], policy)
        if report["errors"]:
            raise ValueError("source policy limits failed: " + "; ".join(report["errors"]))
        source_map["source_policy_report"] = report
    source_map["source_kinds"] = source_kinds(source_map)
    source_map["git_state"] = git_state(root, source_map["selected_paths"])
    source_map["source_snapshots"] = source_snapshots(root, source_map["selected_paths"], policy)
    return source_map


def confirm_sources(root: Path, repo_root: Path, confirmed_by: str, policy: dict[str, Any] | None = None) -> dict[str, Any]:
    path = root / "source-map.json"
    if not path.is_file():
        raise ValueError(f"no source map at {path}: initialize the Domain Memory before confirming its sources")
    if not completed_identifier(confirmed_by):
        raise ValueError(
            "confirming sources records who chose them, so it needs the developer's own identity as they gave it. "
            "An agent cannot confirm on their behalf, and an identity read out of a git config, a commit or a file "
            "in the repository is not the developer answering."
        )
    freshness = verify_source_map(repo_root, path, policy)
    if freshness["status"] == "stale":
        raise ValueError(
            "the selected sources have moved since this map was written, so confirming it would attest to a corpus "
            "that is no longer there; refresh the map first, then confirm what the developer actually looked at: "
            + ", ".join(source["path"] for source in freshness.get("changed_sources", []))
        )
    if freshness["status"] != "current":
        raise ValueError(
            "the source map does not verify, so there is nothing to confirm yet: "
            + str(freshness.get("reason", freshness["status"]))
        )
    source_map = json.loads(path.read_text(encoding="utf-8"))
    source_map["selection_status"] = "developer-confirmed"
    source_map["confirmed_by"] = confirmed_by
    source_map["confirmed_at"] = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    write_source_map(path, source_map)
    append_audit(root, {
        "operation": "confirm-sources",
        "confirmed_by": confirmed_by,
        "selected_paths": source_map.get("selected_paths", []),
    })
    return source_map

def verify_source_map(root: Path, source_map_path: Path, policy: dict[str, Any] | None = None) -> dict[str, Any]:
    source_map = json.loads(source_map_path.read_text(encoding="utf-8"))
    snapshots = source_map.get("source_snapshots")
    selection_status = source_map.get("selection_status", "agent-asserted")
    if not isinstance(snapshots, list):
        return {"status": "unverified", "selection_status": selection_status, "reason": "source map has no snapshots"}
    if not all(isinstance(snapshot, dict) and isinstance(snapshot.get("path"), str) for snapshot in snapshots):
        return {"status": "unverified", "selection_status": selection_status, "reason": "source map has an invalid snapshot"}
    recorded_paths = [snapshot["path"] for snapshot in snapshots]
    actual = {snapshot["path"]: snapshot for snapshot in source_snapshots(root, recorded_paths, policy)}
    changed = []
    for snapshot in snapshots:
        if not (root / snapshot["path"]).exists():
            changed.append({"path": snapshot["path"], "status": "missing"})
        elif actual[snapshot["path"]] != snapshot:
            changed.append({"path": snapshot["path"], "status": "changed", "expected": snapshot, "actual": actual[snapshot["path"]]})
    if policy:
        report = source_policy_report(root, [root / path for path in source_map.get("selected_paths", [])], policy)
        if report["errors"]:
            return {"status": "invalid", "selection_status": selection_status, "reason": "; ".join(report["errors"]), "source_policy_report": report}
    return {"status": "current" if not changed else "stale", "selection_status": selection_status, "changed_sources": changed}


def write_source_map(path: Path, source_map: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(source_map, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def git_state(root: Path, selected_paths: list[str]) -> dict[str, Any]:
    if not selected_paths or not (root / ".git").exists():
        return {}
    try:
        trees = subprocess.run(
            ["git", "-C", str(root), "rev-parse"] + [f"HEAD:{path}" for path in selected_paths],
            capture_output=True,
            text=True,
            check=False,
        )
        dirty = subprocess.run(
            ["git", "-C", str(root), "status", "--porcelain", "-z", "--"] + list(selected_paths),
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError:
        return {}
    if trees.returncode != 0 or dirty.returncode != 0:
        return {}
    objects = trees.stdout.split()
    if len(objects) != len(selected_paths):
        return {}
    return {
        "tracked_objects": dict(zip(selected_paths, objects)),
        "dirty_sources": sorted(entry for entry in dirty.stdout.split(chr(0)) if entry),
    }


def probe_sources(root: Path, source_map_path: Path, policy: dict[str, Any] | None = None) -> dict[str, Any]:
    if not source_map_path.exists():
        return {"status": "absent", "reason": f"no source map at {source_map_path}"}
    source_map = json.loads(source_map_path.read_text(encoding="utf-8"))
    recorded = source_map.get("git_state")
    if isinstance(recorded, dict) and recorded.get("tracked_objects") and not recorded.get("dirty_sources"):
        now = git_state(root, source_map.get("selected_paths", []))
        if now.get("tracked_objects") == recorded["tracked_objects"] and not now.get("dirty_sources"):
            return {
                "status": "current",
                "selection_status": source_map.get("selection_status", "agent-asserted"),
                "changed_sources": [],
                "checked": "git",
            }
    return verify_source_map(root, source_map_path, policy) | {"checked": "hash"}
