import argparse
from pathlib import Path

from .common import ASSET_KEYS
from .policy import AMENDABLE_FIELDS


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Initialize, validate, or assess a file-backed Domain Registry."
    )
    commands = parser.add_subparsers(dest="command", required=True)
    init_parser = commands.add_parser("init")
    init_parser.add_argument("--output", required=True, type=Path)
    migrate_parser = commands.add_parser("migrate-registry")
    migrate_parser.add_argument("--registry-root", required=True, type=Path)
    package_init_parser = commands.add_parser("init-change-package")
    package_init_parser.add_argument("--output", required=True, type=Path)
    validate_parser = commands.add_parser("validate")
    validate_parser.add_argument("--registry-root", required=True, type=Path)
    validate_parser.add_argument("--repo-root", type=Path)
    validate_parser.add_argument("--require-reviewed", action="store_true")
    coverage_parser = commands.add_parser("coverage")
    coverage_parser.add_argument("--registry-root", required=True, type=Path)
    readiness_parser = commands.add_parser("readiness")
    readiness_parser.add_argument("--repo-root", required=True, type=Path)
    readiness_parser.add_argument("--registry-root", type=Path)
    lookup_parser = commands.add_parser("lookup")
    lookup_parser.add_argument("--registry-root", required=True, type=Path)
    lookup_parser.add_argument(
        "--asset",
        required=True,
        choices=[name.removesuffix(".json") for name in ASSET_KEYS],
    )
    lookup_parser.add_argument("--query", required=True)
    term_parser = commands.add_parser("resolve-terms")
    term_parser.add_argument("--registry-root", required=True, type=Path)
    term_parser.add_argument("--query", required=True)
    term_parser.add_argument("--context")
    context_parser = commands.add_parser("get-context")
    context_parser.add_argument("--registry-root", required=True, type=Path)
    context_parser.add_argument("--id", required=True)
    record_parser = commands.add_parser("get-record")
    record_parser.add_argument("--registry-root", required=True, type=Path)
    record_parser.add_argument(
        "--asset",
        required=True,
        choices=[name.removesuffix(".json") for name in ASSET_KEYS],
    )
    record_parser.add_argument("--id", required=True)
    boundary_parser = commands.add_parser("analyze-boundary")
    boundary_parser.add_argument("--registry-root", required=True, type=Path)
    boundary_parser.add_argument("--source-context", required=True)
    boundary_parser.add_argument("--target-context", required=True)
    cite_parser = commands.add_parser("cite")
    cite_parser.add_argument("--repo-root", required=True, type=Path)
    cite_parser.add_argument("--path", required=True)
    cite_parser.add_argument("--start", required=True, type=int)
    cite_parser.add_argument("--end", required=True, type=int)
    cite_parser.add_argument("--registry-root", type=Path)
    candidate_update_parser = commands.add_parser("upsert-candidate")
    candidate_update_parser.add_argument("--registry-root", required=True, type=Path)
    candidate_update_parser.add_argument("--repo-root", required=True, type=Path)
    candidate_update_parser.add_argument(
        "--asset",
        required=True,
        choices=[name.removesuffix(".json") for name in ASSET_KEYS],
    )
    candidate_update_parser.add_argument("--record-file", required=True, type=Path)
    apply_updates_parser = commands.add_parser("apply-approved-updates")
    apply_updates_parser.add_argument("--package-root", required=True, type=Path)
    apply_updates_parser.add_argument("--registry-root", required=True, type=Path)
    apply_updates_parser.add_argument("--repo-root", required=True, type=Path)
    review_empty_parser = commands.add_parser("review-empty-registry")
    review_empty_parser.add_argument("--registry-root", required=True, type=Path)
    review_empty_parser.add_argument("--repo-root", required=True, type=Path)
    review_empty_parser.add_argument("--reviewer", required=True)
    source_parser = commands.add_parser("discover-sources")
    source_parser.add_argument("--repo-root", required=True, type=Path)
    source_parser.add_argument("--output", type=Path)
    source_verify_parser = commands.add_parser("verify-sources")
    source_verify_parser.add_argument("--repo-root", required=True, type=Path)
    source_verify_parser.add_argument("--source-map", required=True, type=Path)
    source_verify_parser.add_argument("--policy", type=Path)
    probe_parser = commands.add_parser("probe")
    probe_parser.add_argument("--repo-root", required=True, type=Path)
    probe_parser.add_argument("--registry-root", required=True, type=Path)
    probe_parser.add_argument("--policy", type=Path)
    evidence_verify_parser = commands.add_parser("verify-evidence")
    evidence_verify_parser.add_argument("--registry-root", required=True, type=Path)
    evidence_verify_parser.add_argument("--repo-root", required=True, type=Path)
    evidence_migrate_parser = commands.add_parser("migrate-evidence")
    evidence_migrate_parser.add_argument("--registry-root", required=True, type=Path)
    evidence_migrate_parser.add_argument("--repo-root", required=True, type=Path)
    recovery_parser = commands.add_parser("recover-registry-update")
    recovery_parser.add_argument("--registry-root", required=True, type=Path)
    recovery_parser.add_argument("--force", action="store_true")
    memory_init_parser = commands.add_parser("init-domain-memory")
    memory_init_parser.add_argument("--repo-root", required=True, type=Path)
    memory_init_parser.add_argument("--output", required=True, type=Path)
    memory_init_parser.add_argument(
        "--source", required=True, action="append", type=Path
    )
    memory_init_parser.add_argument(
        "--storage-mode", required=True, choices=["tracked", "ignored", "external"]
    )
    memory_init_parser.add_argument(
        "--data-classification",
        required=True,
        choices=["public", "internal", "confidential", "restricted"],
    )
    memory_init_parser.add_argument(
        "--review-mode", required=True, choices=["local-draft-only", "scm-verified"]
    )
    memory_init_parser.add_argument("--source-authority", required=True)
    memory_init_parser.add_argument("--include", action="append")
    memory_init_parser.add_argument("--exclude", action="append")
    confirm_parser = commands.add_parser("confirm-sources")
    confirm_parser.add_argument("--registry-root", required=True, type=Path)
    confirm_parser.add_argument("--repo-root", required=True, type=Path)
    confirm_parser.add_argument("--confirmed-by", required=True)
    amend_parser = commands.add_parser("amend-policy")
    amend_parser.add_argument("--registry-root", required=True, type=Path)
    amend_parser.add_argument(
        "--field", required=True, choices=sorted(AMENDABLE_FIELDS)
    )
    amend_parser.add_argument("--value", required=True)
    amend_parser.add_argument("--reason", required=True)
    policy_parser = commands.add_parser("validate-policy")
    policy_parser.add_argument("--policy", required=True, type=Path)
    secret_parser = commands.add_parser("scan-secrets")
    secret_parser.add_argument("--repo-root", required=True, type=Path)
    contract_parser = commands.add_parser("validate-contract")
    contract_parser.add_argument("--schema", required=True, type=Path)
    scm_parser = commands.add_parser("verify-scm-attestation")
    scm_parser.add_argument("--attestation", required=True, type=Path)
    scm_parser.add_argument("--package-root", required=True, type=Path)
    audit_parser = commands.add_parser("verify-audit")
    audit_parser.add_argument("--registry-root", required=True, type=Path)
    submit_parser = commands.add_parser("submit-proposal")
    submit_parser.add_argument("--package-root", required=True, type=Path)
    submit_parser.add_argument("--registry-root", required=True, type=Path)
    submit_parser.add_argument("--repo-root", required=True, type=Path)
    approval_parser = commands.add_parser("record-approval")
    approval_parser.add_argument("--package-root", required=True, type=Path)
    approval_parser.add_argument("--role", required=True)
    approval_parser.add_argument("--reviewer", required=True)
    approval_parser.add_argument("--scope", required=True)
    approval_parser.add_argument("--approved-at")
    supersede_parser = commands.add_parser("supersede-proposal")
    supersede_parser.add_argument("--package-root", required=True, type=Path)
    supersede_parser.add_argument("--reason", required=True)
    supersede_parser.add_argument("--superseded-by")
    verify_parser = commands.add_parser("verify-proposal")
    verify_parser.add_argument("--package-root", required=True, type=Path)
    verify_parser.add_argument("--registry-root", required=True, type=Path)
    verify_parser.add_argument("--repo-root", required=True, type=Path)
    finalize_parser = commands.add_parser("finalize-proposal")
    finalize_parser.add_argument("--package-root", required=True, type=Path)
    finalize_parser.add_argument("--registry-root", type=Path)
    finalize_parser.add_argument("--repo-root", type=Path)
    package_validate_parser = commands.add_parser("validate-change-package")
    package_validate_parser.add_argument("--package-root", required=True, type=Path)
    package_validate_parser.add_argument("--registry-root", type=Path)
    return parser
