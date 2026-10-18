from __future__ import annotations

import argparse
import difflib
import re
import json
import shutil
import sys
from pathlib import Path

from .attestations import audit_attestations, verify_scm
from .audit import anchor_error, review_history
from .audit import verify as verify_audit
from .changes import (
    init_change_package,
    list_proposals,
    missing_approval_roles,
    proof_of,
    rule_coverage,
    redraft_change_package,
    validate_change_package,
)
from .common import ASSET_KEYS, EVIDENCE_FILE, PROPOSAL_FILE, REQUIREMENT_FILE, load_json
from .compare import compare_registries
from .coupling import CouplingLimits, CouplingReading, change_coupling
from .dependents import dependents
from .contracts import validate_schema
from .counterfactual import FORMAT as COUNTERFACTUAL_FORMAT
from .counterfactual import FailingBaselineError, Mutation, Proof, counterfactual
from .evidence import citation, classified, function_span, repeated_note, source_map_for
from .gates import quality_gates
from .git_hooks import (
    governance_readiness,
    install_pre_push_hook,
    verify_git_governance,
)
from .hitl import (
    ObligationRun,
    attest_signed_commit,
    finalize_proposal,
    record_approval,
    record_test_result,
    submit_proposal,
    supersede_proposal,
    verify_proposal,
)
from .policy import (
    amend_policy_fields,
    existing_policy,
    policy_path,
    query_result_limit,
    validate_policy,
    write_policy,
)
from .readiness import assess_readiness
from .recitation_package import recitation_package
from .harvest import harvest_candidates
from .harvest_package import harvest_package, write_harvest_package
from .impact import boundary_impact, registered_collaborations
from .relocate import Recitation, line_range, recite, relocate_evidence
from .registry import (
    CONSTRAINT_ASSETS,
    MATCH_KINDS,
    asset_records,
    unplaced_implementation_paths,
    boundary_analysis,
    cited_paths,
    context_model,
    context_summary,
    coverage,
    every_resolved_term,
    init_registry,
    migrate_evidence,
    migrate_registry,
    prose_mentions,
    ranked_lookup,
    record_by_id,
    validate,
    verify_evidence,
)
from .security import scan_report
from .signing import init_signing_key
from .store import committed_documents, read_document
from .sources import (
    adjusted_selection,
    confirm_sources,
    discover_sources,
    probe_sources,
    refine_sources,
    refresh_sources,
    selected_source_map,
    verify_source_map,
    write_source_map,
)
from .transaction import recover_interrupted_update, transaction_path
from .updates import (
    apply_approved_updates,
    demote_local_reviews,
    add_promoted_evidence,
    promote_candidate,
    reconcile_pending_update,
    reconciliation_path,
    retract_candidate,
    upsert_candidate,
)

COUNTERFACTUAL_EXIT_CODES = {"killed": 0, "survived": 1, "inconclusive": 2, "error": 3}


def report_errors(errors: list[str], success_message: str) -> int:
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print(success_message)
    return 0


def print_json(value: object, indent: int | None = 2) -> None:
    print(json.dumps(value, indent=indent, ensure_ascii=False))


def sources_under(repo_root: Path, sources: list[Path]) -> list[Path]:
    return [repo_root / source for source in sources]


def probe_summary(result: dict, registry_root: Path) -> str:
    where = f"Domain Memory at {registry_root}"
    if result["status"] == "absent" and "hint" in result:
        return f"{where}: no source map here, but {result['hint']}."
    if result["status"] == "absent":
        return (
            f"{where}: none. Nothing is initialized here; "
            "a developer decides whether to create one."
        )
    if result["status"] == "current":
        return (
            f"{where}: current, {result['selection_status']}, "
            f"established by {result['checked']}."
        )
    if result["status"] == "stale":
        names = ", ".join(source["path"] for source in result["changed_sources"][:5])
        return (
            f"{where}: stale. {len(result['changed_sources'])} sources moved "
            f"({names}); refresh before trusting it."
        )
    return f"{where}: {result['status']}. {result.get('reason', '')}".strip()


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
            f"Created {args.output}. Still blank: {', '.join(missing)}; validate-change-package "
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
    print(f"Created {args.output}: a routine re-citation package; submit it, record its one test result, then review it.")
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
    _note_unsettled(args.registry_root.resolve())
    if exit_code == 0 and args.repo_root:
        _note_unplaced(args.registry_root.resolve(), args.repo_root.resolve())
    return exit_code


def _note_unplaced(registry_root: Path, repo_root: Path) -> None:
    unplaced = [
        f"contexts/{identifier} implementation_path {path}"
        for identifier, path, reviewed in unplaced_implementation_paths(asset_records(registry_root, "contexts.json"), repo_root)
        if not reviewed
    ]
    if unplaced:
        print(
            f"NOTE: {', '.join(unplaced)} {'lies' if len(unplaced) == 1 else 'lie'} over no file in the repository; "
            "a candidate may name code to come, and apply-approved-updates refuses to review it until code is there "
            "or the path is corrected.",
            file=sys.stderr,
        )


def _note_unsettled(registry_root: Path) -> None:
    changes_root = _changes_root(registry_root, None)
    rows = list_proposals(changes_root) if changes_root is not None else []
    pending = [row["proposal_id"] for row in rows if row.get("status") == "approved"]
    dangling = [f"{row['proposal_id']} (by {row['superseded_by']})" for row in rows if row.get("superseded_by_unknown")]
    if dangling:
        print(
            f"NOTE: {len(dangling)} superseded Change Package{' names' if len(dangling) == 1 else 's name'} a replacement "
            f"no package carries: {', '.join(dangling)}. Create it with redraft-proposal --proposal-id <id>.",
            file=sys.stderr,
        )
    if pending:
        print(
            f"NOTE: {len(pending)} approved Change Package{'' if len(pending) == 1 else 's'} not applied: "
            f"{', '.join(map(str, pending))}. Apply each with apply-approved-updates; retire one that cannot be "
            "applied with supersede-proposal, and redraft-proposal carries its work into a new draft.",
            file=sys.stderr,
        )


def handle_lookup(args: argparse.Namespace) -> int:
    found, how = ranked_lookup(args.registry_root.resolve(), args.asset, args.query)
    print_json(found)
    notes = {
        "some-words": f"no {args.asset} record holds every word of \"{args.query}\"; these hold some of them, most first.",
        "none": f"no {args.asset} record mentions \"{args.query}\" or any of its words. That is not proof that no "
                "constraint applies: read get-context for the Context you are changing, or look up another asset.",
        "common-words": f"the {args.asset} records share with \"{args.query}\" only words many of them hold, so none "
                        "is listed. That is not proof that no constraint applies: read get-context for the Context "
                        "you are changing, or look up another asset.",
    }
    if how in notes:
        print("NOTE: " + notes[how], file=sys.stderr)
    return 0


def handle_migrate_registry(args: argparse.Namespace) -> int:
    print_json({"created": migrate_registry(args.registry_root.resolve())}, indent=None)
    return 0


def handle_demote_local_reviews(args: argparse.Namespace) -> int:
    demote_local_reviews(args.registry_root.resolve(), args.repo_root.resolve())
    print("Local reviewed records were demoted to Working Memory candidates.")
    return 0


def handle_resolve_terms(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    limit = query_result_limit(root)
    found = {query: every_resolved_term(root, query, args.context, with_mentions=args.with_mentions) for query in args.query}
    print_json([{**match, "query": query} for query, matches in found.items() for match in matches[:limit]])
    for query, matches in found.items():
        if len(matches) > limit:
            print(f"NOTE: \"{query}\" has {len(matches)} matches; the first {limit} are shown, rules and contracts "
                  "first among mentions.", file=sys.stderr)
        constraints = [] if args.with_mentions else [
            mention for mention in prose_mentions(root, query) if mention["asset"] in CONSTRAINT_ASSETS
        ]
        if any(match.get("match_kind") in MATCH_KINDS for match in matches) and constraints:
            print(f"NOTE: {len({(item['asset'], item['id']) for item in constraints})} reviewed rules or contracts "
                  f"mention \"{query}\"; pass --with-mentions to list them.", file=sys.stderr)
    return 0


def handle_export(args: argparse.Namespace) -> int:
    print_json(_exported(args.registry_root.resolve(), args.commit, args.repo_root, "export --commit"))
    return 0


def handle_compare_registry(args: argparse.Namespace) -> int:
    gold = _exported(args.gold_root.resolve(), args.gold_commit, args.repo_root, "compare-registry --gold-commit")
    candidate = _exported(args.candidate_root.resolve(), None, None, "")
    report = compare_registries(gold, candidate, args.context)
    print_json(report)
    attempted_matches = sum(
        len(result["matched"]) for result in report["assets"].values() if result["candidate"] and result["gold"]
    )
    if report["copied_matches"] * 2 > attempted_matches:
        print(
            f"NOTE: {report['copied_matches']} of {attempted_matches} matches cite exactly the gold evidence; "
            "read overall_independent, which leaves them out, as the extraction's own score.",
            file=sys.stderr,
        )
    unpaired = report.get("scope", {}).get("without_candidate")
    if unpaired:
        print(
            f"NOTE: no candidate Context pairs with {', '.join(unpaired)}, so its records cannot pair; "
            "check that the candidate root holds an extraction of that Context.",
            file=sys.stderr,
        )
    return 0


def _exported(root: Path, commit: str | None, repo_root: Path | None, option: str) -> dict:
    if commit is None:
        documents = {name: read_document(root, name) for name in ASSET_KEYS}
    else:
        if repo_root is None:
            raise ValueError(f"{option} needs --repo-root")
        documents = committed_documents(repo_root.resolve(), commit, root, tuple(ASSET_KEYS))
        if documents is None:
            raise ValueError(f"no Domain Memory Registry at {commit}")
    return {name.removesuffix(".json"): document for name, document in documents.items()}


def handle_get_context(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    result = context_model(root, args.id)
    if result is None:
        raise ValueError(f"context not found: {args.id}{_did_you_mean(args.id, asset_records(root, 'contexts.json'))}")
    print_json(context_summary(result) if args.summary else result)
    return 0


def handle_get_record(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    result = record_by_id(root, args.asset, args.id)
    if result is None:
        known = asset_records(root, f"{args.asset}.json")
        raise ValueError(f"{args.asset} record not found: {args.id}{_did_you_mean(args.id, known)}")
    changes_root = _changes_root(root, args.changes_root)
    if changes_root is not None and args.asset in PROVEN_ASSETS:
        result = _with_proofs(root, changes_root, args.asset, args.id, result)
    print_json(result)
    return 0


PROVEN_ASSETS = ("rules", "contracts")


def _changes_root(registry_root: Path, given: Path | None) -> Path | None:
    if given is not None:
        return given.resolve()
    beside = registry_root / "changes"
    return beside if beside.is_dir() else None




def _with_proofs(root: Path, changes_root: Path, asset: str, identifier: str, record: dict) -> dict:
    key = f"{asset}/{identifier}"
    proposals = review_history(root).get(key)
    if not proposals:
        print(f"NOTE: {key} was not reviewed through an applied Change Package, so no test obligation proves it yet.",
              file=sys.stderr)
        return record
    proof = proof_of(changes_root, proposals, (asset.removesuffix("s"), identifier))
    if proof.obligations is None:
        print(f"NOTE: the Change Package of {proof.missing}, which reviewed {key}, is not under {changes_root}, "
              "so the tests that prove it are unknown.", file=sys.stderr)
        return record
    if proof.obligations and proof.since:
        print(f"NOTE: {key} is proven by {proof.obligations[0]['proposal_id']}; {', '.join(proof.since)}, applied since, "
              "changed it without a test obligation, as a re-citation does.", file=sys.stderr)
    return {**record, "proven_by": proof.obligations}


def _did_you_mean(identifier: str, records: list[dict]) -> str:
    known = [record["id"] for record in records if isinstance(record.get("id"), str)]
    starting = [candidate for candidate in known if candidate.startswith(identifier)]
    close = list(dict.fromkeys([*starting, *difflib.get_close_matches(identifier, known, n=3)]))[:3]
    return f"; did you mean {', '.join(close)}?" if close else ""


def handle_change_coupling(args: argparse.Namespace) -> int:
    if args.depth < 1:
        raise ValueError("change-coupling --depth must be at least 1")
    limits = CouplingLimits(args.min_revisions, args.min_shared, args.min_degree, args.max_changeset, args.max_modules)
    registry_root = args.registry_root.resolve() if args.registry_root else None
    contexts = (
        {
            context["id"]: context["implementation_path"]
            for context in asset_records(registry_root, "contexts.json")
            if isinstance(context.get("implementation_path"), str)
        }
        if registry_root
        else {}
    )
    collaborations = registered_collaborations(registry_root) if registry_root else {}
    reading = CouplingReading(args.since, tuple(args.exclude_subject), contexts, collaborations)
    print_json(change_coupling(args.repo_root.resolve(), args.root or ["."], args.depth, limits, reading))
    return 0


def handle_boundary_impact(args: argparse.Namespace) -> int:
    print_json(boundary_impact(args.registry_root.resolve(), args.repo_root.resolve(), args.base))
    return 0


def handle_relocate_evidence(args: argparse.Namespace) -> int:
    repo_root = args.repo_root.resolve()
    relocated = relocate_evidence(args.registry_root.resolve(), repo_root, args.moved_from, args.moved_to)
    print_json(relocated)
    _note_contexts_placed_over_nothing(repo_root, args.moved_to, relocated)
    if not any(relocated[key] for key in ("registry_updates", "candidates", "unmatched")):
        in_place = relocated["citations_in_place"]
        print(
            f"NOTE: {in_place} citation{'' if in_place == 1 else 's'} under {args.moved_from} "
            f"{'is' if in_place == 1 else 'are'} still in place and no Context ends with it; nothing to move."
            if in_place else f"NOTE: no record cites a path under {args.moved_from} and no Context ends with it; "
            "nothing to move. Pass the path as the repository holds it, such as src/billing/orders for a Context at billing/orders.",
            file=sys.stderr,
        )
    return 0


def _note_contexts_placed_over_nothing(repo_root: Path, moved_to: str, relocated: dict[str, list]) -> None:
    placed = [
        item["record"]["id"]
        for item in [*relocated["registry_updates"], *relocated["candidates"]]
        if item["asset"] == "contexts"
    ]
    if placed and not (repo_root / moved_to).exists():
        print(
            f"NOTE: --to {moved_to} lies over no file in the repository; the Context path proposed for "
            f"{', '.join(placed)} would be refused at review. Check --to for a typo.",
            file=sys.stderr,
        )


def handle_recite(args: argparse.Namespace) -> int:
    repo_root = args.repo_root.resolve()
    lines = args.lines or _replaced_lines_in_place(args.path, args.replace)
    print_json(
        recite(
            args.registry_root.resolve(), repo_root,
            Recitation(
                args.asset, args.id, args.path, lines, args.replace,
                load_json(args.onto.resolve()) if args.onto else None,
            ),
        )
    )
    _note_repeated(repo_root, citation(repo_root, args.path, *line_range(lines)))
    return 0


def _replaced_lines_in_place(path: str, replace: str | None) -> str:
    replaced_path, _, replaced_lines = (replace or "").partition(":")
    if replaced_path != path or not replaced_lines:
        raise ValueError(
            "recite needs --lines, unless --replace names lines of the same file as --path; "
            "then those lines are re-cited"
        )
    return replaced_lines


def _note_repeated(repo_root: Path, cited: dict) -> None:
    note = repeated_note(repo_root, cited)
    if note:
        print(note, file=sys.stderr)


def handle_dependents(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    if record_by_id(root, args.asset, args.id) is None:
        known = asset_records(root, f"{args.asset}.json")
        raise ValueError(f"{args.asset} record not found: {args.id}{_did_you_mean(args.id, known)}")
    print_json(dependents(root, args.asset, args.id))
    return 0


def handle_analyze_boundary(args: argparse.Namespace) -> int:
    print_json(
        boundary_analysis(
            args.registry_root.resolve(), args.source_context, args.target_context
        )
    )
    return 0


OUTSIDE_SOURCES_CONSEQUENCE = (
    "A candidate may rest on it, but apply-approved-updates refuses to review a record whose evidence rests "
    "outside the confirmed sources; cite a source the corpus covers, or have the developer confirm this path "
    "before the record is reviewed."
)


def handle_cite(args: argparse.Namespace) -> int:
    reference = citation(args.repo_root.resolve(), args.path, *_cited_lines(args))
    if args.registry_root:
        reference = classified(reference, source_map_for(args.registry_root.resolve()))
    print_json(reference)
    if args.show:
        _show_cited_lines(args.repo_root.resolve(), reference)
    _note_repeated(args.repo_root.resolve(), reference)
    if reference.get("source_kind") == "unclassified":
        print(
            f"NOTE: {reference['path']} is outside the confirmed sources. {OUTSIDE_SOURCES_CONSEQUENCE}",
            file=sys.stderr,
        )
    return 0


def _cited_lines(args: argparse.Namespace) -> tuple[int, int]:
    given = [form for form, value in (("lines", args.lines), ("start", args.start), ("end", args.end),
                                      ("function", args.function)) if value is not None]
    if given == ["start", "end"]:
        return args.start, args.end
    if given == ["lines"]:
        return line_range(args.lines)
    if given == ["function"]:
        return function_span(args.repo_root, args.path, args.function)
    raise ValueError("cite takes --lines START-END, --start and --end, or --function NAME")


def _show_cited_lines(repo_root: Path, reference: dict) -> None:
    rows = (repo_root / reference["path"]).read_text(encoding="utf-8", errors="replace").splitlines()
    start, end = reference["lines"]["start"], reference["lines"]["end"]
    print(f"{reference['path']}:{start}: {rows[start - 1]}", file=sys.stderr)
    if end - start > 1:
        print(f"{reference['path']}: ... {end - start - 1} lines not shown", file=sys.stderr)
    if end > start:
        print(f"{reference['path']}:{end}: {rows[end - 1]}", file=sys.stderr)


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
        "apply-approved-updates reviews it once the proposal is approved."
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


def handle_fill_harvest_package(args: argparse.Namespace) -> int:
    package_root = args.package_root.resolve()
    documents = harvest_package(package_root, load_json(args.spec.resolve()))
    if args.dry_run:
        print_json(documents)
        return 0
    write_harvest_package(package_root, documents)
    print(f"Filled {package_root} from the spec; submit-proposal validates it, or run validate-change-package first.")
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


def handle_apply_approved_updates(args: argparse.Namespace) -> int:
    apply_approved_updates(
        args.package_root.resolve(),
        args.registry_root.resolve(),
        args.repo_root.resolve(),
    )
    print("Approved registry updates applied.")
    return 0


def handle_discover_sources(args: argparse.Namespace) -> int:
    source_map = discover_sources(args.repo_root.resolve())
    if args.output:
        write_source_map(args.output.resolve(), source_map)
    print_json(source_map)
    return 0


def handle_verify_sources(args: argparse.Namespace) -> int:
    policy = load_json(args.policy.resolve()) if args.policy else None
    result = verify_source_map(
        args.repo_root.resolve(), args.source_map.resolve(), policy
    )
    print_json(result)
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
    print_json(assess_readiness(args.repo_root, args.registry_root))
    return 0


def handle_verify_evidence(args: argparse.Namespace) -> int:
    result = verify_evidence(args.registry_root.resolve(), args.repo_root.resolve(), args.path)
    print_json(result)
    summary = result["summary"]
    failing = ("stale", "unhashed", "missing", "invalid", *(("shifted",) if args.strict else ()))
    found = [f"{summary[status]} {status}" for status in failing if summary[status]]
    if found:
        print(
            f"verify-evidence failed: {', '.join(found)} citations; run recite on each to cite its current lines",
            file=sys.stderr,
        )
    return int(bool(found))


def handle_migrate_evidence(args: argparse.Namespace) -> int:
    migrated = migrate_evidence(args.registry_root.resolve(), args.repo_root.resolve())
    print_json({"migrated": migrated}, indent=None)
    return 0


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
        print("ERROR: a pending apply remains; rerun with --repo-root to reconcile it")
        return 1
    return 0


def handle_init_domain_memory(args: argparse.Namespace) -> int:
    repo_root = args.repo_root.resolve()
    output = args.output.resolve()
    if args.review_mode == "scm-verified" and not args.review_verifier:
        raise ValueError("scm-verified Domain Memory requires --review-verifier")
    if args.review_verifier == "git-signed-commit":
        if not args.authorized_signer:
            raise ValueError(
                "git-signed-commit requires at least one --authorized-signer"
            )
        if args.review_trigger != "git-push":
            raise ValueError("git-signed-commit requires --review-trigger git-push")
    if not output.is_relative_to(repo_root):
        raise ValueError("output must be inside the repository")
    selected = sources_under(repo_root, args.source)
    preliminary = selected_source_map(repo_root, selected)
    try:
        init_registry(output)
    except FileExistsError as error:
        raise ValueError(error) from error
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
    except ValueError:
        shutil.rmtree(output)
        raise
    write_source_map(output / "source-map.json", source_map)
    print(
        f"Domain Memory initialized. The sources are agent-asserted: you selected them, and {args.source_authority}, "
        "the source authority, has not said they are the right corpus. Show them the selected paths and ask. When "
        "they answer, record it with "
        "confirm-sources --confirmed-by <the identity they give you>; until then verify-sources reports the map "
        "as unverified, which is the truth and not a problem to work around."
    )
    return 0


def handle_confirm_sources(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    source_map = confirm_sources(
        registry_root,
        args.repo_root.resolve(),
        args.confirmed_by,
        existing_policy(registry_root),
    )
    print(
        f"Sources confirmed by {args.confirmed_by}: "
        + ", ".join(source_map["selected_paths"])
    )
    return 0


def handle_refresh_sources(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    source_map = refresh_sources(
        registry_root, args.repo_root.resolve(), existing_policy(registry_root)
    )
    print(
        "Sources refreshed and require developer confirmation: "
        + ", ".join(source_map["selected_paths"])
    )
    return 0


def handle_refine_sources(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    repo_root = args.repo_root.resolve()
    changes = args.add or args.remove
    if bool(args.source) == bool(changes):
        raise ValueError(
            "give the whole selection with --source, or change it with --add and --remove, not both"
        )
    sources = (
        adjusted_selection(
            registry_root,
            repo_root,
            sources_under(repo_root, args.add),
            sources_under(repo_root, args.remove),
        )
        if changes
        else sources_under(repo_root, args.source)
    )
    source_map = refine_sources(
        registry_root, repo_root, sources, load_json(policy_path(registry_root))
    )
    print(
        "Focused sources selected and require developer confirmation: "
        + ", ".join(source_map["selected_paths"])
    )
    return 0


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


def handle_scan_secrets(args: argparse.Namespace) -> int:
    report = scan_report(args.repo_root.resolve())
    print_json(report, indent=None)
    if report["findings"]:
        return 1
    return 2 if report["status"] == "incomplete" else 0


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
    _note_unsettled(args.registry_root.resolve())
    return exit_code


def handle_install_git_hitl_hook(args: argparse.Namespace) -> int:
    hook = install_pre_push_hook(
        args.registry_root.resolve(),
        args.repo_root.resolve(),
        Path(__file__).parents[1] / "registry_tools.py",
    )
    print(f"Installed Git HITL pre-push hook: {hook}")
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


def handle_verify_proposal(args: argparse.Namespace) -> int:
    verify_proposal(
        args.package_root.resolve(),
        args.registry_root.resolve(),
        args.repo_root.resolve(),
    )
    print("Proposal test evidence verified.")
    return 0


def handle_record_test_result(args: argparse.Namespace) -> int:
    record_test_result(
        args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve(),
        ObligationRun(args.obligation, args.obligation_command, args.command_profile, args.cwd), args.timeout,
    )
    print(f"Recorded a passing result for {args.obligation}.")
    return 0


def handle_attest_signed_commit(args: argparse.Namespace) -> int:
    commit = attest_signed_commit(
        args.package_root.resolve(), args.registry_root.resolve(), args.repo_root.resolve(),
        args.commit,
    )
    print(f"Attested with signed commit {commit}.")
    return 0


def handle_audit_attestations(args: argparse.Namespace) -> int:
    findings = audit_attestations(
        args.repo_root.resolve(), args.changes_root.resolve(), args.verification_token_env
    )
    print(json.dumps(findings, indent=2, ensure_ascii=False))
    return 0 if all(finding["verdict"] == "carried" for finding in findings) else 1


def handle_finalize_proposal(args: argparse.Namespace) -> int:
    if args.registry_root is None or args.repo_root is None:
        raise ValueError("finalize-proposal requires --registry-root and --repo-root")
    finalize_proposal(
        args.package_root.resolve(),
        args.registry_root.resolve(),
        args.repo_root.resolve(),
        args.verification_token_env,
    )
    print("Proposal approved.")
    return 0


def _unchecked_at(status: str, package_root: Path) -> str:
    if status in ("submitted", "verified") and missing_approval_roles(
        load_json(package_root / REQUIREMENT_FILE), load_json(package_root / PROPOSAL_FILE)
    ):
        return ", apart from the approvals, which are given only once it is submitted"
    return ""


def handle_list_proposals(args: argparse.Namespace) -> int:
    print_json(list_proposals(args.changes_root.resolve()))
    return 0


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
        return exit_code
    unchecked = "" if errors else _unchecked_at(args.as_status, args.package_root)
    return report_errors(errors, f"Change package has everything {args.as_status} requires{unchecked}.")


MUTATED_LOCATION = re.compile(r"^(?P<path>[^:\s]+):\d+: ")


def _note_counterfactuals_off_cited_files(package_root: Path, registry_root: Path | None) -> None:
    checks = load_json(package_root / EVIDENCE_FILE).get("counterfactual_check")
    checks = checks if isinstance(checks, list) else [checks] if isinstance(checks, dict) else []
    obligations = {
        entry.get("id"): entry
        for entry in load_json(package_root / "test-obligations.json").get("obligations", []) if isinstance(entry, dict)
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
                f"NOTE: the counterfactual for {obligation.get('id')} mutates {location['path']}, which "
                f"{', '.join(rule_ids)} does not cite. That proves the rule only when the mutated line lies on the "
                "execution path of a cited line, as the query a cited call runs does; otherwise mutate a cited line.",
                file=sys.stderr,
            )


def handle_coverage(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    changes_root = _changes_root(root, args.changes_root)
    print_json({**coverage(root), **rule_coverage(root, changes_root)})
    if changes_root is None:
        print(f"NOTE: no Change Packages at {root / 'changes'}; pass --changes-root to count the rules a test proves.",
              file=sys.stderr)
    return 0


def handle_quality_gates(args: argparse.Namespace) -> int:
    print_json(quality_gates(args.repo_root))
    return 0


def handle_counterfactual(args: argparse.Namespace) -> int:
    try:
        result = counterfactual(
            args.repo_root,
            Mutation(args.file, args.find, args.replace),
            Proof(args.test_command, args.timeout, args.cwd, args.expect, args.obligation_id),
        )
    except (ValueError, OSError) as error:
        details = error.details if isinstance(error, FailingBaselineError) else {}
        result = {"format": COUNTERFACTUAL_FORMAT, "verdict": "error", "reason": str(error), **details}
    print_json(result)
    return COUNTERFACTUAL_EXIT_CODES[result["verdict"]]


HANDLERS = {
    "quality-gates": handle_quality_gates,
    "counterfactual": handle_counterfactual,
    "list-proposals": handle_list_proposals,
    "init": handle_init,
    "init-change-package": handle_init_change_package,
    "redraft-proposal": handle_redraft_proposal,
    "validate": handle_validate,
    "coverage": handle_coverage,
    "export": handle_export,
    "compare-registry": handle_compare_registry,
    "lookup": handle_lookup,
    "migrate-registry": handle_migrate_registry,
    "resolve-terms": handle_resolve_terms,
    "get-context": handle_get_context,
    "get-record": handle_get_record,
    "dependents": handle_dependents,
    "analyze-boundary": handle_analyze_boundary,
    "boundary-impact": handle_boundary_impact,
    "change-coupling": handle_change_coupling,
    "relocate-evidence": handle_relocate_evidence,
    "recite": handle_recite,
    "cite": handle_cite,
    "upsert-candidate": handle_upsert_candidate,
    "harvest-candidates": handle_harvest_candidates,
    "fill-harvest-package": handle_fill_harvest_package,
    "retract-candidate": handle_retract_candidate,
    "promote-candidate": handle_promote_candidate,
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
    "init-signing-key": handle_init_signing_key,
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
    "record-test-result": handle_record_test_result,
    "attest-signed-commit": handle_attest_signed_commit,
    "audit-attestations": handle_audit_attestations,
    "finalize-proposal": handle_finalize_proposal,
    "validate-change-package": handle_validate_change_package,
}


def execute(args: argparse.Namespace) -> int:
    return HANDLERS[args.command](args)
