from __future__ import annotations

import json
import sys
from pathlib import Path

from ..changes import list_proposals, packages_root


def report_errors(errors: list[str], success_message: str) -> int:
    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors))
        return 1
    print(success_message)
    return 0


def print_json(value: object, indent: int | None = 2) -> None:
    print(json.dumps(value, indent=indent, ensure_ascii=False))


def note_unsettled(registry_root: Path) -> None:
    changes_root = packages_root(registry_root, None)
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


OUTSIDE_SOURCES_CONSEQUENCE = (
    "A candidate may rest on it, but apply-approved-updates refuses to review a record whose evidence rests "
    "outside the confirmed sources; cite a source the corpus covers, or have the developer confirm this path "
    "before the record is reviewed."
)
