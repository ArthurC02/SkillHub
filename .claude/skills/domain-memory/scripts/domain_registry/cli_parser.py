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
SINGULAR_ASSETS = {
    asset.removesuffix("ies") + "y" if asset.endswith("ies") else asset.removesuffix("s"): asset for asset in ASSET_CHOICES
}


def add_path(parser: argparse.ArgumentParser, name: str, required: bool = True) -> None:  # noqa: FBT001, FBT002
    parser.add_argument(name, required=required, type=Path)


def add_asset(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--asset", required=True, choices=ASSET_CHOICES, type=asset_name)


def asset_name(text: str) -> str:
    return SINGULAR_ASSETS.get(text, text)


def query_text(text: str) -> str:
    if not text.strip():
        raise argparse.ArgumentTypeError("the query is empty")
    return text


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Initialize, validate, or assess a file-backed Domain Registry."
    )
    commands = parser.add_subparsers(dest="command", required=True, metavar="command")
    for add_commands in (
        _add_registry_commands, _add_evidence_commands, _add_record_and_source_commands, _add_governance_commands,
        _add_review_governance_commands, _add_proposal_commands, _add_proposal_check_commands,
        _add_history_commands, _add_harvest_command,
    ):
        add_commands(commands)
    return parser


def _add_change_package_init(commands: argparse._SubParsersAction) -> None:
    package_init_parser = commands.add_parser("init-change-package", help="Start a Change Package")
    add_path(package_init_parser, "--output")
    package_init_parser.add_argument("--requirement-id")
    package_init_parser.add_argument("--proposal-id")
    package_init_parser.add_argument("--proposer")
    add_path(package_init_parser, "--recitation", required=False)
    add_path(package_init_parser, "--registry-root", required=False)


def _add_export_commands(commands: argparse._SubParsersAction) -> None:
    export_parser = commands.add_parser("export", help="Read the whole Registry from another tool")
    add_path(export_parser, "--registry-root")
    add_path(export_parser, "--repo-root", required=False)
    export_parser.add_argument("--commit")
    compare_parser = commands.add_parser("compare-registry", help="Score an extracted Registry")
    add_path(compare_parser, "--gold-root")
    add_path(compare_parser, "--candidate-root")
    add_path(compare_parser, "--repo-root", required=False)
    compare_parser.add_argument("--gold-commit")
    compare_parser.add_argument("--context", action="append", default=[])


def _add_harvest_command(commands: argparse._SubParsersAction) -> None:
    harvest_parser = commands.add_parser("harvest-candidates", help="Harvest a scratch extraction")
    add_path(harvest_parser, "--registry-root")
    add_path(harvest_parser, "--repo-root")
    add_path(harvest_parser, "--from-root")
    harvest_parser.add_argument(
        "--context-map", action="append", default=[], help="repeatable CANDIDATE=GOLD, over the pairing found by path or name"
    )
    harvest_parser.add_argument("--keep-duplicates", action="store_true")


def _add_history_commands(commands: argparse._SubParsersAction) -> None:
    coupling_parser = commands.add_parser("change-coupling", help="See which modules change together")
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
    init_parser = commands.add_parser("init", help="Create an empty Registry skeleton, not the entry point")
    add_path(init_parser, "--output")
    migrate_parser = commands.add_parser("migrate-registry", help="Move a JSON Registry into concept files")
    add_path(migrate_parser, "--registry-root")
    _add_change_package_init(commands)
    redraft_parser = commands.add_parser("redraft-proposal", help="Redraft a retired proposal")
    add_path(redraft_parser, "--package-root")
    add_path(redraft_parser, "--output")
    redraft_parser.add_argument("--proposal-id", required=True)
    validate_parser = commands.add_parser("validate", help="Validate the Registry")
    add_path(validate_parser, "--registry-root")
    add_path(validate_parser, "--repo-root", required=False)
    validate_parser.add_argument("--require-reviewed", action="store_true")
    _add_export_commands(commands)
    coverage_parser = commands.add_parser("coverage", help="Report modelling gaps")
    add_path(coverage_parser, "--registry-root")
    readiness_parser = commands.add_parser("readiness", help="Assess project readiness")
    add_path(readiness_parser, "--repo-root")
    add_path(readiness_parser, "--registry-root", required=False)
    lookup_parser = commands.add_parser("lookup", help="Search an asset")
    add_path(lookup_parser, "--registry-root")
    add_asset(lookup_parser)
    lookup_parser.add_argument("--query", required=True, type=query_text)
    term_parser = commands.add_parser("resolve-terms", help="Resolve vocabulary")
    add_path(term_parser, "--registry-root")
    term_parser.add_argument("--query", required=True, action="append", type=query_text)
    term_parser.add_argument("--context")
    term_parser.add_argument("--with-mentions", action="store_true")
    context_parser = commands.add_parser("get-context", help="Retrieve a Context model")
    add_path(context_parser, "--registry-root")
    context_parser.add_argument("--id", required=True)
    context_parser.add_argument("--summary", action="store_true", help="ids and one line each, without evidence")
    record_parser = commands.add_parser("get-record", help="Retrieve a record")
    add_path(record_parser, "--registry-root")
    add_asset(record_parser)
    record_parser.add_argument("--id", required=True)
    boundary_parser = commands.add_parser("analyze-boundary", help="Inspect a boundary")
    add_path(boundary_parser, "--registry-root")
    boundary_parser.add_argument("--source-context", required=True)
    boundary_parser.add_argument("--target-context", required=True)
    impact_parser = commands.add_parser("boundary-impact", help="Find boundaries a change touches")
    add_path(impact_parser, "--registry-root")
    add_path(impact_parser, "--repo-root")
    impact_parser.add_argument("--base", required=True)
    relocate_parser = commands.add_parser("relocate-evidence", help="Re-cite evidence after a move")
    add_path(relocate_parser, "--registry-root")
    add_path(relocate_parser, "--repo-root")
    relocate_parser.add_argument("--from", dest="moved_from", required=True)
    relocate_parser.add_argument("--to", dest="moved_to", required=True)


def _add_evidence_commands(commands: argparse._SubParsersAction) -> None:
    evidence_verify_parser = commands.add_parser("verify-evidence", help="Verify citations")
    add_path(evidence_verify_parser, "--registry-root")
    add_path(evidence_verify_parser, "--repo-root")
    evidence_verify_parser.add_argument("--path", action="append")
    evidence_verify_parser.add_argument("--strict", action="store_true")
    recite_parser = commands.add_parser("recite", help="Re-cite one changed citation")
    add_path(recite_parser, "--registry-root")
    add_path(recite_parser, "--repo-root")
    add_asset(recite_parser)
    recite_parser.add_argument("--id", required=True)
    recite_parser.add_argument("--path", required=True)
    recite_parser.add_argument("--lines")
    recite_parser.add_argument("--replace")
    add_path(recite_parser, "--onto", required=False)


def _add_record_and_source_commands(commands: argparse._SubParsersAction) -> None:
    cite_parser = commands.add_parser("cite", help="Build an evidence citation")
    add_path(cite_parser, "--repo-root")
    cite_parser.add_argument("--path", required=True)
    cite_parser.add_argument("--lines", help="START-END, or --start and --end")
    cite_parser.add_argument("--start", type=int)
    cite_parser.add_argument("--end", type=int)
    add_path(cite_parser, "--registry-root", required=False)
    cite_parser.add_argument("--show", action="store_true")
    candidate_update_parser = commands.add_parser("upsert-candidate", help="Add or revise a candidate")
    add_path(candidate_update_parser, "--registry-root")
    add_path(candidate_update_parser, "--repo-root")
    add_asset(candidate_update_parser)
    candidate_update_parser.add_argument(
        "--record-file", required=True, action="append", type=Path, help="repeatable: one record per file"
    )
    retract_parser = commands.add_parser("retract-candidate", help="Withdraw a candidate")
    add_path(retract_parser, "--registry-root")
    add_path(retract_parser, "--repo-root")
    add_asset(retract_parser)
    retract_parser.add_argument("--id", required=True)
    retract_parser.add_argument("--reason", required=True)
    promote_parser = commands.add_parser("promote-candidate", help="Put a candidate into a draft proposal")
    add_path(promote_parser, "--package-root")
    add_path(promote_parser, "--registry-root")
    add_asset(promote_parser)
    promote_parser.add_argument("--id", required=True)
    apply_updates_parser = commands.add_parser("apply-approved-updates", help="Apply an approved change")
    add_path(apply_updates_parser, "--package-root")
    add_path(apply_updates_parser, "--registry-root")
    add_path(apply_updates_parser, "--repo-root")
    demote_parser = commands.add_parser("demote-local-reviews", help="Withdraw local reviewed status")
    add_path(demote_parser, "--registry-root")
    add_path(demote_parser, "--repo-root")
    source_parser = commands.add_parser("discover-sources", help="Map repository sources")
    add_path(source_parser, "--repo-root")
    add_path(source_parser, "--output", required=False)
    source_verify_parser = commands.add_parser("verify-sources", help="Verify selected sources")
    add_path(source_verify_parser, "--repo-root")
    add_path(source_verify_parser, "--source-map")
    add_path(source_verify_parser, "--policy", required=False)
    probe_parser = commands.add_parser("probe", help="Report Registry health")
    add_path(probe_parser, "--repo-root")
    add_path(probe_parser, "--registry-root")
    add_path(probe_parser, "--policy", required=False)
    evidence_migrate_parser = commands.add_parser("migrate-evidence", help="Upgrade legacy citations")
    add_path(evidence_migrate_parser, "--registry-root")
    add_path(evidence_migrate_parser, "--repo-root")
    recovery_parser = commands.add_parser("recover-registry-update", help="Recover an interrupted update")
    add_path(recovery_parser, "--registry-root")
    add_path(recovery_parser, "--repo-root", required=False)
    recovery_parser.add_argument("--force", action="store_true")


def _add_governance_commands(commands: argparse._SubParsersAction) -> None:
    memory_init_parser = commands.add_parser("init-domain-memory", help="Start a Registry from human choices: the entry point")
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
    confirm_parser = commands.add_parser("confirm-sources", help="Confirm the selected sources")
    add_path(confirm_parser, "--registry-root")
    add_path(confirm_parser, "--repo-root")
    confirm_parser.add_argument("--confirmed-by", required=True)
    refresh_parser = commands.add_parser("refresh-sources", help="Re-snapshot the selected sources")
    add_path(refresh_parser, "--registry-root")
    add_path(refresh_parser, "--repo-root")
    refine_parser = commands.add_parser("refine-sources", help="Focus selected sources")
    add_path(refine_parser, "--registry-root")
    add_path(refine_parser, "--repo-root")
    refine_parser.add_argument("--source", action="append", default=[], type=Path, help="repeatable")
    refine_parser.add_argument("--add", action="append", default=[], type=Path)
    refine_parser.add_argument("--remove", action="append", default=[], type=Path)
    amend_parser = commands.add_parser("amend-policy", help="Change governance fields")
    add_path(amend_parser, "--registry-root")
    amend_parser.add_argument("--field", choices=sorted(AMENDABLE_FIELDS))
    amend_parser.add_argument("--value")
    amend_parser.add_argument("--set", action="append", default=[], metavar="FIELD=VALUE")
    amend_parser.add_argument("--reason", required=True)
    amend_parser.add_argument("--verifier")


def _add_review_governance_commands(commands: argparse._SubParsersAction) -> None:
    signing_parser = commands.add_parser("init-signing-key", help="Prepare a signing identity")
    add_path(signing_parser, "--repo-root")
    signing_parser.add_argument("--principal", required=True)
    add_path(signing_parser, "--key-file", required=False)
    signing_parser.add_argument("--force", action="store_true")
    signing_parser.add_argument("--sign-every-commit", action="store_true")
    policy_parser = commands.add_parser("validate-policy", help="Check a policy before adopting it")
    add_path(policy_parser, "--policy")
    secret_parser = commands.add_parser("scan-secrets", help="Scan for secrets")
    add_path(secret_parser, "--repo-root")
    contract_parser = commands.add_parser("validate-contract", help="Check a contract schema file")
    add_path(contract_parser, "--schema")
    scm_parser = commands.add_parser("verify-scm-attestation", help="Check SCM evidence alone")
    add_path(scm_parser, "--attestation")
    add_path(scm_parser, "--package-root")
    git_governance_parser = commands.add_parser("verify-git-governance", help="Check one commit's signature")
    add_path(git_governance_parser, "--registry-root")
    add_path(git_governance_parser, "--repo-root")
    git_governance_parser.add_argument("--commit", required=True)
    hook_parser = commands.add_parser("install-git-hitl-hook", help="Enforce signatures on push")
    add_path(hook_parser, "--registry-root")
    add_path(hook_parser, "--repo-root")
    governance_parser = commands.add_parser("governance-readiness", help="Report what governance still needs")
    add_path(governance_parser, "--registry-root")
    add_path(governance_parser, "--repo-root")
    audit_parser = commands.add_parser("verify-audit", help="Verify audit trail")
    add_path(audit_parser, "--registry-root")
    add_path(audit_parser, "--changes-root", required=False)
    audit_parser.add_argument("--against")


def _add_proposal_commands(commands: argparse._SubParsersAction) -> None:
    submit_parser = commands.add_parser("submit-proposal", help="Submit for review")
    add_path(submit_parser, "--package-root")
    add_path(submit_parser, "--registry-root")
    add_path(submit_parser, "--repo-root")
    approval_parser = commands.add_parser("record-approval", help="Record human approval")
    add_path(approval_parser, "--package-root")
    approval_parser.add_argument("--role", required=True)
    approval_parser.add_argument("--reviewer", required=True)
    approval_parser.add_argument("--scope", required=True)
    approval_parser.add_argument("--approved-at")
    supersede_parser = commands.add_parser("supersede-proposal", help="Retire a proposal")
    add_path(supersede_parser, "--package-root")
    supersede_parser.add_argument("--reason", required=True)
    supersede_parser.add_argument("--superseded-by")
    verify_parser = commands.add_parser("verify-proposal", help="Verify test attestations")
    add_path(verify_parser, "--package-root")
    add_path(verify_parser, "--registry-root")
    add_path(verify_parser, "--repo-root")
    result_parser = commands.add_parser("record-test-result", help="Record a test result")
    add_path(result_parser, "--package-root")
    add_path(result_parser, "--registry-root")
    add_path(result_parser, "--repo-root")
    result_parser.add_argument("--obligation", required=True)
    result_parser.add_argument("--command", dest="obligation_command", required=True)
    result_parser.add_argument("--command-profile", required=True)
    result_parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS)
    result_parser.add_argument("--cwd")
    attest_parser = commands.add_parser("attest-signed-commit", help="Attest a signed commit")
    add_path(attest_parser, "--package-root")
    add_path(attest_parser, "--registry-root")
    add_path(attest_parser, "--repo-root")
    attest_parser.add_argument("--commit", required=True)


def _add_proposal_check_commands(commands: argparse._SubParsersAction) -> None:
    audit_attestations_parser = commands.add_parser("audit-attestations", help="Audit past attestations")
    add_path(audit_attestations_parser, "--repo-root")
    add_path(audit_attestations_parser, "--changes-root")
    audit_attestations_parser.add_argument("--verification-token-env", default="GITHUB_TOKEN")
    finalize_parser = commands.add_parser("finalize-proposal", help="Finalize approval")
    add_path(finalize_parser, "--package-root")
    add_path(finalize_parser, "--registry-root", required=False)
    add_path(finalize_parser, "--repo-root", required=False)
    finalize_parser.add_argument("--verification-token-env", default="GITHUB_TOKEN")
    gates_parser = commands.add_parser("quality-gates", help="Find the repository's own checks")
    add_path(gates_parser, "--repo-root")
    counterfactual_parser = commands.add_parser("counterfactual", help="Break one protection and restore it")
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
    proposals_parser = commands.add_parser("list-proposals", help="List the Change Packages")
    add_path(proposals_parser, "--changes-root")
    package_validate_parser = commands.add_parser("validate-change-package", help="Validate a Change Package")
    add_path(package_validate_parser, "--package-root")
    add_path(package_validate_parser, "--registry-root", required=False)
    package_validate_parser.add_argument("--as-status", choices=PREVIEW_STATUSES)
    add_path(package_validate_parser, "--repo-root", required=False)
