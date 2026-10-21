from __future__ import annotations

import argparse
import re
import sys
from collections.abc import Callable
from pathlib import Path

from ..changes import (
    init_change_package,
    list_proposals,
    missing_approval_roles,
    redraft_change_package,
    validate_change_package,
)
from ..common import EVIDENCE_FILE, OBLIGATIONS_FILE, PROPOSAL_FILE, REQUIREMENT_FILE, load_json
from ..evidence import citation, function_span
from ..harvest import harvest_candidates
from ..harvest_run import discard_harvest, harvest, package_root_of
from ..hitl import (
    ObligationRun,
    rebase_proposal,
    record_approval,
    record_test_result,
    submit_proposal,
    supersede_proposal,
    verify_proposal,
)
from ..recitation_package import recitation_package
from ..registry import (
    cited_paths,
    init_registry,
    migrate_registry,
    package_consistency_notes,
    record_by_id,
    registry_consistency_notes,
    unplaced_implementation_paths,
    validate,
)
from ..relocate import line_range
from ..ship import ship_package
from ..store import read_document
from ..transaction import recover_interrupted_update, transaction_path
from ..updates import (
    add_promoted_evidence,
    demote_local_reviews,
    promote_candidate,
    reconcile_pending_update,
    reconciliation_path,
    retract_candidate,
    upsert_candidate,
)
from .common import OUTSIDE_SOURCES_CONSEQUENCE, note_unsettled, print_json, report_errors


def handle_init(args: argparse.Namespace) -> int:
    init_registry(args.output.resolve())
    return 0


def handle_init_change_package(args: argparse.Namespace) -> int:
    if args.recitation is not None:
        return _init_recitation_package(args)
    init_change_package(args.output.resolve(), args.requirement_id, args.proposal_id, args.proposer)
    missing = [
        flag for flag, value in (
            ("--requirement-id", args.requirement_id), ("--proposal-id", args.proposal_id),
            ("--proposer", args.proposer),
        ) if value is None
    ]
    if missing:
        print(
            f"Created {args.output.as_posix()}. Still blank: {', '.join(missing)}; validate-change-package "
            "refuses the package until they are filled, so pass them here next time or fill them now."
        )
    return 0


def _init_recitation_package(args: argparse.Namespace) -> int:
    identifiers = (args.requirement_id, args.proposal_id, args.proposer)
    if args.registry_root is None or None in identifiers:
        raise ValueError(
            "--recitation needs --registry-root, --requirement-id, --proposal-id and --proposer, "
            "so the package is complete when it is written"
        )
    blank = recitation_package(
        args.output.resolve(), args.registry_root.resolve(), load_json(args.recitation.resolve()), identifiers
    )
    print(f"Created {args.output.as_posix()}: a routine re-citation package; submit it, record its one test result, then review it.")
    if blank:
        print(f"Still blank: {'; '.join(blank)}; fill it before submit-proposal.")
    return 0


def handle_redraft_proposal(args: argparse.Namespace) -> int:
    redraft_change_package(args.package_root.resolve(), args.output.resolve(), args.proposal_id)
    return 0


def handle_validate(args: argparse.Namespace) -> int:
    errors = validate(
        args.registry_root.resolve(),
        args.repo_root.resolve() if args.repo_root else None,
        args.require_reviewed,
    )
    exit_code = report_errors(errors, "Registry is valid.")
    note_unsettled(args.registry_root.resolve())
    if exit_code == 0:
        _print_notes(registry_consistency_notes(args.registry_root.resolve()))
    if exit_code == 0 and args.repo_root:
        _note_unplaced(args.registry_root.resolve(), args.repo_root.resolve())
    return exit_code


def _print_notes(notes: list[str]) -> None:
    for note in notes:
        print(note, file=sys.stderr)


def _note_unplaced(registry_root: Path, repo_root: Path) -> None:
    unplaced = [
        f"contexts/{identifier} implementation_path {path}"
        for identifier, path, reviewed in unplaced_implementation_paths(read_document(registry_root, "contexts.json"), repo_root)
        if not reviewed
    ]
    if unplaced:
        print(
            f"NOTE: {', '.join(unplaced)} {'lies' if len(unplaced) == 1 else 'lie'} over no file in the repository; "
            "a candidate may name code to come, and ship-package refuses to review it until code is there "
            "or the path is corrected.",
            file=sys.stderr,
        )


def handle_migrate_registry(args: argparse.Namespace) -> int:
    print_json({"created": migrate_registry(args.registry_root.resolve())}, indent=None)
    return 0


def handle_demote_local_reviews(args: argparse.Namespace) -> int:
    demote_local_reviews(args.registry_root.resolve(), args.repo_root.resolve())
    print("Local reviewed records were demoted to Working Memory candidates.")
    return 0


def handle_retract_candidate(args: argparse.Namespace) -> int:
    retract_candidate(
        args.registry_root.resolve(),
        args.repo_root.resolve(),
        args.asset,
        args.id,
        args.reason,
    )
    print("Candidate record retracted.")
    return 0


def _added_citations(args: argparse.Namespace) -> tuple[dict, ...]:
    if not (args.cite or args.cite_function):
        return ()
    if args.repo_root is None:
        raise ValueError("--cite and --cite-function need --repo-root")
    repo_root = args.repo_root.resolve()
    spans = [(path, line_range(lines)) for path, _, lines in (given.rpartition(":") for given in args.cite)]
    spans += [(path, function_span(repo_root, path, name)) for path, _, name in (given.rpartition(":") for given in args.cite_function)]
    return tuple(citation(repo_root, path, *lines) for path, lines in spans)


def handle_promote_candidate(args: argparse.Namespace) -> int:
    citations = _added_citations(args)
    superseded = promote_candidate(
        args.package_root.resolve(), args.registry_root.resolve(), args.asset, args.id, amend=args.amend
    )
    if citations:
        add_promoted_evidence(args.package_root.resolve(), args.asset, args.id, citations)
    print(
        f"{args.asset}/{args.id} is in registry_updates; "
        "ship-package reviews it once the proposal is approved."
    )
    if superseded is not None:
        print(f"NOTE: the package supersedes {superseded}, which reviewed it; change the record in registry_updates, "
              "such as adding the citation of the test that proves it.", file=sys.stderr)
    return 0


def handle_upsert_candidate(args: argparse.Namespace) -> int:
    upserted = upsert_candidate(
        args.registry_root.resolve(),
        args.repo_root.resolve(),
        args.asset,
        *(path.resolve() for path in args.record_file),
    )
    print(_upsert_summary(len(args.record_file), upserted.created))
    outside = upserted.outside
    if upserted.same_evidence:
        print(
            "NOTE: its cited excerpts are all among those of " + ", ".join(upserted.same_evidence)
            + ", or theirs among its; it may be a duplicate of that record.",
            file=sys.stderr,
        )
    if outside:
        print(
            "NOTE: this record rests on files the confirmed source map does not cover: "
            + ", ".join(outside)
            + ". That is reach, not an error. A Domain Memory that comes to depend on an unconfirmed file loses its "
            "evidence when that file moves. "
            + OUTSIDE_SOURCES_CONSEQUENCE,
            file=sys.stderr,
        )
    if upserted.status_dropped:
        print(
            f"NOTE: {', '.join(upserted.status_dropped)} carried a review status, stored as candidate: a record "
            "becomes reviewed only through an approved Change Package.",
            file=sys.stderr,
        )
    return 0


def handle_harvest_candidates(args: argparse.Namespace) -> int:
    context_map = {}
    for entry in args.context_map:
        candidate, separator, gold = entry.partition("=")
        if not (separator and candidate.strip() and gold.strip()):
            raise ValueError(f"--context-map takes CANDIDATE=GOLD, got {entry}")
        context_map[candidate.strip()] = gold.strip()
    report = harvest_candidates(
        args.registry_root.resolve(), args.repo_root.resolve(), args.from_root.resolve(), context_map,
        keep_duplicates=args.keep_duplicates,
    )
    print_json(report)
    if report["context_id_collisions"]:
        print("NOTE: " + ", ".join(report["context_id_collisions"]) + " pair with the Context of the same id here only by "
              "that id, not by implementation_path, so their records attach to it; pass --context-map if they are another.",
              file=sys.stderr)
    if report["possible_duplicates"]:
        print("NOTE: some harvested records cite within the evidence of " + ", ".join(report["possible_duplicates"])
              + "; check that they are not duplicates.", file=sys.stderr)
    if report["outside_sources"]:
        print("NOTE: harvested records rest on files the confirmed source map does not cover: "
              + ", ".join(report["outside_sources"]) + ". " + OUTSIDE_SOURCES_CONSEQUENCE, file=sys.stderr)
    return 0


def _upsert_summary(count: int, created: int) -> str:
    if count == 1:
        return "Candidate record created." if created else "Candidate record updated."
    return f"{count} candidate records written: {created} created, {count - created} updated."


def handle_recover_registry_update(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    interrupted = transaction_path(root).is_file()
    recover_interrupted_update(root, args.force)
    print("Interrupted Registry update recovered." if interrupted else "No interrupted Registry update.")
    if args.repo_root is not None:
        outcome = reconcile_pending_update(root, args.repo_root.resolve())
        print(f"Pending apply reconciled: {outcome}." if outcome else "No pending apply to reconcile.")
        return 0
    if reconciliation_path(root).is_file():
        print("ERROR: a pending apply remains; rerun with --repo-root to reconcile it", file=sys.stderr)
        return 1
    return 0


def handle_submit_proposal(args: argparse.Namespace) -> int:
    submit_proposal(
        args.package_root.resolve(),
        args.registry_root.resolve(),
        args.repo_root.resolve(),
    )
    print("Proposal submitted for human review.")
    return 0


def handle_record_approval(args: argparse.Namespace) -> int:
    record_approval(
        args.package_root.resolve(),
        args.role,
        args.reviewer,
        args.scope,
        args.approved_at,
    )
    print("Human approval recorded.")
    return 0


def handle_supersede_proposal(args: argparse.Namespace) -> int:
    supersede_proposal(args.package_root.resolve(), args.reason, args.superseded_by)
    beside = args.package_root.resolve().parent
    if args.superseded_by is not None and args.superseded_by not in {
        row.get("proposal_id") for row in list_proposals(beside)
    }:
        print(
            f"NOTE: no package beside {args.package_root} carries {args.superseded_by} yet; create it with "
            f"redraft-proposal --package-root {args.package_root} --output <dir> --proposal-id {args.superseded_by}, "
            "and validate names this proposal until a package carries it.",
            file=sys.stderr,
        )
    if args.superseded_by is None:
        print("Proposal superseded. Its record stays; create a new draft for the replacement.")
    else:
        print(f"Proposal superseded by {args.superseded_by}. Its record stays.")
    return 0


PAST_VERIFICATION = ("verified", "approved", "applied")


def handle_verify_proposal(args: argparse.Namespace) -> int:
    package_root = args.package_root.resolve()
    proposal = load_json(package_root / PROPOSAL_FILE)
    if proposal.get("status") in PAST_VERIFICATION:
        print(f"already {proposal['status']} at revision {proposal.get('proposal_revision')}; nothing was written")
        return 0
    verify_proposal(package_root, args.registry_root.resolve(), args.repo_root.resolve())
    print("Proposal test evidence verified.")
    return 0


def handle_rebase_proposal(args: argparse.Namespace) -> int:
    print_json(rebase_proposal(args.package_root, args.registry_root, args.repo_root))
    return 0


def handle_record_test_result(args: argparse.Namespace) -> int:
    record_test_result(
        args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve(),
        ObligationRun(args.obligation, args.obligation_command, args.command_profile, args.cwd), args.timeout,
    )
    print(f"Recorded a passing result for {args.obligation}.")
    return 0


def handle_ship_package(args: argparse.Namespace) -> int:
    report = ship_package(args.package_root, args.registry_root, args.repo_root, args.commit, args.verification_token_env)
    print_json(report)
    return 1 if report["errors"] else 0


def _unchecked_at(status: str, package_root: Path) -> str:
    if status in ("submitted", "verified") and missing_approval_roles(
        load_json(package_root / REQUIREMENT_FILE), load_json(package_root / PROPOSAL_FILE)
    ):
        return ", apart from the approvals, which are given only once it is submitted"
    return ""


def handle_validate_change_package(args: argparse.Namespace) -> int:
    errors = validate_change_package(
        args.package_root.resolve(),
        args.registry_root.resolve() if args.registry_root else None,
        args.as_status,
        args.repo_root.resolve() if args.repo_root else None,
    )
    if args.as_status is None:
        exit_code = report_errors(errors, "Change package is valid.")
        if not exit_code and not load_json(args.package_root / PROPOSAL_FILE).get("registry_updates"):
            print("NOTE: this package changes no Registry record; add the updates it is for, or supersede it.")
        if not exit_code:
            _note_counterfactuals_off_cited_files(
                args.package_root, args.registry_root.resolve() if args.registry_root else None
            )
            _print_notes(package_consistency_notes(args.package_root))
        return exit_code
    unchecked = "" if errors else _unchecked_at(args.as_status, args.package_root)
    return report_errors(errors, f"Change package has everything {args.as_status} requires{unchecked}.")


MUTATED_LOCATION = re.compile(r"^(?P<path>[^:\s]+):\d+: ")


def _note_counterfactuals_off_cited_files(package_root: Path, registry_root: Path | None) -> None:
    checks = load_json(package_root / EVIDENCE_FILE).get("counterfactual_check")
    checks = checks if isinstance(checks, list) else [checks] if isinstance(checks, dict) else []
    obligations = {
        entry.get("id"): entry
        for entry in load_json(package_root / OBLIGATIONS_FILE).get("obligations", []) if isinstance(entry, dict)
    }
    updated = {
        update["record"].get("id"): update["record"]
        for update in load_json(package_root / PROPOSAL_FILE).get("registry_updates", [])
        if isinstance(update, dict) and update.get("asset") == "rules" and isinstance(update.get("record"), dict)
    }
    for check in checks:
        location = MUTATED_LOCATION.match(str(check.get("mutated_protection", "")))
        obligation = obligations.get(check.get("obligation_id"))
        if location is None or obligation is None:
            continue
        rule_ids = [
            source["source_id"] for source in (obligation, *obligation.get("also_covers", []))
            if isinstance(source, dict) and source.get("source_type") == "rule" and isinstance(source.get("source_id"), str)
        ]
        rules = [updated.get(rule_id) or (record_by_id(registry_root, "rules", rule_id) if registry_root else None)
                 for rule_id in rule_ids]
        cited = set().union(*(cited_paths(rule) for rule in rules if rule))
        if rule_ids and location["path"] not in cited:
            print(
                f"NOTE: the counterfactual proving {', '.join(rule_ids)} mutates {location['path']}, which the rule "
                "does not cite. That proves the rule only when the mutated line lies on the "
                "execution path of a cited line, as the query a cited call runs does; otherwise mutate a cited line.",
                file=sys.stderr,
            )


def handle_harvest(args: argparse.Namespace) -> int:
    report = harvest(
        args.registry_root, args.repo_root, load_json(args.spec),
        args.changes_root.resolve() if args.changes_root else None, args.through,
    )
    print_json(report)
    return 1 if report["errors"] else 0


def handle_discard_harvest(args: argparse.Namespace) -> int:
    if (args.package_root is None) == (args.spec is None):
        raise ValueError("discard-harvest takes --package-root or --spec, one of them")
    package_root = args.package_root
    if package_root is None:
        package_root = package_root_of(load_json(args.spec), args.registry_root, args.changes_root)
    print_json(discard_harvest(package_root, args.registry_root, args.repo_root, args.reason))
    return 0


HANDLERS: dict[str, Callable[[argparse.Namespace], int]] = {
    "init": handle_init,
    "init-change-package": handle_init_change_package,
    "redraft-proposal": handle_redraft_proposal,
    "validate": handle_validate,
    "migrate-registry": handle_migrate_registry,
    "upsert-candidate": handle_upsert_candidate,
    "harvest-candidates": handle_harvest_candidates,
    "harvest": handle_harvest,
    "discard-harvest": handle_discard_harvest,
    "retract-candidate": handle_retract_candidate,
    "promote-candidate": handle_promote_candidate,
    "demote-local-reviews": handle_demote_local_reviews,
    "recover-registry-update": handle_recover_registry_update,
    "submit-proposal": handle_submit_proposal,
    "record-approval": handle_record_approval,
    "supersede-proposal": handle_supersede_proposal,
    "verify-proposal": handle_verify_proposal,
    "rebase-proposal": handle_rebase_proposal,
    "record-test-result": handle_record_test_result,
    "ship-package": handle_ship_package,
    "validate-change-package": handle_validate_change_package,
}
