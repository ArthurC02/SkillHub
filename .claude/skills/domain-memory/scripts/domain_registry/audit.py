from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
from typing import Any

from .common import (
    APPLY_OPERATION,
    MAX_JSON_BYTES,
    load_json,
    readable_packages,
    reject_duplicate_keys,
    utc_now,
    write_json,
    writer_lock,
)
from .git import resolve_commit, run_git, toplevel
from .store import has_registry

MANIFEST_FORMAT = "domain-memory-audit/v1"
RECONCILE_OPERATION = "reconcile-audit"
CONFLICT_MARKERS = ("<<<<<<< ", "=======", ">>>>>>> ", "||||||| ")


def canonical(value: dict[str, Any]) -> bytes:
    return json.dumps(
        value, ensure_ascii=False, sort_keys=True, separators=(",", ":")
    ).encode("utf-8")


def event_digest(value: dict[str, Any]) -> str:
    return "sha256:" + hashlib.sha256(canonical(value)).hexdigest()


def events_path(root: Path) -> Path:
    return root / "audit" / "events.jsonl"


def audit_manifest_path(root: Path) -> Path:
    return root / "audit" / "manifest.json"


def read_events(root: Path) -> list[dict[str, Any]]:
    return [event for _, event in _parsed_events(_event_lines(root))]


def _event_lines(root: Path) -> list[str]:
    path = events_path(root)
    if not path.is_file():
        return []
    if path.stat().st_size > MAX_JSON_BYTES:
        raise ValueError(f"audit log exceeds {MAX_JSON_BYTES} bytes: {path}")
    return path.read_text(encoding="utf-8").splitlines()


def _parsed_events(lines: list[str]) -> list[tuple[int, dict[str, Any]]]:
    events = []
    for index, line in enumerate(lines, start=1):
        try:
            value = json.loads(line, object_pairs_hook=reject_duplicate_keys)
        except (json.JSONDecodeError, ValueError) as error:
            raise ValueError(f"audit event {index} is invalid JSON") from error
        if not isinstance(value, dict):
            raise ValueError(f"audit event {index} must be an object")
        events.append((index, value))
    return events


def parents_of(event: dict[str, Any]) -> list[str]:
    previous = event.get("previous_event_sha256")
    if previous is None:
        return []
    return list(previous) if isinstance(previous, list) else [previous]


def review_history(root: Path) -> dict[str, list[str]]:
    history: dict[str, list[str]] = {}
    for event in read_events(root):
        if event.get("operation") == APPLY_OPERATION:
            reviewed = dict.fromkeys(event.get("records", []), event.get("proposal_id"))
        elif event.get("operation") == "migrate-registry":
            reviewed = event.get("reviewed_by", {})
        else:
            continue
        for record, proposal_id in reviewed.items():
            history.setdefault(record, []).append(proposal_id)
    return history


def reviewing_proposals(root: Path) -> dict[str, str]:
    return {record: proposals[-1] for record, proposals in review_history(root).items()}


def append_locked(root: Path, event: dict[str, Any]) -> None:
    reserved = {
        "sequence",
        "previous_event_sha256",
        "recorded_at",
        "event_sha256",
    }
    if reserved.intersection(event):
        raise ValueError("audit event contains reserved fields")
    directory = root / "audit"
    directory.mkdir(exist_ok=True)
    existing = verify(root)
    if existing["status"] != "valid":
        raise ValueError(f"audit log is invalid: {existing['reason']}")
    heads = existing["heads"]
    value = {
        "sequence": existing["height"] + 1,
        "previous_event_sha256": None if not heads else heads[0] if len(heads) == 1 else heads,
        "recorded_at": utc_now(),
        **event,
    }
    value["event_sha256"] = event_digest(value)
    with events_path(root).open("a", encoding="utf-8", newline="\n") as output:
        output.write(json.dumps(value, ensure_ascii=False, sort_keys=True) + "\n")
        output.flush()
        os.fsync(output.fileno())
    _write_manifest(root, existing["events"] + 1, [value["event_sha256"]])


def _write_manifest(root: Path, events: int, heads: list[str]) -> None:
    write_json(audit_manifest_path(root), {
        "format": MANIFEST_FORMAT,
        "events": events,
        "head_sha256": heads[0] if len(heads) == 1 else None,
        "heads": heads,
    })


def _invalid(reason: str) -> dict[str, Any]:
    return {"status": "invalid", "reason": reason}


def _dag_heads(events: list[dict[str, Any]]) -> tuple[list[str], int]:
    seen: dict[str, dict[str, Any]] = {}
    for position, event in enumerate(events, start=1):
        unsigned = {key: value for key, value in event.items() if key != "event_sha256"}
        digest = event.get("event_sha256")
        if digest in seen:
            return _raise(f"audit event {position} repeats event {seen[digest]['position']}")
        parents = [seen[parent] for parent in parents_of(event) if parent in seen]
        if len(parents) != len(parents_of(event)):
            return _raise(f"audit chain is broken at event {position}")
        if event.get("sequence") != 1 + max((parent["sequence"] for parent in parents), default=0):
            return _raise(f"audit sequence is invalid at event {position}")
        if digest != event_digest(unsigned):
            return _raise(f"audit digest is invalid at event {position}")
        seen[digest] = {"position": position, "sequence": event["sequence"]}
    referenced = {parent for event in events for parent in parents_of(event)}
    heads = sorted(digest for digest in seen if digest not in referenced)
    return heads, max((entry["sequence"] for entry in seen.values()), default=0)


def _raise(reason: str) -> Any:
    raise ValueError(reason)


def _manifest_error(root: Path, events: int, heads: list[str]) -> str | None:
    manifest_path = audit_manifest_path(root)
    if not manifest_path.is_file():
        return None
    manifest = load_json(manifest_path)
    if (
        manifest.get("format") != MANIFEST_FORMAT
        or manifest.get("events") != events
        or manifest.get("head_sha256") != (heads[0] if len(heads) == 1 else None)
        or manifest.get("heads", heads) != heads
    ):
        return "audit manifest does not match the event log"
    return None


def verify(root: Path) -> dict[str, Any]:
    if not has_registry(root):
        return _invalid(
            f"no Domain Memory Registry at {root}; point --registry-root at the directory "
            "that holds registry/manifest.json"
        )
    try:
        events = read_events(root)
        heads, height = _dag_heads(events)
        error = _manifest_error(root, len(events), heads)
    except ValueError as failure:
        return _invalid(str(failure))
    if error:
        return _invalid(error)
    return {
        "status": "valid", "events": len(events), "height": height,
        "head_sha256": heads[0] if len(heads) == 1 else None, "heads": heads,
    }


def reconcile(root: Path) -> dict[str, Any]:
    if not has_registry(root):
        raise ValueError(
            f"no Domain Memory Registry at {root}; point --registry-root at the directory "
            "that holds registry/manifest.json"
        )
    with writer_lock(root):
        lines = _event_lines(root)
        kept = [line for line in lines if not line.startswith(CONFLICT_MARKERS)]
        events: dict[str, dict[str, Any]] = {}
        for _, event in _parsed_events(kept):
            events.setdefault(str(event.get("event_sha256")), event)
        ordered = sorted(events.values(), key=lambda event: event.get("sequence", 0) if isinstance(event.get("sequence"), int) else 0)
        heads, _ = _dag_heads(ordered)
        text = "".join(json.dumps(event, ensure_ascii=False, sort_keys=True) + "\n" for event in ordered)
        rewritten = text != "".join(line + "\n" for line in lines)
        if rewritten:
            events_path(root).write_text(text, encoding="utf-8", newline="\n")
        manifest_path = audit_manifest_path(root)
        before = manifest_path.read_bytes() if manifest_path.is_file() else None
        _write_manifest(root, len(ordered), heads)
        manifest_rewritten = manifest_path.read_bytes() != before
        joined = heads if len(heads) > 1 else []
        if joined:
            append_locked(root, {"operation": RECONCILE_OPERATION, "joined": joined})
    return {
        "events": len(ordered) + bool(joined),
        "conflict_markers_removed": len(lines) - len(kept),
        "duplicates_dropped": len(kept) - len(ordered),
        "joined": joined,
        "written": rewritten or manifest_rewritten or bool(joined),
    }


def anchor_error(root: Path, changes_root: Path | None, against: str | None) -> str | None:
    if changes_root is not None:
        missing = _applied_without_event(root, changes_root)
        if missing:
            return f"applied proposals have no {APPLY_OPERATION} event in the audit log: " + ", ".join(missing)
    return _committed_prefix_error(root, against) if against is not None else None


def _applied_without_event(root: Path, changes_root: Path) -> list[str]:
    applied = {
        event.get("proposal_id") for event in read_events(root) if event.get("operation") == APPLY_OPERATION
    }
    missing = []
    for package, proposal in readable_packages(changes_root):
        was_applied = "applied" in (proposal.get("status"), proposal.get("superseded_from_status"))
        if was_applied and proposal.get("proposal_id") not in applied:
            missing.append(f"{package.name} ({proposal.get('proposal_id')})")
    return missing


def _committed_prefix_error(root: Path, against: str) -> str | None:
    shown = toplevel(root)
    if shown is None or resolve_commit(root, against) is None:
        raise ValueError(f"git cannot resolve {against} from {root}")
    manifest_path = audit_manifest_path(root).resolve().relative_to(shown).as_posix()
    committed = run_git(shown, "show", f"{against}:{manifest_path}")
    if committed.returncode:
        return f"no audit manifest committed at {against}:{manifest_path}, so nothing anchors the log"
    try:
        manifest = json.loads(committed.stdout)
    except json.JSONDecodeError:
        return f"the audit manifest committed at {against} is not valid JSON"
    count, head = manifest.get("events"), manifest.get("head_sha256")
    heads = manifest.get("heads") if isinstance(manifest.get("heads"), list) else [head] if head else []
    present = {event.get("event_sha256") for event in read_events(root)}
    if not isinstance(count, int) or count > len(present) or any(digest not in present for digest in heads):
        return (
            f"the audit log does not extend the one committed at {against} ({count} events, head {head}); "
            "events were removed or rewritten since"
        )
    return None
