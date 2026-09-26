import pathlib
import typing

import pytest
import yaml
from pydantic import BaseModel

from skillhub_llm.creation import (
    CreationMessage,
    CreationStepRequest,
    CreationStepResponse,
    CreationToolIntent,
)
from skillhub_llm.enrich_checks import Finding
from skillhub_llm.evaluate import (
    CriterionVerdict,
    ImprovementProposal,
    JudgeEvidenceRef,
    JudgeVerdict,
)
from skillhub_llm.gateway import GatewayUsage
from skillhub_llm.generate import GenerateDiagram
from skillhub_llm.intent import SearchFilters

MODELS: dict[str, type[BaseModel]] = {
    "CreationMessage": CreationMessage,
    "CreationStepRequest": CreationStepRequest,
    "CreationStepResponse": CreationStepResponse,
    "CreationToolIntent": CreationToolIntent,
    "CriterionVerdict": CriterionVerdict,
    "EnrichCheck": Finding,
    "GatewayUsage": GatewayUsage,
    "GenerateDiagram": GenerateDiagram,
    "ImprovementProposal": ImprovementProposal,
    "JudgeEvidenceRef": JudgeEvidenceRef,
    "JudgeVerdict": JudgeVerdict,
    "SearchFilters": SearchFilters,
}

NOT_PRODUCED_HERE = {
    "Health.status": "the liveness handler returns the string inline; it has no model to read",
}

CONTRACT = (
    pathlib.Path(__file__).resolve().parents[3] / "contracts" / "openapi" / "llm-internal.yaml"
)


@pytest.mark.parametrize("name", ["SearchIntent", "SearchKeywords", "SearchFilters"])
def test_search_analysis_and_public_correction_share_schema_constraints(name):
    internal = yaml.safe_load(CONTRACT.read_text(encoding="utf-8"))["components"]["schemas"][name]
    public = yaml.safe_load(CONTRACT.with_name("public.yaml").read_text(encoding="utf-8"))[
        "components"
    ]["schemas"][name]
    internal.pop("description", None)
    public.pop("description", None)
    assert internal == public


def contract_enums() -> dict[str, list]:
    spec = yaml.safe_load(CONTRACT.read_text(encoding="utf-8"))
    found: dict[str, list] = {}

    def walk(node, key: str) -> None:
        if isinstance(node, dict):
            if "enum" in node:
                found[key] = node["enum"]
                return
            for value in node.values():
                walk(value, key)
        elif isinstance(node, list):
            for value in node:
                walk(value, key)

    for name, schema in spec["components"]["schemas"].items():
        for prop, definition in (schema.get("properties") or {}).items():
            walk(definition, f"{name}.{prop}")
    return found


def produced_values(annotation) -> set[str] | None:
    """Every string a field can hold, or None when the field is not a closed set."""
    args = typing.get_args(annotation)
    if typing.get_origin(annotation) is typing.Literal:
        return {a for a in args if isinstance(a, str)}
    if not args:
        return None
    unions = [produced_values(a) for a in args]
    closed = [u for u in unions if u is not None]
    return set().union(*closed) if closed else None


def field_values(key: str) -> set[str] | None:
    schema, prop = key.split(".", 1)
    model = MODELS.get(schema)
    if model is None:
        return None
    field = model.model_fields.get(prop)
    return None if field is None else produced_values(field.annotation)


@pytest.mark.parametrize("key", sorted(contract_enums()))
def test_every_closed_set_the_contract_declares_is_produced_by_a_field_here(key: str) -> None:
    if key in NOT_PRODUCED_HERE:
        pytest.skip(NOT_PRODUCED_HERE[key])
    assert field_values(key) is not None, (
        f"{key} is a closed set in the contract and nothing here narrows it; a plain `str` field "
        "accepts a value the contract forbids, and the caller finds out instead of this service"
    )


@pytest.mark.parametrize("key,declared", sorted(contract_enums().items()))
def test_a_field_produces_exactly_the_values_the_contract_declares(
    key: str, declared: list
) -> None:
    produced = field_values(key)
    if produced is None:
        pytest.skip(NOT_PRODUCED_HERE.get(key, "no closed set on this side"))
    assert produced == {v for v in declared if isinstance(v, str)}, (
        f"{key} produces {sorted(produced)} but the contract declares "
        f"{sorted(v for v in declared if isinstance(v, str))}; a value only this side knows about "
        "reaches a caller that has no branch for it, and a value only the contract knows about is "
        "a promise nothing keeps"
    )


def test_the_roster_names_only_schemas_the_contract_still_has() -> None:
    spec = yaml.safe_load(CONTRACT.read_text(encoding="utf-8"))
    missing = sorted(set(MODELS) - set(spec["components"]["schemas"]))
    assert not missing, (
        f"{missing} are on the roster but the contract has no such schema; a renamed schema leaves "
        "its model comparing against nothing and both tests above pass on an empty set"
    )
