from __future__ import annotations

from .common import MAX_JSON_BYTES, registry_dir, writer_lock
from .revision import registry_digest
from .registry import validate

import json
import os
import shutil
import uuid
from pathlib import Path
from typing import Any, Callable


def write_json(path: Path, value: dict) -> None:
    temporary = path.with_suffix(f"{path.suffix}.tmp")
    with temporary.open("w", encoding="utf-8") as output:
        output.write(json.dumps(value, indent=2, ensure_ascii=False) + "\n")
        output.flush()
        os.fsync(output.fileno())
    temporary.replace(path)


def transaction_path(root: Path) -> Path:
    return root / ".domain-registry-transaction.json"


def recover(root: Path) -> None:
    journal = transaction_path(root)
    if not journal.is_file():
        return
    value = json.loads(journal.read_text(encoding="utf-8"))
    if not isinstance(value, dict) or not all(isinstance(value.get(key), str) for key in ("backup", "staging")):
        raise ValueError("registry recovery journal is malformed")
    if any(Path(value[key]).name != value[key] or not value[key].startswith(f".domain-registry-{kind}-") for key, kind in (("backup", "backup"), ("staging", "stage"))):
        raise ValueError("registry recovery journal contains an unsafe path")
    backup = root / value["backup"]
    staging = root / value["staging"]
    target = registry_dir(root)
    legacy = "phase" not in value or "operation_id" not in value
    if legacy:
        if not target.exists() and backup.exists():
            backup.replace(target)
        elif target.exists() and backup.exists():
            shutil.rmtree(backup)
        if staging.exists():
            shutil.rmtree(staging)
        journal.unlink(missing_ok=True)
        return
    if value["phase"] not in {"prepared", "installed", "audited", "reconciliation-required"}:
        raise ValueError("registry recovery journal has an invalid phase")
    if value["phase"] == "prepared":
        if backup.exists():
            shutil.rmtree(backup)
        if staging.exists():
            shutil.rmtree(staging)
        journal.unlink(missing_ok=True)
        return
    if value["phase"] == "installed":
        from .audit import read_events
        events = read_events(root)
        audited = bool(events and events[-1].get("operation_id") == value["operation_id"])
        if not audited and backup.exists() and staging.parent.exists():
            staging_registry = registry_dir(staging)
            if target.exists():
                target.replace(staging_registry)
            backup.replace(target)
        value["phase"] = "audited" if audited else "committed"
        write_json(journal, value)
    if value["phase"] == "reconciliation-required":
        raise ValueError("registry transaction requires manual reconciliation")
    if backup.exists():
        shutil.rmtree(backup)
    if staging.exists():
        shutil.rmtree(staging)
    journal.unlink(missing_ok=True)


def recover_interrupted_update(root: Path, force: bool) -> None:
    lock = root / ".domain-registry.lock"
    if lock.exists():
        if not force:
            raise ValueError("registry update lock exists; confirm the writer stopped, then rerun recovery with --force")
        lock.rmdir()
    recover(root)


def mutate_registry(root: Path, repo_root: Path | None, mutate: Callable[[Path], None], expected_digest: str | None = None, audit_event: Callable[[], dict[str, Any]] | None = None) -> None:
    with writer_lock(root):
        recover(root)
        if expected_digest is not None and registry_digest(root) != expected_digest:
            raise ValueError("registry revision changed before update; rebase and obtain fresh approval")
        operation = uuid.uuid4().hex
        staging = root / f".domain-registry-stage-{operation}"
        backup = root / f".domain-registry-backup-{operation}"
        staging_registry = registry_dir(staging)
        try:
            shutil.copytree(registry_dir(root), staging_registry)
            mutate(staging)
            errors = validate(staging, repo_root, False)
            if errors:
                raise ValueError("registry update is invalid: " + "; ".join(errors))
            journal = transaction_path(root)
            write_json(journal, {
                "format": "domain-registry-transaction/v2",
                "operation_id": operation,
                "phase": "prepared",
                "staging": staging.name,
                "backup": backup.name,
                "expected_digest": expected_digest,
            })
            registry_dir(root).replace(backup)
            staging_registry.replace(registry_dir(root))
            journal_value = load_journal(journal)
            journal_value["phase"] = "installed"
            write_json(journal, journal_value)
            if audit_event is not None:
                try:
                    from .audit import append_locked
                    event = audit_event()
                    event["operation_id"] = operation
                    append_locked(root, event)
                except Exception:
                    registry_dir(root).replace(staging_registry)
                    backup.replace(registry_dir(root))
                    journal.unlink(missing_ok=True)
                    raise
                journal_value["phase"] = "audited"
                write_json(journal, journal_value)
            shutil.rmtree(backup)
            shutil.rmtree(staging)
            journal.unlink(missing_ok=True)
        except Exception:
            if staging.exists():
                shutil.rmtree(staging)
            raise


def load_journal(path: Path) -> dict[str, Any]:
    if path.stat().st_size > MAX_JSON_BYTES:
        raise ValueError("registry recovery journal is too large")
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict) or value.get("format") != "domain-registry-transaction/v2":
        raise ValueError("registry recovery journal is malformed")
    return value
