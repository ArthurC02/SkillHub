from __future__ import annotations

import argparse
from collections.abc import Callable
from importlib import import_module

from .cli_parser import build_parser

__all__ = [
    "COMMANDS_BY_MODULE",
    "execute",
    "handler",
    "run_command",
]

COMMANDS_BY_MODULE = {
    "read": (
        "mcp-serve", "quality-gates", "counterfactual", "list-proposals", "coverage", "export", "compare-registry",
        "lookup", "resolve-terms", "get-context", "get-record", "dependents", "brief",
        "analyze-boundary", "boundary-impact", "change-coupling", "readiness", "probe",
    ),
    "evidence": (
        "relocate-evidence", "recite", "cite", "discover-sources", "verify-sources", "verify-evidence",
        "init-domain-memory", "confirm-sources", "refresh-sources", "refresh-listing", "refine-sources", "scan-secrets",
    ),
    "lifecycle": (
        "init-change-package", "redraft-proposal", "validate", "upsert-candidate", "harvest-candidates",
        "harvest", "discard-harvest", "retract-candidate", "promote-candidate",
        "demote-local-reviews", "recover-registry-update", "submit-proposal", "record-approval", "supersede-proposal",
        "verify-proposal", "rebase-proposal", "record-test-result", "ship-package", "validate-change-package",
    ),
    "governance": (
        "amend-policy", "init-signing-key", "validate-policy", "validate-contract", "verify-scm-attestation",
        "verify-git-governance", "install-git-hitl-hook", "install-pre-commit-hook", "governance-readiness",
        "verify-audit", "reconcile-audit", "refresh-indexes", "audit-attestations",
    ),
}
MODULE_OF = {command: module for module, commands in COMMANDS_BY_MODULE.items() for command in commands}


def run_command(words: list[str]) -> int:
    return execute(build_parser().parse_args(words))


def execute(args: argparse.Namespace) -> int:
    return handler(args.command)(args)


def handler(command: str) -> Callable[[argparse.Namespace], int]:
    module = import_module(f".commands.{MODULE_OF[command]}", __package__)
    return module.HANDLERS[command]
