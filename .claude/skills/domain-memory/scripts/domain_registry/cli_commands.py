from __future__ import annotations

import argparse

from .cli_parser import build_parser
from .commands import evidence, governance, lifecycle, read

__all__ = [
    "HANDLERS",
    "execute",
    "run_command",
]

HANDLERS = {**read.HANDLERS, **evidence.HANDLERS, **lifecycle.HANDLERS, **governance.HANDLERS}


def run_command(words: list[str]) -> int:
    return execute(build_parser().parse_args(words))


def execute(args: argparse.Namespace) -> int:
    return HANDLERS[args.command](args)
