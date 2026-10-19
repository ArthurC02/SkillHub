from __future__ import annotations

import argparse
import sys
from collections.abc import Callable
from pathlib import Path

from ..changes import parsed_update
from ..common import FAILING_EVIDENCE, PROPOSAL_FILE, load_json
from ..evidence import (
    citation,
    classified,
    function_span,
    repeated_note,
    source_map_for,
    staged_content,
)
from ..policy import existing_policy, policy_path, write_policy
from ..registry import init_registry, migrate_evidence, verify_evidence
from ..relocate import Recitation, line_range, recite, relocate_evidence
from ..security import scan_report
from ..sources import (
    adjusted_selection,
    confirm_sources,
    discover_sources,
    discovery_drift_note,
    refine_sources,
    refresh_confirmation_notice,
    refresh_listing,
    refresh_sources,
    selected_source_map,
    verify_against_discovery,
    write_source_map,
)
from ..transaction import remove_tree
from .common import OUTSIDE_SOURCES_CONSEQUENCE, print_json


def sources_under(repo_root: Path, sources: list[Path]) -> list[Path]:
    return [repo_root / source for source in sources]


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


def handle_discover_sources(args: argparse.Namespace) -> int:
    source_map = discover_sources(args.repo_root.resolve())
    if args.output:
        write_source_map(args.output.resolve(), source_map)
    print_json(source_map)
    return 0


def handle_verify_sources(args: argparse.Namespace) -> int:
    policy = load_json(args.policy.resolve()) if args.policy else None
    result = verify_against_discovery(
        args.repo_root.resolve(), args.source_map.resolve(), policy
    )
    print_json(result)
    if note := discovery_drift_note(result, args.source_map.parent, args.repo_root):
        print(note, file=sys.stderr)
    return 0 if result["status"] in ("current", "discovery_drifted") else 1


def _failing_citation_line(item: dict) -> str:
    lines = item.get("cited_lines")
    span = item.get("path", "its evidence") + (f":{lines['start']}-{lines['end']}" if lines else "")
    record = f"{item['asset']}/{item['id']}"
    if item["status"] == "stale":
        return f"{record}: {span} changed; check that its statement still holds; recite only if it does, otherwise amend it"
    if item["status"] == "missing":
        return f"{record}: {item['path']} is not in this commit; relocate-evidence if it moved, otherwise amend or retire the record"
    return f"{record}: {span} is {item['status']}" + (f"; {item['reason']}" if "reason" in item else "")


def _staged_failure(result: dict, failing: tuple[str, ...], found: list[str]) -> str:
    count = sum(result["summary"][status] for status in failing)
    return "\n".join([
        *(_failing_citation_line(item) for item in result["results"] if item["status"] in failing),
        f"verify-evidence failed: {', '.join(found)} citation{'' if count == 1 else 's'}",
    ])


def handle_verify_evidence(args: argparse.Namespace) -> int:
    repo_root = args.repo_root.resolve()
    content_of = staged_content(repo_root) if args.staged else None
    overlay = package_overlay(args.package_root.resolve()) if args.package_root else None
    result = verify_evidence(args.registry_root.resolve(), repo_root, args.path, content_of, overlay)
    print_json(result)
    summary = result["summary"]
    failing = (*FAILING_EVIDENCE, *(("shifted",) if args.strict else ()))
    found = [f"{summary[status]} {status}" for status in failing if summary[status]]
    if found:
        print(
            _staged_failure(result, failing, found) if args.staged
            else f"verify-evidence failed: {', '.join(found)} citations; run recite on each to cite its current lines",
            file=sys.stderr,
        )
    return int(bool(found))


def package_overlay(package_root: Path) -> dict[str, dict[str, dict | None]]:
    overlay: dict[str, dict[str, dict | None]] = {}
    for raw in load_json(package_root / PROPOSAL_FILE).get("registry_updates") or []:
        update = parsed_update(raw)
        overlay.setdefault(update.document_name, {})[update.record_id] = update.record
    return overlay


def handle_migrate_evidence(args: argparse.Namespace) -> int:
    migrated = migrate_evidence(args.registry_root.resolve(), args.repo_root.resolve())
    print_json({"migrated": migrated}, indent=None)
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
    init_registry(output)
    try:
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
        source_map = selected_source_map(
            repo_root, selected, load_json(policy_path(output))
        )
    except ValueError:
        remove_tree(output)
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
    print(refresh_confirmation_notice(source_map["selected_paths"]))
    return 0


def handle_refresh_listing(args: argparse.Namespace) -> int:
    registry_root = args.registry_root.resolve()
    change = refresh_listing(
        registry_root, args.repo_root.resolve(), existing_policy(registry_root)
    )
    added, removed = len(change["added"]), len(change["removed"])
    if added or removed:
        print(f"Listing refreshed: +{added} -{removed} discovered {'path' if added + removed == 1 else 'paths'}")
    else:
        print("Listing already current")
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


def handle_scan_secrets(args: argparse.Namespace) -> int:
    report = scan_report(args.repo_root.resolve())
    print_json(report, indent=None)
    if report["findings"]:
        return 1
    return 2 if report["status"] == "incomplete" else 0


HANDLERS: dict[str, Callable[[argparse.Namespace], int]] = {
    "relocate-evidence": handle_relocate_evidence,
    "recite": handle_recite,
    "cite": handle_cite,
    "discover-sources": handle_discover_sources,
    "verify-sources": handle_verify_sources,
    "verify-evidence": handle_verify_evidence,
    "migrate-evidence": handle_migrate_evidence,
    "init-domain-memory": handle_init_domain_memory,
    "confirm-sources": handle_confirm_sources,
    "refresh-sources": handle_refresh_sources,
    "refresh-listing": handle_refresh_listing,
    "refine-sources": handle_refine_sources,
    "scan-secrets": handle_scan_secrets,
}
