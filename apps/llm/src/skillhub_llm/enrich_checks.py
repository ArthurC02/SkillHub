"""Deterministic checks on one enrichment, against the document it came from.

The cheap half of 05 R-34. `enrich-skill`'s prompt is at v6, and v4, v5 and v6
each exist because a human audit caught the model breaking a rule that can be
stated - and the only tool the service had was to add another sentence to the
prompt and hope. v6's own comment says why that keeps failing: "each half of the
sentence is individually true, which is exactly what makes it read as
supported". A rule that needs the output compared against the source cannot be
enforced by a sentence sitting next to the instructions.

R-34 asked a cheaper question before sizing a graph: which of those rules need a
model at all? Splitting them put the two with MEASURED violation rates on the
free side:

  - the runtime rule (v5): 11 of 33 Python-dependent Skills named the dependency
    in `tags` and not in `limitations`. Import lines, shebangs, file extensions
    and frontmatter entries are all string evidence.
  - the English-example rule (v7, still owed to the prompt): the audit found a
    Simplified-Chinese typeface name inside an English example sentence. A CJK
    codepoint in an `en` field is a codepoint test.

So this module runs on every enrichment, costs nothing, and needs no gateway.
What is left for a model - restated modality, neighbouring capabilities,
composing two facts, the locale gloss - is genuinely semantic and is NOT
attempted here.

Two boundaries this module keeps:

  - It REPORTS, it does not reject. Iron rule 6: Python is a capability
    provider; whether a finding blocks an index, downgrades a field or merely
    annotates it is Go's to decide.
  - No model text is ever copied into a finding. TM-SCN-02: the enrichment is
    derived from untrusted package content, and `enrich.py` already refuses to
    echo it in an error. Every string in a Finding comes from this file's own
    vocabulary or from a field name - never from the model's output or the
    package.
"""

from __future__ import annotations

import re
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

# Rule identifiers. Stable, because a stored finding outlives this file and the
# audit that reads it has to be able to say which rule fired.
RULE_RUNTIME_NOT_IN_LIMITATIONS = "runtime_not_in_limitations"
RULE_UNSUPPORTED_APPRAISAL = "unsupported_appraisal"
RULE_NON_ENGLISH_IN_EN_EXAMPLE = "non_english_in_en_example"


class Finding(BaseModel):
    """One rule, one field, and nothing quoted from the model."""

    model_config = ConfigDict(extra="forbid")

    rule: str = Field(..., max_length=64)
    # Dotted path into the enrichment: `limitations`, `task_examples[2].en`.
    # Built here from an index and a literal, never from model output.
    field: str = Field(..., max_length=64)
    # A token from this module's own vocabulary that says WHICH instance fired -
    # a runtime name, an appraisal word. Never a span of the model's text.
    token: str = Field("", max_length=64)
    severity: Literal["warning"] = "warning"


# --- the runtime rule (prompt v5) --------------------------------------------
#
# Each entry is one runtime, the signals that say the document needs it, and the
# names that count as `limitations` mentioning it. Names are matched
# case-insensitively against the joined limitations text in either language,
# because the limitations are written in the caller's language and a runtime's
# name is a proper noun that survives translation.
#
# Deliberately small. A signal set that guesses is worse than one that misses:
# every entry here is a thing the document itself typed.
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


# --- the appraisal rule (prompt rule 2) --------------------------------------
#
# The prompt names the words itself: "clear, concise, polished, accurate,
# professional or well-structured are appraisals; write them only when the
# content itself claims that property of its own output".
#
# So the check is: an appraisal word in the enrichment needs the source to have
# made that claim. The enrichment is written in the caller's language and the
# document is usually English, so each entry carries both spellings and either
# one in the source counts as support.
#
# Reported as a candidate, not a verdict. A document that says "well-organised"
# and an enrichment that says 清晰 is a real miss for this check, and that is the
# right way round: a warning a human or a later verify step adjudicates costs
# less than a rule that stays silent.
_APPRAISALS: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("clear", ("clear", "清晰", "清楚")),
    ("concise", ("concise", "簡潔", "精簡")),
    ("polished", ("polished", "精緻", "細緻")),
    ("accurate", ("accurate", "accuracy", "準確", "精確")),
    ("professional", ("professional", "專業")),
    ("well-structured", ("well-structured", "well structured", "結構良好", "結構清晰")),
)


# Longest spelling first, and a matched span is consumed before the next entry
# looks. 結構清晰 is ONE appraisal; without this it reports as both
# `well-structured` and `clear`, and the violation rate this checker exists to
# measure would count one phrase twice.
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
            # Consume it either way: whether or not the document supports this
            # appraisal, the span is spoken for and a shorter word inside it is
            # not a second finding.
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


# --- the English-example rule (owed to prompt v7) ----------------------------
#
# content-review-report 12.4 (b): an English example sentence naming a proper
# noun should use its English name. The audit's instance was a Simplified-Chinese
# typeface name sitting inside an `en` sentence, and that shape is a codepoint
# test - CJK ideographs, hiragana and katakana have no business in a sentence
# whose whole job is to be the English half of the pair.
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

    `source` is the document AND its file list: prompt v5 counts a script's file
    extension as the document saying the Skill needs that runtime, and the file
    list is where an extension usually appears.
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
