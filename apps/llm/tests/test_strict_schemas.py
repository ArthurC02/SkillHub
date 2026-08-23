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

from skillhub_llm.app import MatchReasonsResponse, SuggestCriteriaResponse
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
    MatchReasonsResponse,
    SuggestCriteriaResponse,
    Enrichment,
    JudgeVerdict,
    ImprovementProposals,
    GeneratedSkill,
]


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
            refused = REFUSED_KEYWORDS & set(prop)
            assert not refused, (
                f"{model.__name__}.{title}.{name}: strict refuses {sorted(refused)}; "
                "apply the cap to the answer instead"
            )
