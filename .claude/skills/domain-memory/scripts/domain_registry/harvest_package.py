from pathlib import Path
from typing import Any

from .common import EVIDENCE_FILE, OBLIGATIONS_FILE, PROPOSAL_FILE, REQUIREMENT_FILE, load_json, write_json
from .counterfactual import OBLIGATION_PLACEHOLDER
from .evidence import all_references
from .hitl import same_command

PACKAGE_DOCUMENTS = (REQUIREMENT_FILE, OBLIGATIONS_FILE, PROPOSAL_FILE, EVIDENCE_FILE)
REQUIREMENT_FIELDS = ("intent", "actor", "observable_outcome")
DESIGN_FIELDS = ("domain_forces", "decision", "invariants_preserved", "rejected_alternatives")
LIST_COUNTERFACTUAL = (
    "Each rule's protection is broken in turn and the rule's own test fails; the evidence bundle's "
    "counterfactual_check list records every run."
)


def criterion_of(rule_id: str) -> str:
    return f"enforced-{rule_id}"


def obligation_of(rule_id: str) -> str:
    return f"rule-{rule_id}"


def harvest_package(package_root: Path, spec: dict[str, Any]) -> dict[str, dict[str, Any]]:
    if not isinstance(spec, dict):
        raise ValueError("the harvest spec must be a JSON object")
    documents = {name: load_json(package_root / name) for name in PACKAGE_DOCUMENTS}
    proposal = documents[PROPOSAL_FILE]
    if proposal.get("status") != "draft":
        raise ValueError(f"the package is {proposal.get('status')}; harvest --spec fills a draft")
    rules = _promoted_rules(proposal)
    if not rules:
        raise ValueError("the package promotes no rule; run promote-candidate for each harvested rule first")
    entries = spec.get("rules") if isinstance(spec.get("rules"), dict) else {}
    errors = _spec_errors(rules, entries)
    if errors:
        raise ValueError("the harvest spec cannot fill this package: " + "; ".join(errors))
    _fill(documents, rules, entries, spec)
    return documents


def write_harvest_package(package_root: Path, documents: dict[str, dict[str, Any]]) -> None:
    for name, document in documents.items():
        write_json(package_root / name, document)


def _promoted_rules(proposal: dict[str, Any]) -> list[dict[str, Any]]:
    return [
        update["record"]
        for update in proposal.get("registry_updates", [])
        if isinstance(update, dict) and update.get("asset") == "rules" and update.get("operation") == "upsert"
        and isinstance(update.get("record"), dict) and isinstance(update["record"].get("id"), str)
    ]


def _spec_errors(rules: list[dict[str, Any]], entries: dict[str, Any]) -> list[str]:
    promoted = [rule["id"] for rule in rules]
    errors = [f"rules.{rule_id} is missing" for rule_id in promoted if rule_id not in entries]
    errors += [f"rules.{rule_id} names a rule the package does not promote" for rule_id in entries if rule_id not in promoted]
    for rule_id in promoted:
        if rule_id in entries:
            errors += [f"rules.{rule_id}: {error}" for error in _entry_errors(rule_id, entries[rule_id])]
    return errors


def _entry_errors(rule_id: str, entry: Any) -> list[str]:
    if not isinstance(entry, dict) or not isinstance(entry.get("check"), str) or not entry["check"].strip():
        return ["give the check that runs the rule's test"]
    run = entry.get("counterfactual")
    if not isinstance(run, dict) or "verdict" not in run:
        return ["counterfactual must be the whole output of counterfactual, so its verdict and test_command can be read"]
    return _run_errors(rule_id, entry, run)


def _run_errors(rule_id: str, entry: dict[str, Any], run: dict[str, Any]) -> list[str]:
    if run.get("verdict") != "killed":
        return [f"the counterfactual is {run.get('verdict')}, not killed; the rule is not shown to fail without its protection"]
    if not isinstance(run.get("counterfactual_check"), dict):
        return ["the counterfactual output has no counterfactual_check; give the whole output of counterfactual"]
    if not same_command(str(run.get("test_command")), entry["check"]):
        return [f"the counterfactual ran {run.get('test_command')!r}, not the check {entry['check']!r}"]
    named = run["counterfactual_check"].get("obligation_id")
    if named not in (OBLIGATION_PLACEHOLDER, obligation_of(rule_id)):
        return [f"the counterfactual names obligation {named}, not {obligation_of(rule_id)}"]
    return []


def _fill(
    documents: dict[str, dict[str, Any]], rules: list[dict[str, Any]], entries: dict[str, Any], spec: dict[str, Any]
) -> None:
    sources = _cited_paths(rules)
    requirement = documents[REQUIREMENT_FILE]
    requirement.update(_chosen(spec.get("requirement"), REQUIREMENT_FIELDS))
    requirement["acceptance_criteria"] = [
        {"id": criterion_of(rule["id"]), "statement": rule.get("statement")} for rule in rules
    ]
    requirement["sources"] = sources
    documents[OBLIGATIONS_FILE]["obligations"] = [_obligation(rule, entries[rule["id"]]) for rule in rules]
    proposal = documents[PROPOSAL_FILE]
    proposal["affected_contexts"] = sorted({context for rule in rules for context in rule.get("contexts") or []})
    proposal["rule_ids"] = [rule["id"] for rule in rules]
    for decision in ("ownership_decision", "boundary_decision"):
        if isinstance(spec.get(decision), dict):
            proposal[decision] = spec[decision]
    design = proposal.get("implementation_design") if isinstance(proposal.get("implementation_design"), dict) else {}
    proposal["implementation_design"] = {
        **design,
        **_chosen(spec.get("implementation_design"), DESIGN_FIELDS),
        "proof_obligations": [obligation_of(rule["id"]) for rule in rules],
        "counterfactual_check": LIST_COUNTERFACTUAL,
    }
    evidence = documents[EVIDENCE_FILE]
    evidence["sources"] = sources
    evidence["counterfactual_check"] = [
        {**entries[rule["id"]]["counterfactual"]["counterfactual_check"], "obligation_id": obligation_of(rule["id"])}
        for rule in rules
    ]


def _obligation(rule: dict[str, Any], entry: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": obligation_of(rule["id"]),
        "source_type": "acceptance-criterion",
        "source_id": criterion_of(rule["id"]),
        "also_covers": [{"source_type": "rule", "source_id": rule["id"]}],
        "expected_outcome": rule.get("statement"),
        "level": entry.get("level", "unit"),
        "status": "planned",
        "check": entry["check"].strip(),
    }


def _chosen(value: Any, fields: tuple[str, ...]) -> dict[str, Any]:
    return {field: value[field] for field in fields if field in value} if isinstance(value, dict) else {}


def _cited_paths(rules: list[dict[str, Any]]) -> list[str]:
    return sorted({
        reference["path"] if isinstance(reference, dict) else str(reference).split(":", 1)[0]
        for reference in all_references(rules)
        if isinstance(reference, (dict, str)) and (not isinstance(reference, dict) or isinstance(reference.get("path"), str))
    })
