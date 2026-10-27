from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

ENGINE = Path(__file__).resolve().parents[1]
PLUGIN_SCRIPTS = ENGINE.parent / "scripts"
CONTRACT = ENGINE / "contract" / "cli.json"
PROG = "registry_tools.py"
PYTHON = "3.13"
TYPES = {"Path": "path", "int": "int", "asset_name": "asset", "query_text": "query", "str": "str"}
KINDS = {
    argparse._StoreAction: "store",
    argparse._AppendAction: "append",
    argparse._StoreTrueAction: "store_true",
    argparse._HelpAction: "help",
}


def described(action: argparse.Action) -> dict:
    type_name = getattr(action.type, "__name__", None) or "str"
    default = action.default
    return {
        "dest": action.dest,
        "options": list(action.option_strings),
        "kind": KINDS[type(action)],
        "required": bool(action.required),
        "type": TYPES[type_name],
        "choices": list(action.choices) if action.choices else None,
        "default": str(default) if isinstance(default, (str, os.PathLike)) else default,
    }


def contract() -> dict:
    sys.path.insert(0, str(PLUGIN_SCRIPTS))
    from domain_registry.cli_parser import build_parser

    parser = build_parser()
    parser.prog = PROG
    subcommands = next(action for action in parser._actions if isinstance(action, argparse._SubParsersAction))
    commands = {}
    for name, sub in subcommands.choices.items():
        sub.prog = f"{PROG} {name}"
        commands[name] = {
            "prog": sub.prog,
            "usage": sub.format_usage(),
            "help": sub.format_help(),
            "actions": [described(action) for action in sub._actions],
            "mutex": [
                {"required": group.required, "dests": [action.dest for action in group._group_actions]}
                for group in sub._mutually_exclusive_groups
            ],
        }
    return {
        "python": f"{sys.version_info.major}.{sys.version_info.minor}",
        "prog": PROG,
        "usage": parser.format_usage(),
        "help": parser.format_help(),
        "commands": commands,
    }


def main() -> int:
    if f"{sys.version_info.major}.{sys.version_info.minor}" != PYTHON:
        print(f"extract the CLI contract with Python {PYTHON}; this is {sys.version.split()[0]}", file=sys.stderr)
        return 2
    os.environ["COLUMNS"] = "80"
    text = json.dumps(contract(), indent=1, ensure_ascii=False) + "\n"
    if sys.argv[1:] == ["--check"]:
        if CONTRACT.read_text(encoding="utf-8") != text:
            print(f"{CONTRACT} is out of date; run parity/extract_cli_contract.py with Python {PYTHON}", file=sys.stderr)
            return 1
        return 0
    CONTRACT.parent.mkdir(parents=True, exist_ok=True)
    CONTRACT.write_text(text, encoding="utf-8", newline="\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
