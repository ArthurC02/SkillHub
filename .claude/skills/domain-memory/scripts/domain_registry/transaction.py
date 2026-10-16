from __future__ import annotations

import json
import os
import shutil
import stat
import sys
import uuid
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .common import (
    MAX_JSON_BYTES,
    registry_dir,
    reject_duplicate_keys,
    replace_path,
    write_json,
    writer_lock,
)
from .registry import validate
from .revision import registry_digest


def transaction_path(root: Path) -> Path:
    return root / ".domain-registry-transaction.json"


def read_journal_document(journal: Path) -> Any:
    if journal.stat().st_size > MAX_JSON_BYTES:
        raise ValueError("registry recovery journal is too large")
    return json.loads(
        journal.read_text(encoding="utf-8"), object_pairs_hook=reject_duplicate_keys
    )


def _clear_read_only_and_retry(function: Callable[[str], object], path: str, _error: object) -> None:
    writable = stat.S_IREAD | stat.S_IWRITE | stat.S_IEXEC
    os.chmod(os.path.dirname(path), writable)
    os.chmod(path, writable)
    function(path)


def remove_tree(path: Path) -> None:
    if sys.version_info >= (3, 12):
        shutil.rmtree(path, onexc=_clear_read_only_and_retry)
    else:
        shutil.rmtree(path, onerror=_clear_read_only_and_retry)


def discard_transaction(journal: Path, backup: Path, staging: Path) -> None:
    for leftover in (backup, staging):
        if leftover.exists():
            remove_tree(leftover)
    journal.unlink(missing_ok=True)


@dataclass(frozen=True)
class InterruptedTransaction:
    root: Path
    journal: Path
    operation_id: str | None
    backup: Path
    staging: Path

    def discard(self) -> None:
        discard_transaction(self.journal, self.backup, self.staging)


def _checked_journal(journal: Path) -> dict[str, Any]:
    value = read_journal_document(journal)
    if not isinstance(value, dict) or not all(
        isinstance(value.get(key), str) for key in ("backup", "staging")
    ):
        raise ValueError("registry recovery journal is malformed")
    if any(
        Path(value[key]).name != value[key]
        or not value[key].startswith(f".domain-registry-{kind}-")
        for key, kind in (("backup", "backup"), ("staging", "stage"))
    ):
        raise ValueError("registry recovery journal contains an unsafe path")
    return value


def _recover_legacy(transaction: InterruptedTransaction) -> None:
    target = registry_dir(transaction.root)
    if transaction.backup.exists():
        if target.exists():
            remove_tree(transaction.backup)
        else:
            replace_path(transaction.backup, target)
    transaction.discard()


def _was_audited(transaction: InterruptedTransaction) -> bool:
    from .audit import read_events

    events = read_events(transaction.root)
    return bool(events and events[-1].get("operation_id") == transaction.operation_id)


def _recover_installed(transaction: InterruptedTransaction) -> None:
    if _was_audited(transaction):
        transaction.discard()
        return
    if not transaction.backup.exists() or not transaction.staging.parent.exists():
        raise ValueError("registry transaction requires manual reconciliation")
    target = registry_dir(transaction.root)
    if target.exists():
        replace_path(target, registry_dir(transaction.staging))
    replace_path(transaction.backup, target)
    transaction.journal.unlink(missing_ok=True)


def _recover_prepared(transaction: InterruptedTransaction) -> None:
    target = registry_dir(transaction.root)
    if not target.exists() and transaction.backup.exists():
        replace_path(transaction.backup, target)
    transaction.discard()


def _require_reconciliation(_transaction: InterruptedTransaction) -> None:
    raise ValueError("registry transaction requires manual reconciliation")


RECOVERY_BY_PHASE: dict[str, Callable[[InterruptedTransaction], None]] = {
    "prepared": _recover_prepared,
    "installed": _recover_installed,
    "audited": InterruptedTransaction.discard,
    "reconciliation-required": _require_reconciliation,
}


def recover(root: Path) -> None:
    journal = transaction_path(root)
    if not journal.is_file():
        return
    value = _checked_journal(journal)
    transaction = InterruptedTransaction(
        root, journal, value.get("operation_id"), root / value["backup"], root / value["staging"]
    )
    if "phase" not in value or "operation_id" not in value:
        _recover_legacy(transaction)
        return
    handler = RECOVERY_BY_PHASE.get(value["phase"])
    if handler is None:
        raise ValueError("registry recovery journal has an invalid phase")
    handler(transaction)


def recover_interrupted_update(root: Path, force: bool) -> None:  # noqa: FBT001
    lock = root / ".domain-registry.lock"
    if lock.exists():
        if not force:
            raise ValueError(
                "registry update lock exists; confirm the writer stopped, then rerun recovery with --force"
            )
        lock.rmdir()
    recover(root)


def mutate_registry(
    root: Path,
    repo_root: Path | None,
    mutate: Callable[[Path], None],
    expected_digest: str | None = None,
    audit_event: Callable[[], dict[str, Any]] | None = None,
) -> None:
    with writer_lock(root):
        recover(root)
        if expected_digest is not None and registry_digest(root) != expected_digest:
            raise ValueError(
                "registry revision changed before update; rebase and obtain fresh approval"
            )
        operation = uuid.uuid4().hex
        staging = root / f".domain-registry-stage-{operation}"
        backup = root / f".domain-registry-backup-{operation}"
        staging_registry = registry_dir(staging)
        try:
            already = set(validate(root, repo_root, require_reviewed=False))
            shutil.copytree(registry_dir(root), staging_registry)
            mutate(staging)
            errors = [error for error in validate(staging, repo_root, require_reviewed=False) if error not in already]
            if errors:
                raise ValueError("registry update is invalid: " + "; ".join(errors))
            journal = transaction_path(root)
            write_json(
                journal,
                {
                    "format": "domain-registry-transaction/v2",
                    "operation_id": operation,
                    "phase": "prepared",
                    "staging": staging.name,
                    "backup": backup.name,
                    "expected_digest": expected_digest,
                },
            )
            try:
                replace_path(registry_dir(root), backup)
            except OSError as error:
                journal.unlink(missing_ok=True)
                raise held_open(root, error) from error
            _install(staging_registry, root, backup, journal)
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
                    replace_path(registry_dir(root), staging_registry)
                    replace_path(backup, registry_dir(root))
                    journal.unlink(missing_ok=True)
                    raise
                journal_value["phase"] = "audited"
                write_json(journal, journal_value)
            remove_tree(backup)
            remove_tree(staging)
            journal.unlink(missing_ok=True)
        except Exception:
            if staging.exists():
                remove_tree(staging)
            raise


def _install(staging_registry: Path, root: Path, backup: Path, journal: Path) -> None:
    try:
        replace_path(staging_registry, registry_dir(root))
    except OSError as error:
        replace_path(backup, registry_dir(root))
        journal.unlink(missing_ok=True)
        raise held_open(root, error) from error


def held_open(root: Path, error: OSError) -> ValueError:
    code = getattr(error, "winerror", None) or error.errno
    return ValueError(
        f"another process is holding {registry_dir(root)} or a file in it open (error {code}), so the update "
        "could not be swapped; nothing was changed. Close it and rerun."
    )


def load_journal(path: Path) -> dict[str, Any]:
    value = read_journal_document(path)
    if (
        not isinstance(value, dict)
        or value.get("format") != "domain-registry-transaction/v2"
    ):
        raise ValueError("registry recovery journal is malformed")
    return value
