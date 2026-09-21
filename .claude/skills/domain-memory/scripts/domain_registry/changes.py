import shutil
from pathlib import Path
from typing import Any

from .attestations import verify_scm_value
from .common import (
    CHANGE_CLASSIFICATIONS,
    CHANGE_PACKAGE_FILES,
    CHANGE_PACKAGE_FORMATS,
    OBLIGATION_SOURCE_TYPES,
    OBLIGATION_STATUSES,
    PROPOSAL_STATUSES,
    RISK_FLAGS,
    change_template_dir,
    completed_identifier,
    iso_timestamp,
    load_json,
)
from .policy import approved_command_profiles
from .registry import asset_records
from .revision import valid_registry_revision


def valid_digest(value: Any) -> bool:
    return (
        isinstance(value, str)
        and len(value) == 71
        and value.startswith("sha256:")
        and all(character in "0123456789abcdef" for character in value[7:])
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


def init_change_package(output: Path) -> None:
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"output directory is not empty: {output}")
    output.mkdir(parents=True, exist_ok=True)
    templates = change_template_dir()
    for name in CHANGE_PACKAGE_FILES:
        shutil.copyfile(templates / name, output / name)


def implementation_design_errors(
    proposal: dict[str, Any], obligation_ids: set[str]
) -> list[str]:
    classification = proposal.get("change_classification")
    if classification not in CHANGE_CLASSIFICATIONS:
        return ["domain-change-proposal.json has an invalid change_classification"]
    design = proposal.get("implementation_design")
    if not isinstance(design, dict):
        if classification == "material":
            return [
                "a material proposal requires implementation_design with domain forces, decision, invariants, alternatives, proof obligations, and counterfactual check"
            ]
        return []
    errors: list[str] = []
    for field in ("domain_forces", "invariants_preserved", "rejected_alternatives"):
        values = design.get(field)
        if not isinstance(values, list) or not all(
            completed_identifier(value) for value in values
        ):
            errors.append(f"implementation_design requires a valid {field} list")
    if not completed_identifier(design.get("decision")):
        errors.append("implementation_design requires a completed decision")
    proof_obligations = design.get("proof_obligations")
    if not isinstance(proof_obligations, list) or not all(
        completed_identifier(value) for value in proof_obligations
    ):
        errors.append("implementation_design requires proof obligation IDs")
    elif set(proof_obligations) - obligation_ids:
        errors.append("implementation_design references an unknown proof obligation")
    if not completed_identifier(design.get("counterfactual_check")):
        errors.append("implementation_design requires a counterfactual check")
    if classification == "material" and not proof_obligations:
        errors.append("a material proposal requires at least one proof obligation")
    return errors


def counterfactual_errors(
    evidence: dict[str, Any], obligation_ids: set[str]
) -> list[str]:
    check = evidence.get("counterfactual_check")
    if not isinstance(check, dict):
        return ["a material verified proposal requires counterfactual_check evidence"]
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
    errors: list[str] = []
    obligation_records = obligations.get("obligations")
    obligation_ids: set[str] = set()
    obligation_sources: set[tuple[str, str]] = set()
    if not isinstance(obligation_records, list) or not obligation_records:
        return ["test-obligations.json requires obligations"], obligation_ids
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
        source_type = obligation.get("source_type")
        if source_type not in OBLIGATION_SOURCE_TYPES:
            allowed = ", ".join(sorted(OBLIGATION_SOURCE_TYPES))
            errors.append(
                f"test obligation {obligation_id} has source_type {source_type!r}; a source_type names what the "
                f"obligation is derived from, not the kind of check, and must be one of {allowed}"
            )
        elif not completed_identifier(obligation.get("source_id")):
            errors.append(
                f"test obligation {obligation_id} has an incomplete source_id: {obligation.get('source_id')!r}"
            )
        else:
            obligation_sources.add((source_type, obligation["source_id"]))
        if not completed_identifier(
            obligation.get("expected_outcome")
        ) or not completed_identifier(obligation.get("check")):
            errors.append(
                f"test obligation {obligation_id} requires expected_outcome and check"
            )
        if obligation.get("status") not in OBLIGATION_STATUSES:
            errors.append(f"test obligation {obligation_id} has an invalid status")
    for criterion_id in criterion_ids:
        if ("acceptance-criterion", criterion_id) not in obligation_sources:
            errors.append(
                f"acceptance criterion {criterion_id} lacks a test obligation: "
                f"add one whose source_type is acceptance-criterion and whose source_id is {criterion_id}"
            )
    for rule_id in rule_ids:
        if ("rule", rule_id) not in obligation_sources:
            errors.append(f"rule {rule_id} lacks a test obligation")
    for contract_id in contract_ids:
        if ("contract", contract_id) not in obligation_sources:
            errors.append(f"contract {contract_id} lacks a test obligation")
    return errors, obligation_ids


def _approval_and_revision_errors(
    requirement: dict[str, Any],
    proposal: dict[str, Any],
    evidence: dict[str, Any],
    status: Any,
    revision: Any,
) -> list[str]:
    errors: list[str] = []
    required_roles = requirement.get("required_approval_roles", [])
    if not isinstance(required_roles, list) or not all(
        completed_identifier(role) for role in required_roles
    ):
        errors.append(
            "requirement-normalization.json has invalid required_approval_roles"
        )
    approvals = proposal.get("approvals", [])
    approved_roles: set[str] = set()
    if not isinstance(approvals, list):
        errors.append("domain-change-proposal.json has invalid approvals")
    else:
        for approval in approvals:
            if not isinstance(approval, dict):
                errors.append("domain-change-proposal.json has invalid approval entry")
                continue
            if (
                approval.get("decision") == "approved"
                and approval.get("proposal_revision") == revision
            ):
                if not all(
                    (
                        completed_identifier(approval.get("role")),
                        completed_identifier(approval.get("reviewer")),
                        iso_timestamp(approval.get("approved_at")),
                        completed_identifier(approval.get("scope")),
                    )
                ):
                    errors.append(
                        "an approval requires role, reviewer, approved_at, and scope"
                    )
                elif approval.get("base_registry_revision") != proposal.get(
                    "base_registry_revision"
                ):
                    errors.append(
                        "an approval must bind to the proposal base_registry_revision"
                    )
                else:
                    approved_roles.add(approval["role"])
    if (
        status in {"approved", "applied"}
        and set(required_roles if isinstance(required_roles, list) else [])
        - approved_roles
    ):
        errors.append("an approved or applied proposal lacks required approvals")
    base_revision = proposal.get("base_registry_revision")
    if status in {
        "submitted",
        "verified",
        "approved",
        "applied",
    } and not valid_registry_revision(base_revision):
        errors.append(
            "a submitted, approved, or applied proposal requires a Registry digest base revision"
        )
    if (
        status in {"submitted", "verified", "approved", "applied"}
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


def _registry_reference_errors(
    registry_root: Path,
    affected_contexts: Any,
    rule_ids: Any,
    contract_ids: Any,
) -> list[str]:
    errors: list[str] = []
    registry_contexts = {
        entry.get("id") for entry in asset_records(registry_root, "contexts.json")
    }
    registry_rules = {
        entry.get("id") for entry in asset_records(registry_root, "rules.json")
    }
    registry_contracts = {
        entry.get("id") for entry in asset_records(registry_root, "contracts.json")
    }
    if (
        isinstance(affected_contexts, list)
        and set(affected_contexts) - registry_contexts
    ):
        errors.append(
            "domain-change-proposal.json references an unknown registry context"
        )
    if isinstance(rule_ids, list) and set(rule_ids) - registry_rules:
        errors.append("domain-change-proposal.json references an unknown registry rule")
    if isinstance(contract_ids, list) and set(contract_ids) - registry_contracts:
        errors.append(
            "domain-change-proposal.json references an unknown registry contract"
        )
    return errors


def _load_change_documents(
    root: Path,
) -> tuple[dict[str, Path], dict[str, dict[str, Any]], list[str]]:
    paths = {name: root / name for name in CHANGE_PACKAGE_FILES}
    errors = [
        f"missing change package file: {path}"
        for path in paths.values()
        if not path.is_file()
    ]
    if errors:
        return paths, {}, errors
    documents = {
        "requirement": load_json(paths["requirement-normalization.json"]),
        "proposal": load_json(paths["domain-change-proposal.json"]),
        "obligations": load_json(paths["test-obligations.json"]),
        "evidence": load_json(paths["evidence-bundle.json"]),
    }
    return paths, documents, errors


def _identity_errors(
    documents: dict[str, dict[str, Any]],
) -> tuple[list[str], Any, Any]:
    requirement = documents["requirement"]
    proposal = documents["proposal"]
    obligations = documents["obligations"]
    evidence = documents["evidence"]
    errors: list[str] = []
    for name, value in (
        ("requirement-normalization.json", requirement),
        ("domain-change-proposal.json", proposal),
        ("test-obligations.json", obligations),
        ("evidence-bundle.json", evidence),
    ):
        if value.get("format") != CHANGE_PACKAGE_FORMATS[name]:
            errors.append(f"{name} has an invalid format")
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
    for name, value in (
        ("domain-change-proposal.json", proposal),
        ("test-obligations.json", obligations),
        ("evidence-bundle.json", evidence),
    ):
        if value.get("requirement_id") != requirement_id:
            errors.append(f"{name} must reference the requirement_id")
    for name, value in (
        ("test-obligations.json", obligations),
        ("evidence-bundle.json", evidence),
    ):
        if value.get("proposal_id") != proposal_id:
            errors.append(f"{name} must reference the proposal_id")
    return errors, requirement_id, proposal_id


def _requirement_and_proposal_errors(
    requirement: dict[str, Any], proposal: dict[str, Any]
) -> tuple[list[str], set[str], Any, Any, Any, Any, Any, Any]:
    errors: list[str] = []
    acceptance_criteria = requirement.get("acceptance_criteria")
    criterion_ids: set[str] = set()
    if not isinstance(acceptance_criteria, list) or not acceptance_criteria:
        errors.append("requirement-normalization.json requires acceptance_criteria")
    else:
        for criterion in acceptance_criteria:
            if (
                not isinstance(criterion, dict)
                or not completed_identifier(criterion.get("id"))
                or not completed_identifier(criterion.get("statement"))
            ):
                errors.append(
                    "each acceptance criterion requires a completed id and statement"
                )
                continue
            criterion_ids.add(criterion["id"])
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
    status = proposal.get("status")
    if status not in PROPOSAL_STATUSES:
        errors.append("domain-change-proposal.json has an invalid status")
    classification = proposal.get("change_classification")
    revision = proposal.get("proposal_revision")
    if not isinstance(revision, int) or revision < 1:
        errors.append(
            "domain-change-proposal.json requires a positive proposal_revision"
        )
    affected_contexts = proposal.get("affected_contexts")
    if (
        not isinstance(affected_contexts, list)
        or not affected_contexts
        or not all(completed_identifier(context) for context in affected_contexts)
    ):
        errors.append("domain-change-proposal.json requires affected_contexts")
    rule_ids = proposal.get("rule_ids", [])
    contract_ids = proposal.get("contract_ids", [])
    if not isinstance(rule_ids, list) or not all(
        completed_identifier(value) for value in rule_ids
    ):
        errors.append("domain-change-proposal.json has invalid rule_ids")
    if not isinstance(contract_ids, list) or not all(
        completed_identifier(value) for value in contract_ids
    ):
        errors.append("domain-change-proposal.json has invalid contract_ids")
    return (
        errors,
        criterion_ids,
        status,
        classification,
        revision,
        affected_contexts,
        rule_ids,
        contract_ids,
    )


def validate_change_package(root: Path, registry_root: Path | None) -> list[str]:
    _, documents, errors = _load_change_documents(root)
    if errors:
        return errors
    requirement = documents["requirement"]
    proposal = documents["proposal"]
    obligations = documents["obligations"]
    evidence = documents["evidence"]
    identity_errors, _, _ = _identity_errors(documents)
    errors.extend(identity_errors)
    (
        proposal_errors,
        criterion_ids,
        status,
        classification,
        revision,
        affected_contexts,
        rule_ids,
        contract_ids,
    ) = _requirement_and_proposal_errors(requirement, proposal)
    errors.extend(proposal_errors)
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
    if status in {"verified", "approved", "applied"}:
        errors.extend(test_attestation_errors(evidence, obligation_ids, registry_root))
    if classification == "material" and status in {"verified", "approved", "applied"}:
        errors.extend(counterfactual_errors(evidence, obligation_ids))
    if status in {"approved", "applied"}:
        attestation = evidence.get("scm_attestation")
        if not isinstance(attestation, dict):
            errors.append("an approved or applied proposal requires SCM attestation")
        else:
            errors.extend(verify_scm_value(attestation, proposal))
    if registry_root is not None:
        errors.extend(
            _registry_reference_errors(
                registry_root, affected_contexts, rule_ids, contract_ids
            )
        )
    return errors
