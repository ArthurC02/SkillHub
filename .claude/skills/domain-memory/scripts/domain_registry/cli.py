from .changes import init_change_package, validate_change_package
from .common import ASSET_KEYS, load_json
from .hitl import finalize_proposal, record_approval, submit_proposal, supersede_proposal, verify_proposal
from .policy import AMENDABLE_FIELDS, amend_policy, policy_path, validate_policy, write_policy
from .registry import boundary_analysis, context_model, coverage, init_registry, lookup, migrate_evidence, migrate_registry, record_by_id, resolve_terms, validate, verify_evidence
from .sources import confirmed_source_map, discover_sources, probe_sources, verify_source_map, write_source_map
from .transaction import recover_interrupted_update
from .security import scan as scan_secrets
from .contracts import validate_schema
from .attestations import verify_scm
from .audit import verify as verify_audit
from .updates import apply_approved_updates, upsert_candidate
from .evidence import citation, classified, source_map_for

import argparse
import json
from pathlib import Path

def probe_summary(result: dict, registry_root: Path) -> str:
    where = f"Domain Memory at {registry_root}"
    if result["status"] == "absent":
        return f"{where}: none. Nothing is initialized here; a developer decides whether to create one."
    if result["status"] == "current":
        return f"{where}: current, {result['selection_status']}, established by {result['checked']}."
    if result["status"] == "stale":
        names = ", ".join(source["path"] for source in result["changed_sources"][:5])
        return f"{where}: stale. {len(result['changed_sources'])} sources moved ({names}); refresh before trusting it."
    return f"{where}: {result['status']}. {result.get('reason', '')}".strip()


def main() -> int:
    parser = argparse.ArgumentParser(description="Initialize, validate, or assess a file-backed Domain Registry.")
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
    lookup_parser = commands.add_parser("lookup")
    lookup_parser.add_argument("--registry-root", required=True, type=Path)
    lookup_parser.add_argument("--asset", required=True, choices=[name.removesuffix(".json") for name in ASSET_KEYS])
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
    record_parser.add_argument("--asset", required=True, choices=[name.removesuffix(".json") for name in ASSET_KEYS])
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
    candidate_update_parser.add_argument("--asset", required=True, choices=[name.removesuffix(".json") for name in ASSET_KEYS])
    candidate_update_parser.add_argument("--record-file", required=True, type=Path)
    apply_updates_parser = commands.add_parser("apply-approved-updates")
    apply_updates_parser.add_argument("--package-root", required=True, type=Path)
    apply_updates_parser.add_argument("--registry-root", required=True, type=Path)
    apply_updates_parser.add_argument("--repo-root", required=True, type=Path)
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
    memory_init_parser.add_argument("--source", required=True, action="append", type=Path)
    memory_init_parser.add_argument("--storage-mode", required=True, choices=["tracked", "ignored", "external"])
    memory_init_parser.add_argument("--data-classification", required=True, choices=["public", "internal", "confidential", "restricted"])
    memory_init_parser.add_argument("--review-mode", required=True, choices=["local-draft-only", "scm-verified"])
    memory_init_parser.add_argument("--source-authority", required=True)
    memory_init_parser.add_argument("--confirmed-by")
    memory_init_parser.add_argument("--include", action="append")
    memory_init_parser.add_argument("--exclude", action="append")
    amend_parser = commands.add_parser("amend-policy")
    amend_parser.add_argument("--registry-root", required=True, type=Path)
    amend_parser.add_argument("--field", required=True, choices=sorted(AMENDABLE_FIELDS))
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
    args = parser.parse_args()
    if args.command == "init":
        init_registry(args.output.resolve())
        return 0
    if args.command == "init-change-package":
        init_change_package(args.output.resolve())
        return 0
    if args.command == "validate":
        errors = validate(args.registry_root.resolve(), args.repo_root.resolve() if args.repo_root else None, args.require_reviewed)
        if errors:
            print("\n".join(f"ERROR: {error}" for error in errors))
            return 1
        print("Registry is valid.")
        return 0
    if args.command == "lookup":
        print(json.dumps(lookup(args.registry_root.resolve(), args.asset, args.query), indent=2, ensure_ascii=False))
        return 0
    if args.command == "migrate-registry":
        print(json.dumps({"created": migrate_registry(args.registry_root.resolve())}, ensure_ascii=False))
        return 0
    if args.command == "resolve-terms":
        print(json.dumps(resolve_terms(args.registry_root.resolve(), args.query, args.context), indent=2, ensure_ascii=False))
        return 0
    if args.command == "get-context":
        result = context_model(args.registry_root.resolve(), args.id)
        if result is None:
            print(f"ERROR: context not found: {args.id}")
            return 1
        print(json.dumps(result, indent=2, ensure_ascii=False))
        return 0
    if args.command == "get-record":
        result = record_by_id(args.registry_root.resolve(), args.asset, args.id)
        if result is None:
            print(f"ERROR: {args.asset} record not found: {args.id}")
            return 1
        print(json.dumps(result, indent=2, ensure_ascii=False))
        return 0
    if args.command == "analyze-boundary":
        print(json.dumps(boundary_analysis(args.registry_root.resolve(), args.source_context, args.target_context), indent=2, ensure_ascii=False))
        return 0
    if args.command == "cite":
        try:
            reference = citation(args.repo_root.resolve(), args.path, args.start, args.end)
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        if args.registry_root:
            reference = classified(reference, source_map_for(args.registry_root.resolve()))
        print(json.dumps(reference, indent=2, ensure_ascii=False))
        return 0
    if args.command == "upsert-candidate":
        try:
            outside = upsert_candidate(args.registry_root.resolve(), args.repo_root.resolve(), args.asset, args.record_file.resolve())
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Candidate record updated.")
        if outside:
            print("NOTE: this record rests on files the confirmed source map does not cover: " + ", ".join(outside) +
                  ". That is reach, not an error. A Domain Memory that comes to depend on an unconfirmed file loses its "
                  "evidence when that file moves; either have the developer confirm the source, or cite something the "
                  "corpus already covers.")
        return 0
    if args.command == "apply-approved-updates":
        try:
            apply_approved_updates(args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve())
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Approved registry updates applied.")
        return 0
    if args.command == "discover-sources":
        source_map = discover_sources(args.repo_root.resolve())
        if args.output:
            write_source_map(args.output.resolve(), source_map)
        print(json.dumps(source_map, indent=2, ensure_ascii=False))
        return 0
    if args.command == "verify-sources":
        policy = load_json(args.policy.resolve()) if args.policy else None
        result = verify_source_map(args.repo_root.resolve(), args.source_map.resolve(), policy)
        print(json.dumps(result, indent=2, ensure_ascii=False))
        return 0 if result["status"] == "current" else 1
    if args.command == "probe":
        registry_root = args.registry_root.resolve()
        policy_file = args.policy.resolve() if args.policy else policy_path(registry_root)
        policy = load_json(policy_file) if policy_file.exists() else None
        result = probe_sources(args.repo_root.resolve(), registry_root / "source-map.json", policy)
        print(probe_summary(result, args.registry_root))
        return {"current": 0, "absent": 2}.get(result["status"], 1)
    if args.command == "verify-evidence":
        result = verify_evidence(args.registry_root.resolve(), args.repo_root.resolve())
        print(json.dumps(result, indent=2, ensure_ascii=False))
        return 0 if not any(result["summary"][status] for status in ("stale", "missing", "invalid")) else 1
    if args.command == "migrate-evidence":
        try:
            migrated = migrate_evidence(args.registry_root.resolve(), args.repo_root.resolve())
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print(json.dumps({"migrated": migrated}, ensure_ascii=False))
        return 0
    if args.command == "recover-registry-update":
        try:
            recover_interrupted_update(args.registry_root.resolve(), args.force)
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Interrupted Registry update recovered.")
        return 0
    if args.command == "init-domain-memory":
        repo_root = args.repo_root.resolve()
        output = args.output.resolve()
        try:
            output.relative_to(repo_root)
        except ValueError:
            print("ERROR: output must be inside the repository")
            return 1
        selected = [path if path.is_absolute() else repo_root / path for path in args.source]
        try:
            preliminary = confirmed_source_map(repo_root, selected, confirmed_by=args.confirmed_by)
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        try:
            init_registry(output)
        except FileExistsError as error:
            print(f"ERROR: {error}")
            return 1
        write_policy(output, preliminary, args.storage_mode, args.data_classification, args.review_mode, args.source_authority, args.include or ["**"], args.exclude or [])
        try:
            source_map = confirmed_source_map(repo_root, selected, load_json(policy_path(output)), confirmed_by=args.confirmed_by)
        except ValueError as error:
            import shutil
            shutil.rmtree(output)
            print(f"ERROR: {error}")
            return 1
        write_source_map(output / "source-map.json", source_map)
        if args.confirmed_by:
            print(f"Domain Memory initialized from sources confirmed by {args.confirmed_by}.")
        else:
            print("Domain Memory initialized. Sources are agent-asserted, not developer-confirmed: "
                  "re-run with --confirmed-by <identity> once a developer has chosen them, "
                  "or verify-sources will report the map as unverified.")
        return 0
    if args.command == "amend-policy":
        change = amend_policy(args.registry_root.resolve(), args.field, args.value, args.reason)
        print(f"{change['field']}: {change['from']} -> {change['to']}. The Registry revision moved, so every captured "
              "base revision is now stale and needs fresh approval.")
        return 0
    if args.command == "validate-policy":
        errors = validate_policy(load_json(args.policy.resolve()))
        if errors:
            print("\n".join(f"ERROR: {error}" for error in errors))
            return 1
        print("Domain Memory policy is valid.")
        return 0
    if args.command == "scan-secrets":
        findings = scan_secrets(args.repo_root.resolve())
        print(json.dumps({"findings": findings}, ensure_ascii=False))
        return 1 if findings else 0
    if args.command == "validate-contract":
        result = validate_schema(args.schema.resolve())
        print(json.dumps(result, ensure_ascii=False))
        return 0 if result["status"] == "valid" else 1
    if args.command == "verify-scm-attestation":
        proposal = load_json(args.package_root.resolve() / "domain-change-proposal.json")
        errors = verify_scm(args.attestation.resolve(), proposal)
        if errors:
            print("\n".join(f"ERROR: {error}" for error in errors))
            return 1
        print("SCM attestation is valid.")
        return 0
    if args.command == "verify-audit":
        result = verify_audit(args.registry_root.resolve())
        print(json.dumps(result, ensure_ascii=False))
        return 0 if result["status"] == "valid" else 1
    if args.command == "submit-proposal":
        try:
            submit_proposal(args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve())
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Proposal submitted for human review.")
        return 0
    if args.command == "record-approval":
        try:
            record_approval(args.package_root.resolve(), args.role, args.reviewer, args.scope, args.approved_at)
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Human approval recorded.")
        return 0
    if args.command == "supersede-proposal":
        supersede_proposal(args.package_root.resolve(), args.reason, args.superseded_by)
        print("Proposal superseded. Its record stays; create a new draft for the replacement.")
        return 0
    if args.command == "verify-proposal":
        try:
            verify_proposal(args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve())
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Proposal test evidence verified.")
        return 0
    if args.command == "finalize-proposal":
        if args.registry_root is None or args.repo_root is None:
            print("ERROR: finalize-proposal requires --registry-root and --repo-root")
            return 1
        try:
            finalize_proposal(args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve())
        except ValueError as error:
            print(f"ERROR: {error}")
            return 1
        print("Proposal approved.")
        return 0
    if args.command == "validate-change-package":
        errors = validate_change_package(args.package_root.resolve(), args.registry_root.resolve() if args.registry_root else None)
        if errors:
            print("\n".join(f"ERROR: {error}" for error in errors))
            return 1
        print("Change package is valid.")
        return 0
    print(json.dumps(coverage(args.registry_root.resolve()), indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
