from __future__ import annotations

from pathlib import Path
from typing import Any

from .dependents import dependents
from .impact import boundary_impact, touched_records
from .registry import cited_paths, context_model, context_summary, record_by_id, summarized, verify_evidence
from .store import read_document

FORMAT = "domain-memory-brief/v1"
FAILING_EVIDENCE = ("stale", "unhashed", "missing", "invalid")
CHANGED_CITATION = ("stale", "missing", "invalid")
VERDICTS = {
    "unregistered-co-change": "Contexts changed together with no registered collaboration between them; register the interaction, "
                              "contract or dependency policy, or split the change.",
    "boundary-evidence-changed": "A collaboration's cited lines changed; re-read each record below, and recite only what the code still states.",
    "registered-co-change": "Contexts changed together through registered collaborations; check that each one below still holds.",
    "no-cross-context-change": "The change stays inside one Context.",
    "unknown": "No Context records an implementation_path, so the changed files cannot be mapped to Contexts.",
}
CONTEXT_FIELDS = ("owner_context", "producer_context", "consumer_context", "from_context", "to_context")
CONTEXT_LISTS = ("contexts", "consumer_contexts", "consumers")


def brief_for_change(root: Path, repo_root: Path, base: str) -> dict[str, Any]:
    impact = boundary_impact(root, repo_root, base)
    touched: dict[tuple[str, Any], dict[str, Any]] = {}
    for entry in [*touched_records(root, repo_root, base), *impact["boundary_evidence_touched"]]:
        touched.setdefault((entry["asset"], entry["id"]), {})["touched_evidence"] = entry["touched_evidence"]
    for entry in impact["boundary_evidence_changed"]:
        touched.setdefault((entry["asset"], entry["id"]), {})["changed_evidence"] = entry["changed_evidence"]
    changed_paths = sorted({path for files in impact["contexts_touched"].values() for path in files} | set(impact["outside_every_context"]))
    evidence = verify_evidence(root, repo_root, changed_paths) if changed_paths else None
    for item in (evidence or {}).get("results", []):
        if item["status"] in CHANGED_CITATION:
            changed = touched.setdefault((item["asset"], item["id"]), {}).setdefault("changed_evidence", [])
            changed[:] = sorted({*changed, item["path"]})
    brief = {
        "format": FORMAT,
        "subject": {"base": base, "changed_files": impact["changed_files"]},
        "verdict": impact["verdict"],
        "contexts": [_context(root, identifier, files) for identifier, files in impact["contexts_touched"].items()],
        "records": [_record(root, asset, record_id, notes) for (asset, record_id), notes in sorted(touched.items(), key=str)],
        "evidence": _evidence(evidence),
        "outside_every_context": impact["cited_outside_contexts"],
    }
    brief["attention"] = _change_attention(brief)
    return brief


def brief_for_record(root: Path, repo_root: Path, asset: str, record_id: str) -> dict[str, Any]:
    record = record_by_id(root, asset, record_id)
    if record is None:
        raise ValueError(f"{asset} record not found: {record_id}")
    paths = sorted(cited_paths(record))
    brief = {
        "format": FORMAT,
        "subject": {"asset": asset, "id": record_id},
        "record": {**_summarized(root, asset, record), "asset": asset},
        "contexts": [_context(root, identifier, []) for identifier in _contexts_of(record)],
        "dependents": _with_status(root, dependents(root, asset, record_id)["reached"]),
        "evidence": _evidence(verify_evidence(root, repo_root, paths) if paths else None),
    }
    brief["attention"] = _record_attention(brief)
    return brief


def to_markdown(brief: dict[str, Any]) -> str:
    subject = brief["subject"]
    title = f"since {subject['base']}" if "base" in subject else f"{subject['asset']}/{subject['id']}"
    lines = [f"# Change brief: {title}", ""]
    if "verdict" in brief:
        lines += [f"**Verdict:** `{brief['verdict']}`", ""]
    if "record" in brief:
        lines += [f"**Record:** {_line(brief['record'])}", ""]
    lines += ["## Attention", "", *(f"- {item}" for item in brief["attention"]), ""]
    if brief["contexts"]:
        lines += ["## Contexts", ""]
        for context in brief["contexts"]:
            lines += _context_lines(context)
    if brief.get("records"):
        lines += ["## Records to re-read", ""]
        for record in brief["records"]:
            lines += _record_lines(record)
    if brief.get("dependents") is not None:
        lines += ["## Dependents", "", *(_dependent_lines(brief["dependents"])), ""]
    lines += ["## Evidence", "", *_evidence_lines(brief["evidence"]), ""]
    return "\n".join(lines)


def _context(root: Path, identifier: str, files: list[str]) -> dict[str, Any]:
    model = context_model(root, identifier)
    if model is None:
        return {"id": identifier, "changed_files": files, "model": None}
    summary = context_summary(model)
    summary["context"] = {**summary["context"], **_summarized(root, "contexts", model["context"])}
    return {"id": identifier, "changed_files": files, "model": summary}


def _record(root: Path, asset: str, record_id: Any, notes: dict[str, Any]) -> dict[str, Any]:
    record = record_by_id(root, asset, record_id)
    shown = {**_summarized(root, asset, record), "asset": asset, **notes} if record is not None else {"id": record_id, "asset": asset, **notes}
    shown["dependents"] = _with_status(root, dependents(root, asset, record_id)["reached"]) if record is not None else []
    return shown


def _summarized(root: Path, asset: str, record: dict[str, Any]) -> dict[str, Any]:
    shown = summarized(record)
    if shown["status"] is None:
        shown["status"] = read_document(root, f"{asset}.json").get("status")
    return shown


def _with_status(root: Path, reached: list[dict[str, Any]]) -> list[dict[str, Any]]:
    defaults = {item["asset"]: read_document(root, f"{item['asset']}.json").get("status") for item in reached if item["status"] is None}
    return [{**item, "status": defaults[item["asset"]]} if item["status"] is None else item for item in reached]


def _contexts_of(record: dict[str, Any]) -> list[str]:
    found = [record[name] for name in CONTEXT_FIELDS if isinstance(record.get(name), str)]
    found += [item for name in CONTEXT_LISTS for item in (record.get(name) or []) if isinstance(item, str)]
    return sorted(set(found))


def _evidence(report: dict[str, Any] | None) -> dict[str, Any] | None:
    if report is None:
        return None
    return {"summary": {key: value for key, value in report["summary"].items() if key != "by_source_kind"}, "paths": report.get("paths", {})}


def _failing(evidence: dict[str, Any] | None) -> list[str]:
    if evidence is None:
        return []
    return [f"{evidence['summary'][status]} {status}" for status in FAILING_EVIDENCE if evidence["summary"].get(status)]


def _change_attention(brief: dict[str, Any]) -> list[str]:
    attention = [VERDICTS[brief["verdict"]]]
    for record in brief["records"]:
        name = f"{record['asset']}/{record['id']}"
        if "changed_evidence" in record:
            attention.append(
                f"{name} cites {', '.join(record['changed_evidence'])} at lines that changed; check that its statement still holds "
                f"before reciting: {record.get('summary')}"
            )
        elif "touched_evidence" in record:
            attention.append(f"{name} cites {', '.join(record['touched_evidence'])}, which the change overlaps; re-read it.")
    attention += [
        f"{entry['path']} lies outside every Context but is cited by {', '.join(entry['cited_by'])}; place it or move the citation."
        for entry in brief["outside_every_context"]
    ]
    rules = [record for record in brief["records"] if record["asset"] == "rules"]
    if rules:
        attention.append(f"{len(rules)} rules cite lines this change touched; re-run their checks, which get-record names.")
    if failing := _failing(brief["evidence"]):
        attention.append(f"{', '.join(failing)} citations of the changed files; recite only a record whose statement still holds.")
    return attention


def _record_attention(brief: dict[str, Any]) -> list[str]:
    record = brief["record"]
    attention = []
    if record["status"] != "reviewed":
        attention.append(f"The record is {record['status']}, not a reviewed constraint.")
    if brief["dependents"]:
        attention.append(f"{len(brief['dependents'])} records reach it through relations and may need the same change; they are listed.")
    if brief["evidence"] is None:
        attention.append("The record cites nothing; it cannot be verified against the code.")
    elif failing := _failing(brief["evidence"]):
        attention.append(f"{', '.join(failing)} citations; recite them before relying on the record.")
    return attention or ["Nothing stands out; the record is reviewed, cited and verified."]


def _line(entry: dict[str, Any]) -> str:
    summary = f": {entry['summary']}" if entry.get("summary") else ""
    return f"{entry.get('asset', '')}/{entry['id']} ({entry.get('status')}){summary}".lstrip("/")


def _context_lines(context: dict[str, Any]) -> list[str]:
    model = context["model"]
    heading = f"### {context['id']}" if model is None else f"### {_line(model['context'])}"
    lines = [heading, ""]
    if context["changed_files"]:
        lines += [f"Changed: {', '.join(context['changed_files'])}", ""]
    if model is None:
        return [*lines, "Not in the Registry.", ""]
    for key in ("rules", "interactions", "contracts", "events"):
        if model.get(key):
            lines += [f"{key}:", *(f"- {_line(entry)}" for entry in model[key]), ""]
    return lines


def _record_lines(record: dict[str, Any]) -> list[str]:
    lines = [f"### {_line(record)}", ""]
    for key in ("changed_evidence", "touched_evidence"):
        if key in record:
            lines += [f"{key.replace('_', ' ')}: {', '.join(record[key])}", ""]
    return [*lines, "Dependents:", *_dependent_lines(record["dependents"]), ""]


def _dependent_lines(reached: list[dict[str, Any]]) -> list[str]:
    return [f"- {_line(item)} via {item['through']} at depth {item['depth']}" for item in reached] or ["- none"]


def _evidence_lines(evidence: dict[str, Any] | None) -> list[str]:
    if evidence is None:
        return ["No citations to verify."]
    return [f"- {status}: {count}" for status, count in evidence["summary"].items() if count]
