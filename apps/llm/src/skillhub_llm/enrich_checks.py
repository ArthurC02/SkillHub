"""Deterministic checks on one enrichment, against the document it came from.

Reports findings only; every string in a Finding comes from this module's
own vocabulary, never from model output or package content.
"""

from __future__ import annotations

import re
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

RULE_RUNTIME_NOT_IN_LIMITATIONS = "runtime_not_in_limitations"
RULE_UNSUPPORTED_APPRAISAL = "unsupported_appraisal"
RULE_NON_ENGLISH_IN_EN_EXAMPLE = "non_english_in_en_example"


class Finding(BaseModel):
    """One rule, one field, and nothing quoted from the model."""

    model_config = ConfigDict(extra="forbid")

    rule: str = Field(..., max_length=64)
    field: str = Field(..., max_length=64)
    token: str = Field("", max_length=64)
    severity: Literal["warning"] = "warning"


# Each entry: a runtime, regexes over the document that imply it is needed, and
# the names that count as `limitations` already mentioning it (matched
# case-insensitively, since a proper noun survives translation).
_RUNTIMES: tuple[tuple[str, tuple[str, ...], tuple[str, ...]], ...] = (
    (
        "python",
        (r"(?m)^\s*(?:import|from)\s+\w", r"#!/usr/bin/env python", r"\bpython3?\b", r"\.py\b"),
        ("python", "py3"),
    ),
    (
        "node",
        (
            r"#!/usr/bin/env node",
            r"\bnpm\s+(?:i|install|run)\b",
            r"\bnode\s+\S+\.m?js\b",
            r"\.mjs\b",
        ),
        ("node", "node.js", "nodejs", "npm"),
    ),
    (
        "pandoc",
        (r"\bpandoc\b",),
        ("pandoc",),
    ),
)


def _runtime_findings(source: str, limitations: list[str]) -> list[Finding]:
    haystack = "\n".join(limitations).lower()
    out: list[Finding] = []
    for runtime, signals, names in _RUNTIMES:
        if not any(re.search(s, source, re.IGNORECASE) for s in signals):
            continue
        if any(name in haystack for name in names):
            continue
        out.append(
            Finding(rule=RULE_RUNTIME_NOT_IN_LIMITATIONS, field="limitations", token=runtime)
        )
    return out


_APPRAISALS: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("clear", ("clear", "清晰", "清楚")),
    ("concise", ("concise", "簡潔", "精簡")),
    ("polished", ("polished", "精緻", "細緻")),
    ("accurate", ("accurate", "accuracy", "準確", "精確")),
    ("professional", ("professional", "專業")),
    ("well-structured", ("well-structured", "well structured", "結構良好", "結構清晰")),
)


# Longest spelling first: a matched span is consumed before shorter entries
# look, so "結構清晰" is one appraisal rather than two overlapping ones.
_APPRAISAL_SPELLINGS: tuple[tuple[str, str], ...] = tuple(
    sorted(
        ((spelling.lower(), token) for token, spellings in _APPRAISALS for spelling in spellings),
        key=lambda pair: len(pair[0]),
        reverse=True,
    )
)


def _appraisal_findings(source: str, fields: list[tuple[str, str]]) -> list[Finding]:
    low_source = source.lower()
    out: list[Finding] = []
    seen: set[tuple[str, str]] = set()
    for name, text in fields:
        low = text.lower()
        for spelling, token in _APPRAISAL_SPELLINGS:
            if spelling not in low:
                continue
            # Consume the span either way, so a shorter word inside it can't
            # also match.
            low = low.replace(spelling, " ")
            if any(s in low_source for s in _spellings_of(token)):
                continue
            key = (name, token)
            if key in seen:
                continue
            seen.add(key)
            out.append(Finding(rule=RULE_UNSUPPORTED_APPRAISAL, field=name, token=token))
    return out


def _spellings_of(token: str) -> tuple[str, ...]:
    for name, spellings in _APPRAISALS:
        if name == token:
            return spellings
    return ()


# Hiragana, katakana and CJK ideograph ranges: any of these in an en field is
# non-English regardless of language detection.
_CJK = re.compile(r"[぀-ヿ㐀-䶿一-鿿豈-﫿]")


def _english_example_findings(examples: list[str]) -> list[Finding]:
    return [
        Finding(rule=RULE_NON_ENGLISH_IN_EN_EXAMPLE, field=f"task_examples[{i}].en")
        for i, text in enumerate(examples)
        if _CJK.search(text)
    ]


def check_enrichment(
    *,
    skill_md: str,
    file_tree: list[str],
    summary: str,
    limitations: list[str],
    task_examples_en: list[str],
    tags_flat: list[str],
) -> list[Finding]:
    """Every deterministic finding for one enrichment, in rule order.

    `source` is the document AND its file list: a script's file extension
    also counts as evidence the Skill needs that runtime.
    """
    source = skill_md + "\n" + "\n".join(file_tree)
    appraisal_fields = [("summary", summary)]
    appraisal_fields += [(f"limitations[{i}]", t) for i, t in enumerate(limitations)]
    appraisal_fields += [(f"tags[{i}]", t) for i, t in enumerate(tags_flat)]
    return (
        _runtime_findings(source, limitations)
        + _appraisal_findings(source, appraisal_fields)
        + _english_example_findings(task_examples_en)
    )
