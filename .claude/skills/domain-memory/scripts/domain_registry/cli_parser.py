import argparse
import textwrap
from pathlib import Path
from typing import Any, NoReturn

from .changes import PREVIEW_STATUSES
from .common import ASSET_KEYS
from .counterfactual import DEFAULT_TIMEOUT_SECONDS
from .harvest_run import STAGES as HARVEST_STAGES
from .policy import (
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
COMMAND_GROUPS = {
    "everyday": (
        "readiness", "probe", "lookup", "resolve-terms", "get-context", "get-record", "dependents", "describe-table", "brief",
        "analyze-boundary", "boundary-impact", "validate", "coverage", "verify-evidence", "verify-sources",
        "verify-audit", "reconcile-audit", "refresh-indexes", "cite", "recite", "relocate-evidence", "upsert-candidate", "retract-candidate",
        "promote-candidate", "harvest", "discard-harvest", "init-change-package", "validate-change-package", "submit-proposal",
        "record-test-result", "record-approval", "verify-proposal", "rebase-proposal", "ship-package", "supersede-proposal",
        "list-proposals", "quality-gates", "counterfactual",
    ),
    "advanced": (
        "discover-sources", "init-domain-memory", "confirm-sources", "refine-sources", "refresh-sources", "refresh-listing",
        "recover-registry-update", "demote-local-reviews", "redraft-proposal",
        "export", "change-coupling", "compare-registry", "harvest-candidates", "extract-sql-schema", "governance-readiness",
        "init-signing-key", "amend-policy", "install-git-hitl-hook", "install-pre-commit-hook", "verify-git-governance", "validate-policy",
        "verify-scm-attestation", "audit-attestations", "scan-secrets", "validate-contract", "mcp-serve",
    ),
}


COMMAND_ORDER = {name: position for position, name in enumerate(name for names in COMMAND_GROUPS.values() for name in names)}


def command_groups_text() -> str:
    return "\n\n".join(
        textwrap.fill(
            f"{group.capitalize()} commands: {', '.join(names)}.", width=100, subsequent_indent="  ",
            break_on_hyphens=False,
        )
        for group, names in COMMAND_GROUPS.items()
    )


def add_path(parser: argparse.ArgumentParser, name: str, required: bool = True) -> None:  # noqa: FBT001, FBT002
    parser.add_argument(name, required=required, type=Path)


def add_profile_flags(parser: argparse.ArgumentParser) -> None:
    for flag in ("--implementation-root", "--excluded-directory", "--decision-root", "--boundary-marker"):
        parser.add_argument(flag, action="append", help="repeatable")


def add_lifecycle_roots(parser: argparse.ArgumentParser) -> None:
    add_path(parser, "--registry-root", required=False)
    add_path(parser, "--repo-root", required=False)


def add_asset(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--asset", required=True, choices=ASSET_CHOICES, type=asset_name)


def asset_name(text: str) -> str:
    return SINGULAR_ASSETS.get(text, text)


def query_text(text: str) -> str:
    if not text.strip():
        raise argparse.ArgumentTypeError("the query is empty")
    return text


class UsageError(Exception):
    def __init__(self, usage: str, message: str) -> None:
        super().__init__(message)
        self.usage = usage


class CliParser(argparse.ArgumentParser):
    def error(self, message: str) -> NoReturn:
        raise UsageError(self.format_usage(), f"{self.prog}: error: {message}")

    def parse_args(self, args: Any = None, namespace: Any = None) -> argparse.Namespace:  # type: ignore[override]
        parsed, extras = self.parse_known_args(args, namespace)
        if extras:
            command_parser(self, getattr(parsed, "command", None)).error(f"unrecognized arguments: {' '.join(extras)}")
        return parsed


def command_parser(parser: argparse.ArgumentParser, command: str | None) -> argparse.ArgumentParser:
    for action in parser._actions:
        if isinstance(action, argparse._SubParsersAction) and command in action.choices:
            return action.choices[command]
    return parser


def build_parser() -> argparse.ArgumentParser:
    parser = CliParser(
        description="Initialize, validate, or assess a file-backed Domain Registry.",
        epilog=command_groups_text(), formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    commands = parser.add_subparsers(dest="command", required=True, metavar="command")
    for add_commands in (
        _add_read_commands, _add_evidence_commands, _add_lifecycle_commands, _add_governance_commands,
    ):
        add_commands(commands)
    commands._choices_actions.sort(key=lambda action: COMMAND_ORDER[action.dest])
    return parser


def _add_read_commands(commands: argparse._SubParsersAction) -> None:
    _add_lookup_commands(commands)
    _add_export_commands(commands)
    _add_coverage_command(commands)
    _add_assessment_commands(commands)
    _add_history_commands(commands)
    _add_check_commands(commands)
    _add_mcp_command(commands)


def _add_lookup_commands(commands: argparse._SubParsersAction) -> None:
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
    add_path(record_parser, "--changes-root", required=False)
    dependents_parser = commands.add_parser("dependents", help="Follow a record's relations")
    add_path(dependents_parser, "--registry-root")
    add_asset(dependents_parser)
    dependents_parser.add_argument("--id", required=True)
    describe_parser = commands.add_parser("describe-table", help="Describe a table from its migrations and the Registry, or list the tables")
    add_path(describe_parser, "--registry-root", required=False)
    add_path(describe_parser, "--repo-root")
    add_path(describe_parser, "--migrations")
    add_path(describe_parser, "--queries", required=False)
    describe_parser.add_argument("--table")
    describe_parser.add_argument("--base")
    describe_parser.add_argument("--require-complete", action="store_true")
    brief_parser = commands.add_parser("brief", help="Brief a change or a record")
    add_path(brief_parser, "--registry-root")
    add_path(brief_parser, "--repo-root")
    brief_parser.add_argument("--base")
    brief_parser.add_argument("--asset", choices=ASSET_CHOICES, type=asset_name)
    brief_parser.add_argument("--id")
    brief_parser.add_argument("--markdown", action="store_true")


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


def _add_coverage_command(commands: argparse._SubParsersAction) -> None:
    coverage_parser = commands.add_parser("coverage", help="Report modelling gaps")
    add_path(coverage_parser, "--registry-root")
    add_path(coverage_parser, "--changes-root", required=False)
    add_path(coverage_parser, "--repo-root", required=False)


def _add_assessment_commands(commands: argparse._SubParsersAction) -> None:
    readiness_parser = commands.add_parser("readiness", help="Assess project readiness")
    add_path(readiness_parser, "--repo-root")
    add_path(readiness_parser, "--registry-root", required=False)
    boundary_parser = commands.add_parser("analyze-boundary", help="Inspect a boundary")
    add_path(boundary_parser, "--registry-root")
    boundary_parser.add_argument("--source-context", required=True)
    boundary_parser.add_argument("--target-context", required=True)
    impact_parser = commands.add_parser("boundary-impact", help="Find boundaries a change touches")
    add_path(impact_parser, "--registry-root")
    add_path(impact_parser, "--repo-root")
    impact_parser.add_argument("--base", required=True)
    probe_parser = commands.add_parser("probe", help="Report Registry health")
    add_path(probe_parser, "--repo-root")
    add_path(probe_parser, "--registry-root")
    add_path(probe_parser, "--policy", required=False)


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


def _add_check_commands(commands: argparse._SubParsersAction) -> None:
    gates_parser = commands.add_parser("quality-gates", help="Find the repository's own checks")
    add_path(gates_parser, "--repo-root")
    counterfactual_parser = commands.add_parser("counterfactual", help="Break one protection and restore it")
    add_path(counterfactual_parser, "--repo-root")
    add_path(counterfactual_parser, "--file", required=False)
    counterfactual_parser.add_argument("--find")
    counterfactual_parser.add_argument("--replace")
    counterfactual_parser.add_argument("--test-command")
    add_path(counterfactual_parser, "--suite", required=False)
    counterfactual_parser.add_argument(
        "--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS
    )
    counterfactual_parser.add_argument("--cwd")
    counterfactual_parser.add_argument(
        "--expect",
        help="regular expression the failing output must match, such as the assertion message; "
        "write --expect=PATTERN when the pattern starts with -",
    )
    counterfactual_parser.add_argument("--obligation-id")
    add_path(counterfactual_parser, "--package-root", required=False)
    proposals_parser = commands.add_parser("list-proposals", help="List the Change Packages")
    add_path(proposals_parser, "--changes-root")


def _add_mcp_command(commands: argparse._SubParsersAction) -> None:
    serve_parser = commands.add_parser("mcp-serve", help="Serve the read commands to a host over MCP")
    add_path(serve_parser, "--registry-root")
    add_path(serve_parser, "--repo-root")


def _add_evidence_commands(commands: argparse._SubParsersAction) -> None:
    _add_citation_commands(commands)
    _add_source_commands(commands)
    _add_source_selection_commands(commands)


def _add_citation_commands(commands: argparse._SubParsersAction) -> None:
    cite_parser = commands.add_parser("cite", help="Build an evidence citation")
    add_path(cite_parser, "--repo-root")
    cite_parser.add_argument("--path", required=True)
    cite_parser.add_argument("--lines", help="START-END")
    cite_parser.add_argument("--function", help="a function defined in --path (Go, Python, TypeScript, JavaScript, Java, Kotlin, Rust); cites its whole body")
    add_path(cite_parser, "--registry-root", required=False)
    cite_parser.add_argument("--show", action="store_true")
    evidence_verify_parser = commands.add_parser("verify-evidence", help="Verify citations")
    add_path(evidence_verify_parser, "--registry-root")
    add_path(evidence_verify_parser, "--repo-root")
    evidence_verify_parser.add_argument("--path", action="append")
    evidence_verify_parser.add_argument("--strict", action="store_true")
    evidence_verify_parser.add_argument("--staged", action="store_true")
    add_path(evidence_verify_parser, "--package-root", required=False)
    recite_parser = commands.add_parser("recite", help="Re-cite one changed citation")
    add_path(recite_parser, "--registry-root")
    add_path(recite_parser, "--repo-root")
    add_asset(recite_parser)
    recite_parser.add_argument("--id", required=True)
    recite_parser.add_argument("--path", required=True)
    recite_parser.add_argument("--lines")
    recite_parser.add_argument("--replace")
    add_path(recite_parser, "--onto", required=False)
    relocate_parser = commands.add_parser("relocate-evidence", help="Re-cite evidence after a move")
    add_path(relocate_parser, "--registry-root")
    add_path(relocate_parser, "--repo-root")
    relocate_parser.add_argument("--from", dest="moved_from", required=True)
    relocate_parser.add_argument("--to", dest="moved_to", required=True)


def _add_source_commands(commands: argparse._SubParsersAction) -> None:
    source_parser = commands.add_parser("discover-sources", help="Map repository sources")
    add_path(source_parser, "--repo-root")
    add_path(source_parser, "--output", required=False)
    add_profile_flags(source_parser)
    source_verify_parser = commands.add_parser("verify-sources", help="Verify selected sources")
    add_path(source_verify_parser, "--repo-root")
    add_path(source_verify_parser, "--source-map")
    add_path(source_verify_parser, "--policy", required=False)
    secret_parser = commands.add_parser("scan-secrets", help="Scan for secrets")
    add_path(secret_parser, "--repo-root")


def _add_source_selection_commands(commands: argparse._SubParsersAction) -> None:
    memory_init_parser = commands.add_parser("init-domain-memory", help="Start a Registry from human choices: the entry point")
    add_path(memory_init_parser, "--repo-root")
    memory_init_parser.add_argument(
        "--output", required=True, type=Path, help="Registry directory to create, inside the repository",
    )
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
    memory_init_parser.add_argument(
        "--source-authority", required=True,
        help="who confirms the selected sources are the right ones, such as the domain owner's name",
    )
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
    memory_init_parser.add_argument("--distinct-signer", action="store_true")
    add_profile_flags(memory_init_parser)
    memory_init_parser.add_argument("--include", action="append")
    memory_init_parser.add_argument("--exclude", action="append")
    confirm_parser = commands.add_parser("confirm-sources", help="Confirm the selected sources")
    add_path(confirm_parser, "--registry-root")
    add_path(confirm_parser, "--repo-root")
    confirm_parser.add_argument("--confirmed-by", required=True)
    refresh_parser = commands.add_parser("refresh-sources", help="Re-snapshot the selected sources")
    add_path(refresh_parser, "--registry-root")
    add_path(refresh_parser, "--repo-root")
    listing_parser = commands.add_parser("refresh-listing", help="Refresh the discovered listing")
    add_path(listing_parser, "--registry-root")
    add_path(listing_parser, "--repo-root")
    refine_parser = commands.add_parser("refine-sources", help="Focus selected sources")
    add_path(refine_parser, "--registry-root")
    add_path(refine_parser, "--repo-root")
    refine_parser.add_argument("--source", action="append", default=[], type=Path, help="repeatable")
    refine_parser.add_argument("--add", action="append", default=[], type=Path)
    refine_parser.add_argument("--remove", action="append", default=[], type=Path)


def _add_lifecycle_commands(commands: argparse._SubParsersAction) -> None:
    _add_registry_commands(commands)
    _add_candidate_commands(commands)
    _add_harvest_command(commands)
    _add_proposal_commands(commands)
    _add_proposal_check_commands(commands)


def _add_registry_commands(commands: argparse._SubParsersAction) -> None:
    _add_change_package_init(commands)
    redraft_parser = commands.add_parser("redraft-proposal", help="Redraft a retired proposal")
    add_path(redraft_parser, "--package-root")
    add_path(redraft_parser, "--output")
    redraft_parser.add_argument("--proposal-id", required=True)
    add_lifecycle_roots(redraft_parser)
    validate_parser = commands.add_parser("validate", help="Validate the Registry")
    add_path(validate_parser, "--registry-root")
    add_path(validate_parser, "--repo-root", required=False)
    validate_parser.add_argument("--require-reviewed", action="store_true")
    recovery_parser = commands.add_parser("recover-registry-update", help="Recover an interrupted update")
    add_path(recovery_parser, "--registry-root")
    add_path(recovery_parser, "--repo-root", required=False)
    recovery_parser.add_argument("--force", action="store_true")
    demote_parser = commands.add_parser("demote-local-reviews", help="Withdraw local reviewed status")
    add_path(demote_parser, "--registry-root")
    add_path(demote_parser, "--repo-root")


def _add_change_package_init(commands: argparse._SubParsersAction) -> None:
    package_init_parser = commands.add_parser("init-change-package", help="Start a Change Package")
    add_path(package_init_parser, "--output")
    package_init_parser.add_argument("--requirement-id")
    package_init_parser.add_argument("--proposal-id")
    package_init_parser.add_argument("--proposer")
    add_path(package_init_parser, "--recitation", required=False)
    add_path(package_init_parser, "--registry-root", required=False)


def _add_candidate_commands(commands: argparse._SubParsersAction) -> None:
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
    retract_parser.add_argument("--id", required=True, action="append", help="repeatable: every record is withdrawn in one update")
    retract_parser.add_argument("--reason", required=True)
    _add_promote_command(commands)


def _add_promote_command(commands: argparse._SubParsersAction) -> None:
    promote_parser = commands.add_parser("promote-candidate", help="Put a candidate into a draft proposal")
    add_path(promote_parser, "--package-root")
    add_path(promote_parser, "--registry-root")
    add_asset(promote_parser)
    promote_parser.add_argument("--id", required=True)
    promote_parser.add_argument(
        "--amend", action="store_true",
        help="a reviewed record: copy it without its review fields and supersede the proposal that reviewed it",
    )
    add_path(promote_parser, "--repo-root", required=False)
    promote_parser.add_argument(
        "--cite", action="append", default=[], help="repeatable PATH:START-END to add to the promoted record's evidence"
    )
    promote_parser.add_argument(
        "--cite-function", action="append", default=[], help="repeatable PATH:NAME, a function the file defines, to add as evidence"
    )
    promote_parser.add_argument(
        "--drop-cite", action="append", default=[], help="repeatable PATH:START-END to remove from the promoted record's evidence"
    )


def _add_harvest_command(commands: argparse._SubParsersAction) -> None:
    harvest_parser = commands.add_parser("harvest-candidates", help="Harvest a scratch extraction")
    add_path(harvest_parser, "--registry-root")
    add_path(harvest_parser, "--repo-root")
    add_path(harvest_parser, "--from-root")
    harvest_parser.add_argument(
        "--context-map", action="append", default=[], help="repeatable CANDIDATE=GOLD, over the pairing found by path or name"
    )
    harvest_parser.add_argument("--keep-duplicates", action="store_true")
    schema_parser = commands.add_parser("extract-sql-schema", help="Extract SQL constraints as candidate rules")
    add_path(schema_parser, "--registry-root")
    add_path(schema_parser, "--repo-root")
    add_path(schema_parser, "--migrations")
    schema_parser.add_argument(
        "--table-context", action="append", default=[], help="repeatable TABLE=CONTEXT, the Context a table's rules belong to"
    )
    schema_parser.add_argument("--default-context", help="the Context for a table --table-context does not name")
    prove_parser = commands.add_parser("harvest", help="Prove and package harvested rules")
    add_path(prove_parser, "--registry-root")
    add_path(prove_parser, "--repo-root")
    add_path(prove_parser, "--spec")
    add_path(prove_parser, "--changes-root", required=False)
    prove_parser.add_argument("--through", choices=HARVEST_STAGES, default="draft")
    discard_parser = commands.add_parser("discard-harvest", help="Withdraw a Change Package that is not applied, written by harvest or by hand")
    add_path(discard_parser, "--package-root", required=False)
    add_path(discard_parser, "--spec", required=False)
    add_path(discard_parser, "--changes-root", required=False)
    add_path(discard_parser, "--registry-root")
    add_path(discard_parser, "--repo-root")
    discard_parser.add_argument("--reason", required=True)


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
    add_lifecycle_roots(approval_parser)
    supersede_parser = commands.add_parser("supersede-proposal", help="Retire a proposal")
    add_path(supersede_parser, "--package-root")
    supersede_parser.add_argument("--reason", required=True)
    supersede_parser.add_argument("--superseded-by")
    add_lifecycle_roots(supersede_parser)
    verify_parser = commands.add_parser("verify-proposal", help="Verify test attestations")
    add_path(verify_parser, "--package-root")
    add_path(verify_parser, "--registry-root")
    add_path(verify_parser, "--repo-root")
    rebase_parser = commands.add_parser("rebase-proposal", help="Restamp a stale proposal on the current Registry")
    add_path(rebase_parser, "--package-root")
    add_path(rebase_parser, "--registry-root")
    add_path(rebase_parser, "--repo-root")
    result_parser = commands.add_parser("record-test-result", help="Record a test result")
    add_path(result_parser, "--package-root")
    add_path(result_parser, "--registry-root")
    add_path(result_parser, "--repo-root")
    result_parser.add_argument("--obligation", required=True)
    result_parser.add_argument(
        "--command-profile", required=True,
        help="a label for the kind of run, one of the policy's approved_command_profiles; it never expands into a command",
    )
    result_parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS)
    result_parser.add_argument("--cwd")
    ship_parser = commands.add_parser("ship-package", help="Attest, finalize and apply a verified package")
    add_path(ship_parser, "--package-root")
    add_path(ship_parser, "--registry-root")
    add_path(ship_parser, "--repo-root")
    ship_parser.add_argument("--commit")


def _add_proposal_check_commands(commands: argparse._SubParsersAction) -> None:
    package_validate_parser = commands.add_parser("validate-change-package", help="Validate a Change Package")
    add_path(package_validate_parser, "--package-root")
    add_path(package_validate_parser, "--registry-root", required=False)
    package_validate_parser.add_argument("--as-status", choices=PREVIEW_STATUSES)
    add_path(package_validate_parser, "--repo-root", required=False)


def _add_governance_commands(commands: argparse._SubParsersAction) -> None:
    _add_policy_commands(commands)
    _add_attestation_commands(commands)
    _add_hook_commands(commands)


def _add_policy_commands(commands: argparse._SubParsersAction) -> None:
    amend_parser = commands.add_parser("amend-policy", help="Change governance fields")
    add_path(amend_parser, "--registry-root")
    amend_parser.add_argument("--set", action="append", default=[], metavar="FIELD=VALUE")
    amend_parser.add_argument("--reason", required=True)
    amend_parser.add_argument("--verifier")
    signing_parser = commands.add_parser("init-signing-key", help="Prepare a signing identity")
    add_path(signing_parser, "--repo-root")
    signing_parser.add_argument("--principal", required=True)
    add_path(signing_parser, "--key-file", required=False)
    signing_parser.add_argument("--force", action="store_true")
    signing_parser.add_argument("--sign-every-commit", action="store_true")
    policy_parser = commands.add_parser("validate-policy", help="Check a policy before adopting it")
    add_path(policy_parser, "--policy")
    contract_parser = commands.add_parser("validate-contract", help="Check a contract schema file")
    add_path(contract_parser, "--schema")


def _add_attestation_commands(commands: argparse._SubParsersAction) -> None:
    scm_parser = commands.add_parser("verify-scm-attestation", help="Check SCM evidence alone")
    add_path(scm_parser, "--attestation")
    add_path(scm_parser, "--package-root")
    git_governance_parser = commands.add_parser("verify-git-governance", help="Check commit signatures")
    add_path(git_governance_parser, "--registry-root")
    add_path(git_governance_parser, "--repo-root")
    commit_source = git_governance_parser.add_mutually_exclusive_group(required=True)
    commit_source.add_argument("--commit")
    commit_source.add_argument("--stdin", action="store_true", help="Read the commits, one per line, from standard input")
    audit_attestations_parser = commands.add_parser("audit-attestations", help="Audit past attestations")
    add_path(audit_attestations_parser, "--repo-root")
    add_path(audit_attestations_parser, "--changes-root")
    audit_parser = commands.add_parser("verify-audit", help="Verify audit trail")
    add_path(audit_parser, "--registry-root")
    add_path(audit_parser, "--changes-root", required=False)
    audit_parser.add_argument("--against")
    reconcile_parser = commands.add_parser("reconcile-audit", help="Reconcile audit trail")
    add_path(reconcile_parser, "--registry-root")
    indexes_parser = commands.add_parser("refresh-indexes", help="Rebuild concept indexes")
    add_path(indexes_parser, "--registry-root")


def _add_hook_commands(commands: argparse._SubParsersAction) -> None:
    hook_parser = commands.add_parser("install-git-hitl-hook", help="Enforce signatures on push")
    add_path(hook_parser, "--registry-root")
    add_path(hook_parser, "--repo-root")
    commit_hook_parser = commands.add_parser("install-pre-commit-hook", help="Check staged citations on commit")
    add_path(commit_hook_parser, "--registry-root")
    add_path(commit_hook_parser, "--repo-root")
    governance_parser = commands.add_parser("governance-readiness", help="Report what governance still needs")
    add_path(governance_parser, "--registry-root")
    add_path(governance_parser, "--repo-root")
