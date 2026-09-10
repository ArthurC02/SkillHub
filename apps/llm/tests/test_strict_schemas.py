"""Every schema handed to a model has to be legal under `strict: true`.

Kept as a list of models rather than a scan of `response_format=` call sites,
so that adding an endpoint means adding a line here.
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
            # A nested object's own `properties` is visited when _objects()
            # reaches that object; descending into it here would double-count it.
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
        assert obj.get("additionalProperties") is False, (
            f"{model.__name__}.{title}: strict requires additionalProperties=false, "
            f"got {obj.get('additionalProperties')!r}"
        )
        for name, prop in obj.get("properties", {}).items():
            # Recursive, not just the top level: a nullable field renders as
            # `anyOf: [{type, ...}, {type: null}]`, so a refused keyword can
            # sit one level down inside a branch.
            refused = _refused_keywords(prop)
            assert not refused, (
                f"{model.__name__}.{title}.{name}: strict refuses {sorted(refused)}; "
                "apply the cap to the answer instead"
            )
