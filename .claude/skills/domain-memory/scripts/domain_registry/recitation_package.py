from __future__ import annotations

from pathlib import Path
from typing import Any

from .changes import init_change_package, parsed_update
from .common import PROPOSAL_FILE
from .common import load_json, write_json
from .evidence import all_references, with_references
from .registry import asset_records

CONTEXT_FIELDS = ("context", "owner_context", "producer_context", "consumer_context", "from_context", "to_context")
CONTEXT_LIST_FIELDS = ("contexts", "consumer_contexts")
CITATIONS_CURRENT = "citations-current"


def recitation_package(
    output: Path, registry_root: Path, recitation: dict[str, Any], identifiers: tuple[str, str, str]
) -> list[str]:
    updates = _evidence_only_updates(registry_root, recitation)
    contexts = sorted({context for asset, record in updates for context in _contexts_named_by(asset, record)})
    cited = sorted({
        reference["path"] for _, record in updates for reference in all_references(record)
        if isinstance(reference, dict) and isinstance(reference.get("path"), str)
    })
    requirement_id, proposal_id, proposer = identifiers
    names = ", ".join(f"{asset}/{record['id']}" for asset, record in updates)
    init_change_package(output, requirement_id, proposal_id, proposer)
    _fill(output / "requirement-normalization.json", {
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
    _fill(output / "test-obligations.json", {"obligations": [{
        "id": "evidence-current",
        "source_type": "acceptance-criterion",
        "source_id": CITATIONS_CURRENT,
        "expected_outcome": "verify-evidence reports current for every citation of the updated records.",
        "level": "architecture",
        "status": "planned",
        "check": "registry_tools.py verify-evidence " + " ".join(f"--path {path}" for path in cited),
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
        if _without_evidence(current) != _without_evidence(update.record):
            raise ValueError(
                f"{update.asset}/{update.record_id} changes more than its citations; "
                "write that Change Package with init-change-package instead"
            )
        updates.append((update.asset, update.record))
    return updates


def _without_evidence(record: dict[str, Any]) -> Any:
    return with_references({key: value for key, value in record.items() if key not in ("review", "verified")}, lambda _: None)


def _contexts_named_by(asset: str, record: dict[str, Any]) -> set[str]:
    named = {record.get(field) for field in CONTEXT_FIELDS}
    for field in CONTEXT_LIST_FIELDS:
        value = record.get(field)
        named.update(value if isinstance(value, list) else [])
    if asset == "contexts":
        named.add(record.get("id"))
    return {context for context in named if isinstance(context, str)}
