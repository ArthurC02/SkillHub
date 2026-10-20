from __future__ import annotations

import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .audit import append_locked
from .changes import classify_all, init_change_package, unclassified_paths, validate_change_package
from .common import (
    EVIDENCE_FILE,
    OBLIGATIONS_FILE,
    PROPOSAL_FILE,
    is_reviewed,
    iter_packages,
    load_json,
    shown_path,
    write_json,
    writer_lock,
)
from .counterfactual import counterfactual_suite
from .evidence import citation, classified, function_span, source_map_for
from .harvest_package import harvest_package, obligation_of, write_harvest_package
from .hitl import ObligationRun, record_approval, record_test_result, same_command, submit_proposal, verify_proposal
from .registry import as_stored, record_by_id
from .sources import relisting_hint
from .store import read_document
from .transaction import remove_tree
from .updates import promote_candidate, replace_promoted_fields, retract_candidate, upsert_candidate

FORMAT = "domain-memory-harvest/v1"
STAGES = ("draft", "verified")
SPEC_FIELDS = ("name", "context", "proposer", "intent", "outcome", "scope")
RULE_FIELDS = ("id", "statement", "test_file", "test", "check")
DECISIONS = (("ownership", "ownership_decision"), ("boundary", "boundary_decision"), ("design", "implementation_design"))
DEFAULT_TIMEOUT_SECONDS = 600
SPAN_PARTS = 3
RULE_DEFAULTS = {"cwd": None, "command_profile": "unit", "level": "unit", "timeout": DEFAULT_TIMEOUT_SECONDS}
SUPERSEDING = ("amended", "reproven")
HARVESTED_FIELDS = ("statement", "contexts", "examples", "evidence")
UNFINISHED = ("draft", "submitted")
DISCARDABLE = ("draft", "submitted", "verified")
NOTHING_WRITTEN = {
    "spec": "no package was written; fix what the spec errors name, then run harvest again",
    "citation": "no package was written; confirm the sources the cited paths belong to, or cite others, then run harvest again",
    "package": (
        "nothing was written; give the spec another name, or, when the error says the harvest there did not finish, "
        "discard-harvest that package, then run harvest again"
    ),
    "counterfactual": "no package was written; make each counterfactual killed, then run harvest again",
}
NEXT_STEPS = {
    "draft": (
        "the draft package passes the submitted preview; run harvest --through verified, or submit-proposal, "
        "record-test-result per obligation, record-approval and verify-proposal"
    ),
    "submitted": "the proposal is submitted; run harvest --through verified to record its test results, approval and verification",
    "tested": "the proposal is submitted with its test results; record-approval, then verify-proposal",
    "verified": "sign a commit holding the package, then ship-package --commit <that commit>",
}


@dataclass
class Harvest:
    registry_root: Path
    repo_root: Path
    spec: Any
    package_root: Path | None
    through: str
    rules: list[dict[str, Any]] = field(default_factory=list)
    errors: list[str] = field(default_factory=list)
    stopped_at: str = "spec"
    written: bool = False
    resumed_from: str | None = None
    interrupted: bool = False

    def report(self) -> dict[str, Any]:
        return {
            "format": FORMAT,
            "package": _shown(self.package_root, self.repo_root) if self.written else None,
            "written": self.written,
            "resumed": self.resumed_from is not None,
            "stopped_at": self.stopped_at,
            "rules": self.rules,
            "errors": self.errors,
            "next": _next_step(self),
        }


def harvest(registry_root: Path, repo_root: Path, spec: Any, changes_root: Path | None, through: str) -> dict[str, Any]:
    registry_root, repo_root = registry_root.resolve(), repo_root.resolve()
    run = Harvest(registry_root, repo_root, spec, None, through, errors=spec_errors(spec))
    if run.errors:
        return run.report()
    run.package_root = (changes_root or registry_root / "changes") / spec["name"]
    run.stopped_at = "citation"
    records = _records(run)
    if run.errors:
        return run.report()
    run.stopped_at = "package"
    _check_package_directory(run, records)
    if run.errors:
        return run.report()
    run.stopped_at = "counterfactual"
    verdicts = _counterfactuals(run)
    if run.errors:
        return run.report()
    try:
        _write(run, records, verdicts)
    except (ValueError, OSError) as error:
        if not run.written:
            raise
        run.errors.append(str(error))
        run.interrupted = True
    return run.report()


def _write(run: Harvest, records: list[dict[str, Any] | None], verdicts: dict[str, dict[str, Any]]) -> None:
    run.written = run.resumed_from is not None
    run.stopped_at = "submitted" if run.resumed_from == "submitted" else "draft"
    _candidates(run, records)
    if run.resumed_from != "submitted":
        _package(run, records, verdicts)
        preview = validate_change_package(run.package_root, run.registry_root, "submitted", run.repo_root)
        if preview:
            run.errors += [f"once submitted: {error}" for error in preview]
            return
    if run.through == "verified":
        _verify(run)


def _check_package_directory(run: Harvest, records: list[dict[str, Any] | None]) -> None:
    package_root, name = run.package_root, run.spec["name"]
    if not package_root.is_dir() or not any(package_root.iterdir()):
        return
    shown = _shown(package_root, run.repo_root)
    proposal = _proposal_or_none(package_root)
    if proposal is None or proposal.get("proposal_id") != name:
        run.errors.append(
            f"output directory is not empty: {shown}; it holds no package of the harvest {name}, so give the spec "
            "another name or move what is there"
        )
        return
    status = proposal.get("status")
    if status not in UNFINISHED:
        run.errors.append(f"the package at {shown} is {status}; a finished harvest is not re-run")
        return
    changed = _changed_since_submitted(run, records, proposal) if status == "submitted" else []
    if changed:
        run.errors.append(
            f"the package at {shown} is submitted and {', '.join(changed)} changed since, but a submitted package is "
            f"frozen; discard-harvest --package-root {shown}, then run harvest again"
        )
        return
    run.resumed_from = status


def _proposal_or_none(package_root: Path) -> dict[str, Any] | None:
    try:
        return load_json(package_root / PROPOSAL_FILE)
    except ValueError:
        return None


def _reviewed(registry_root: Path, asset: str, record: dict[str, Any]) -> bool:
    return is_reviewed(asset, read_document(registry_root, f"{asset}.json"), record)


def _changed_since_submitted(run: Harvest, records: list[dict[str, Any] | None], proposal: dict[str, Any]) -> list[str]:
    promoted = {record_id for asset, record_id in _updated_records(proposal) if asset == "rules"}
    changed = {rule["id"] for rule in run.spec["rules"]} ^ promoted
    for record in records:
        existing = record_by_id(run.registry_root, "rules", record["id"])
        if existing is None or (not _reviewed(run.registry_root, "rules", existing) and not _same_draft(existing, record)):
            changed.add(record["id"])
    return sorted(changed)


def _updated_records(proposal: dict[str, Any]) -> list[tuple[str, str]]:
    updates = proposal.get("registry_updates")
    return [
        (update["asset"], update["record"]["id"]) for update in (updates if isinstance(updates, list) else [])
        if isinstance(update, dict) and isinstance(update.get("asset"), str) and isinstance(update.get("record"), dict)
        and isinstance(update["record"].get("id"), str)
    ]


def spec_errors(spec: Any) -> list[str]:
    if not isinstance(spec, dict):
        return ["the spec must be a JSON object"]
    errors = [f"{name} must be text" for name in SPEC_FIELDS if not _text(spec.get(name))]
    errors += [f"{name} must be an object" for name, _ in DECISIONS if not isinstance(spec.get(name), dict)]
    approval = spec.get("approval")
    if approval is not None and not (isinstance(approval, dict) and _text(approval.get("role")) and _text(approval.get("reviewer"))):
        errors.append("approval must be an object with role and reviewer")
    rules = spec.get("rules")
    if not isinstance(rules, list) or not rules:
        return [*errors, "rules must be a non-empty list"]
    errors.extend(f"{_rule_name(index, rule)}: {error}" for index, rule in enumerate(rules, start=1) for error in _rule_errors(rule))
    return errors


def _rule_name(index: int, rule: Any) -> str:
    return f"rule {rule['id']}" if isinstance(rule, dict) and _text(rule.get("id")) else f"rule {index}"


def _rule_errors(rule: Any) -> list[str]:
    if not isinstance(rule, dict):
        return ["must be an object"]
    errors = [f"{name} must be text" for name in RULE_FIELDS if not _text(rule.get(name))]
    spans = rule.get("enforcing")
    if not isinstance(spans, list) or not spans or any(_span(span) is None for span in spans):
        errors.append("enforcing must be a non-empty list of [path, start, end] or path:start-end")
    mutate = rule.get("mutate")
    if not isinstance(mutate, dict) or not all(_text(mutate.get(name)) for name in ("file", "find", "replace")):
        errors.append("mutate must be an object with file, find and replace")
    errors.extend(
        f"{name} must be text" for name in ("cwd", "command_profile", "level", "example")
        if rule.get(name) is not None and not _text(rule[name])
    )
    return errors


def _setting(spec: dict[str, Any], rule: dict[str, Any], name: str) -> Any:
    for holder in (rule, spec):
        if holder.get(name) is not None:
            return holder[name]
    return RULE_DEFAULTS[name]


def _text(value: Any) -> bool:
    return isinstance(value, str) and bool(value.strip())


def _span(span: Any) -> tuple[str, int, int] | None:
    if isinstance(span, str):
        path, _, lines = span.rpartition(":")
        start, _, end = lines.partition("-")
        if path and start.isdigit() and (end or start).isdigit():
            return path, int(start), int(end or start)
        return None
    if isinstance(span, list) and len(span) == SPAN_PARTS and _text(span[0]) and all(isinstance(n, int) for n in span[1:]):
        return span[0], span[1], span[2]
    return None


def _shown(package_root: Path | None, repo_root: Path) -> str | None:
    if package_root is None:
        return None
    return shown_path(package_root, repo_root)


def _records(run: Harvest) -> list[dict[str, Any] | None]:
    source_map = source_map_for(run.registry_root)
    records = []
    for rule in run.spec["rules"]:
        shown = {"id": rule["id"], "obligation": obligation_of(rule["id"])}
        run.rules.append(shown)
        record = _record(run, rule, source_map)
        if record is not None:
            shown["test_citation"] = record["evidence"][-1]
        records.append(record)
    return records


def _record(run: Harvest, rule: dict[str, Any], source_map: dict[str, Any]) -> dict[str, Any] | None:
    try:
        start, end = function_span(run.repo_root, rule["test_file"], rule["test"])
        evidence = [
            classified(citation(run.repo_root, path, first, last), source_map)
            for path, first, last in (_span(span) for span in rule["enforcing"])
        ]
        evidence.append(classified(citation(run.repo_root, rule["test_file"], start, end), source_map))
    except ValueError as error:
        run.errors.append(f"{rule['id']}: {error}")
        return None
    outside = unclassified_paths(classify_all({"evidence": evidence}, source_map))
    if outside:
        hint = relisting_hint(run.registry_root, run.repo_root, outside)
        run.errors.append(
            f"{rule['id']} cites {', '.join(outside)} outside the confirmed sources; ship-package would refuse "
            "to review it, so confirm those sources first or cite others" + (f"; {hint}" if hint else "")
        )
        return None
    examples = rule.get("examples") if isinstance(rule.get("examples"), list) else [rule["example"]] if rule.get("example") else []
    return {
        "id": rule["id"], "contexts": [run.spec["context"]], "statement": rule["statement"],
        **({"examples": examples} if examples else {}), "status": "candidate", "evidence": evidence,
    }


def _counterfactuals(run: Harvest) -> dict[str, dict[str, Any]]:
    suite = {"mutations": [
        {
            **rule["mutate"], "test_command": rule["check"], "cwd": _setting(run.spec, rule, "cwd"),
            "timeout": _setting(run.spec, rule, "timeout"), "obligation_id": obligation_of(rule["id"]),
        }
        for rule in run.spec["rules"]
    ]}
    results = counterfactual_suite(run.repo_root, suite)["results"]
    verdicts = {}
    for rule, shown, result in zip(run.spec["rules"], run.rules, results, strict=True):
        verdicts[rule["id"]] = result
        shown["counterfactual"] = result["verdict"]
        if result["verdict"] != "killed":
            run.errors.append(f"{rule['id']}: the counterfactual is {result['verdict']}: {result.get('reason')}")
    return verdicts


def _candidates(run: Harvest, records: list[dict[str, Any] | None]) -> None:
    pending = []
    for record, shown in zip(records, run.rules, strict=True):
        existing = record_by_id(run.registry_root, "rules", record["id"])
        if existing is not None and _reviewed(run.registry_root, "rules", existing):
            shown["record"] = "reproven" if _same_content(existing, record) else "amended"
        elif run.resumed_from is not None and existing is not None and _same_draft(existing, record):
            shown["record"] = "unchanged"
        else:
            pending.append((record, shown))
    with tempfile.TemporaryDirectory() as temporary:
        for index, (record, shown) in enumerate(pending):
            path = Path(temporary) / f"{index}-{record['id']}.json"
            write_json(path, record)
            upserted = upsert_candidate(run.registry_root, run.repo_root, "rules", path)
            shown["record"] = "created" if upserted.created else "revised"


def _same_content(existing: dict[str, Any], record: dict[str, Any]) -> bool:
    return (
        existing.get("statement") == record["statement"]
        and sorted(existing.get("contexts") or []) == sorted(record["contexts"])
        and (existing.get("examples") or []) == (record.get("examples") or [])
        and _cited_spans(existing.get("evidence") or []) == _cited_spans(record["evidence"])
    )


def _same_draft(existing: dict[str, Any], record: dict[str, Any]) -> bool:
    return as_stored("rules", existing) == as_stored("rules", record)


def _cited_spans(evidence: list[Any]) -> set[tuple[Any, Any, Any]]:
    spans = set()
    for reference in evidence:
        if isinstance(reference, dict):
            lines = reference.get("lines") or {}
            spans.add((reference.get("path"), lines.get("start"), lines.get("end")))
        else:
            spans.add(_span(reference))
    return spans


def _package(run: Harvest, records: list[dict[str, Any] | None], verdicts: dict[str, dict[str, Any]]) -> None:
    spec, package_root = run.spec, run.package_root
    if run.resumed_from is None:
        init_change_package(package_root, spec["name"], spec["name"], spec["proposer"])
        run.written = True
    promoted = {record_id for asset, record_id in _updated_records(load_json(package_root / PROPOSAL_FILE)) if asset == "rules"}
    for rule, shown, record in zip(spec["rules"], run.rules, records, strict=True):
        if rule["id"] not in promoted or shown["record"] in ("created", "revised"):
            promote_candidate(package_root, run.registry_root, "rules", rule["id"], amend=shown["record"] in SUPERSEDING)
        if shown["record"] in SUPERSEDING:
            replace_promoted_fields(package_root, "rules", rule["id"], {name: record.get(name) for name in HARVESTED_FIELDS})
    fill = {
        "rules": {
            rule["id"]: {"check": rule["check"], "level": _setting(spec, rule, "level"), "counterfactual": verdicts[rule["id"]]}
            for rule in spec["rules"]
        },
        "requirement": {"intent": spec["intent"], "actor": spec.get("actor", "coding-agent"), "observable_outcome": spec["outcome"]},
        **{target: spec[name] for name, target in DECISIONS},
    }
    documents = harvest_package(package_root, fill)
    if any(document != load_json(package_root / name) for name, document in documents.items()):
        write_harvest_package(package_root, documents)


def _verify(run: Harvest) -> None:
    spec, package_root = run.spec, run.package_root
    if load_json(package_root / PROPOSAL_FILE).get("status") == "draft":
        submit_proposal(package_root, run.registry_root, run.repo_root)
    run.stopped_at = "submitted"
    tested = _already_tested(package_root)
    for rule in spec["rules"]:
        if obligation_of(rule["id"]) in tested:
            continue
        record_test_result(
            package_root, run.registry_root, run.repo_root,
            ObligationRun(obligation_of(rule["id"]), None, _setting(spec, rule, "command_profile"), _setting(spec, rule, "cwd")),
            _setting(spec, rule, "timeout"),
        )
    run.stopped_at = "tested"
    approval = spec.get("approval")
    if approval is None:
        run.errors.append("no approval in the spec; record-approval, then verify-proposal")
        return
    if not _already_approved(package_root, approval):
        record_approval(package_root, approval["role"], approval["reviewer"], spec["scope"], None)
    verify_proposal(package_root, run.registry_root, run.repo_root)
    run.stopped_at = "verified"


def _already_tested(package_root: Path) -> set[str]:
    checks = {
        entry.get("id"): entry.get("check")
        for entry in load_json(package_root / OBLIGATIONS_FILE).get("obligations", []) if isinstance(entry, dict)
    }
    return {
        result["obligation_id"] for result in load_json(package_root / EVIDENCE_FILE).get("test_results", [])
        if isinstance(result, dict) and result.get("status") == "passed" and result.get("obligation_id") in checks
        and same_command(str(result.get("evidence")), checks[result["obligation_id"]])
    }


def _already_approved(package_root: Path, approval: dict[str, Any]) -> bool:
    return any(
        isinstance(entry, dict) and entry.get("role") == approval["role"] and entry.get("reviewer") == approval["reviewer"]
        for entry in load_json(package_root / PROPOSAL_FILE).get("approvals", [])
    )


def _next_step(run: Harvest) -> str:
    if run.stopped_at in NOTHING_WRITTEN:
        return NOTHING_WRITTEN[run.stopped_at]
    if run.interrupted:
        shown = _shown(run.package_root, run.repo_root)
        return (
            f"the draft records and the package at {shown} are written; run the same harvest --spec again to resume "
            f"from {run.stopped_at}, or discard-harvest --package-root {shown} to undo it"
        )
    if run.errors and run.stopped_at == "draft":
        return "the draft package is written; fix what the preview names, then validate-change-package --as-status submitted"
    return NEXT_STEPS[run.stopped_at]


def discard_harvest(package_root: Path, registry_root: Path, repo_root: Path, reason: str) -> dict[str, Any]:
    package_root, registry_root, repo_root = package_root.resolve(), registry_root.resolve(), repo_root.resolve()
    shown = _shown(package_root, repo_root)
    if not (package_root / PROPOSAL_FILE).is_file():
        raise ValueError(f"no package at {shown}")
    if not reason.strip():
        raise ValueError("discarding a harvest requires the reason it is undone")
    proposal = load_json(package_root / PROPOSAL_FILE)
    status = proposal.get("status")
    if status not in DISCARDABLE:
        raise ValueError(f"the package at {shown} is {status}; only a draft, submitted or verified harvest is discarded")
    named_elsewhere = _named_by_other_packages(package_root)
    retracted, kept = [], []
    for asset, record_id in _updated_records(proposal):
        record = record_by_id(registry_root, asset, record_id)
        if record is None or _reviewed(registry_root, asset, record):
            continue
        if (asset, record_id) in named_elsewhere:
            kept.append(f"{asset}/{record_id}")
            continue
        retract_candidate(registry_root, repo_root, asset, record_id, reason)
        retracted.append(f"{asset}/{record_id}")
    event = {"operation": "discard-harvest", "package": shown, "retracted": retracted, "kept": kept, "reason": reason.strip()}
    with writer_lock(registry_root):
        append_locked(registry_root, event)
        remove_tree(package_root)
    return event


def _named_by_other_packages(package_root: Path) -> set[tuple[str, str]]:
    named = set()
    for other, proposal in iter_packages(package_root.parent):
        if other != package_root and isinstance(proposal, dict) and proposal.get("status") != "superseded":
            named.update(_updated_records(proposal))
    return named
