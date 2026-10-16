import argparse
from pathlib import Path

from .changes import PREVIEW_STATUSES
from .common import ASSET_KEYS
from .counterfactual import DEFAULT_TIMEOUT_SECONDS
from .policy import (
    AMENDABLE_FIELDS,
    CI_REQUIREMENTS,
    CLASSIFICATIONS,
    GOVERNANCE_TRIGGERS,
    GOVERNANCE_VERIFIERS,
    REVIEW_MODES,
    STORAGE_MODES,
)

ASSET_CHOICES = [name.removesuffix(".json") for name in ASSET_KEYS]


def add_path(parser: argparse.ArgumentParser, name: str, required: bool = True) -> None:  # noqa: FBT001, FBT002
    parser.add_argument(name, required=required, type=Path)


def add_asset(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--asset", required=True, choices=ASSET_CHOICES)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Initialize, validate, or assess a file-backed Domain Registry."
    )
    commands = parser.add_subparsers(dest="command", required=True)
    for add_commands in (
        _add_registry_commands, _add_evidence_commands, _add_record_and_source_commands, _add_governance_commands,
        _add_review_governance_commands, _add_proposal_commands, _add_proposal_check_commands,
        _add_history_commands, _add_harvest_command,
    ):
        add_commands(commands)
    return parser


def _add_change_package_init(commands: argparse._SubParsersAction) -> None:
    package_init_parser = commands.add_parser("init-change-package")
    add_path(package_init_parser, "--output")
    package_init_parser.add_argument("--requirement-id")
    package_init_parser.add_argument("--proposal-id")
    package_init_parser.add_argument("--proposer")
    add_path(package_init_parser, "--recitation", required=False)
    add_path(package_init_parser, "--registry-root", required=False)


def _add_export_commands(commands: argparse._SubParsersAction) -> None:
    export_parser = commands.add_parser("export")
    add_path(export_parser, "--registry-root")
    add_path(export_parser, "--repo-root", required=False)
    export_parser.add_argument("--commit")
    compare_parser = commands.add_parser("compare-registry")
    add_path(compare_parser, "--gold-root")
    add_path(compare_parser, "--candidate-root")
    add_path(compare_parser, "--repo-root", required=False)
    compare_parser.add_argument("--gold-commit")
    compare_parser.add_argument("--context", action="append", default=[])


def _add_harvest_command(commands: argparse._SubParsersAction) -> None:
    harvest_parser = commands.add_parser("harvest-candidates")
    add_path(harvest_parser, "--registry-root")
    add_path(harvest_parser, "--repo-root")
    add_path(harvest_parser, "--from-root")
    harvest_parser.add_argument(
        "--context-map", action="append", default=[], help="repeatable CANDIDATE=GOLD, over the pairing found by path or name"
    )
    harvest_parser.add_argument("--keep-duplicates", action="store_true")


def _add_history_commands(commands: argparse._SubParsersAction) -> None:
    coupling_parser = commands.add_parser("change-coupling")
    add_path(coupling_parser, "--repo-root")
    coupling_parser.add_argument("--root", action="append", default=[])
    coupling_parser.add_argument("--depth", type=int, default=2)
    coupling_parser.add_argument("--since")
    coupling_parser.add_argument("--exclude-subject", action="append", default=[], help="repeatable regex")
    coupling_parser.add_argument("--max-modules", type=int)
    add_path(coupling_parser, "--registry-root", required=False)
    for option, default in (("--min-revisions", 5), ("--min-shared", 5), ("--min-degree", 30), ("--max-changeset", 30)):
        coupling_parser.add_argument(option, type=int, default=default)


def _add_registry_commands(commands: argparse._SubParsersAction) -> None:
    init_parser = commands.add_parser("init")
    add_path(init_parser, "--output")
    migrate_parser = commands.add_parser("migrate-registry")
    add_path(migrate_parser, "--registry-root")
    _add_change_package_init(commands)
    redraft_parser = commands.add_parser("redraft-proposal")
    add_path(redraft_parser, "--package-root")
    add_path(redraft_parser, "--output")
    redraft_parser.add_argument("--proposal-id", required=True)
    validate_parser = commands.add_parser("validate")
    add_path(validate_parser, "--registry-root")
    add_path(validate_parser, "--repo-root", required=False)
    validate_parser.add_argument("--require-reviewed", action="store_true")
    _add_export_commands(commands)
    coverage_parser = commands.add_parser("coverage")
    add_path(coverage_parser, "--registry-root")
    readiness_parser = commands.add_parser("readiness")
    add_path(readiness_parser, "--repo-root")
    add_path(readiness_parser, "--registry-root", required=False)
    lookup_parser = commands.add_parser("lookup")
    add_path(lookup_parser, "--registry-root")
    add_asset(lookup_parser)
    lookup_parser.add_argument("--query", required=True)
    term_parser = commands.add_parser("resolve-terms")
    add_path(term_parser, "--registry-root")
    term_parser.add_argument("--query", required=True, action="append")
    term_parser.add_argument("--context")
    term_parser.add_argument("--with-mentions", action="store_true")
    context_parser = commands.add_parser("get-context")
    add_path(context_parser, "--registry-root")
    context_parser.add_argument("--id", required=True)
    context_parser.add_argument("--summary", action="store_true", help="ids and one line each, without evidence")
    record_parser = commands.add_parser("get-record")
    add_path(record_parser, "--registry-root")
    add_asset(record_parser)
    record_parser.add_argument("--id", required=True)
    boundary_parser = commands.add_parser("analyze-boundary")
    add_path(boundary_parser, "--registry-root")
    boundary_parser.add_argument("--source-context", required=True)
    boundary_parser.add_argument("--target-context", required=True)
    impact_parser = commands.add_parser("boundary-impact")
    add_path(impact_parser, "--registry-root")
    add_path(impact_parser, "--repo-root")
    impact_parser.add_argument("--base", required=True)
    relocate_parser = commands.add_parser("relocate-evidence")
    add_path(relocate_parser, "--registry-root")
    add_path(relocate_parser, "--repo-root")
    relocate_parser.add_argument("--from", dest="moved_from", required=True)
    relocate_parser.add_argument("--to", dest="moved_to", required=True)


def _add_evidence_commands(commands: argparse._SubParsersAction) -> None:
    evidence_verify_parser = commands.add_parser("verify-evidence")
    add_path(evidence_verify_parser, "--registry-root")
    add_path(evidence_verify_parser, "--repo-root")
    evidence_verify_parser.add_argument("--path", action="append")
    evidence_verify_parser.add_argument("--strict", action="store_true")
    recite_parser = commands.add_parser("recite")
    add_path(recite_parser, "--registry-root")
    add_path(recite_parser, "--repo-root")
    add_asset(recite_parser)
    recite_parser.add_argument("--id", required=True)
    recite_parser.add_argument("--path", required=True)
    recite_parser.add_argument("--lines")
    recite_parser.add_argument("--replace")
    add_path(recite_parser, "--onto", required=False)


def _add_record_and_source_commands(commands: argparse._SubParsersAction) -> None:
    cite_parser = commands.add_parser("cite")
    add_path(cite_parser, "--repo-root")
    cite_parser.add_argument("--path", required=True)
    cite_parser.add_argument("--start", required=True, type=int)
    cite_parser.add_argument("--end", required=True, type=int)
    add_path(cite_parser, "--registry-root", required=False)
    cite_parser.add_argument("--show", action="store_true")
    candidate_update_parser = commands.add_parser("upsert-candidate")
    add_path(candidate_update_parser, "--registry-root")
    add_path(candidate_update_parser, "--repo-root")
    add_asset(candidate_update_parser)
    candidate_update_parser.add_argument(
        "--record-file", required=True, action="append", type=Path, help="repeatable: one record per file"
    )
    retract_parser = commands.add_parser("retract-candidate")
    add_path(retract_parser, "--registry-root")
    add_path(retract_parser, "--repo-root")
    add_asset(retract_parser)
    retract_parser.add_argument("--id", required=True)
    retract_parser.add_argument("--reason", required=True)
    promote_parser = commands.add_parser("promote-candidate")
    add_path(promote_parser, "--package-root")
    add_path(promote_parser, "--registry-root")
    add_asset(promote_parser)
    promote_parser.add_argument("--id", required=True)
    apply_updates_parser = commands.add_parser("apply-approved-updates")
    add_path(apply_updates_parser, "--package-root")
    add_path(apply_updates_parser, "--registry-root")
    add_path(apply_updates_parser, "--repo-root")
    demote_parser = commands.add_parser("demote-local-reviews")
    add_path(demote_parser, "--registry-root")
    add_path(demote_parser, "--repo-root")
    source_parser = commands.add_parser("discover-sources")
    add_path(source_parser, "--repo-root")
    add_path(source_parser, "--output", required=False)
    source_verify_parser = commands.add_parser("verify-sources")
    add_path(source_verify_parser, "--repo-root")
    add_path(source_verify_parser, "--source-map")
    add_path(source_verify_parser, "--policy", required=False)
    probe_parser = commands.add_parser("probe")
    add_path(probe_parser, "--repo-root")
    add_path(probe_parser, "--registry-root")
    add_path(probe_parser, "--policy", required=False)
    evidence_migrate_parser = commands.add_parser("migrate-evidence")
    add_path(evidence_migrate_parser, "--registry-root")
    add_path(evidence_migrate_parser, "--repo-root")
    recovery_parser = commands.add_parser("recover-registry-update")
    add_path(recovery_parser, "--registry-root")
    add_path(recovery_parser, "--repo-root", required=False)
    recovery_parser.add_argument("--force", action="store_true")


def _add_governance_commands(commands: argparse._SubParsersAction) -> None:
    memory_init_parser = commands.add_parser("init-domain-memory")
    add_path(memory_init_parser, "--repo-root")
    add_path(memory_init_parser, "--output")
    memory_init_parser.add_argument(
        "--source", required=True, action="append", type=Path, help="repeatable"
    )
    memory_init_parser.add_argument(
        "--storage-mode", required=True, choices=STORAGE_MODES
    )
    memory_init_parser.add_argument(
        "--data-classification",
        required=True,
        choices=CLASSIFICATIONS,
    )
    memory_init_parser.add_argument(
        "--review-mode", required=True, choices=REVIEW_MODES
    )
    memory_init_parser.add_argument("--source-authority", required=True)
    memory_init_parser.add_argument(
        "--review-verifier", choices=GOVERNANCE_VERIFIERS
    )
    memory_init_parser.add_argument(
        "--review-trigger", choices=GOVERNANCE_TRIGGERS
    )
    memory_init_parser.add_argument(
        "--ci-requirement", choices=CI_REQUIREMENTS, default="none"
    )
    memory_init_parser.add_argument("--authorized-signer", action="append")
    memory_init_parser.add_argument("--include", action="append")
    memory_init_parser.add_argument("--exclude", action="append")
    confirm_parser = commands.add_parser("confirm-sources")
    add_path(confirm_parser, "--registry-root")
    add_path(confirm_parser, "--repo-root")
    confirm_parser.add_argument("--confirmed-by", required=True)
    refresh_parser = commands.add_parser("refresh-sources")
    add_path(refresh_parser, "--registry-root")
    add_path(refresh_parser, "--repo-root")
    refine_parser = commands.add_parser("refine-sources")
    add_path(refine_parser, "--registry-root")
    add_path(refine_parser, "--repo-root")
    refine_parser.add_argument("--source", action="append", default=[], type=Path, help="repeatable")
    refine_parser.add_argument("--add", action="append", default=[], type=Path)
    refine_parser.add_argument("--remove", action="append", default=[], type=Path)
    amend_parser = commands.add_parser("amend-policy")
    add_path(amend_parser, "--registry-root")
    amend_parser.add_argument("--field", choices=sorted(AMENDABLE_FIELDS))
    amend_parser.add_argument("--value")
    amend_parser.add_argument("--set", action="append", default=[], metavar="FIELD=VALUE")
    amend_parser.add_argument("--reason", required=True)
    amend_parser.add_argument("--verifier")


def _add_review_governance_commands(commands: argparse._SubParsersAction) -> None:
    signing_parser = commands.add_parser("init-signing-key")
    add_path(signing_parser, "--repo-root")
    signing_parser.add_argument("--principal", required=True)
    add_path(signing_parser, "--key-file", required=False)
    signing_parser.add_argument("--force", action="store_true")
    signing_parser.add_argument("--sign-every-commit", action="store_true")
    policy_parser = commands.add_parser("validate-policy")
    add_path(policy_parser, "--policy")
    secret_parser = commands.add_parser("scan-secrets")
    add_path(secret_parser, "--repo-root")
    contract_parser = commands.add_parser("validate-contract")
    add_path(contract_parser, "--schema")
    scm_parser = commands.add_parser("verify-scm-attestation")
    add_path(scm_parser, "--attestation")
    add_path(scm_parser, "--package-root")
    git_governance_parser = commands.add_parser("verify-git-governance")
    add_path(git_governance_parser, "--registry-root")
    add_path(git_governance_parser, "--repo-root")
    git_governance_parser.add_argument("--commit", required=True)
    hook_parser = commands.add_parser("install-git-hitl-hook")
    add_path(hook_parser, "--registry-root")
    add_path(hook_parser, "--repo-root")
    governance_parser = commands.add_parser("governance-readiness")
    add_path(governance_parser, "--registry-root")
    add_path(governance_parser, "--repo-root")
    audit_parser = commands.add_parser("verify-audit")
    add_path(audit_parser, "--registry-root")
    add_path(audit_parser, "--changes-root", required=False)
    audit_parser.add_argument("--against")


def _add_proposal_commands(commands: argparse._SubParsersAction) -> None:
    submit_parser = commands.add_parser("submit-proposal")
    add_path(submit_parser, "--package-root")
    add_path(submit_parser, "--registry-root")
    add_path(submit_parser, "--repo-root")
    approval_parser = commands.add_parser("record-approval")
    add_path(approval_parser, "--package-root")
    approval_parser.add_argument("--role", required=True)
    approval_parser.add_argument("--reviewer", required=True)
    approval_parser.add_argument("--scope", required=True)
    approval_parser.add_argument("--approved-at")
    supersede_parser = commands.add_parser("supersede-proposal")
    add_path(supersede_parser, "--package-root")
    supersede_parser.add_argument("--reason", required=True)
    supersede_parser.add_argument("--superseded-by")
    verify_parser = commands.add_parser("verify-proposal")
    add_path(verify_parser, "--package-root")
    add_path(verify_parser, "--registry-root")
    add_path(verify_parser, "--repo-root")
    result_parser = commands.add_parser("record-test-result")
    add_path(result_parser, "--package-root")
    add_path(result_parser, "--registry-root")
    add_path(result_parser, "--repo-root")
    result_parser.add_argument("--obligation", required=True)
    result_parser.add_argument("--command", dest="obligation_command", required=True)
    result_parser.add_argument("--command-profile", required=True)
    result_parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS)
    result_parser.add_argument("--cwd")
    attest_parser = commands.add_parser("attest-signed-commit")
    add_path(attest_parser, "--package-root")
    add_path(attest_parser, "--registry-root")
    add_path(attest_parser, "--repo-root")
    attest_parser.add_argument("--commit", required=True)


def _add_proposal_check_commands(commands: argparse._SubParsersAction) -> None:
    audit_attestations_parser = commands.add_parser("audit-attestations")
    add_path(audit_attestations_parser, "--repo-root")
    add_path(audit_attestations_parser, "--changes-root")
    audit_attestations_parser.add_argument("--verification-token-env", default="GITHUB_TOKEN")
    finalize_parser = commands.add_parser("finalize-proposal")
    add_path(finalize_parser, "--package-root")
    add_path(finalize_parser, "--registry-root", required=False)
    add_path(finalize_parser, "--repo-root", required=False)
    finalize_parser.add_argument("--verification-token-env", default="GITHUB_TOKEN")
    gates_parser = commands.add_parser("quality-gates")
    add_path(gates_parser, "--repo-root")
    counterfactual_parser = commands.add_parser("counterfactual")
    add_path(counterfactual_parser, "--repo-root")
    add_path(counterfactual_parser, "--file")
    counterfactual_parser.add_argument("--find", required=True)
    counterfactual_parser.add_argument("--replace", required=True)
    counterfactual_parser.add_argument("--test-command", required=True)
    counterfactual_parser.add_argument(
        "--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS
    )
    counterfactual_parser.add_argument("--cwd")
    counterfactual_parser.add_argument("--expect")
    counterfactual_parser.add_argument("--obligation-id")
    proposals_parser = commands.add_parser("list-proposals")
    add_path(proposals_parser, "--changes-root")
    package_validate_parser = commands.add_parser("validate-change-package")
    add_path(package_validate_parser, "--package-root")
    add_path(package_validate_parser, "--registry-root", required=False)
    package_validate_parser.add_argument("--as-status", choices=PREVIEW_STATUSES)
    add_path(package_validate_parser, "--repo-root", required=False)
