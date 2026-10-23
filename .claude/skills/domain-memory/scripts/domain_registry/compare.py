from __future__ import annotations

import re
from collections.abc import Callable
from pathlib import PurePosixPath
from typing import Any

from .common import ASSET_KEYS, whole_number
from .evidence import all_references, one_cites_within_the_other
from .okf import contexts_named_by

PAIRED_ASSETS = {
    "interactions": ("producer_context", "consumer_context"),
    "dependency-policies": ("from_context", "to_context"),
}
NAME_FIELDS = {"contexts": "name", "vocabulary": "name", "aggregates": "root"}
WORDING_FIELDS = ("id", "name", "statement", "definition", "responsibility", "description")
EXACT_PATH, PATH_SUFFIX, NESTED_PATH, SAME_NAME, SHARED_EVIDENCE = 5, 4, 3, 2, 1
SCORE_LABELS = {
    EXACT_PATH: "path",
    PATH_SUFFIX: "path-suffix",
    NESTED_PATH: "nested-path",
    SAME_NAME: "name",
    SHARED_EVIDENCE: "context-and-evidence",
}
POSSIBLE_PAIR_SIMILARITY = 0.25
MIN_WORD_LENGTH = 3

Score = tuple[int, int]
Citation = tuple[str, int, int]


def compare_registries(
    gold: dict[str, Any], candidate: dict[str, Any], contexts: list[str] | None = None
) -> dict[str, Any]:
    gold_records, candidate_records = _records_by_asset(gold), _records_by_asset(candidate)
    matched_contexts = _matched("contexts", gold_records["contexts"], candidate_records["contexts"], {})
    context_of = {pair["candidate"]: pair["gold"] for pair in matched_contexts["matched"]}
    if contexts:
        unknown = sorted(set(contexts) - {record["id"] for record in gold_records["contexts"]})
        if unknown:
            raise ValueError(f"no Context {', '.join(unknown)} in the gold Registry")
        gold_records = _within(gold_records, set(contexts))
        candidate_records = _within(candidate_records, {pair for pair, paired in context_of.items() if paired in contexts})
        matched_contexts = _matched("contexts", gold_records["contexts"], candidate_records["contexts"], {})
    assets = {"contexts": matched_contexts}
    for asset in gold_records:
        if asset != "contexts":
            assets[asset] = _matched(asset, gold_records[asset], candidate_records[asset], context_of)
    attempted = [result for result in assets.values() if result["candidate"] and result["gold"]]
    copied = sum(1 for result in attempted for pair in result["matched"] if pair.get("copied_evidence"))
    report = {
        "assets": assets,
        "overall": _overall(assets.values()),
        "overall_attempted": _overall(attempted),
        "overall_independent": _overall(attempted, copied),
        "copied_matches": copied,
    }
    if contexts:
        paired = {pair["gold"] for pair in matched_contexts["matched"]}
        report["scope"] = {"contexts": sorted(set(contexts)), "without_candidate": sorted(set(contexts) - paired)}
    return report


def _overall(results: Any, left_out: int = 0) -> dict[str, int | float | None]:
    results = list(results)
    counts = {
        "matched": sum(len(result["matched"]) for result in results) - left_out,
        "gold": sum(result["gold"] for result in results) - left_out,
        "candidate": sum(result["candidate"] for result in results) - left_out,
    }
    return {**counts, **_scores(counts["matched"], counts["gold"], counts["candidate"])}


def _within(records: dict[str, list[dict[str, Any]]], contexts: set[str]) -> dict[str, list[dict[str, Any]]]:
    return {
        asset: [record for record in entries if contexts_named_by(asset, record) & contexts]
        if asset != "contexts"
        else [record for record in entries if record["id"] in contexts]
        for asset, entries in records.items()
    }


def _records_by_asset(documents: dict[str, Any]) -> dict[str, list[dict[str, Any]]]:
    records = {}
    for name, key in ASSET_KEYS.items():
        asset = name.removesuffix(".json")
        document = documents.get(asset) or {}
        value = document.get(key) if isinstance(document, dict) else None
        records[asset] = [record for record in value or [] if isinstance(record, dict) and record.get("id")]
    return records


def _matched(
    asset: str, gold: list[dict[str, Any]], candidate: list[dict[str, Any]], context_of: dict[str, str]
) -> dict[str, Any]:
    match_score = _scorer(asset, gold, candidate, context_of)
    scored = sorted(
        (
            (score, index, gold_index)
            for index in range(len(candidate))
            for gold_index in range(len(gold))
            if (score := match_score(gold_index, index))
        ),
        key=lambda entry: (-entry[0][0], -entry[0][1], entry[1], entry[2]),
    )
    taken_candidates, taken_gold, matched = set(), set(), []
    for score, index, gold_index in scored:
        if index in taken_candidates or gold_index in taken_gold:
            continue
        taken_candidates.add(index)
        taken_gold.add(gold_index)
        pair = {"candidate": candidate[index]["id"], "gold": gold[gold_index]["id"], "by": SCORE_LABELS[score[0]]}
        if asset != "contexts" and one_cites_within_the_other(gold[gold_index], candidate[index]):
            pair["copied_evidence"] = True
        matched.append(pair)
    missed = [gold_index for gold_index in range(len(gold)) if gold_index not in taken_gold]
    extra = [index for index in range(len(candidate)) if index not in taken_candidates]
    return {
        "gold": len(gold),
        "candidate": len(candidate),
        "matched": sorted(matched, key=lambda pair: pair["gold"]),
        "missed": sorted(gold[gold_index]["id"] for gold_index in missed),
        "extra": sorted(candidate[index]["id"] for index in extra),
        "possible_pairs": _possible_pairs(asset, gold, candidate, context_of, (missed, extra)),
        **_scores(len(matched), len(gold), len(candidate)),
    }


def _scorer(
    asset: str, gold: list[dict[str, Any]], candidate: list[dict[str, Any]], context_of: dict[str, str]
) -> Callable[[int, int], Score | None]:
    if asset in PAIRED_ASSETS:
        ends = PAIRED_ASSETS[asset]
        gold_ends = [tuple(record.get(end) for end in ends) for record in gold]
        candidate_ends = [tuple(context_of.get(record.get(end)) for end in ends) for record in candidate]
        return lambda gold_index, index: (EXACT_PATH, 0) if candidate_ends[index] == gold_ends[gold_index] else None
    gold_names = [_names(asset, record, with_synonyms=True) for record in gold]
    candidate_names = [_names(asset, record, with_synonyms=False) for record in candidate]
    if asset == "contexts":
        gold_parts = [_path_parts(record.get("implementation_path")) for record in gold]
        candidate_parts = [_path_parts(record.get("implementation_path")) for record in candidate]

        def context_score(gold_index: int, index: int) -> Score | None:
            tier = _path_score(gold_parts[gold_index], candidate_parts[index])
            tier = tier or (SAME_NAME if gold_names[gold_index] & candidate_names[index] else 0)
            return (tier, 0) if tier else None

        return context_score
    gold_contexts = [contexts_named_by(asset, record) for record in gold]
    candidate_contexts = [_mapped(asset, record, context_of) for record in candidate]
    gold_citations = [_citations(record) for record in gold]
    candidate_citations = [_citations(record) for record in candidate]

    def score(gold_index: int, index: int) -> Score | None:
        if gold_names[gold_index] & candidate_names[index]:
            return SAME_NAME, 0
        if not gold_contexts[gold_index] & candidate_contexts[index]:
            return None
        shared = _shared_files(gold_citations[gold_index], candidate_citations[index])
        if not shared:
            return None
        return SHARED_EVIDENCE, _overlapping_lines(gold_citations[gold_index], candidate_citations[index], shared)

    return score


def _possible_pairs(
    asset: str,
    gold: list[dict[str, Any]],
    candidate: list[dict[str, Any]],
    context_of: dict[str, str],
    unpaired: tuple[list[int], list[int]],
) -> list[dict[str, Any]]:
    if asset == "contexts" or asset in PAIRED_ASSETS:
        return []
    missed, extra = unpaired
    words = {index: _words(candidate[index]) for index in extra}
    mapped = {index: _mapped(asset, candidate[index], context_of) for index in extra}
    possible = []
    for gold_index in missed:
        expected, named = _words(gold[gold_index]), contexts_named_by(asset, gold[gold_index])
        ranked = [
            (_similarity(expected, words[index]), candidate[index]["id"])
            for index in extra
            if named & mapped[index]
        ]
        best = max(ranked, default=None, key=lambda entry: (entry[0], entry[1]))
        if best and best[0] >= POSSIBLE_PAIR_SIMILARITY:
            possible.append({"gold": gold[gold_index]["id"], "candidate": best[1], "similarity": round(best[0], 2)})
    return sorted(possible, key=lambda pair: (-pair["similarity"], pair["gold"]))


def _mapped(asset: str, record: dict[str, Any], context_of: dict[str, str]) -> set[str | None]:
    return {context_of.get(context) for context in contexts_named_by(asset, record)}


def _words(record: dict[str, Any]) -> set[str]:
    text = " ".join(str(record.get(field, "")) for field in WORDING_FIELDS).casefold()
    return {word for word in re.findall(r"[0-9a-z]+", text) if len(word) >= MIN_WORD_LENGTH}


def _similarity(first: set[str], second: set[str]) -> float:
    return len(first & second) / len(first | second) if first | second else 0.0


def _path_parts(path: Any) -> tuple[str, ...] | None:
    parts = PurePosixPath(path.strip("/")).parts if isinstance(path, str) else None
    return parts or None


def _path_score(gold: tuple[str, ...] | None, candidate: tuple[str, ...] | None) -> int:
    if gold is None or candidate is None:
        return 0
    if gold == candidate:
        return EXACT_PATH
    shorter, longer = sorted((gold, candidate), key=len)
    if not shorter:
        return 0
    if longer[-len(shorter):] == shorter:
        return PATH_SUFFIX
    inside = any(longer[start : start + len(shorter)] == shorter for start in range(len(longer) - len(shorter) + 1))
    return NESTED_PATH if inside else 0


def _names(asset: str, record: dict[str, Any], *, with_synonyms: bool) -> set[str]:
    field = NAME_FIELDS.get(asset)
    names = {_normalized(record.get("id"))} | ({_normalized(record.get(field))} if field else set())
    if with_synonyms and asset == "vocabulary":
        names |= {_normalized(synonym) for synonym in record.get("synonyms") or []}
    return names - {""}


def _normalized(value: Any) -> str:
    return re.sub(r"[^0-9a-z]+", "", value.casefold()) if isinstance(value, str) else ""


def _citations(record: dict[str, Any]) -> list[Citation]:
    citations = []
    for reference in all_references(record):
        if isinstance(reference, dict) and isinstance(reference.get("path"), str):
            lines = reference.get("lines") if isinstance(reference.get("lines"), dict) else {}
            start, end = lines.get("start"), lines.get("end")
            valid = whole_number(start) and whole_number(end)
            citations.append((reference["path"], start if valid else 0, end if valid else -1))
    return citations


def _shared_files(gold: list[Citation], candidate: list[Citation]) -> set[str]:
    return {path for path, _, _ in gold} & {path for path, _, _ in candidate}


def _overlapping_lines(gold: list[Citation], candidate: list[Citation], shared: set[str]) -> int:
    return sum(
        max(0, min(end, other_end) - max(start, other_start) + 1)
        for path, start, end in gold
        if path in shared
        for other_path, other_start, other_end in candidate
        if other_path == path
    )


def _scores(matched: int, gold: int, candidate: int) -> dict[str, float | None]:
    return {
        "precision": matched / candidate if candidate else None,
        "recall": matched / gold if gold else None,
        "f1": 2 * matched / (gold + candidate) if gold or candidate else None,
    }
