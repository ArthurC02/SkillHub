import fnmatch
import hashlib
import json
import subprocess
from collections.abc import Iterable
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Any

from .common import (
    GIT_COMMAND_TIMEOUT_SECONDS,
    completed_identifier,
    load_json,
    writer_lock,
)

EXCLUDED_DIRECTORIES = {
    ".git",
    ".hg",
    ".svn",
    ".devctl",
    "node_modules",
    ".venv",
    "vendor",
    "dist",
    "build",
    "generated",
}

IMPLEMENTATION_ROOTS = ("apps", "src", "services", "packages")

CI_MARKERS = {
    ".github/workflows": "github-actions",
    ".gitlab-ci.yml": "gitlab-ci",
    "Jenkinsfile": "jenkins",
    ".circleci/config.yml": "circleci",
    ".buildkite/pipeline.yml": "buildkite",
    "azure-pipelines.yml": "azure-pipelines",
}


def discover_ci_tools(root: Path) -> list[str]:
    return sorted(name for path, name in CI_MARKERS.items() if (root / path).exists())


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
    except (OSError, subprocess.TimeoutExpired):
        return set()
    if result.returncode not in (0, 1):
        return set()
    return {
        by_relative[entry]
        for entry in result.stdout.split(chr(0))
        if entry in by_relative
    }


def relative(root: Path, path: Path) -> str:
    return path.relative_to(root).as_posix()


def in_excluded_directory(path: Path) -> bool:
    return any(part in EXCLUDED_DIRECTORIES for part in path.parts)


def existing_directories(root: Path, candidates: tuple[str, ...]) -> list[str]:
    return [candidate for candidate in candidates if (root / candidate).is_dir()]


def in_nested_checkout(root: Path, entry: str) -> bool:
    return any(
        (root / parent / ".git").exists()
        for parent in PurePosixPath(entry).parents[:-1]
    )


def without_ignored(root: Path, relatives: list[str]) -> list[str]:
    kept = [entry for entry in relatives if not in_nested_checkout(root, entry)]
    ignored = git_ignored(root, [root / entry for entry in kept])
    return [entry for entry in kept if (root / entry) not in ignored]


def instruction_files(root: Path) -> list[str]:
    files = {
        relative(root, path)
        for name in ("AGENTS.md", "CLAUDE.md")
        for path in root.rglob(name)
        if not in_excluded_directory(path)
    }
    return without_ignored(root, sorted(files))


def boundary_files(root: Path) -> list[str]:
    files = [
        relative(root, path)
        for base in existing_directories(root, IMPLEMENTATION_ROOTS)
        for path in (root / base).rglob("doc.go")
        if not in_excluded_directory(path)
    ]
    return without_ignored(root, sorted(files))


def is_test_file(name: str) -> bool:
    return (
        name.endswith(("_test.go", "_test.py"))
        or ".test." in name
        or ".spec." in name
        or (name.startswith("test_") and name.endswith(".py"))
    )


def test_locations(root: Path) -> list[str]:
    candidates = [
        root,
        *(
            root / name
            for name in ("tests", "test", "e2e", "integration", *IMPLEMENTATION_ROOTS)
        ),
    ]
    locations = {
        relative(root, path)
        for directory in candidates
        if directory.is_dir()
        for path in directory.rglob("*")
        if not in_excluded_directory(path)
        and path.is_file()
        and is_test_file(path.name)
    }
    return without_ignored(root, sorted(locations))


def discover_sources(root: Path) -> dict[str, Any]:
    root = root.resolve()
    ci_tools = discover_ci_tools(root)
    groups = [
        {
            "kind": "repository_instructions",
            "authority": "instructions to coding agents, not a statement of the business domain",
            "caution": "These files exist to steer agents working in this repository. They are rewritten whenever the workflow changes, and a Domain Memory that rests on one loses its evidence when it is. Select them only for a domain fact no other source states, and record the gap.",
            "paths": instruction_files(root),
        },
        {
            "kind": "decisions",
            "authority": "current approved decisions when their status says so",
            "paths": existing_directories(
                root, ("docs/adr", "adr", "docs/architecture", "architecture")
            ),
        },
        {
            "kind": "requirements",
            "authority": "intended behavior and acceptance criteria",
            "paths": existing_directories(
                root,
                (
                    "docs/plans",
                    "docs/spec",
                    "docs/requirements",
                    "spec",
                    "requirements",
                ),
            ),
        },
        {
            "kind": "contracts",
            "authority": "inter-process promises and compatibility",
            "paths": existing_directories(root, ("contracts", "openapi", "asyncapi")),
        },
        {
            "kind": "bounded_contexts",
            "authority": "candidate technical boundaries requiring domain review",
            "paths": boundary_files(root),
        },
        {
            "kind": "implementation",
            "authority": "current executable behavior",
            "paths": existing_directories(root, IMPLEMENTATION_ROOTS),
        },
        {
            "kind": "tests",
            "authority": "evidence that a specific behavior is checked",
            "paths": test_locations(root),
        },
    ]
    return {
        "format": "domain-source-map/v1",
        "repo_root": ".",
        "source_order": [group["kind"] for group in groups],
        "source_groups": groups,
        "missing_groups": [group["kind"] for group in groups if not group["paths"]],
        "selection_status": "discovered",
        "governance_candidates": {
            "ci_tools": ci_tools,
            "recommended_verifier": "github-pr" if ci_tools else "git-signed-commit",
            "recommended_trigger": "external-scm" if ci_tools else "git-push",
        },
        "note": "The map locates candidate sources. It does not establish a domain fact or source authority by itself.",
    }


def policy_matches(path: str, include: list[str], exclude: list[str]) -> bool:
    included = any(
        pattern == "**" or fnmatch.fnmatchcase(path, pattern) for pattern in include
    )
    excluded = any(fnmatch.fnmatchcase(path, pattern) for pattern in exclude)
    return included and not excluded


def source_files(
    root: Path, selected_paths: list[Path], policy: dict[str, Any] | None = None
) -> list[Path]:
    source_policy = policy.get("source_policy", {}) if policy else {}
    include = source_policy.get("include", ["**"])
    exclude = source_policy.get("exclude", [])
    files: set[Path] = set()
    roots = []
    for selected in selected_paths:
        resolved = selected.resolve()
        if any(
            resolved == parent or parent in resolved.parents
            for parent in roots
            if parent.is_dir()
        ):
            continue
        roots.append(resolved)
    for resolved in roots:
        candidates = [resolved] if resolved.is_file() else resolved.rglob("*")
        for candidate in candidates:
            if (
                candidate.is_file()
                and not candidate.is_symlink()
                and not in_excluded_directory(candidate)
            ):
                relative_path = relative(root, candidate)
                if policy_matches(relative_path, include, exclude):
                    files.add(candidate)
    return sorted(files - git_ignored(root, sorted(files)))


def source_policy_report(
    root: Path, selected_paths: list[Path], policy: dict[str, Any]
) -> dict[str, Any]:
    files = source_files(root, selected_paths, policy)
    limits = policy["limits"]
    sizes = {path: path.stat().st_size for path in files}
    total_bytes = sum(sizes.values())
    oversized = [
        relative(root, path)
        for path, size in sizes.items()
        if size > limits["max_file_bytes"]
    ]
    errors = []
    if len(files) > limits["max_file_count"]:
        errors.append(
            f"selected sources hold {len(files)} files, over max_file_count {limits['max_file_count']}"
        )
    if total_bytes > limits["max_total_bytes"]:
        errors.append(
            f"selected sources hold {total_bytes} bytes, over max_total_bytes {limits['max_total_bytes']}"
        )
    if oversized:
        named = ", ".join(oversized[:5])
        remainder = "" if len(oversized) <= 5 else f", and {len(oversized) - 5} more"
        errors.append(
            f"over max_file_bytes {limits['max_file_bytes']}: {named}{remainder}"
        )
    return {
        "files": len(files),
        "total_bytes": total_bytes,
        "oversized_files": oversized,
        "errors": errors,
    }


def owned_source_files(
    root: Path, selected: list[str], policy: dict[str, Any] | None = None
) -> dict[str, list[Path]]:
    files = source_files(root, [root / path for path in selected], policy)
    owner = {}
    for file in files:
        for path in selected:
            selected_path = (root / path).resolve()
            if (
                file == selected_path
                or (selected_path.is_dir() and selected_path in file.parents)
            ) and len(path) > len(owner.get(file, "")):
                owner[file] = path
    return {
        path: [file for file in files if owner[file] == path] for path in selected
    }


def source_snapshot(root: Path, path: str, files: list[Path]) -> dict[str, Any]:
    digest = hashlib.sha256()
    listing = hashlib.sha256()
    for file in files:
        name = relative(root, file).encode("utf-8")
        listing.update(name)
        listing.update(b"\0")
        digest.update(name)
        digest.update(b"\0")
        digest.update(file.read_bytes())
        digest.update(b"\0")
    return {
        "path": path,
        "file_count": len(files),
        "digest": f"sha256:{digest.hexdigest()}",
        "listing_sha256": f"sha256:{listing.hexdigest()}",
    }


def selection_moved(recorded: dict[str, Any], current: dict[str, Any]) -> bool:
    listing = recorded.get("listing_sha256")
    return recorded.get("file_count") != current.get("file_count") or (
        isinstance(listing, str) and listing != current.get("listing_sha256")
    )


def source_snapshots(
    root: Path, selected: list[str], policy: dict[str, Any] | None = None
) -> list[dict[str, Any]]:
    owned = owned_source_files(root, selected, policy)
    return [source_snapshot(root, path, owned[path]) for path in selected]


def source_kinds(source_map: dict[str, Any]) -> dict[str, str]:
    kind_of = {
        path: group["kind"]
        for group in source_map.get("source_groups", [])
        for path in group["paths"]
    }
    return {
        path: kind_of.get(path, "unclassified")
        for path in source_map.get("selected_paths", [])
    }


def longest_source_containing(paths: Iterable[str], cited_path: str) -> str:
    return max(
        (
            path
            for path in paths
            if cited_path == path or cited_path.startswith(path + "/")
        ),
        key=len,
        default="",
    )


def source_kind_for(source_map: dict[str, Any], cited_path: str) -> str:
    kinds = source_map.get("source_kinds") or source_kinds(source_map)
    owner = longest_source_containing(kinds, cited_path)
    if owner and kinds[owner] != "unclassified":
        return kinds[owner]
    owner = ""
    kind = "unclassified"
    for group in source_map.get("source_groups", []):
        group_owner = longest_source_containing(group.get("paths", []), cited_path)
        if len(group_owner) > len(owner):
            owner = group_owner
            kind = group.get("kind", "unclassified")
    return kind if isinstance(kind, str) else "unclassified"


def selected_source_map(
    root: Path, selected_paths: list[Path], policy: dict[str, Any] | None = None
) -> dict[str, Any]:
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
        report = source_policy_report(
            root, [root / path for path in source_map["selected_paths"]], policy
        )
        if report["errors"]:
            raise ValueError(
                "source policy limits failed: " + "; ".join(report["errors"])
            )
        source_map["source_policy_report"] = report
    source_map["source_kinds"] = source_kinds(source_map)
    source_map["git_state"] = git_state(root, source_map["selected_paths"])
    source_map["source_snapshots"] = source_snapshots(
        root, source_map["selected_paths"], policy
    )
    return source_map


def confirm_sources(
    root: Path, repo_root: Path, confirmed_by: str, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    path = root / "source-map.json"
    if not path.is_file():
        raise ValueError(
            f"no source map at {path}: initialize the Domain Memory before confirming its sources"
        )
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
            + ", ".join(
                source["path"] for source in freshness.get("changed_sources", [])
            )
        )
    if freshness["status"] != "current":
        raise ValueError(
            "the source map does not verify, so there is nothing to confirm yet: "
            + str(freshness.get("reason", freshness["status"]))
        )
    source_map = load_json(path)
    source_map["selection_status"] = "developer-confirmed"
    source_map["confirmed_by"] = confirmed_by
    source_map["confirmed_at"] = (
        datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    )
    with writer_lock(root):
        write_and_audit(
            root,
            source_map,
            {
                "operation": "confirm-sources",
                "confirmed_by": confirmed_by,
                "selected_paths": source_map.get("selected_paths", []),
            },
        )
    return source_map


def refresh_sources(
    root: Path, repo_root: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    path = root / "source-map.json"
    if not path.is_file():
        raise ValueError(
            f"no source map at {path}: "
            "initialize the Domain Memory before refreshing it"
        )
    selected = load_json(path).get("selected_paths")
    if not isinstance(selected, list) or not all(
        isinstance(item, str) and item.strip() for item in selected
    ):
        raise ValueError("source map has no valid selected paths to refresh")
    source_map = selected_source_map(
        repo_root, [repo_root / item for item in selected], policy
    )
    with writer_lock(root):
        write_and_audit(
            root,
            source_map,
            {
                "operation": "refresh-sources",
                "selected_paths": source_map["selected_paths"],
            },
        )
    return source_map


def refine_sources(
    root: Path, repo_root: Path, selected_paths: list[Path], policy: dict[str, Any]
) -> dict[str, Any]:
    if not selected_paths:
        raise ValueError("refining sources requires at least one focused path")
    source_map = selected_source_map(repo_root, selected_paths, policy)
    policy["source_policy"]["selected_paths"] = source_map["selected_paths"]
    with writer_lock(root):
        (root / "domain-memory-policy.json").write_text(
            json.dumps(policy, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
        )
        write_and_audit(
            root,
            source_map,
            {
                "operation": "refine-sources",
                "selected_paths": source_map["selected_paths"],
            },
        )
    return source_map


def source_verdict(status: str, selection_status: str, reason: str) -> dict[str, str]:
    return {"status": status, "selection_status": selection_status, "reason": reason}


def verify_source_map(
    root: Path, source_map_path: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    root = root.resolve()
    source_map = load_json(source_map_path)
    snapshots = source_map.get("source_snapshots")
    selection_status = source_map.get("selection_status", "agent-asserted")
    if not isinstance(snapshots, list):
        return source_verdict(
            "unverified", selection_status, "source map has no snapshots"
        )
    if not all(
        isinstance(snapshot, dict) and isinstance(snapshot.get("path"), str)
        for snapshot in snapshots
    ):
        return source_verdict(
            "unverified", selection_status, "source map has an invalid snapshot"
        )
    recorded_paths = [snapshot["path"] for snapshot in snapshots]
    selected_paths = source_map.get("selected_paths")
    if (
        not isinstance(selected_paths, list)
        or not all(isinstance(path, str) and path.strip() for path in selected_paths)
        or len(selected_paths) != len(set(selected_paths))
        or set(selected_paths) != set(recorded_paths)
    ):
        return source_verdict(
            "unverified",
            selection_status,
            "source map snapshots do not match selected_paths",
        )
    for selected_path in recorded_paths:
        try:
            (root / selected_path).resolve().relative_to(root)
        except ValueError:
            return source_verdict(
                "invalid",
                selection_status,
                f"source path escapes repository: {selected_path}",
            )
    if policy is not None:
        policy_paths = policy.get("source_policy", {}).get("selected_paths")
        if isinstance(policy_paths, list) and sorted(policy_paths) != sorted(
            selected_paths
        ):
            return source_verdict(
                "invalid",
                selection_status,
                "policy selected_paths do not match the source map",
            )
    actual = {
        snapshot["path"]: snapshot
        for snapshot in source_snapshots(root, recorded_paths, policy)
    }
    changed = []
    drifted = []
    for snapshot in snapshots:
        current = actual[snapshot["path"]]
        if not (root / snapshot["path"]).exists():
            changed.append({"path": snapshot["path"], "status": "missing"})
        elif selection_moved(snapshot, current):
            changed.append(
                {
                    "path": snapshot["path"],
                    "status": "changed",
                    "expected": snapshot,
                    "actual": current,
                }
            )
        elif snapshot.get("digest") != current.get("digest"):
            drifted.append({"path": snapshot["path"], "status": "content-changed"})
    if policy:
        report = source_policy_report(
            root, [root / path for path in selected_paths], policy
        )
        if report["errors"]:
            return source_verdict(
                "invalid", selection_status, "; ".join(report["errors"])
            ) | {"source_policy_report": report}
    return {
        "status": "stale" if changed else "current",
        "selection_status": selection_status,
        "changed_sources": changed,
        "content_changed": drifted,
    }


def write_source_map(path: Path, source_map: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(source_map, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )


def write_and_audit(
    root: Path, source_map: dict[str, Any], event: dict[str, Any]
) -> None:
    from .audit import append_locked

    write_source_map(root / "source-map.json", source_map)
    append_locked(root, event)


def run_git(root: Path, *arguments: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(root), *arguments],
        capture_output=True,
        text=True,
        check=False,
        timeout=GIT_COMMAND_TIMEOUT_SECONDS,
    )


def git_state(root: Path, selected_paths: list[str]) -> dict[str, Any]:
    if not selected_paths or not (root / ".git").exists():
        return {}
    try:
        trees = run_git(root, "rev-parse", *(f"HEAD:{path}" for path in selected_paths))
        dirty = run_git(root, "status", "--porcelain", "-z", "--", *selected_paths)
    except (OSError, subprocess.TimeoutExpired):
        return {}
    if trees.returncode != 0 or dirty.returncode != 0:
        return {}
    objects = trees.stdout.split()
    if len(objects) != len(selected_paths):
        return {}
    return {
        "tracked_objects": dict(zip(selected_paths, objects, strict=True)),
        "dirty_sources": sorted(entry for entry in dirty.stdout.split(chr(0)) if entry),
    }


def probe_sources(
    root: Path, source_map_path: Path, policy: dict[str, Any] | None = None
) -> dict[str, Any]:
    if not source_map_path.exists():
        return {"status": "absent", "reason": f"no source map at {source_map_path}"}
    source_map = load_json(source_map_path)
    recorded = source_map.get("git_state")
    selection_status = source_map.get("selection_status", "agent-asserted")
    if policy is not None:
        from .policy import validate_policy

        errors = validate_policy(policy)
        if errors:
            return source_verdict("invalid", selection_status, "; ".join(errors))
        policy_paths = policy["source_policy"]["selected_paths"]
        if sorted(policy_paths) != sorted(source_map.get("selected_paths", [])):
            return source_verdict(
                "invalid",
                selection_status,
                "policy selected_paths do not match the source map",
            )
        try:
            report = source_policy_report(
                root, [root / path for path in policy_paths], policy
            )
        except (OSError, ValueError) as error:
            return source_verdict("invalid", selection_status, str(error))
        if report["errors"]:
            return source_verdict(
                "invalid", selection_status, "; ".join(report["errors"])
            ) | {"source_policy_report": report}
    if (
        isinstance(recorded, dict)
        and recorded.get("tracked_objects")
        and not recorded.get("dirty_sources")
    ):
        now = git_state(root, source_map.get("selected_paths", []))
        if now.get("tracked_objects") == recorded["tracked_objects"] and not now.get(
            "dirty_sources"
        ):
            return {
                "status": "current",
                "selection_status": selection_status,
                "changed_sources": [],
                "checked": "git",
            }
    return verify_source_map(root, source_map_path, policy) | {"checked": "hash"}
