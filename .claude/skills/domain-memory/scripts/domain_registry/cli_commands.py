from __future__ import annotations

import argparse
import json
import shutil
from pathlib import Path

from .attestations import verify_scm
from .git_hooks import governance_readiness, install_pre_push_hook, verify_git_governance
from .audit import verify as verify_audit
from .changes import init_change_package, validate_change_package
from .common import load_json
from .contracts import validate_schema
from .evidence import citation, classified, source_map_for
from .hitl import (
    finalize_proposal,
    record_approval,
    submit_proposal,
    supersede_proposal,
    verify_proposal,
)
from .policy import (
    amend_policy,
    policy_path,
    validate_policy,
    write_policy,
)
from .readiness import assess_readiness
from .registry import (
    boundary_analysis,
    context_model,
    coverage,
    init_registry,
    lookup,
    migrate_evidence,
    migrate_registry,
    record_by_id,
    resolve_terms,
    validate,
    verify_evidence,
)
from .security import scan_report
from .sources import (
    confirm_sources,
    discover_sources,
    probe_sources,
    refresh_sources,
    refine_sources,
    selected_source_map,
    verify_source_map,
    write_source_map,
)
from .transaction import recover_interrupted_update
from .updates import apply_approved_updates, demote_local_reviews, upsert_candidate


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


def handle_init(args: argparse.Namespace) -> int:
    init_registry(args.output.resolve())
    return 0


def handle_init_change_package(args: argparse.Namespace) -> int:
    init_change_package(args.output.resolve())
    return 0


def handle_validate(args: argparse.Namespace) -> int:
    errors = validate(
        args.registry_root.resolve(),
        args.repo_root.resolve() if args.repo_root else None,
        args.require_reviewed,
    )
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print("Registry is valid.")
    return 0


def handle_lookup(args: argparse.Namespace) -> int:
    print(
        json.dumps(
            lookup(args.registry_root.resolve(), args.asset, args.query),
            indent=2,
            ensure_ascii=False,
        )
    )
    return 0


def handle_migrate_registry(args: argparse.Namespace) -> int:
    print(
        json.dumps(
            {"created": migrate_registry(args.registry_root.resolve())},
            ensure_ascii=False,
        )
    )
    return 0


def handle_demote_local_reviews(args: argparse.Namespace) -> int:
    try:
        demote_local_reviews(args.registry_root.resolve(), args.repo_root.resolve())
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Local reviewed records were demoted to Working Memory candidates.")
    return 0


def handle_resolve_terms(args: argparse.Namespace) -> int:
    print(
        json.dumps(
            resolve_terms(args.registry_root.resolve(), args.query, args.context),
            indent=2,
            ensure_ascii=False,
        )
    )
    return 0


def handle_get_context(args: argparse.Namespace) -> int:
    result = context_model(args.registry_root.resolve(), args.id)
    if result is None:
        print(f"ERROR: context not found: {args.id}")
        return 1
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0


def handle_get_record(args: argparse.Namespace) -> int:
    result = record_by_id(args.registry_root.resolve(), args.asset, args.id)
    if result is None:
        print(f"ERROR: {args.asset} record not found: {args.id}")
        return 1
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0


def handle_analyze_boundary(args: argparse.Namespace) -> int:
    print(
        json.dumps(
            boundary_analysis(
                args.registry_root.resolve(),
                args.source_context,
                args.target_context,
            ),
            indent=2,
            ensure_ascii=False,
        )
    )
    return 0


def handle_cite(args: argparse.Namespace) -> int:
    try:
        reference = citation(args.repo_root.resolve(), args.path, args.start, args.end)
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    if args.registry_root:
        reference = classified(reference, source_map_for(args.registry_root.resolve()))
    print(json.dumps(reference, indent=2, ensure_ascii=False))
    return 0


def handle_upsert_candidate(args: argparse.Namespace) -> int:
    try:
        outside = upsert_candidate(
            args.registry_root.resolve(),
            args.repo_root.resolve(),
            args.asset,
            args.record_file.resolve(),
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Candidate record updated.")
    if outside:
        print(
            "NOTE: this record rests on files the confirmed source map does not cover: "
            + ", ".join(outside)
            + ". That is reach, not an error. A Domain Memory that comes to depend on an unconfirmed file loses its "
            "evidence when that file moves; either have the developer confirm the source, or cite something the "
            "corpus already covers."
        )
    return 0


def handle_apply_approved_updates(args: argparse.Namespace) -> int:
    try:
        apply_approved_updates(
            args.package_root.resolve(),
            args.registry_root.resolve(),
            args.repo_root.resolve(),
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Approved registry updates applied.")
    return 0


def handle_discover_sources(args: argparse.Namespace) -> int:
    source_map = discover_sources(args.repo_root.resolve())
    if args.output:
        write_source_map(args.output.resolve(), source_map)
    print(json.dumps(source_map, indent=2, ensure_ascii=False))
    return 0


def handle_verify_sources(args: argparse.Namespace) -> int:
    policy = load_json(args.policy.resolve()) if args.policy else None
    result = verify_source_map(
        args.repo_root.resolve(), args.source_map.resolve(), policy
    )
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0 if result["status"] == "current" else 1


def handle_probe(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    policy_file = args.policy.resolve() if args.policy else policy_path(registry_root)
    policy = load_json(policy_file) if policy_file.exists() else None
    result = probe_sources(
        args.repo_root.resolve(), registry_root / "source-map.json", policy
    )
    print(probe_summary(result, args.registry_root))
    return {"current": 0, "absent": 2}.get(result["status"], 1)


def handle_readiness(args: argparse.Namespace) -> int:
    print(
        json.dumps(
            assess_readiness(args.repo_root, args.registry_root),
            indent=2,
            ensure_ascii=False,
        )
    )
    return 0


def handle_verify_evidence(args: argparse.Namespace) -> int:
    result = verify_evidence(args.registry_root.resolve(), args.repo_root.resolve())
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return (
        0
        if not any(
            result["summary"][status] for status in ("stale", "missing", "invalid")
        )
        else 1
    )


def handle_migrate_evidence(args: argparse.Namespace) -> int:
    try:
        migrated = migrate_evidence(
            args.registry_root.resolve(), args.repo_root.resolve()
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print(json.dumps({"migrated": migrated}, ensure_ascii=False))
    return 0


def handle_recover_registry_update(args: argparse.Namespace) -> int:
    try:
        recover_interrupted_update(args.registry_root.resolve(), args.force)
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Interrupted Registry update recovered.")
    return 0


def handle_init_domain_memory(args: argparse.Namespace) -> int:
    repo_root = args.repo_root.resolve()
    output = args.output.resolve()
    if args.review_mode == "scm-verified" and not args.review_verifier:
        print("ERROR: scm-verified Domain Memory requires --review-verifier")
        return 1
    if args.review_verifier == "git-signed-commit" and not args.authorized_signer:
        print("ERROR: git-signed-commit requires at least one --authorized-signer")
        return 1
    if args.review_verifier == "git-signed-commit" and args.review_trigger != "git-push":
        print("ERROR: git-signed-commit requires --review-trigger git-push")
        return 1
    try:
        output.relative_to(repo_root)
    except ValueError:
        print("ERROR: output must be inside the repository")
        return 1
    selected = [
        path if path.is_absolute() else repo_root / path for path in args.source
    ]
    try:
        preliminary = selected_source_map(repo_root, selected)
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    try:
        init_registry(output)
    except FileExistsError as error:
        print(f"ERROR: {error}")
        return 1
    write_policy(
        output,
        preliminary,
        args.storage_mode,
        args.data_classification,
        args.review_mode,
        args.source_authority,
        args.include or ["**"],
        args.exclude or [],
        {
            "verifier": args.review_verifier or "none",
            "trigger": args.review_trigger or "none",
            "ci_requirement": args.ci_requirement,
            "authorized_signers": args.authorized_signer or [],
        },
    )
    try:
        source_map = selected_source_map(
            repo_root, selected, load_json(policy_path(output))
        )
    except ValueError as error:
        shutil.rmtree(output)
        print(f"ERROR: {error}")
        return 1
    write_source_map(output / "source-map.json", source_map)
    print(
        "Domain Memory initialized. The sources are agent-asserted: you selected them, the developer has not "
        "said they are the right corpus. Show them the selected paths and ask. When they answer, record it with "
        "confirm-sources --confirmed-by <the identity they give you>; until then verify-sources reports the map "
        "as unverified, which is the truth and not a problem to work around."
    )
    return 0


def handle_confirm_sources(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    policy = policy_path(registry_root)
    try:
        source_map = confirm_sources(
            registry_root,
            args.repo_root.resolve(),
            args.confirmed_by,
            load_json(policy) if policy.is_file() else None,
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print(
        f"Sources confirmed by {args.confirmed_by}: "
        + ", ".join(source_map["selected_paths"])
    )
    return 0


def handle_refresh_sources(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    policy = policy_path(registry_root)
    try:
        source_map = refresh_sources(registry_root, args.repo_root.resolve(), load_json(policy) if policy.is_file() else None)
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Sources refreshed and require developer confirmation: " + ", ".join(source_map["selected_paths"]))
    return 0


def handle_refine_sources(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    policy = load_json(policy_path(registry_root))
    selected = [path if path.is_absolute() else args.repo_root.resolve() / path for path in args.source]
    try:
        source_map = refine_sources(registry_root, args.repo_root.resolve(), selected, policy)
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Focused sources selected and require developer confirmation: " + ", ".join(source_map["selected_paths"]))
    return 0


def handle_amend_policy(args: argparse.Namespace) -> int:
    change = amend_policy(
        args.registry_root.resolve(),
        args.field,
        args.value,
        args.reason,
        args.verifier,
    )
    print(
        f"{change['field']}: {change['from']} -> {change['to']}. The Registry revision moved, so every captured "
        "base revision is now stale and needs fresh approval."
    )
    return 0


def handle_validate_policy(args: argparse.Namespace) -> int:
    errors = validate_policy(load_json(args.policy.resolve()))
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print("Domain Memory policy is valid.")
    return 0


def handle_scan_secrets(args: argparse.Namespace) -> int:
    report = scan_report(args.repo_root.resolve())
    print(json.dumps(report, ensure_ascii=False))
    if report["findings"]:
        return 1
    return 2 if report["status"] == "incomplete" else 0


def handle_validate_contract(args: argparse.Namespace) -> int:
    result = validate_schema(args.schema.resolve())
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result["status"] == "valid" else 1


def handle_verify_scm_attestation(args: argparse.Namespace) -> int:
    proposal = load_json(args.package_root.resolve() / "domain-change-proposal.json")
    errors = verify_scm(args.attestation.resolve(), proposal)
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print("SCM attestation is valid.")
    return 0


def handle_verify_git_governance(args: argparse.Namespace) -> int:
    errors = verify_git_governance(args.registry_root.resolve(), args.repo_root.resolve(), args.commit)
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print("Git governance is valid.")
    return 0


def handle_install_git_hitl_hook(args: argparse.Namespace) -> int:
    try:
        hook = install_pre_push_hook(args.registry_root.resolve(), args.repo_root.resolve(), Path(__file__).parents[1] / "registry_tools.py")
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print(f"Installed Git HITL pre-push hook: {hook}")
    return 0


def handle_governance_readiness(args: argparse.Namespace) -> int:
    result = governance_readiness(args.registry_root.resolve(), args.repo_root.resolve())
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result["status"] in {"ready", "working-memory"} else 1


def handle_verify_audit(args: argparse.Namespace) -> int:
    result = verify_audit(args.registry_root.resolve())
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result["status"] == "valid" else 1


def handle_submit_proposal(args: argparse.Namespace) -> int:
    try:
        submit_proposal(
            args.package_root.resolve(),
            args.registry_root.resolve(),
            args.repo_root.resolve(),
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Proposal submitted for human review.")
    return 0


def handle_record_approval(args: argparse.Namespace) -> int:
    try:
        record_approval(
            args.package_root.resolve(),
            args.role,
            args.reviewer,
            args.scope,
            args.approved_at,
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Human approval recorded.")
    return 0


def handle_supersede_proposal(args: argparse.Namespace) -> int:
    supersede_proposal(args.package_root.resolve(), args.reason, args.superseded_by)
    print(
        "Proposal superseded. Its record stays; create a new draft for the replacement."
    )
    return 0


def handle_verify_proposal(args: argparse.Namespace) -> int:
    try:
        verify_proposal(
            args.package_root.resolve(),
            args.registry_root.resolve(),
            args.repo_root.resolve(),
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Proposal test evidence verified.")
    return 0


def handle_finalize_proposal(args: argparse.Namespace) -> int:
    if args.registry_root is None or args.repo_root is None:
        print("ERROR: finalize-proposal requires --registry-root and --repo-root")
        return 1
    try:
        finalize_proposal(
            args.package_root.resolve(),
            args.registry_root.resolve(),
            args.repo_root.resolve(),
            args.verification_token_env,
        )
    except ValueError as error:
        print(f"ERROR: {error}")
        return 1
    print("Proposal approved.")
    return 0


def handle_validate_change_package(args: argparse.Namespace) -> int:
    errors = validate_change_package(
        args.package_root.resolve(),
        args.registry_root.resolve() if args.registry_root else None,
    )
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print("Change package is valid.")
    return 0


def handle_coverage(args: argparse.Namespace) -> int:
    print(
        json.dumps(coverage(args.registry_root.resolve()), indent=2, ensure_ascii=False)
    )
    return 0


HANDLERS = {
    "init": handle_init,
    "init-change-package": handle_init_change_package,
    "validate": handle_validate,
    "coverage": handle_coverage,
    "lookup": handle_lookup,
    "migrate-registry": handle_migrate_registry,
    "resolve-terms": handle_resolve_terms,
    "get-context": handle_get_context,
    "get-record": handle_get_record,
    "analyze-boundary": handle_analyze_boundary,
    "cite": handle_cite,
    "upsert-candidate": handle_upsert_candidate,
    "apply-approved-updates": handle_apply_approved_updates,
    "demote-local-reviews": handle_demote_local_reviews,
    "discover-sources": handle_discover_sources,
    "readiness": handle_readiness,
    "verify-sources": handle_verify_sources,
    "probe": handle_probe,
    "verify-evidence": handle_verify_evidence,
    "migrate-evidence": handle_migrate_evidence,
    "recover-registry-update": handle_recover_registry_update,
    "init-domain-memory": handle_init_domain_memory,
    "confirm-sources": handle_confirm_sources,
    "refresh-sources": handle_refresh_sources,
    "refine-sources": handle_refine_sources,
    "amend-policy": handle_amend_policy,
    "validate-policy": handle_validate_policy,
    "scan-secrets": handle_scan_secrets,
    "validate-contract": handle_validate_contract,
    "verify-scm-attestation": handle_verify_scm_attestation,
    "verify-git-governance": handle_verify_git_governance,
    "install-git-hitl-hook": handle_install_git_hitl_hook,
    "governance-readiness": handle_governance_readiness,
    "verify-audit": handle_verify_audit,
    "submit-proposal": handle_submit_proposal,
    "record-approval": handle_record_approval,
    "supersede-proposal": handle_supersede_proposal,
    "verify-proposal": handle_verify_proposal,
    "finalize-proposal": handle_finalize_proposal,
    "validate-change-package": handle_validate_change_package,
}


def execute(args: argparse.Namespace) -> int:
    return HANDLERS[args.command](args)
