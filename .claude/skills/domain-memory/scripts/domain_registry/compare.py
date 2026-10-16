from __future__ import annotations

import re
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
MATCH_KINDS = {EXACT_PATH: "path", NESTED_PATH: "nested-path", SAME_NAME: "name", SHARED_EVIDENCE: "context-and-evidence"}


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
    scored = sorted(
        (
            (score, index, gold_index)
            for index, record in enumerate(candidate)
            for gold_index, expected in enumerate(gold)
            if (score := _match_score(asset, expected, record, context_of))
        ),
        key=lambda entry: (-entry[0], entry[1], entry[2]),
    )
    taken_candidates, taken_gold, matched = set(), set(), []
    for score, index, gold_index in scored:
        if index in taken_candidates or gold_index in taken_gold:
            continue
        taken_candidates.add(index)
        taken_gold.add(gold_index)
        matched.append({"candidate": candidate[index]["id"], "gold": gold[gold_index]["id"], "by": MATCH_KINDS[score]})
    return {
        "gold": len(gold),
        "candidate": len(candidate),
        "matched": sorted(matched, key=lambda pair: pair["gold"]),
        "missed": sorted(record["id"] for index, record in enumerate(gold) if index not in taken_gold),
        "extra": sorted(record["id"] for index, record in enumerate(candidate) if index not in taken_candidates),
        **_scores(len(matched), len(gold), len(candidate)),
    }


def _match_score(asset: str, gold: dict[str, Any], candidate: dict[str, Any], context_of: dict[str, str]) -> int:
    if asset == "contexts":
        return _path_score(gold.get("implementation_path"), candidate.get("implementation_path")) or (
            SAME_NAME if _same_name(asset, gold, candidate) else 0
        )
    if asset in PAIRED_ASSETS:
        ends = PAIRED_ASSETS[asset]
        return EXACT_PATH if all(context_of.get(candidate.get(end)) == gold.get(end) for end in ends) else 0
    if _same_name(asset, gold, candidate):
        return SAME_NAME
    shares_context = bool(_contexts(gold) & {context_of.get(context) for context in _contexts(candidate)})
    return SHARED_EVIDENCE if shares_context and _cited_paths(gold) & _cited_paths(candidate) else 0


def _path_score(gold: Any, candidate: Any) -> int:
    if not isinstance(gold, str) or not isinstance(candidate, str):
        return 0
    gold_parts = PurePosixPath(gold.strip("/")).parts
    candidate_parts = PurePosixPath(candidate.strip("/")).parts
    if gold_parts == candidate_parts:
        return EXACT_PATH
    shorter = min(len(gold_parts), len(candidate_parts))
    return NESTED_PATH if shorter and gold_parts[:shorter] == candidate_parts[:shorter] else 0


def _same_name(asset: str, gold: dict[str, Any], candidate: dict[str, Any]) -> bool:
    field = NAME_FIELDS.get(asset)
    names = {_normalized(gold.get("id"))} | ({_normalized(gold.get(field))} if field else set())
    if asset == "vocabulary":
        names |= {_normalized(synonym) for synonym in gold.get("synonyms") or []}
    wanted = {_normalized(candidate.get("id"))} | ({_normalized(candidate.get(field))} if field else set())
    return bool((names & wanted) - {""})


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
