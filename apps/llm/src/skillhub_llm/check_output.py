"""Count sentences, characters and items in a file and check them against limits."""

from __future__ import annotations

import argparse
import re
import sys

SENTENCE_SPLIT = re.compile(r"[。！？!?]|\n")
ITEM_LINE = re.compile(r"^\s*(?:[-*•]|\d+[.)、．])\s")


def count_sentences(text: str) -> int:
    return sum(1 for part in SENTENCE_SPLIT.split(text) if part.strip())


def count_chars(text: str) -> int:
    return sum(1 for ch in text if not ch.isspace())


def count_items(text: str) -> int:
    return sum(1 for line in text.splitlines() if ITEM_LINE.match(line))


def missing_requirements(text: str, requirements: list[str]) -> list[str]:
    return [req for req in requirements if req not in text]


def check(
    text: str,
    max_sentences: int | None,
    max_chars: int | None,
    max_items: int | None,
    require: list[str],
) -> list[str]:
    failures: list[str] = []
    sentences = count_sentences(text)
    if max_sentences is not None and sentences > max_sentences:
        failures.append(f"sentences {sentences} > {max_sentences}")
    chars = count_chars(text)
    if max_chars is not None and chars > max_chars:
        failures.append(f"chars {chars} > {max_chars}")
    items = count_items(text)
    if max_items is not None and items > max_items:
        failures.append(f"items {items} > {max_items}")
    missing = missing_requirements(text, require)
    if missing:
        failures.append("missing: " + ", ".join(missing))
    return failures


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=("Check a file's draft answer against countable limits and required facts.")
    )
    parser.add_argument(
        "--max-sentences",
        type=int,
        help=(
            "reject if the file has more sentences than this; a sentence is a run of "
            "text separated by 。！？!? or a newline, trimmed and non-empty"
        ),
    )
    parser.add_argument(
        "--max-chars",
        type=int,
        help="reject if the file has more non-whitespace characters than this",
    )
    parser.add_argument(
        "--max-items",
        type=int,
        help=(
            "reject if the file has more list items than this; an item is a line "
            "starting with -, * or •, or a numbered marker such as '1.' '2)' '3、'"
        ),
    )
    parser.add_argument(
        "--require",
        action="append",
        default=[],
        metavar="TEXT",
        help="reject if this exact text is missing from the file; repeatable",
    )
    parser.add_argument("file", metavar="FILE", help="path to check, or - for stdin")
    return parser


def _read(path: str) -> str:
    if path == "-":
        return sys.stdin.read()
    with open(path, encoding="utf-8") as handle:
        return handle.read()


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    no_checks = (
        args.max_sentences is None
        and args.max_chars is None
        and args.max_items is None
        and not args.require
    )
    if no_checks:
        print(
            "no checks given: pass at least one of --max-sentences, --max-chars, "
            "--max-items, --require"
        )
        return 2
    try:
        text = _read(args.file)
    except OSError as exc:
        print(f"cannot read {args.file}: {exc}")
        return 2
    except UnicodeDecodeError as exc:
        print(f"cannot decode {args.file} as UTF-8: {exc}")
        return 2
    failures = check(text, args.max_sentences, args.max_chars, args.max_items, args.require)
    if not failures:
        print("OK")
        return 0
    for failure in failures:
        print(failure)
    return 1


if __name__ == "__main__":
    sys.exit(main())
