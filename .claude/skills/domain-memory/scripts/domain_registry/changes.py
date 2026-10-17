import re
import shutil
from collections import Counter
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .attestations import verify_scm_value
from .common import (
    ASSET_KEYS,
    CHANGE_CLASSIFICATIONS,
    CHANGE_PACKAGE_FILES,
    EVIDENCE_FILE,
    PROPOSAL_FILE,
    CHANGE_PACKAGE_FORMATS,
    OBLIGATION_SOURCE_TYPES,
    OBLIGATION_STATUSES,
    PROPOSAL_STATUSES,
    RISK_FLAGS,
    change_template_dir,
    completed_identifier,
    iso_timestamp,
    load_json,
    utc_now,
    write_json,
)
from .policy import approved_command_profiles
from .evidence import all_references, verifier
from .registry import asset_records, cross_reference_errors
from .revision import DIGEST_HEX_LENGTH, DIGEST_PREFIX, valid_digest, valid_registry_revision

SUBMITTED_OR_LATER = {"submitted", "verified", "approved", "applied"}
VERIFIED_OR_LATER = {"verified", "approved", "applied"}
APPROVED_OR_LATER = {"approved", "applied"}
PREVIEW_STATUSES = ("submitted", "verified", "approved", "applied")
STAND_IN_REVISION = {"observed_commit": None, "registry_digest": DIGEST_PREFIX + "0" * DIGEST_HEX_LENGTH}
STAND_IN_TIME = "1970-01-01T00:00:00Z"


@dataclass(frozen=True)
class ParsedUpdate:
    asset: str
    record_id: str
    record: dict[str, Any] | None

    @property
    def document_name(self) -> str:
        return f"{self.asset}.json"


def parsed_update(update: Any) -> ParsedUpdate:
    if not isinstance(update, dict) or update.get("operation") not in ("upsert", "remove"):
        raise ValueError("each registry update must be an upsert or a remove")
    removing = update.get("operation") == "remove"
    asset = update.get("asset")
    if not isinstance(asset, str) or f"{asset}.json" not in ASSET_KEYS:
        raise ValueError(unknown_asset_message(asset))
    record = None if removing else update.get("record")
    record_id = update.get("id") if removing else (record or {}).get("id")
    if (not removing and not isinstance(record, dict)) or not completed_identifier(record_id):
        raise ValueError(f"registry update for {asset} needs a record with an id, or a remove with an id")
    return ParsedUpdate(asset, record_id, record)


def unknown_asset_message(asset: Any) -> str:
    valid = sorted(name.removesuffix(".json") for name in ASSET_KEYS)
    message = f"registry update names unknown asset {asset!r}; valid assets: {', '.join(valid)}"
    holder = next((name for name, key in ASSET_KEYS.items() if key == asset), None)
    if holder is not None:
        message += f"; {asset!r} is the list inside {holder}, whose asset is {holder.removesuffix('.json')!r}"
    return message


def registry_update_errors(proposal: dict[str, Any]) -> list[str]:
    updates = proposal.get("registry_updates")
    if updates is None:
        return []
    if not isinstance(updates, list):
        return ["domain-change-proposal.json registry_updates must be a list"]
    return [error for error in map(_update_error, updates) if error]


def _update_error(update: Any) -> str | None:
    try:
        parsed_update(update)
    except ValueError as error:
        return str(error)
    return None


def as_if_moved_to(status: str, documents: dict[str, Any]) -> dict[str, Any]:
    proposal = {**documents[PROPOSAL_FILE], "status": status}
    evidence = dict(documents[EVIDENCE_FILE])
    if status in SUBMITTED_OR_LATER and not valid_registry_revision(proposal.get("base_registry_revision")):
        proposal["base_registry_revision"] = STAND_IN_REVISION
        evidence["registry_revision"] = STAND_IN_REVISION
    if status == "verified" and not iso_timestamp(proposal.get("verified_at")):
        proposal["verified_at"] = STAND_IN_TIME
    if status == "applied":
        if not iso_timestamp(proposal.get("applied_at")):
            proposal["applied_at"] = STAND_IN_TIME
        if not valid_registry_revision(proposal.get("applied_registry_revision")):
            proposal["applied_registry_revision"] = STAND_IN_REVISION
    return {**documents, PROPOSAL_FILE: proposal, EVIDENCE_FILE: evidence}


def completed_identifiers(values: Any) -> bool:
    return isinstance(values, list) and all(
        completed_identifier(value) for value in values
    )


def test_attestation_errors(
    evidence: dict[str, Any], obligation_ids: set[str], registry_root: Path | None
) -> list[str]:
    errors: list[str] = []
    results = evidence.get("test_results", [])
    if not isinstance(results, list):
        return ["evidence-bundle.json has invalid test_results"]
    profiles = approved_command_profiles(registry_root) if registry_root else set()
    completed = set()
    for result in results:
        if (
            not isinstance(result, dict)
            or result.get("status") != "passed"
            or not completed_identifier(result.get("obligation_id"))
        ):
            errors.append("each verified test result requires a passed obligation_id")
            continue
        obligation_id = result["obligation_id"]
        if obligation_id not in obligation_ids:
            errors.append("a test result references an unknown obligation")
            continue
        if obligation_id in completed:
            errors.append("a test obligation may have only one passing attestation")
        completed.add(obligation_id)
        if not completed_identifier(result.get("evidence")) or not completed_identifier(
            result.get("command_profile")
        ):
            errors.append(
                "a verified test result requires evidence and command_profile"
            )
        if profiles and result.get("command_profile") not in profiles:
            errors.append(
                "a test result uses a command profile not approved by Domain Memory policy"
            )
        if (
            result.get("exit_code") != 0
            or not valid_digest(result.get("output_sha256"))
            or not iso_timestamp(result.get("finished_at"))
        ):
            errors.append(
                "a verified test result requires exit_code 0, output_sha256, and finished_at"
            )
    if obligation_ids - completed:
        errors.append("every test obligation requires a passing attestation")
    return errors


def init_change_package(
    output: Path, requirement_id: str | None = None, proposal_id: str | None = None, proposer: str | None = None
) -> None:
    identifiers = {"requirement_id": requirement_id, "proposal_id": proposal_id, "proposer": proposer}
    blank = [
        key
        for key, value in identifiers.items()
        if value is not None and not completed_identifier(value)
    ]
    if blank:
        raise ValueError(f"{', '.join(blank)} must not be blank")
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"output directory is not empty: {output}")
    if proposal_id is not None:
        _refuse_used_proposal_id(output, proposal_id)
    with _whole_package(output):
        output.mkdir(parents=True, exist_ok=True)
        templates = change_template_dir()
        for name in CHANGE_PACKAGE_FILES:
            shutil.copyfile(templates / name, output / name)
        given = {key: value for key, value in identifiers.items() if value is not None}
        if given:
            _fill_identifiers(output, given)


def _refuse_used_proposal_id(output: Path, proposal_id: str) -> None:
    siblings = output.resolve().parent
    for package in siblings.iterdir() if siblings.is_dir() else ():
        try:
            used = load_json(package / PROPOSAL_FILE).get("proposal_id") if package.is_dir() else None
        except (OSError, ValueError):
            continue
        if used == proposal_id and package.resolve() != output.resolve():
            raise ValueError(f"proposal_id {proposal_id} is already used by {package.name}; choose another")


@contextmanager
def _whole_package(output: Path) -> Iterator[None]:
    existed = output.exists()
    try:
        yield
    except BaseException:
        shutil.rmtree(output, ignore_errors=True)
        if existed:
            output.mkdir(exist_ok=True)
        raise


def _fill_identifiers(output: Path, given: dict[str, str]) -> None:
    for name in CHANGE_PACKAGE_FORMATS:
        document = load_json(output / name)
        document.update({key: value for key, value in given.items() if key in document})
        write_json(output / name, document)


REVIEW_STATE = {
    PROPOSAL_FILE: (
        "status", "proposal_revision", "submitted_at", "verified_at", "finalized_at", "applied_at",
        "base_registry_revision", "approvals",
    ),
    EVIDENCE_FILE: (
        "registry_revision", "approvals", "scm_attestation", "counterfactual_check", "test_results",
        "contract_checks", "architecture_checks", "security_checks",
    ),
}
STATE_AFTER_SUBMISSION = (
    "applied_registry_revision", "superseded_from_status", "superseded_at", "superseded_reason",
    "superseded_by",
)


def redraft_change_package(source: Path, output: Path, proposal_id: str) -> None:
    if load_json(source / PROPOSAL_FILE).get("status") != "superseded":
        raise ValueError(
            "only a superseded proposal is redrafted; supersede it first with its reason, "
            "so its own record says why it was replaced"
        )
    if not completed_identifier(proposal_id):
        raise ValueError("proposal_id must not be blank")
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"output directory is not empty: {output}")
    _refuse_used_proposal_id(output, proposal_id)
    replaced = load_json(source / PROPOSAL_FILE).get("proposal_id")
    with _whole_package(output):
        shutil.copytree(source, output, dirs_exist_ok=True)
        _fill_identifiers(output, {"proposal_id": proposal_id})
        _reset_review_state(output, replaced)


def _reset_review_state(output: Path, replaced: Any) -> None:
    templates = change_template_dir()
    for name, fields in REVIEW_STATE.items():
        document = load_json(output / name)
        template = load_json(templates / name)
        document.update({field: template[field] for field in fields})
        for field in STATE_AFTER_SUBMISSION:
            document.pop(field, None)
        if name == PROPOSAL_FILE:
            document["supersedes"] = sorted({*(superseded_ids(document) or set()), replaced} - {None})
            document["redrafted_at"] = utc_now()
        write_json(output / name, document)


def implementation_design_errors(
    proposal: dict[str, Any], obligation_ids: set[str]
) -> list[str]:
    classification = proposal.get("change_classification")
    if classification not in CHANGE_CLASSIFICATIONS:
        return ["domain-change-proposal.json has an invalid change_classification"]
    design = proposal.get("implementation_design")
    if isinstance(design, dict):
        return _design_field_errors(design, obligation_ids, classification)
    if classification == "material":
        return [
            "a material proposal requires implementation_design with domain forces, decision, invariants, alternatives, proof obligations, and counterfactual check"
        ]
    return []


def _design_field_errors(
    design: dict[str, Any], obligation_ids: set[str], classification: str
) -> list[str]:
    errors: list[str] = []
    for field in ("domain_forces", "invariants_preserved", "rejected_alternatives"):
        values = design.get(field)
        if not completed_identifiers(values):
            errors.append(f"implementation_design requires a valid {field} list")
        elif classification == "material" and not values:
            errors.append(f"a material proposal requires at least one entry in {field}")
    if not completed_identifier(design.get("decision")):
        errors.append("implementation_design requires a completed decision")
    proof_obligations = design.get("proof_obligations")
    if not completed_identifiers(proof_obligations):
        errors.append("implementation_design requires proof obligation IDs")
    elif set(proof_obligations) - obligation_ids:
        errors.append("implementation_design references an unknown proof obligation")
    if not completed_identifier(design.get("counterfactual_check")):
        errors.append("implementation_design requires a counterfactual check")
    if classification == "material" and not proof_obligations:
        errors.append("a material proposal requires at least one proof obligation")
    return errors


BEHAVIOR_ASSETS = {"rules", "contracts", "aggregates"}


SETTLED_STATUSES = {"applied", "superseded"}
DECISIONS_WITH_EVIDENCE = ("ownership_decision", "boundary_decision")
PROPOSAL_MOMENTS = ("redrafted_at", "submitted_at", "verified_at", "finalized_at", "applied_at", "superseded_at")
CITED_FILE = re.compile(r"^(?P<path>[^\s:]+)(?::\d+(?:-\d+)?)?$")


def _changes_behavior(proposal: dict[str, Any]) -> bool:
    updates = proposal.get("registry_updates")
    return bool(proposal.get("rule_ids") or proposal.get("contract_ids")) or any(
        isinstance(update, dict) and update.get("asset") in BEHAVIOR_ASSETS
        for update in (updates if isinstance(updates, list) else [])
    )


def _inapplicable_counterfactual_errors(check: dict[str, Any], proposal: dict[str, Any]) -> list[str]:
    if _changes_behavior(proposal):
        return [
            "counterfactual_check may be not_applicable only when the proposal changes no rule, "
            "contract, or aggregate; a changed rule needs a test shown to fail without it"
        ]
    if not completed_identifier(check.get("reason")):
        return ["a not_applicable counterfactual_check requires a reason"]
    return []


def counterfactual_errors(
    evidence: dict[str, Any], obligation_ids: set[str], proposal: dict[str, Any]
) -> list[str]:
    check = evidence.get("counterfactual_check")
    if isinstance(check, dict) and check.get("status") == "not_applicable":
        return _inapplicable_counterfactual_errors(check, proposal)
    checks = check if isinstance(check, list) else [check]
    if not checks or not all(isinstance(entry, dict) for entry in checks):
        return ["a material verified proposal requires counterfactual_check evidence"]
    targets = [str(entry.get("obligation_id")) for entry in checks]
    if len(set(targets)) < len(targets):
        return ["each counterfactual_check in a list targets a different obligation"]
    return [error for entry in checks for error in _killed_counterfactual_errors(entry, obligation_ids, proposal)]


def _killed_counterfactual_errors(
    check: dict[str, Any], obligation_ids: set[str], proposal: dict[str, Any]
) -> list[str]:
    design = proposal.get("implementation_design")
    planned = design.get("proof_obligations") if isinstance(design, dict) else None
    if proposal.get("status") not in SETTLED_STATUSES and isinstance(planned, list) and check.get("obligation_id") not in planned:
        return ["counterfactual_check must target one of implementation_design's proof_obligations"]
    if (
        check.get("status") != "passed"
        or check.get("restored") is not True
        or check.get("obligation_id") not in obligation_ids
        or not completed_identifier(check.get("mutated_protection"))
        or not completed_identifier(check.get("failure_evidence"))
    ):
        return [
            "counterfactual_check requires passed status, a known obligation_id, mutated_protection, failure_evidence, and restored=true"
        ]
    return []


def _obligation_errors(
    obligations: dict[str, Any],
    criterion_ids: set[str],
    rule_ids: list[Any],
    contract_ids: list[Any],
) -> tuple[list[str], set[str]]:
    obligation_records = obligations.get("obligations")
    if not isinstance(obligation_records, list) or not obligation_records:
        return ["test-obligations.json requires obligations"], set()
    errors: list[str] = []
    obligation_ids: set[str] = set()
    obligation_sources: set[tuple[str, str]] = set()
    named = _named_sources(criterion_ids, rule_ids, contract_ids)
    for obligation in obligation_records:
        if not isinstance(obligation, dict) or not completed_identifier(
            obligation.get("id")
        ):
            errors.append("each test obligation requires a completed id")
            continue
        obligation_id = obligation["id"]
        if obligation_id in obligation_ids:
            errors.append("test-obligations.json contains duplicate obligation id")
        obligation_ids.add(obligation_id)
        also_covers = obligation.get("also_covers", [])
        if not isinstance(also_covers, list) or not all(isinstance(source, dict) for source in also_covers):
            errors.append(f"test obligation {obligation_id} has also_covers that is not a list of sources")
            also_covers = []
        for source in (obligation, *also_covers):
            error = _obligation_source_error(obligation_id, source)
            if error:
                errors.append(error)
            else:
                obligation_sources.add((source["source_type"], source["source_id"]))
        errors.extend(_unnamed_also_covered(obligation_id, also_covers, named))
        if not completed_identifier(
            obligation.get("expected_outcome")
        ) or not completed_identifier(obligation.get("check")):
            errors.append(
                f"test obligation {obligation_id} requires expected_outcome and check"
            )
        if obligation.get("status") not in OBLIGATION_STATUSES:
            errors.append(f"test obligation {obligation_id} has an invalid status")
    errors.extend(
        f"acceptance criterion {criterion_id} lacks a test obligation: "
        f"add one whose source_type is acceptance-criterion and whose source_id is {criterion_id}"
        for criterion_id in criterion_ids
        if ("acceptance-criterion", criterion_id) not in obligation_sources
    )
    errors.extend(
        f"{source_type} {source_id} lacks a test obligation"
        for source_type, source_ids in (
            ("rule", rule_ids),
            ("contract", contract_ids),
        )
        for source_id in source_ids
        if (source_type, source_id) not in obligation_sources
    )
    return errors, obligation_ids


def _named_sources(criterion_ids: set[str], rule_ids: list[Any], contract_ids: list[Any]) -> set[tuple[str, str]]:
    return {
        (source_type, source_id)
        for source_type, source_ids in (("acceptance-criterion", criterion_ids), ("rule", rule_ids), ("contract", contract_ids))
        for source_id in source_ids
    }


def _unnamed_also_covered(obligation_id: str, also_covers: list[dict[str, Any]], named: set[tuple[str, str]]) -> list[str]:
    return [
        f"test obligation {obligation_id} lists {source['source_type']} {source['source_id']} in also_covers, but the "
        f"proposal names no such {source['source_type']}; also_covers counts only toward the acceptance criteria and "
        "the rule_ids and contract_ids the proposal names"
        for source in also_covers
        if source.get("source_type") != "invariant"
        and _obligation_source_error(obligation_id, source) is None
        and (source["source_type"], source["source_id"]) not in named
    ]


def _obligation_source_error(obligation_id: str, source: dict[str, Any]) -> str | None:
    source_type = source.get("source_type")
    if source_type not in OBLIGATION_SOURCE_TYPES:
        allowed = ", ".join(sorted(OBLIGATION_SOURCE_TYPES))
        return (
            f"test obligation {obligation_id} has source_type {source_type!r}; a source_type names what the "
            f"obligation is derived from, not the kind of check, and must be one of {allowed}"
        )
    if not completed_identifier(source.get("source_id")):
        return f"test obligation {obligation_id} has an incomplete source_id: {source.get('source_id')!r}"
    return None


def _counts_as_approval(approval: Any, proposal: dict[str, Any]) -> bool:
    return (
        isinstance(approval, dict)
        and approval.get("decision") == "approved"
        and approval.get("proposal_revision") == proposal.get("proposal_revision")
        and approval.get("base_registry_revision") == proposal.get("base_registry_revision")
        and _signed_in_full(approval)
    )


def _signed_in_full(approval: dict[str, Any]) -> bool:
    return bool(
        all(completed_identifier(approval.get(field)) for field in ("role", "reviewer", "scope"))
        and iso_timestamp(approval.get("approved_at"))
    )


def missing_approval_roles(requirement: dict[str, Any], proposal: dict[str, Any]) -> list[str]:
    required = requirement.get("required_approval_roles", [])
    approvals = proposal.get("approvals", [])
    approved = {
        approval["role"]
        for approval in (approvals if isinstance(approvals, list) else [])
        if _counts_as_approval(approval, proposal)
    }
    return sorted(set(required if isinstance(required, list) else []) - approved)


def _approval_entry_error(approval: Any, revision: Any, base_revision: Any) -> str | None:
    if not isinstance(approval, dict):
        return "domain-change-proposal.json has invalid approval entry"
    if approval.get("decision") != "approved" or approval.get("proposal_revision") != revision:
        return None
    if not _signed_in_full(approval):
        return "an approval requires role, reviewer, approved_at, and scope"
    if approval.get("base_registry_revision") != base_revision:
        return "an approval must bind to the proposal base_registry_revision"
    return None


def _approval_list_errors(proposal: dict[str, Any], revision: Any) -> list[str]:
    approvals = proposal.get("approvals", [])
    if not isinstance(approvals, list):
        return ["domain-change-proposal.json has invalid approvals"]
    base_revision = proposal.get("base_registry_revision")
    found = (_approval_entry_error(approval, revision, base_revision) for approval in approvals)
    return [error for error in found if error]


def _approval_and_revision_errors(
    requirement: dict[str, Any],
    proposal: dict[str, Any],
    evidence: dict[str, Any],
    status: Any,
    revision: Any,
) -> list[str]:
    errors: list[str] = []
    if not completed_identifiers(requirement.get("required_approval_roles", [])):
        errors.append(
            "requirement-normalization.json has invalid required_approval_roles"
        )
    errors.extend(_approval_list_errors(proposal, revision))
    return errors + _lifecycle_field_errors(requirement, proposal, evidence, status)


def _lifecycle_field_errors(
    requirement: dict[str, Any], proposal: dict[str, Any], evidence: dict[str, Any], status: Any
) -> list[str]:
    errors: list[str] = []
    base_revision = proposal.get("base_registry_revision")
    missing = missing_approval_roles(requirement, proposal)
    if status in APPROVED_OR_LATER and missing:
        errors.append(
            "an approved or applied proposal lacks required approvals: " + ", ".join(missing)
        )
    if status in SUBMITTED_OR_LATER and not valid_registry_revision(base_revision):
        errors.append(
            "a submitted, approved, or applied proposal requires a Registry digest base revision"
        )
    if (
        status in SUBMITTED_OR_LATER
        and evidence.get("registry_revision") != base_revision
    ):
        errors.append(
            "evidence-bundle.json must retain the proposal base_registry_revision"
        )
    if status == "verified" and not iso_timestamp(proposal.get("verified_at")):
        errors.append("a verified proposal requires verified_at")
    if status == "applied":
        if not iso_timestamp(proposal.get("applied_at")):
            errors.append("an applied proposal requires applied_at")
        if not valid_registry_revision(proposal.get("applied_registry_revision")):
            errors.append("an applied proposal requires applied_registry_revision")
    return errors


def _evidence_result_errors(
    evidence: dict[str, Any], obligation_ids: set[str]
) -> list[str]:
    errors: list[str] = []
    results = evidence.get("test_results", [])
    if not isinstance(results, list):
        return ["evidence-bundle.json has invalid test_results"]
    for result in results:
        if (
            not isinstance(result, dict)
            or result.get("status") != "passed"
            or not completed_identifier(result.get("obligation_id"))
            or not completed_identifier(result.get("evidence"))
        ):
            errors.append(
                "each test result requires a passed obligation_id and evidence"
            )
            continue
        if result["obligation_id"] not in obligation_ids:
            errors.append("a test result references an unknown obligation")
    return errors


def _registry_reference_errors(staged: dict[str, list[dict[str, Any]]], proposal: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    for asset, kind, referenced in (
        ("contexts.json", "context", proposal.get("affected_contexts")),
        ("rules.json", "rule", proposal.get("rule_ids", [])),
        ("contracts.json", "contract", proposal.get("contract_ids", [])),
    ):
        known = {entry.get("id") for entry in staged[asset]}
        if isinstance(referenced, list) and set(referenced) - known:
            errors.append(
                f"domain-change-proposal.json references an unknown registry {kind}"
            )
    return errors


def _identity_errors(documents: dict[str, dict[str, Any]]) -> list[str]:
    requirement = documents["requirement-normalization.json"]
    proposal = documents[PROPOSAL_FILE]
    errors = [
        f"{name} has an invalid format"
        for name, document in documents.items()
        if document.get("format") != CHANGE_PACKAGE_FORMATS[name]
    ]
    requirement_id = requirement.get("requirement_id")
    proposal_id = proposal.get("proposal_id")
    if not completed_identifier(requirement_id):
        errors.append(
            "requirement-normalization.json requires a completed requirement_id"
        )
    if not completed_identifier(proposal_id):
        errors.append("domain-change-proposal.json requires a completed proposal_id")
    if not completed_identifier(proposal.get("proposer")):
        errors.append("domain-change-proposal.json requires a completed proposer")
    errors.extend(
        f"{name} must reference the requirement_id"
        for name in (
            "domain-change-proposal.json",
            "test-obligations.json",
            "evidence-bundle.json",
        )
        if documents[name].get("requirement_id") != requirement_id
    )
    errors.extend(
        f"{name} must reference the proposal_id"
        for name in ("test-obligations.json", "evidence-bundle.json")
        if documents[name].get("proposal_id") != proposal_id
    )
    return errors


def _acceptance_criteria(requirement: dict[str, Any]) -> tuple[list[str], set[str]]:
    acceptance_criteria = requirement.get("acceptance_criteria")
    if not isinstance(acceptance_criteria, list) or not acceptance_criteria:
        return ["requirement-normalization.json requires acceptance_criteria"], set()
    errors: list[str] = []
    criterion_ids: set[str] = set()
    for criterion in acceptance_criteria:
        if (
            not isinstance(criterion, dict)
            or not completed_identifier(criterion.get("id"))
            or not completed_identifier(criterion.get("statement"))
        ):
            errors.append("each acceptance criterion requires a completed id and statement")
            continue
        criterion_ids.add(criterion["id"])
    return errors, criterion_ids


def _risk_flag_errors(requirement: dict[str, Any], proposal: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    requirement_flags = requirement.get("risk_flags", [])
    proposal_flags = proposal.get("risk_flags", [])
    if not isinstance(requirement_flags, list) or set(requirement_flags) - RISK_FLAGS:
        errors.append("requirement-normalization.json has an unknown risk flag")
    if not isinstance(proposal_flags, list) or set(proposal_flags) - RISK_FLAGS:
        errors.append("domain-change-proposal.json has an unknown risk flag")
    elif isinstance(requirement_flags, list) and set(requirement_flags) - set(
        proposal_flags
    ):
        errors.append("domain-change-proposal.json must retain requirement risk flags")
    return errors


def _proposal_field_errors(proposal: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if proposal.get("status") not in PROPOSAL_STATUSES:
        errors.append("domain-change-proposal.json has an invalid status")
    revision = proposal.get("proposal_revision")
    if not isinstance(revision, int) or revision < 1:
        errors.append(
            "domain-change-proposal.json requires a positive proposal_revision"
        )
    affected_contexts = proposal.get("affected_contexts")
    if not affected_contexts or not completed_identifiers(affected_contexts):
        errors.append("domain-change-proposal.json requires affected_contexts")
    if not completed_identifiers(proposal.get("rule_ids", [])):
        errors.append("domain-change-proposal.json has invalid rule_ids")
    if not completed_identifiers(proposal.get("contract_ids", [])):
        errors.append("domain-change-proposal.json has invalid contract_ids")
    if superseded_ids(proposal) is None:
        errors.append(
            "domain-change-proposal.json supersedes must be a proposal id or a list of distinct proposal ids"
        )
    return errors


def superseded_ids(proposal: dict[str, Any]) -> set[str] | None:
    value = proposal.get("supersedes")
    if value is None:
        return set()
    if completed_identifier(value):
        return {value}
    if completed_identifiers(value) and value and len(set(value)) == len(value):
        return set(value)
    return None


def _requirement_and_proposal_errors(
    requirement: dict[str, Any], proposal: dict[str, Any]
) -> tuple[list[str], set[str]]:
    errors, criterion_ids = _acceptance_criteria(requirement)
    errors += _risk_flag_errors(requirement, proposal) + _proposal_field_errors(proposal)
    return errors, criterion_ids


def validate_change_package(
    root: Path, registry_root: Path | None, as_status: str | None = None, repo_root: Path | None = None
) -> list[str]:
    errors = [
        f"missing change package file: {root / name}"
        for name in CHANGE_PACKAGE_FILES
        if not (root / name).is_file()
    ]
    if errors:
        return errors
    documents = {name: load_json(root / name) for name in CHANGE_PACKAGE_FORMATS}
    if as_status is not None:
        documents = as_if_moved_to(as_status, documents)
    requirement, proposal, obligations, evidence = documents.values()
    errors.extend(_identity_errors(documents))
    proposal_errors, criterion_ids = _requirement_and_proposal_errors(
        requirement, proposal
    )
    errors.extend(proposal_errors)
    errors.extend(registry_update_errors(proposal))
    status = proposal.get("status")
    revision = proposal.get("proposal_revision")
    rule_ids = proposal.get("rule_ids", [])
    contract_ids = proposal.get("contract_ids", [])
    obligation_errors, obligation_ids = _obligation_errors(
        obligations,
        criterion_ids,
        rule_ids if isinstance(rule_ids, list) else [],
        contract_ids if isinstance(contract_ids, list) else [],
    )
    errors.extend(obligation_errors)
    errors.extend(implementation_design_errors(proposal, obligation_ids))
    errors.extend(
        _approval_and_revision_errors(requirement, proposal, evidence, status, revision)
    )
    errors.extend(_evidence_result_errors(evidence, obligation_ids))
    if status in VERIFIED_OR_LATER:
        errors.extend(test_attestation_errors(evidence, obligation_ids, registry_root))
        if proposal.get("change_classification") == "material":
            errors.extend(counterfactual_errors(evidence, obligation_ids, proposal))
    if status in APPROVED_OR_LATER:
        attestation = evidence.get("scm_attestation")
        if not isinstance(attestation, dict):
            errors.append("an approved or applied proposal requires SCM attestation")
        else:
            errors.extend(verify_scm_value(attestation, proposal))
    if registry_root is not None and status not in SETTLED_STATUSES:
        current = {name: asset_records(registry_root, name) for name in ASSET_KEYS}
        staged = _staged(current, proposal)
        errors.extend(_registry_reference_errors(staged, proposal))
        errors.extend(_staged_registry_errors(current, staged, proposal, repo_root))
    return errors


def _staged(current: dict[str, list[dict[str, Any]]], proposal: dict[str, Any]) -> dict[str, list[dict[str, Any]]]:
    records = {name: list(entries) for name, entries in current.items()}
    updates = proposal.get("registry_updates")
    for raw in updates if isinstance(updates, list) else []:
        try:
            update = parsed_update(raw)
        except ValueError:
            continue
        entries = records[update.document_name]
        entries[:] = [entry for entry in entries if entry.get("id") != update.record_id]
        if update.record is not None:
            entries.append(update.record)
    return records


def _staged_registry_errors(
    current: dict[str, list[dict[str, Any]]],
    staged: dict[str, list[dict[str, Any]]],
    proposal: dict[str, Any],
    repo_root: Path | None,
) -> list[str]:
    before = set(cross_reference_errors(current))
    errors = [
        f"once applied: {error}"
        for error in cross_reference_errors(staged)
        if error not in before
    ]
    if repo_root is not None:
        errors.extend(_updated_evidence_errors(proposal, repo_root))
        errors.extend(_decision_evidence_errors(proposal, repo_root))
    return errors


def _decision_evidence_errors(proposal: dict[str, Any], repo_root: Path) -> list[str]:
    errors = []
    for decision in DECISIONS_WITH_EVIDENCE:
        value = proposal.get(decision)
        evidence = value.get("evidence") if isinstance(value, dict) else None
        for item in evidence if isinstance(evidence, list) else []:
            path = _cited_file(item)
            if path and not (repo_root / path).is_file():
                errors.append(f"{decision} evidence names {path}, which is not a file in the repository")
    return errors


def _cited_file(item: Any) -> str | None:
    if isinstance(item, dict):
        return item.get("path") if isinstance(item.get("path"), str) else None
    match = CITED_FILE.match(item) if isinstance(item, str) else None
    return match["path"] if match and "/" in match["path"] else None


def obligations_proving(
    changes_root: Path, proposal_id: str, source: tuple[str, str]
) -> list[dict[str, Any]] | None:
    packages = [
        package for package in sorted(changes_root.iterdir())
        if (package / PROPOSAL_FILE).is_file() and load_json(package / PROPOSAL_FILE).get("proposal_id") == proposal_id
    ]
    if not packages:
        return None
    return [
        {"proposal_id": proposal_id, "package": package.name, "obligation_id": entry.get("id"), "check": entry.get("check")}
        for package in packages
        for entry in load_json(package / "test-obligations.json").get("obligations", [])
        if isinstance(entry, dict) and source in _covered_sources(entry)
    ]


def _covered_sources(obligation: dict[str, Any]) -> set[tuple[Any, Any]]:
    also = obligation.get("also_covers")
    return {
        (item.get("source_type"), item.get("source_id"))
        for item in [obligation, *(also if isinstance(also, list) else [])]
        if isinstance(item, dict)
    }


def list_proposals(changes_root: Path) -> list[dict[str, Any]]:
    rows = []
    replacements = {}
    for package in sorted(path for path in changes_root.iterdir() if (path / PROPOSAL_FILE).is_file()):
        try:
            proposal = load_json(package / PROPOSAL_FILE)
        except ValueError as error:
            rows.append({"package": package.name, "error": str(error)})
            continue
        updates = proposal.get("registry_updates")
        moments = [proposal.get(field) for field in PROPOSAL_MOMENTS]
        rows.append({
            "package": package.name,
            "proposal_id": proposal.get("proposal_id"),
            "status": proposal.get("status"),
            "last_changed_at": max((moment for moment in moments if isinstance(moment, str)), default=None),
            "registry_updates": len(updates) if isinstance(updates, list) else 0,
            "supersedes": proposal.get("supersedes", []),
        })
        if completed_identifier(proposal.get("superseded_by")):
            replacements[len(rows) - 1] = proposal["superseded_by"]
    known = Counter(row.get("proposal_id") for row in rows)
    for row in rows:
        if row.get("proposal_id") is not None and known[row["proposal_id"]] > 1:
            row["duplicate_proposal_id"] = True
    for index, replacement in replacements.items():
        rows[index]["superseded_by"] = replacement
        if replacement not in known:
            rows[index]["superseded_by_unknown"] = True
    for row in rows:
        unknown = sorted((superseded_ids(row) or set()) - set(known))
        if unknown:
            row["supersedes_unknown"] = unknown
    return rows


def _updated_evidence_errors(proposal: dict[str, Any], repo_root: Path) -> list[str]:
    updates = proposal.get("registry_updates")
    errors = []
    check = verifier(repo_root)
    for raw in updates if isinstance(updates, list) else []:
        if not isinstance(raw, dict) or not isinstance(raw.get("record"), dict):
            continue
        for reference in all_references(raw["record"]):
            result = check(reference)
            if result["status"] != "current":
                errors.append(
                    f"registry update {raw.get('asset')}/{raw['record'].get('id')} cites evidence that is "
                    f"{result['status']}: {result.get('reason') or result.get('path') or result.get('reference')}"
                )
    return errors
