from __future__ import annotations

import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .changes import classify_all, init_change_package, unclassified_paths, validate_change_package
from .common import write_json
from .counterfactual import counterfactual_suite
from .evidence import citation, classified, function_span, source_map_for
from .harvest_package import harvest_package, obligation_of, write_harvest_package
from .hitl import ObligationRun, record_approval, record_test_result, submit_proposal, verify_proposal
from .registry import is_reviewed, record_by_id
from .updates import add_promoted_evidence, promote_candidate, upsert_candidate

FORMAT = "domain-memory-harvest/v1"
STAGES = ("draft", "verified")
SPEC_FIELDS = ("name", "context", "proposer", "intent", "outcome", "scope")
RULE_FIELDS = ("id", "statement", "test_file", "test", "check")
DECISIONS = (("ownership", "ownership_decision"), ("boundary", "boundary_decision"), ("design", "implementation_design"))
DEFAULT_TIMEOUT_SECONDS = 600
SPAN_PARTS = 3
RULE_DEFAULTS = {"cwd": None, "command_profile": "unit", "level": "unit", "timeout": DEFAULT_TIMEOUT_SECONDS}
SUPERSEDING = ("amended", "reproven")
NOTHING_WRITTEN = {
    "spec": "no package was written; fix what the spec errors name, then run harvest again",
    "citation": "no package was written; confirm the sources the cited paths belong to, or cite others, then run harvest again",
    "counterfactual": "no package was written; make each counterfactual killed, then run harvest again",
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

    def report(self) -> dict[str, Any]:
        return {
            "format": FORMAT,
            "package": _shown(self.package_root, self.repo_root) if self.written else None,
            "stopped_at": self.stopped_at,
            "rules": self.rules,
            "errors": self.errors,
            "next": _next_step(self.stopped_at, failed=bool(self.errors)),
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
    run.stopped_at = "counterfactual"
    verdicts = _counterfactuals(run)
    if run.errors:
        return run.report()
    _candidates(run, records)
    _package(run, verdicts)
    run.stopped_at = "draft"
    preview = validate_change_package(run.package_root, registry_root, "submitted", repo_root)
    if preview:
        run.errors += [f"once submitted: {error}" for error in preview]
        return run.report()
    if through == "verified":
        _verify(run)
    return run.report()


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
    errors.extend(f"rules[{index}]: {error}" for index, rule in enumerate(rules, start=1) for error in _rule_errors(rule))
    return errors


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
    return package_root.relative_to(repo_root).as_posix() if package_root.is_relative_to(repo_root) else str(package_root)


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
        run.errors.append(
            f"{rule['id']} cites {', '.join(outside)} outside the confirmed sources; apply-approved-updates would refuse "
            "to review it, so confirm those sources first or cite others"
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
        if existing is not None and is_reviewed(existing):
            shown["record"] = "reproven" if _same_content(existing, record) else "amended"
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


def _cited_spans(evidence: list[Any]) -> set[tuple[Any, Any, Any]]:
    spans = set()
    for reference in evidence:
        if isinstance(reference, dict):
            lines = reference.get("lines") or {}
            spans.add((reference.get("path"), lines.get("start"), lines.get("end")))
        else:
            spans.add(_span(reference))
    return spans


def _package(run: Harvest, verdicts: dict[str, dict[str, Any]]) -> None:
    spec, package_root = run.spec, run.package_root
    init_change_package(package_root, spec["name"], spec["name"], spec["proposer"])
    run.written = True
    for rule, shown in zip(spec["rules"], run.rules, strict=True):
        promote_candidate(package_root, run.registry_root, "rules", rule["id"], amend=shown["record"] in SUPERSEDING)
        if shown["record"] in SUPERSEDING:
            add_promoted_evidence(package_root, "rules", rule["id"], (shown["test_citation"],))
    fill = {
        "rules": {
            rule["id"]: {"check": rule["check"], "level": _setting(spec, rule, "level"), "counterfactual": verdicts[rule["id"]]}
            for rule in spec["rules"]
        },
        "requirement": {"intent": spec["intent"], "actor": spec.get("actor", "coding-agent"), "observable_outcome": spec["outcome"]},
        **{target: spec[name] for name, target in DECISIONS},
    }
    write_harvest_package(package_root, harvest_package(package_root, fill))


def _verify(run: Harvest) -> None:
    spec, package_root = run.spec, run.package_root
    submit_proposal(package_root, run.registry_root, run.repo_root)
    run.stopped_at = "submitted"
    for rule in spec["rules"]:
        record_test_result(
            package_root, run.registry_root, run.repo_root,
            ObligationRun(obligation_of(rule["id"]), None, _setting(spec, rule, "command_profile"), _setting(spec, rule, "cwd")),
            _setting(spec, rule, "timeout"),
        )
    approval = spec.get("approval")
    if approval is None:
        run.errors.append("no approval in the spec; record-approval, then verify-proposal")
        return
    record_approval(package_root, approval["role"], approval["reviewer"], spec["scope"], None)
    verify_proposal(package_root, run.registry_root, run.repo_root)
    run.stopped_at = "verified"


def _next_step(stopped_at: str, *, failed: bool) -> str:
    if stopped_at in NOTHING_WRITTEN:
        return NOTHING_WRITTEN[stopped_at]
    if failed and stopped_at == "draft":
        return "the draft package is written; fix what the preview names, then validate-change-package --as-status submitted"
    if stopped_at == "draft":
        return ("the draft package passes the submitted preview; run harvest --through verified, or submit-proposal, "
                "record-test-result per obligation, record-approval and verify-proposal")
    if stopped_at == "submitted":
        return "the proposal is submitted with its test results; record-approval, then verify-proposal"
    return "sign a commit holding the package, then ship-package --commit <that commit>"
