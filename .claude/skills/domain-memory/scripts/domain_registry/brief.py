from __future__ import annotations

from collections import Counter
from collections.abc import Callable
from functools import cache
from pathlib import Path
from typing import Any

from .audit import review_history, reviewing_proposals
from .changes import packages_root, proof_of
from .common import ASSET_KEYS, FAILING_EVIDENCE, readable_packages
from .dependents import DependencyGraph
from .evidence import source_map_for
from .impact import boundary_impact, changed_files_since, touched_records
from .registry import asset_records, cited_paths, context_model, context_summary, record_by_id, summarized, verify_evidence
from .revision import committed_json
from .store import committed_documents, read_document

FORMAT = "domain-memory-brief/v1"
PROOF_KIND = "tests"
CONFIRMED = "developer-confirmed"
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
    changes = changed_files_since(repo_root, base)
    impact = boundary_impact(root, repo_root, base, changes)
    touched: dict[tuple[str, Any], dict[str, Any]] = {}
    for entry in [*touched_records(root, repo_root, base, changes), *impact["boundary_evidence_touched"]]:
        touched.setdefault((entry["asset"], entry["id"]), {})["touched_evidence"] = entry["touched_evidence"]
    for entry in impact["boundary_evidence_changed"]:
        touched.setdefault((entry["asset"], entry["id"]), {})["changed_evidence"] = entry["changed_evidence"]
    changed_paths = sorted({path for files in impact["contexts_touched"].values() for path in files} | set(impact["outside_every_context"]))
    evidence = verify_evidence(root, repo_root, changed_paths) if changed_paths else None
    for item in (evidence or {}).get("results", []):
        if item["status"] in CHANGED_CITATION:
            changed = touched.setdefault((item["asset"], item["id"]), {}).setdefault("changed_evidence", [])
            changed[:] = sorted({*changed, item["path"]})
    records = _records(root)
    graph, statuses = DependencyGraph.from_documents(records), _default_statuses(root)
    history, changes_root = review_history(root), packages_root(root, None)
    packages = cache(lambda: readable_packages(changes_root))
    brief = {
        "format": FORMAT,
        "subject": {"base": base, "changed_files": impact["changed_files"]},
        "verdict": impact["verdict"],
        "contexts": [
            _context(root, records, statuses, identifier, files) for identifier, files in impact["contexts_touched"].items()
        ],
        "records": [
            _with_check(_record(graph, statuses, asset, record_id, notes), history, changes_root, packages)
            for (asset, record_id), notes in sorted(touched.items(), key=str)
        ],
        "registry_changes": _registry_changes(root, repo_root, base),
        "source_map": _source_map_change(root, repo_root, base),
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
    records = _records(root)
    graph, statuses = DependencyGraph.from_documents(records), _default_statuses(root)
    brief = {
        "format": FORMAT,
        "subject": {"asset": asset, "id": record_id},
        "record": {**_summarized(statuses, asset, record), "asset": asset},
        "contexts": [_context(root, records, statuses, identifier, []) for identifier in _contexts_of(record)],
        "dependents": _with_status(statuses, graph.dependents(asset, record_id)["reached"]),
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
    if "registry_changes" in brief:
        lines += ["## Registry changes", "", *_registry_change_lines(brief["registry_changes"]), ""]
    if brief.get("source_map", {}).get("changed"):
        lines += ["## Source map", "", f"- {_source_map_line(brief['source_map'])}", ""]
    if brief.get("dependents") is not None:
        lines += ["## Dependents", "", *(_dependent_lines(brief["dependents"])), ""]
    lines += ["## Evidence", "", *_evidence_lines(brief["evidence"]), ""]
    return "\n".join(lines)


def _records(root: Path) -> dict[str, list[dict[str, Any]]]:
    return {name: asset_records(root, name) for name in ASSET_KEYS}


def _context(root: Path, records: dict[str, list[dict[str, Any]]], statuses: dict[str, Any], identifier: str,
             files: list[str]) -> dict[str, Any]:
    model = context_model(root, identifier, records)
    if model is None:
        return {"id": identifier, "changed_files": files, "model": None}
    summary = context_summary(model)
    summary["context"] = {**summary["context"], **_summarized(statuses, "contexts", model["context"])}
    return {"id": identifier, "changed_files": files, "model": summary}


def _record(graph: DependencyGraph, statuses: dict[str, Any], asset: str, record_id: Any, notes: dict[str, Any]) -> dict[str, Any]:
    record = graph.record(asset, record_id)
    shown = {**_summarized(statuses, asset, record), "asset": asset, **notes} if record is not None else {"id": record_id, "asset": asset, **notes}
    shown["dependents"] = _with_status(statuses, graph.dependents(asset, record_id)["reached"]) if record is not None else []
    return shown


def _with_check(record: dict[str, Any], history: dict[str, list[str]], changes_root: Path | None,
                packages: Callable[[], list[tuple[Path, dict[str, Any]]]]) -> dict[str, Any]:
    if record["asset"] != "rules":
        return record
    proposals = history.get(f"rules/{record['id']}")
    obligations = (
        proof_of(changes_root, proposals, ("rule", record["id"]), packages()).obligations if changes_root and proposals else None
    )
    if not obligations:
        return {**record, "check": None, "cwd": None}
    return {**record, "check": obligations[0]["check"], "cwd": obligations[0]["working_directory"]}


def _registry_changes(root: Path, repo_root: Path, base: str) -> list[dict[str, Any]]:
    before = committed_documents(repo_root, base, root, tuple(ASSET_KEYS)) or {}
    through = reviewing_proposals(root)
    changes = []
    for name, key in ASSET_KEYS.items():
        asset = name.removesuffix(".json")
        then_document, now_document = before.get(name) or {}, read_document(root, name)
        then, now = _by_id(then_document.get(key)), _by_id(now_document.get(key))
        for record_id in sorted(then.keys() | now.keys(), key=str):
            if record_id not in now:
                change, record, document = "removed", then[record_id], then_document
            elif record_id not in then or then[record_id] != now[record_id]:
                change, record, document = "added" if record_id not in then else "amended", now[record_id], now_document
            else:
                continue
            shown = summarized(record)
            changes.append({"asset": asset, "id": record_id, "change": change, "status": shown["status"] or document.get("status"),
                            "summary": shown["summary"], "through": through.get(f"{asset}/{record_id}")})
    return changes


def _source_map_change(root: Path, repo_root: Path, base: str) -> dict[str, Any]:
    _, then = committed_json(repo_root, base, root / "source-map.json")
    then, now = then or {}, source_map_for(root)
    if then == now:
        return {"changed": False}
    before, after = _discovered(then), _discovered(now)
    return {"changed": True, "confirmed_by": now.get("confirmed_by"), "confirmation": now.get("selection_status") if now else "removed",
            "discovered_added": len(after - before), "discovered_removed": len(before - after)}


def _discovered(source_map: dict[str, Any]) -> set[str]:
    return {path for group in source_map.get("source_groups") or [] for path in group.get("paths") or []}


def _source_map_line(change: dict[str, Any]) -> str:
    added, removed = change["discovered_added"], change["discovered_removed"]
    counts = f"(+{added} -{removed} discovered {'file' if added + removed == 1 else 'files'})"
    if change["confirmation"] == CONFIRMED:
        return f"source map re-confirmed in this range by {change['confirmed_by']} {counts}"
    return f"source map changed in this range and is {change['confirmation']}, not confirmed {counts}"


def _by_id(records: Any) -> dict[Any, dict[str, Any]]:
    return {record.get("id"): record for record in records or [] if isinstance(record, dict)}


def _counted(count: int, noun: str) -> str:
    return f"{count} {noun}" if count == 1 else f"{count} {noun}s"


def _default_statuses(root: Path) -> dict[str, Any]:
    return {name.removesuffix(".json"): read_document(root, name).get("status") for name in ASSET_KEYS}


def _summarized(statuses: dict[str, Any], asset: str, record: dict[str, Any]) -> dict[str, Any]:
    shown = summarized(record)
    if shown["status"] is None:
        shown["status"] = statuses[asset]
    return shown


def _with_status(statuses: dict[str, Any], reached: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [{**item, "status": statuses[item["asset"]]} if item["status"] is None else item for item in reached]


def _contexts_of(record: dict[str, Any]) -> list[str]:
    found = [record[name] for name in CONTEXT_FIELDS if isinstance(record.get(name), str)]
    found += [item for name in CONTEXT_LISTS for item in (record.get(name) or []) if isinstance(item, str)]
    return sorted(set(found))


def _evidence(report: dict[str, Any] | None) -> dict[str, Any] | None:
    if report is None:
        return None
    return {"summary": {key: value for key, value in report["summary"].items() if key != "by_source_kind"}, "paths": report.get("paths", {})}


def _failing(evidence: dict[str, Any] | None) -> str:
    if evidence is None:
        return ""
    counts = {status: evidence["summary"][status] for status in FAILING_EVIDENCE if evidence["summary"].get(status)}
    if not counts:
        return ""
    noun = "citation" if sum(counts.values()) == 1 else "citations"
    return f"{', '.join(f'{count} {status}' for status, count in counts.items())} {noun}"


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
        if entry["cited_as"] != PROOF_KIND
    ]
    rules = [record for record in brief["records"] if record["asset"] == "rules"]
    if rules:
        verb = "cites" if len(rules) == 1 else "cite"
        then = "run the checks listed on them" if all(rule["check"] for rule in rules) else "re-run their checks, which get-record names"
        attention.append(f"{_counted(len(rules), 'rule')} {verb} lines this change touched; {then}.")
    if failing := _failing(brief["evidence"]):
        attention.append(f"{failing} of the changed files; recite only a record whose statement still holds.")
    groups = Counter((change["asset"], change["through"]) for change in brief["registry_changes"])
    attention += [
        f"{asset}: {_counted(count, 'record')} changed in this range, {'last reviewed ' if through else ''}{_reviewed_through(through)}."
        for (asset, through), count in groups.items()
    ]
    if brief["source_map"]["changed"]:
        attention.append(_source_map_line(brief["source_map"]))
    return attention


def _record_attention(brief: dict[str, Any]) -> list[str]:
    record = brief["record"]
    attention = []
    if record["status"] != "reviewed":
        attention.append(f"The record is {record['status']}, not a reviewed constraint.")
    if reached := len(brief["dependents"]):
        verb = "reaches" if reached == 1 else "reach"
        attention.append(f"{_counted(reached, 'record')} {verb} it through relations and may need the same change; they are listed.")
    if brief["evidence"] is None:
        attention.append("The record cites nothing; it cannot be verified against the code.")
    elif failing := _failing(brief["evidence"]):
        attention.append(f"{failing}; recite them before relying on the record.")
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
    if "check" in record:
        shown = f"`{record['check']}` in `{record['cwd']}`" if record["check"] else "none known; get-record names what proves it"
        lines += [f"Check: {shown}", ""]
    return [*lines, "Dependents:", *_dependent_lines(record["dependents"]), ""]


def _registry_change_lines(changes: list[dict[str, Any]]) -> list[str]:
    return [f"- {change['change']} {_reviewed_through(change['through'])}: {_line(change)}" for change in changes] or ["- none"]


def _reviewed_through(proposal: str | None) -> str:
    return f"through {proposal}" if proposal else "without a reviewing proposal"


def _dependent_lines(reached: list[dict[str, Any]]) -> list[str]:
    return [f"- {_line(item)} via {item['through']} at depth {item['depth']}" for item in reached] or ["- none"]


def _evidence_lines(evidence: dict[str, Any] | None) -> list[str]:
    if evidence is None:
        return ["No citations to verify."]
    return [f"- {status}: {count}" for status, count in evidence["summary"].items() if count]
