from __future__ import annotations

import re
from collections.abc import Callable
from pathlib import PurePosixPath
from typing import Any

from .common import ASSET_KEYS
from .evidence import all_references
from .okf import RELATION_TARGETS

PAIRED_ASSETS = {
    "interactions": ("producer_context", "consumer_context"),
    "dependency-policies": ("from_context", "to_context"),
}
NAME_FIELDS = {"contexts": "name", "vocabulary": "name", "aggregates": "root"}
EXACT_PATH, NESTED_PATH, SAME_NAME, SHARED_EVIDENCE = 4, 3, 2, 1
SCORE_LABELS = {EXACT_PATH: "path", NESTED_PATH: "nested-path", SAME_NAME: "name", SHARED_EVIDENCE: "context-and-evidence"}


def compare_registries(gold: dict[str, Any], candidate: dict[str, Any]) -> dict[str, Any]:
    gold_records = _records_by_asset(gold)
    candidate_records = _records_by_asset(candidate)
    contexts = _matched("contexts", gold_records["contexts"], candidate_records["contexts"], {})
    context_of = {pair["candidate"]: pair["gold"] for pair in contexts["matched"]}
    assets = {"contexts": contexts}
    for asset in gold_records:
        if asset != "contexts":
            assets[asset] = _matched(asset, gold_records[asset], candidate_records[asset], context_of)
    return {"assets": assets, "overall": _scores(
        sum(len(result["matched"]) for result in assets.values()),
        sum(result["gold"] for result in assets.values()),
        sum(result["candidate"] for result in assets.values()),
    )}


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
        key=lambda entry: (-entry[0], entry[1], entry[2]),
    )
    taken_candidates, taken_gold, matched = set(), set(), []
    for score, index, gold_index in scored:
        if index in taken_candidates or gold_index in taken_gold:
            continue
        taken_candidates.add(index)
        taken_gold.add(gold_index)
        matched.append({"candidate": candidate[index]["id"], "gold": gold[gold_index]["id"], "by": SCORE_LABELS[score]})
    return {
        "gold": len(gold),
        "candidate": len(candidate),
        "matched": sorted(matched, key=lambda pair: pair["gold"]),
        "missed": sorted(record["id"] for index, record in enumerate(gold) if index not in taken_gold),
        "extra": sorted(record["id"] for index, record in enumerate(candidate) if index not in taken_candidates),
        **_scores(len(matched), len(gold), len(candidate)),
    }


def _scorer(
    asset: str, gold: list[dict[str, Any]], candidate: list[dict[str, Any]], context_of: dict[str, str]
) -> Callable[[int, int], int]:
    if asset in PAIRED_ASSETS:
        ends = PAIRED_ASSETS[asset]
        gold_ends = [tuple(record.get(end) for end in ends) for record in gold]
        candidate_ends = [tuple(context_of.get(record.get(end)) for end in ends) for record in candidate]
        return lambda gold_index, index: EXACT_PATH if candidate_ends[index] == gold_ends[gold_index] else 0
    gold_names = [_names(asset, record, with_synonyms=True) for record in gold]
    candidate_names = [_names(asset, record, with_synonyms=False) for record in candidate]
    if asset == "contexts":
        gold_parts = [_path_parts(record.get("implementation_path")) for record in gold]
        candidate_parts = [_path_parts(record.get("implementation_path")) for record in candidate]
        return lambda gold_index, index: _path_score(gold_parts[gold_index], candidate_parts[index]) or (
            SAME_NAME if gold_names[gold_index] & candidate_names[index] else 0
        )
    gold_contexts = [_contexts(record) for record in gold]
    candidate_contexts = [{context_of.get(context) for context in _contexts(record)} for record in candidate]
    gold_paths = [_cited_paths(record) for record in gold]
    candidate_paths = [_cited_paths(record) for record in candidate]

    def score(gold_index: int, index: int) -> int:
        if gold_names[gold_index] & candidate_names[index]:
            return SAME_NAME
        shares_context = gold_contexts[gold_index] & candidate_contexts[index]
        return SHARED_EVIDENCE if shares_context and gold_paths[gold_index] & candidate_paths[index] else 0

    return score


def _path_parts(path: Any) -> tuple[str, ...] | None:
    return PurePosixPath(path.strip("/")).parts if isinstance(path, str) else None


def _path_score(gold: tuple[str, ...] | None, candidate: tuple[str, ...] | None) -> int:
    if gold is None or candidate is None:
        return 0
    if gold == candidate:
        return EXACT_PATH
    shorter = min(len(gold), len(candidate))
    return NESTED_PATH if shorter and gold[:shorter] == candidate[:shorter] else 0


def _names(asset: str, record: dict[str, Any], *, with_synonyms: bool) -> set[str]:
    field = NAME_FIELDS.get(asset)
    names = {_normalized(record.get("id"))} | ({_normalized(record.get(field))} if field else set())
    if with_synonyms and asset == "vocabulary":
        names |= {_normalized(synonym) for synonym in record.get("synonyms") or []}
    return names - {""}


def _normalized(value: Any) -> str:
    return re.sub(r"[^0-9a-z]+", "", value.casefold()) if isinstance(value, str) else ""


def _contexts(record: dict[str, Any]) -> set[str]:
    named = set()
    for field, target in RELATION_TARGETS.items():
        if target != "contexts":
            continue
        value = record.get(field)
        named |= {value} if isinstance(value, str) else {item for item in value or [] if isinstance(item, str)}
    return named


def _cited_paths(record: dict[str, Any]) -> set[str]:
    paths = set()
    for reference in all_references(record):
        if isinstance(reference, dict) and isinstance(reference.get("path"), str):
            paths.add(reference["path"])
        elif isinstance(reference, str):
            paths.add(reference.split(":", 1)[0])
    return paths


def _scores(matched: int, gold: int, candidate: int) -> dict[str, float | None]:
    return {
        "precision": matched / candidate if candidate else None,
        "recall": matched / gold if gold else None,
        "f1": 2 * matched / (gold + candidate) if gold or candidate else None,
    }
