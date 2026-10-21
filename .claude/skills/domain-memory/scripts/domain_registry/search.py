from __future__ import annotations

import json
import re
from functools import cache
from itertools import pairwise
from pathlib import Path
from typing import Any

from .common import ASSET_KEYS, record_status
from .okf import CONTEXT_RELATIONS, PROSE_FIELDS, RELATION_TARGETS
from .policy import query_result_limit
from .registry import SUMMARY_FIELDS, asset_records
from .store import read_document


def ranked_lookup(root: Path, asset: str, query: str) -> tuple[list[dict[str, Any]], str]:
    name = f"{asset}.json"
    if name not in ASSET_KEYS:
        raise ValueError(f"unknown asset: {asset}")
    entries, limit = asset_records(root, name), query_result_limit(root)
    needle = query.casefold()
    phrase = [entry for entry in entries if needle in json.dumps(entry, ensure_ascii=False).casefold()]
    if phrase:
        return phrase[:limit], "phrase"
    wanted = query_words(query)
    held = [held_stems(" ".join(_strings(entry))) for entry in entries]
    every = [
        entry for entry, words in zip(entries, held, strict=True) if wanted and all(holds(word, words) for word in wanted)
    ]
    if every:
        return every[:limit], "all-words"
    common = {word for word in wanted if _is_common(sum(holds(word, words) for words in held), len(entries))}
    shared = [
        (sum(holds(word, words) for word in wanted if word not in common), entry)
        for entry, words in zip(entries, held, strict=True)
    ]
    some = [entry for count, entry in sorted(shared, key=lambda pair: -pair[0]) if count]
    if some:
        return some[:limit], "some-words"
    return [], "common-words" if common else "none"


COMMON_SHARE = 4
MIN_COMMON_RECORDS = 3


def _is_common(holders: int, records: int) -> bool:
    return holders >= MIN_COMMON_RECORDS and holders * COMMON_SHARE > records


SUFFIXES = (
    "ations", "ation", "ings", "ing", "ency", "ence", "ents", "ent", "ions", "ion", "ures", "ured", "ure", "ens", "en", "ed", "es", "s",
    "e", "y",
)
MIN_STEM_LENGTH = 3
STOP_WORDS = frozenset({
    "a", "after", "an", "and", "are", "at", "be", "before", "by", "can", "do", "does", "for", "how", "in", "is", "it",
    "of", "on", "or", "that", "the", "this", "to", "what", "when", "who", "with",
})


CAMEL_BOUNDARY = re.compile(r"(?<=[a-z0-9])(?=[A-Z])")


def _words(text: str) -> list[str]:
    return [word for word in re.findall(r"[a-z0-9]+", CAMEL_BOUNDARY.sub(" ", text).casefold()) if word not in STOP_WORDS]


def held_stems(text: str) -> set[str]:
    stems = [_stem(word) for word in _words(text)]
    return {*stems, *(first + second for first, second in pairwise(stems))}


def query_words(query: str) -> list[str]:
    return list({_stem(word): word for word in _words(query)}.values())


def holds(word: str, held: set[str]) -> bool:
    return not _candidate_stems(word).isdisjoint(held)


@cache
def _candidate_stems(word: str) -> frozenset[str]:
    return frozenset({
        _stem(word), *(_stem(word[:split]) + _stem(word[split:]) for split in range(MIN_STEM_LENGTH, len(word) - 1))
    })


@cache
def _stem(word: str) -> str:
    return next(
        (word[: -len(suffix)] for suffix in SUFFIXES
         if word.endswith(suffix) and len(word) - len(suffix) >= MIN_STEM_LENGTH),
        word,
    )


def _strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, dict):
        return [text for child in value.values() for text in _strings(child)]
    if isinstance(value, list):
        return [text for child in value for text in _strings(child)]
    return []


MATCH_KINDS = ("name", "synonym", "definition")


def _match_kind(term: dict[str, Any], needle: str) -> str | None:
    labels = {
        "name": [str(term.get(field, "")).casefold() for field in ("id", "name")],
        "synonym": [str(label).casefold() for label in term.get("synonyms") or []],
    }
    for kind, values in labels.items():
        if any(value and (value in needle or needle in value) for value in values):
            return kind
    if needle in str(term.get("definition", "")).casefold():
        return "definition"
    return None


def every_resolved_term(
    root: Path, query: str, context: str | None, *, with_mentions: bool = False
) -> list[dict[str, Any]]:
    needle = query.casefold()
    matches = []
    for term in asset_records(root, "vocabulary.json"):
        if context is not None and context not in term.get("contexts", []):
            continue
        kind = _match_kind(term, needle)
        if kind is not None:
            matches.append({**term, "match_kind": kind})
    matches.sort(key=lambda match: MATCH_KINDS.index(match["match_kind"]))
    if matches and not with_mentions:
        return matches
    matched = {match["id"] for match in matches}
    mentions = [
        mention for mention in prose_mentions(root, needle)
        if mention["asset"] != "vocabulary" or mention["id"] not in matched
    ]
    return matches + mentions


def _reviewed_entries(root: Path, name: str) -> list[dict[str, Any]]:
    document = read_document(root, name)
    asset = name.removesuffix(".json")
    return [
        entry for entry in document.get(ASSET_KEYS[name], [])
        if record_status(asset, document, entry) == "reviewed"
    ]


DEFINING_FIELDS = ("id", "name", "responsibility", "statement", "definition", "description")
UNSEARCHED_FIELDS = ("status", "review_status")
CONSTRAINT_ASSETS = ("rules", "contracts")


SENTENCE_END = re.compile(r"(?<=[.!?])\s")


def _mention(name: str, entry: dict[str, Any], fields: list[str]) -> dict[str, Any]:
    asset = name.removesuffix(".json")
    summary = next(
        (entry[field] for field in (PROSE_FIELDS.get(asset), *SUMMARY_FIELDS) if isinstance(entry.get(field), str)), ""
    )
    return {
        "match_kind": "mentioned", "asset": asset, "id": entry.get("id"), "status": "reviewed",
        "contexts": _contexts_named(asset, entry), "summary": SENTENCE_END.split(summary.strip(), maxsplit=1)[0],
        "fields": fields,
    }


def _contexts_named(asset: str, entry: dict[str, Any]) -> list[Any]:
    if asset == "contexts":
        return [entry.get("id")]
    named = [
        context
        for field, value in entry.items()
        if field in CONTEXT_RELATIONS
        for context in (value if isinstance(value, list) else [value])
    ]
    return list(dict.fromkeys(named))


def prose_mentions(root: Path, needle: str) -> list[dict[str, Any]]:
    wanted = query_words(needle)
    if not wanted:
        return []
    mentions = [
        _mention(name, entry, fields)
        for name in sorted(ASSET_KEYS)
        for entry in _reviewed_entries(root, name)
        if (fields := _mentioning_fields(entry, wanted))
    ]
    return sorted(
        mentions,
        key=lambda mention: (
            not any(field in DEFINING_FIELDS for field in mention["fields"]),
            mention["asset"] not in CONSTRAINT_ASSETS,
        ),
    )


def _mentioning_fields(entry: dict[str, Any], wanted: list[str]) -> list[str]:
    return [
        field for field, value in entry.items()
        if field not in UNSEARCHED_FIELDS
        and any(all(holds(word, stems) for word in wanted) for stems in map(held_stems, _searchable(field, value)))
    ]


def _searchable(field: str, value: Any) -> list[str]:
    if field == "id":
        return [value.replace("-", " ")] if isinstance(value, str) else []
    if isinstance(value, str):
        return [value]
    if isinstance(value, list) and field not in RELATION_TARGETS:
        return [item for item in value if isinstance(item, str)]
    return []
