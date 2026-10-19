from __future__ import annotations

import shlex
import subprocess
import sys
from pathlib import Path
from typing import Any

from .changes import init_change_package, parsed_update
from .common import OBLIGATIONS_FILE, PROPOSAL_FILE, REQUIREMENT_FILE, load_json, shown_path, write_json
from .evidence import all_references, with_references
from .okf import contexts_named_by
from .git import toplevel
from .registry import asset_records

CITATIONS_CURRENT = "citations-current"


def recitation_package(
    output: Path, registry_root: Path, recitation: dict[str, Any], identifiers: tuple[str, str, str]
) -> list[str]:
    updates = _evidence_only_updates(registry_root, recitation)
    contexts = sorted({context for asset, record in updates for context in contexts_named_by(asset, record)})
    cited = sorted({
        reference["path"] for _, record in updates for reference in all_references(record)
        if isinstance(reference, dict) and isinstance(reference.get("path"), str)
    })
    requirement_id, proposal_id, proposer = identifiers
    names = ", ".join(f"{asset}/{record['id']}" for asset, record in updates)
    init_change_package(output, requirement_id, proposal_id, proposer)
    _fill(output / REQUIREMENT_FILE, {
        "intent": f"Re-cite {names} "
        "after the code they cite moved or changed; the facts they state are unchanged.",
        "actor": proposer,
        "observable_outcome": "verify-evidence reports every citation of the updated records current.",
        "acceptance_criteria": [{
            "id": CITATIONS_CURRENT,
            "statement": "Every citation in the updated records matches the lines it cites.",
        }],
        "sources": cited,
    })
    _fill(output / OBLIGATIONS_FILE, {"obligations": [{
        "id": "evidence-current",
        "source_type": "acceptance-criterion",
        "source_id": CITATIONS_CURRENT,
        "expected_outcome": "verify-evidence reports current for every citation of the updated records.",
        "level": "architecture",
        "status": "planned",
        "check": _verify_command(registry_root, output, cited),
    }]})
    _fill(output / PROPOSAL_FILE, {
        "change_classification": "routine",
        "affected_contexts": contexts,
        "implementation_design": None,
        "registry_updates": recitation["registry_updates"],
        "supersedes": recitation.get("supersedes", []),
    })
    return [] if contexts else ["affected_contexts: none of the updated records names a Context"]


def _fill(path: Path, values: dict[str, Any]) -> None:
    write_json(path, load_json(path) | values)


def _evidence_only_updates(registry_root: Path, recitation: dict[str, Any]) -> list[tuple[str, dict[str, Any]]]:
    if recitation.get("candidates"):
        raise ValueError("the recitation carries candidates; they go to upsert-candidate, so leave them out")
    if recitation.get("unmatched"):
        raise ValueError(
            "the recitation still has unmatched citations; re-cite them with recite --onto before packaging it"
        )
    raw_updates = recitation.get("registry_updates")
    if not isinstance(raw_updates, list) or not raw_updates:
        raise ValueError("the recitation has no registry_updates to package")
    updates = []
    for raw in raw_updates:
        update = parsed_update(raw)
        if update.record is None:
            raise ValueError(f"{update.asset}/{update.record_id} is removed, which is more than a re-citation")
        current = next(
            (entry for entry in asset_records(registry_root, update.document_name) if entry.get("id") == update.record_id),
            {},
        )
        fields = _changed_fields(current, update.record)
        if fields:
            added = len(all_references(update.record)) - len(all_references(current))
            raise ValueError(
                f"{update.asset}/{update.record_id} {_beyond_recitation(fields, added)}; "
                "write that Change Package with init-change-package instead"
            )
        updates.append((update.asset, update.record))
    return updates


def _changed_fields(current: dict[str, Any], record: dict[str, Any]) -> list[str]:
    before, after = _without_evidence(current), _without_evidence(record)
    return sorted(key for key in before.keys() | after.keys() if key not in before or key not in after or before[key] != after[key])


def _beyond_recitation(fields: list[str], added: int) -> str:
    if fields == ["evidence"] and added:
        how = f"adds {added}" if added > 0 else f"drops {-added}"
        return (f"{how} citation(s) and changes nothing else; a re-citation package only moves the citations a record "
                "already has, so adding or dropping one is a change of claim")
    return "changes more than its citations: " + ", ".join(fields)


def _without_evidence(record: dict[str, Any]) -> Any:
    return with_references({key: value for key, value in record.items() if key not in ("review", "verified")}, lambda _: None)


def _verify_command(registry_root: Path, package_root: Path, cited: list[str]) -> str:
    top = _repository_top(registry_root)
    script = Path(__file__).resolve().parents[1] / "registry_tools.py"
    arguments = [
        Path(sys.executable).stem, shown_path(script, top), "verify-evidence",
        "--registry-root", shown_path(registry_root, top), "--repo-root", ".",
        "--package-root", shown_path(package_root, top),
        *(argument for path in cited for argument in ("--path", path)),
    ]
    return shlex.join(arguments)


def _repository_top(path: Path) -> Path | None:
    try:
        return toplevel(path)
    except (OSError, subprocess.TimeoutExpired):
        return None
