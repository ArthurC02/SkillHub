from __future__ import annotations

from .common import registry_dir
from .revision import registry_digest
from .registry import validate

import json
import shutil
import uuid
from contextlib import contextmanager
from pathlib import Path
from typing import Callable


def write_json(path: Path, value: dict) -> None:
    temporary = path.with_suffix(f"{path.suffix}.tmp")
    temporary.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
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
    if not target.exists() and backup.exists():
        backup.replace(target)
    elif target.exists() and backup.exists():
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


@contextmanager
def writer_lock(root: Path):
    lock = root / ".domain-registry.lock"
    try:
        lock.mkdir()
    except FileExistsError as error:
        raise ValueError(f"another Domain Registry update is in progress: {lock}") from error
    try:
        yield
    finally:
        lock.rmdir()


def mutate_registry(root: Path, repo_root: Path | None, mutate: Callable[[Path], None], expected_digest: str | None = None) -> None:
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
            write_json(journal, {"staging": staging.name, "backup": backup.name})
            registry_dir(root).replace(backup)
            staging_registry.replace(registry_dir(root))
            shutil.rmtree(backup)
            shutil.rmtree(staging)
            journal.unlink(missing_ok=True)
        except Exception:
            if staging.exists():
                shutil.rmtree(staging)
            raise
