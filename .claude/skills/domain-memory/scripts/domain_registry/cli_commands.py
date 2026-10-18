from __future__ import annotations

import argparse
import sys

from .cli_parser import LEGACY_REPLACEMENTS, build_parser
from .commands import evidence, governance, lifecycle, read
from .commands.common import print_json
from .commands.governance import policy_assignments
from .commands.read import COUNTERFACTUAL_EXIT_CODES, COUNTERFACTUAL_FORMAT

__all__ = [
    "COUNTERFACTUAL_EXIT_CODES",
    "COUNTERFACTUAL_FORMAT",
    "HANDLERS",
    "execute",
    "policy_assignments",
    "print_json",
    "run_command",
]

HANDLERS = {**read.HANDLERS, **evidence.HANDLERS, **lifecycle.HANDLERS, **governance.HANDLERS}


def run_command(words: list[str]) -> int:
    return execute(build_parser().parse_args(words))


def execute(args: argparse.Namespace) -> int:
    if args.command in LEGACY_REPLACEMENTS:
        print(
            f"NOTE: {args.command} is legacy; use {LEGACY_REPLACEMENTS[args.command]}; it is removed in 1.0.0",
            file=sys.stderr,
        )
    return HANDLERS[args.command](args)
