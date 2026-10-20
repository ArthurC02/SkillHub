from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Callable
from pathlib import Path

from ..attestations import audit_attestations, verify_scm
from ..audit import anchor_error, reconcile, verify as verify_audit
from ..common import PROPOSAL_FILE, load_json, shown_path
from ..contracts import validate_schema
from ..git_hooks import (
    governance_readiness,
    install_pre_commit_hook,
    install_pre_push_hook,
    verify_git_governance,
)
from ..policy import amend_policy_fields, validate_policy
from ..signing import init_signing_key
from ..store import refresh_indexes
from .common import note_unsettled, print_json, report_errors


def policy_assignments(args: argparse.Namespace) -> list[tuple[str, str]]:
    single = (args.field, args.value)
    if any(part is not None for part in single):
        if None in single or args.set:
            raise ValueError("give --field with --value, or one or more --set FIELD=VALUE, not both")
        return [single]
    malformed = [entry for entry in args.set if "=" not in entry]
    if not args.set or malformed:
        raise ValueError("each --set takes FIELD=VALUE; give at least one, or --field with --value")
    return [tuple(entry.split("=", 1)) for entry in args.set]


def handle_amend_policy(args: argparse.Namespace) -> int:
    changes = amend_policy_fields(
        args.registry_root.resolve(),
        policy_assignments(args),
        args.reason,
        args.verifier,
    )
    for change in changes:
        print(f"{change['field']}: {change['from']} -> {change['to']}")
    print(
        "The Registry revision moved once, so every captured base revision is now stale and needs fresh approval."
    )
    return 0


def handle_init_signing_key(args: argparse.Namespace) -> int:
    result = init_signing_key(
        args.repo_root.resolve(),
        args.principal,
        args.key_file,
        args.force,
        args.sign_every_commit,
    )
    print(json.dumps(result, indent=2))
    return 0


def handle_validate_policy(args: argparse.Namespace) -> int:
    errors = validate_policy(load_json(args.policy.resolve()))
    return report_errors(errors, "Domain Memory policy is valid.")


def handle_validate_contract(args: argparse.Namespace) -> int:
    result = validate_schema(args.schema.resolve())
    print_json(result, indent=None)
    return 0 if result["status"] == "valid" else 1


def handle_verify_scm_attestation(args: argparse.Namespace) -> int:
    proposal = load_json(args.package_root.resolve() / PROPOSAL_FILE)
    errors = verify_scm(args.attestation.resolve(), proposal)
    return report_errors(errors, "SCM attestation is valid.")


def handle_verify_git_governance(args: argparse.Namespace) -> int:
    commits = [args.commit] if args.commit else sys.stdin.read().split()
    errors = verify_git_governance(args.registry_root.resolve(), args.repo_root.resolve(), commits)
    exit_code = report_errors(errors, f"Git governance is valid for {len(commits)} commit{'' if len(commits) == 1 else 's'}.")
    note_unsettled(args.registry_root.resolve())
    return exit_code


def handle_install_git_hitl_hook(args: argparse.Namespace) -> int:
    hook = install_pre_push_hook(
        args.registry_root.resolve(),
        args.repo_root.resolve(),
        Path(__file__).parents[2] / "registry_tools.py",
    )
    print(f"Installed Git HITL pre-push hook: {hook}")
    return 0


def handle_install_pre_commit_hook(args: argparse.Namespace) -> int:
    hook = install_pre_commit_hook(
        args.registry_root.resolve(),
        args.repo_root.resolve(),
        Path(__file__).parents[2] / "registry_tools.py",
    )
    print(f"Installed Domain Memory pre-commit hook: {hook}")
    return 0


def handle_governance_readiness(args: argparse.Namespace) -> int:
    result = governance_readiness(
        args.registry_root.resolve(), args.repo_root.resolve()
    )
    print_json(result, indent=None)
    return 0 if result["status"] in {"ready", "working-memory"} else 1


def handle_verify_audit(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    result = verify_audit(root)
    if result["status"] == "valid":
        changes_root = args.changes_root.resolve() if args.changes_root else None
        reason = anchor_error(root, changes_root, args.against)
        if reason:
            result = {"status": "invalid", "reason": reason}
    print_json(result, indent=None)
    return 0 if result["status"] == "valid" else 1


def handle_reconcile_audit(args: argparse.Namespace) -> int:
    result = reconcile(args.registry_root.resolve())
    print_json(result, indent=None)
    if result["joined"]:
        print(f"NOTE: {len(result['joined'])} audit heads joined; commit audit/events.jsonl and audit/manifest.json", file=sys.stderr)
    return 0


def handle_refresh_indexes(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    print_json({"refreshed": [shown_path(path, root) for path in refresh_indexes(root)]}, indent=None)
    return 0


def handle_audit_attestations(args: argparse.Namespace) -> int:
    findings = audit_attestations(
        args.repo_root.resolve(), args.changes_root.resolve(), args.verification_token_env
    )
    print(json.dumps(findings, indent=2, ensure_ascii=False))
    return 0 if all(finding["verdict"] == "carried" for finding in findings) else 1


HANDLERS: dict[str, Callable[[argparse.Namespace], int]] = {
    "amend-policy": handle_amend_policy,
    "init-signing-key": handle_init_signing_key,
    "validate-policy": handle_validate_policy,
    "validate-contract": handle_validate_contract,
    "verify-scm-attestation": handle_verify_scm_attestation,
    "verify-git-governance": handle_verify_git_governance,
    "install-git-hitl-hook": handle_install_git_hitl_hook,
    "install-pre-commit-hook": handle_install_pre_commit_hook,
    "governance-readiness": handle_governance_readiness,
    "verify-audit": handle_verify_audit,
    "reconcile-audit": handle_reconcile_audit,
    "refresh-indexes": handle_refresh_indexes,
    "audit-attestations": handle_audit_attestations,
}
