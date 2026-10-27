from __future__ import annotations

import argparse
import io
import sys
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

from ..changes import list_proposals, packages_root, proof_of, rule_coverage
from ..common import ASSET_KEYS, load_json, shown_path
from ..locate import domain_memory_hint
from ..policy import policy_path, query_result_limit
from ..registry import (
    asset_records,
    boundary_analysis,
    context_model,
    context_summary,
    coverage,
    record_by_id,
)
from ..sources import counted, discovery_drift_note, probe_sources, refresh_listing_advice
from ..store import committed_documents, documents_read_once, read_document
from .common import print_json

if TYPE_CHECKING:
    from ..counterfactual import Mutation


COUNTERFACTUAL_EXIT_CODES = {"killed": 0, "survived": 1, "inconclusive": 2, "error": 3}


def probe_summary(result: dict, registry_root: Path, repo_root: Path) -> str:
    where = f"Domain Memory at {shown_path(registry_root, repo_root)}"
    if result["status"] == "absent" and "hint" in result:
        return f"{where}: no source map here, but {result['hint']}."
    if result["status"] == "absent":
        return _closed(f"{where}: none; {domain_memory_hint(registry_root, repo_root)}")
    if result["status"] in ("current", "discovery_drifted"):
        changed_inside = result.get("content_changed", [])
        inside = (
            f"; {len(changed_inside)} selected sources changed inside ({_named(changed_inside)}); "
            "verify-evidence checks the citations"
            if changed_inside else ""
        )
        gone, unlisted = len(result.get("discovered_but_gone", [])), len(result.get("discovered_but_unlisted", []))
        drifted = (
            f"; discovery drifted: {counted(gone, 'listed path')} gone, {counted(unlisted, 'matching path')} "
            f"unlisted; {refresh_listing_advice(registry_root, repo_root)}"
            if result["status"] == "discovery_drifted" else ""
        )
        return f"{where}: current, {result['selection_status']}, established by {result['checked']}{inside}{drifted}."
    if result["status"] == "stale":
        return (
            f"{where}: stale. {len(result['changed_sources'])} selected sources gained, lost or renamed files "
            f"({_named(result['changed_sources'])}); run refresh-sources, then have a developer run confirm-sources."
        )
    return f"{where}: {result['status']}. {result.get('reason', '')}".strip()


def _closed(sentence: str) -> str:
    return sentence if sentence.endswith("?") else f"{sentence}."


def _named(sources: list[dict]) -> str:
    return ", ".join(source["path"] for source in sources[:5])


def handle_lookup(args: argparse.Namespace) -> int:
    from ..search import ranked_lookup

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


def handle_resolve_terms(args: argparse.Namespace) -> int:
    from ..search import CONSTRAINT_ASSETS, MATCH_KINDS, every_resolved_term, prose_mentions

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
    from ..compare import compare_registries

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
    changes_root = packages_root(root, args.changes_root)
    if changes_root is not None and args.asset in PROVEN_ASSETS:
        result = _with_proofs(root, changes_root, args.asset, args.id, result)
    print_json(result)
    return 0


PROVEN_ASSETS = ("rules", "contracts")


def _with_proofs(root: Path, changes_root: Path, asset: str, identifier: str, record: dict) -> dict:
    from ..audit import review_history

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
    import difflib

    known = [record["id"] for record in records if isinstance(record.get("id"), str)]
    starting = [candidate for candidate in known if candidate.startswith(identifier)]
    close = list(dict.fromkeys([*starting, *difflib.get_close_matches(identifier, known, n=3)]))[:3]
    return f"; did you mean {', '.join(close)}?" if close else ""


def handle_change_coupling(args: argparse.Namespace) -> int:
    from ..coupling import CouplingLimits, CouplingReading, change_coupling
    from ..impact import registered_collaborations

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
    from ..impact import boundary_impact

    print_json(boundary_impact(args.registry_root.resolve(), args.repo_root.resolve(), args.base))
    return 0


def _brief_subject_refusal(base: str | None, asset: str | None, record_id: str | None) -> str | None:
    by_record = asset is not None or record_id is not None
    if base is not None and by_record:
        return "brief takes --base <revision> or --asset <asset> --id <id>, not both"
    if base is None and not by_record:
        return "brief needs --base <revision>, or --asset <asset> --id <id>"
    if by_record and not (asset and record_id):
        return "brief by record needs both --asset <asset> and --id <id>"
    return None


def handle_brief(args: argparse.Namespace) -> int:
    with documents_read_once():
        return _handle_brief(args)


def _handle_brief(args: argparse.Namespace) -> int:
    from ..brief import brief_for_change, brief_for_record, to_markdown

    refusal = _brief_subject_refusal(args.base, args.asset, args.id)
    if refusal:
        raise ValueError(refusal)
    root, repo_root = args.registry_root.resolve(), args.repo_root.resolve()
    brief = brief_for_change(root, repo_root, args.base) if args.base else brief_for_record(root, repo_root, args.asset, args.id)
    if args.markdown:
        print(to_markdown(brief))
    else:
        print_json(brief)
    return 0


def handle_dependents(args: argparse.Namespace) -> int:
    from ..dependents import dependents

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


def handle_probe(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    policy_file = args.policy.resolve() if args.policy else policy_path(registry_root)
    policy = load_json(policy_file) if policy_file.exists() else None
    result = probe_sources(
        args.repo_root.resolve(), registry_root / "source-map.json", policy
    )
    print(probe_summary(result, args.registry_root, args.repo_root))
    return {"current": 0, "discovery_drifted": 0, "absent": 2}.get(result["status"], 1)


def handle_readiness(args: argparse.Namespace) -> int:
    with documents_read_once():
        return _handle_readiness(args)


def _handle_readiness(args: argparse.Namespace) -> int:
    from ..readiness import readiness_report

    result, located = readiness_report(args.repo_root, args.registry_root)
    print_json(result)
    if located:
        print(located, file=sys.stderr)
    registry_root = args.registry_root or (args.repo_root / result["registry_root"] if result["registry_root"] else None)
    drifted = next((s for s in result["signals"] if s.get("kind") == "listing" and s.get("status") == "drifted"), None)
    if drifted and (note := discovery_drift_note(drifted | {"status": "discovery_drifted"}, registry_root, args.repo_root)):
        print(note, file=sys.stderr)
    return 0


def handle_list_proposals(args: argparse.Namespace) -> int:
    print_json(list_proposals(args.changes_root.resolve()))
    return 0


def handle_coverage(args: argparse.Namespace) -> int:
    with documents_read_once():
        return _handle_coverage(args)


def _handle_coverage(args: argparse.Namespace) -> int:
    root = args.registry_root.resolve()
    changes_root = packages_root(root, args.changes_root)
    print_json({**coverage(root), **rule_coverage(root, changes_root)})
    if changes_root is None:
        print(f"NOTE: no Change Packages at {root / 'changes'}; pass --changes-root to count the rules a test proves.",
              file=sys.stderr)
    return 0


def handle_quality_gates(args: argparse.Namespace) -> int:
    from ..gates import quality_gates

    print_json(quality_gates(args.repo_root))
    return 0


def handle_counterfactual(args: argparse.Namespace) -> int:
    from ..counterfactual import FORMAT as COUNTERFACTUAL_FORMAT
    from ..counterfactual import FailingBaselineError, Proof, counterfactual, counterfactual_suite
    from ..hitl import require_recordable_counterfactual

    package = args.package_root
    try:
        if args.suite is not None:
            if package is not None:
                raise ValueError(
                    "--package-root records one counterfactual; run each obligation's mutation on its own with --obligation-id"
                )
            suite = counterfactual_suite(args.repo_root, load_json(args.suite))
            print_json(suite)
            return max((COUNTERFACTUAL_EXIT_CODES[entry["verdict"]] for entry in suite["results"]), default=0)
        if package is not None:
            require_recordable_counterfactual(package, args.obligation_id)
        result = counterfactual(
            args.repo_root,
            _one_mutation(args),
            Proof(args.test_command, args.timeout, args.cwd, args.expect, args.obligation_id),
        )
    except (ValueError, OSError) as error:
        details = error.details if isinstance(error, FailingBaselineError) else {}
        result = {"format": COUNTERFACTUAL_FORMAT, "verdict": "error", "reason": str(error), **details}
    if package is not None and result["verdict"] != "error":
        result = _with_record(package, args.repo_root, result)
    print_json(result)
    return COUNTERFACTUAL_EXIT_CODES["error" if "not_recorded" in result and result["verdict"] == "killed" else result["verdict"]]


def _with_record(package: Path, repo_root: Path, result: dict) -> dict:
    from ..hitl import record_counterfactual

    check = result.get("counterfactual_check")
    if check is None:
        return {**result, "not_recorded": "only a killed counterfactual whose file was restored is recorded"}
    try:
        recorded = record_counterfactual(package, check)
    except ValueError as error:
        return {**result, "not_recorded": str(error)}
    return {**result, "recorded_in": shown_path(recorded, repo_root)}


def _one_mutation(args: argparse.Namespace) -> Mutation:
    from ..counterfactual import Mutation

    missing = [
        name for name, value in (
            ("--file", args.file), ("--find", args.find), ("--replace", args.replace), ("--test-command", args.test_command)
        ) if value is None
    ]
    if missing:
        raise ValueError(f"counterfactual needs {', '.join(missing)}, or --suite <json file> for several mutations")
    return Mutation(args.file, args.find, args.replace)


def handle_mcp_serve(args: argparse.Namespace) -> int:
    from ..cli_commands import run_command
    from ..mcp_server import ServerRoots, serve

    if isinstance(sys.stdin, io.TextIOWrapper):
        sys.stdin.reconfigure(encoding="utf-8")
    serve(ServerRoots(args.registry_root.resolve(), args.repo_root.resolve()), run_command)
    return 0


HANDLERS: dict[str, Callable[[argparse.Namespace], int]] = {
    "mcp-serve": handle_mcp_serve,
    "quality-gates": handle_quality_gates,
    "counterfactual": handle_counterfactual,
    "list-proposals": handle_list_proposals,
    "coverage": handle_coverage,
    "export": handle_export,
    "compare-registry": handle_compare_registry,
    "lookup": handle_lookup,
    "resolve-terms": handle_resolve_terms,
    "get-context": handle_get_context,
    "get-record": handle_get_record,
    "dependents": handle_dependents,
    "brief": handle_brief,
    "analyze-boundary": handle_analyze_boundary,
    "boundary-impact": handle_boundary_impact,
    "change-coupling": handle_change_coupling,
    "readiness": handle_readiness,
    "probe": handle_probe,
}
