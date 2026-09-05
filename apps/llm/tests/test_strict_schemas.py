"""Every schema handed to a model has to be legal under `strict: true`.

The rules are the gateway's and they are not negotiable at call time: each
object must list *every* property in `required` and set
`additionalProperties: false`, and the keywords below are refused outright. A
schema that breaks one of them does not degrade - the call returns 400 and the
endpoint fails 100% of the time.

That is not hypothetical. `GeneratedSkill` shipped with four defaults (each of
which drops its property out of `required`), a `dict[str, str]` and six length
constraints, so POST /v1/generate-skill answered 502 to everything it was ever
asked. Nothing noticed, because every test of every one of these endpoints
monkeypatches the client away - which is the right thing for those tests to do
and the reason this file exists instead.

Kept as a list of models rather than a scan of `response_format=` call sites so
that adding an endpoint means adding a line here; a scan would pass silently on
the schema it failed to find.
"""

from __future__ import annotations

import pytest
from pydantic import BaseModel

from skillhub_llm.app import MatchReasons, SuggestedCriteria
from skillhub_llm.creation import CreationDecision
from skillhub_llm.enrich import Enrichment
from skillhub_llm.evaluate import ImprovementProposals, JudgeVerdict
from skillhub_llm.generate import GeneratedSkill

# https://platform.openai.com/docs/guides/structured-outputs - "Supported
# properties". Anything else on a property is rejected rather than ignored.
REFUSED_KEYWORDS = {
    "default",
    "minLength",
    "maxLength",
    "pattern",
    "format",
    "minItems",
    "maxItems",
    "minimum",
    "maximum",
    "exclusiveMinimum",
    "exclusiveMaximum",
    "multipleOf",
    "oneOf",
    "allOf",
    "not",
    "patternProperties",
    "unevaluatedProperties",
    "propertyNames",
    "minProperties",
    "maxProperties",
    "contains",
    "minContains",
    "maxContains",
    "uniqueItems",
}

# The model-facing halves only. `MatchReasonsResponse` and
# `SuggestCriteriaResponse` are their WIRE subclasses and carry `usage`, which
# is exactly what must not be in a schema the model answers - and which strict
# would refuse anyway, GatewayUsage having defaults and a `minimum`. Listing the
# subclass here would make this file report that the pair is illegal while the
# real call is fine.
MODEL_FACING = [
    MatchReasons,
    SuggestedCriteria,
    Enrichment,
    JudgeVerdict,
    ImprovementProposals,
    GeneratedSkill,
    CreationDecision,
]


def _refused_keywords(node: object) -> set[str]:
    """Every refused keyword anywhere under a property, `anyOf` branches and
    array `items` included."""
    found: set[str] = set()
    if isinstance(node, dict):
        found |= REFUSED_KEYWORDS & set(node)
        for key, value in node.items():
            # `properties` belongs to a nested object, which _objects() visits
            # in its own right; descending here would report it twice under a
            # confusing name.
            if key == "properties":
                continue
            found |= _refused_keywords(value)
    elif isinstance(node, list):
        for item in node:
            found |= _refused_keywords(item)
    return found


def _objects(node: object) -> list[dict]:
    """Every object schema in the tree, `$defs` included."""
    found = []
    if isinstance(node, dict):
        if node.get("type") == "object":
            found.append(node)
        for value in node.values():
            found.extend(_objects(value))
    elif isinstance(node, list):
        for item in node:
            found.extend(_objects(item))
    return found


@pytest.mark.parametrize("model", MODEL_FACING, ids=lambda m: m.__name__)
def test_the_schema_is_legal_under_strict_json_schema(model: type[BaseModel]) -> None:
    schema = model.model_json_schema()
    for obj in _objects(schema):
        title = obj.get("title", "<root>")
        properties = set(obj.get("properties", {}))
        assert set(obj.get("required", [])) == properties, (
            f"{model.__name__}.{title}: strict requires every property in `required`; "
            f"missing {sorted(properties - set(obj.get('required', [])))}"
        )
        # An open-ended map answers `{"type": "string"}` here and cannot be
        # expressed at all under strict - the field has to go, not be relaxed.
        assert obj.get("additionalProperties") is False, (
            f"{model.__name__}.{title}: strict requires additionalProperties=false, "
            f"got {obj.get('additionalProperties')!r}"
        )
        for name, prop in obj.get("properties", {}).items():
            # Recursive, not just the top level of the property dict. A nullable
            # field renders as `anyOf: [{type: string, maxLength: N}, {type:
            # null}]`, and a top-level scan reads only the anyOf key — so the
            # refused keyword hides one level down. Nullable is not exotic here:
            # strict REQUIRES an inapplicable field to be sent as null rather
            # than omitted, which is why JudgeEvidenceRef has two of them. A
            # guard against the M5 defect that could not see into anyOf was the
            # M5 defect wearing a different hat (found by the M3 audit).
            refused = _refused_keywords(prop)
            assert not refused, (
                f"{model.__name__}.{title}.{name}: strict refuses {sorted(refused)}; "
                "apply the cap to the answer instead"
            )
