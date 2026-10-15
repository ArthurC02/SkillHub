import pathlib

import pytest
import yaml
from skillhub_api_stub.generated import models as generated

from skillhub_llm.app import app


@pytest.mark.parametrize(
    "name",
    [
        "EmbedRequest",
        "EmbedResponse",
        "EnrichSkillRequest",
        "EnrichSkillResponse",
        "MatchReasonsRequest",
        "MatchReasonsResponse",
        "SuggestCriteriaRequest",
        "SuggestCriteriaResponse",
        "JudgeRunRequest",
        "JudgeRunResponse",
        "SuggestImprovementsRequest",
        "SuggestImprovementsResponse",
        "GenerateSkillRequest",
        "GenerateSkillResponse",
        "GenerateDiagram",
        "GenerateReference",
        "CreationMessage",
        "CreationToolIntent",
        "CreationDraftValidation",
        "CreationStepRequest",
        "CreationStepResponse",
        "AgentStepRequest",
        "AgentStepResponse",
        "AgentTool",
        "AgentStepRecord",
        "AgentToolIntent",
        "AnalyzeSearchIntentRequest",
        "AnalyzeSearchIntentResponse",
        "SearchIntent",
        "SearchFilters",
        "Readiness",
    ],
)
def test_runtime_transport_shape_matches_generated_contract(name: str) -> None:
    """Hand-written FastAPI DTOs may not drift from the generated boundary."""
    runtime = app.openapi()["components"]["schemas"][name]
    contract = getattr(generated, name).model_json_schema()

    assert set(runtime.get("properties", {})) == set(contract.get("properties", {}))
    assert set(runtime.get("required", [])) == set(contract.get("required", []))


CONTRACT = (
    pathlib.Path(__file__).resolve().parents[3] / "contracts" / "openapi" / "llm-internal.yaml"
)


def _spec():
    return yaml.safe_load(CONTRACT.read_text(encoding="utf-8"))


def _operations():
    spec = _spec()
    for path, methods in spec["paths"].items():
        for method, op in methods.items():
            if isinstance(op, dict) and "responses" in op:
                yield f"{method.upper()} {path}", op["responses"]


def _described(spec, response) -> str:
    """A response's description, following one `$ref` into components."""
    ref = response.get("$ref")
    if ref:
        response = spec["components"]["responses"][ref.rsplit("/", 1)[-1]]
    return response.get("description", "")


def _served_operations():
    for path, operations in app.openapi()["paths"].items():
        for method in operations:
            yield f"{method.upper()} {path}"


def test_every_route_this_service_serves_is_in_the_contract():
    declared = {name for name, _ in _operations()}
    undeclared = sorted(set(_served_operations()) - declared)
    assert undeclared == [], f"add these to llm-internal.yaml before serving them: {undeclared}"


def test_every_operation_in_the_contract_is_served():
    served = set(_served_operations())
    unserved = sorted({name for name, _ in _operations()} - served)
    assert unserved == [], f"the contract declares operations nothing here serves: {unserved}"


def test_every_endpoint_that_can_fail_on_the_gateway_declares_both_ways_it_can():
    """502 and 503 come in a pair; both must be declared together."""
    missing = [
        name for name, responses in _operations() if "502" in responses and "503" not in responses
    ]
    assert missing == [], f"declare a 503 for: {missing}"


def test_the_gateway_checks_reach_the_endpoints_that_call_the_gateway():
    gateway_endpoints = {name for name, responses in _operations() if "502" in responses}
    assert {"POST /v1/creation/step", "POST /judge-run", "POST /embed"} <= gateway_endpoints, (
        f"the 502/503 checks above only cover {sorted(gateway_endpoints)}"
    )


def test_the_503_says_both_of_the_things_it_can_mean():
    """The 503 description must name both causes: missing workload token and
    unconfigured gateway.
    """
    spec = _spec()
    for name, responses in _operations():
        if "502" not in responses:
            continue
        text = _described(spec, responses["503"]).lower()
        assert "workload token" in text, f"{name}: the 503 does not mention the workload token"
        assert "gateway" in text, f"{name}: the 503 does not mention the gateway"


_LIMITS = (
    "maxLength",
    "minLength",
    "maxItems",
    "minItems",
    "minimum",
    "maximum",
    "exclusiveMinimum",
    "exclusiveMaximum",
)


def _limits(schema, schemas, prefix=""):
    """Every limit a property states, keyed by where it sits, following refs to non-objects."""
    ref = schema.get("$ref")
    if ref:
        target = schemas.get(ref.rsplit("/", 1)[-1], {})
        return {} if "properties" in target else _limits(target, schemas, prefix)
    found = {prefix + key: schema[key] for key in _LIMITS if key in schema}
    if "enum" in schema:
        found[prefix + "enum"] = sorted(schema["enum"])
    for alternative in schema.get("anyOf", []) + schema.get("allOf", []):
        found.update(_limits(alternative, schemas, prefix))
    if "items" in schema:
        found.update(_limits(schema["items"], schemas, prefix + "items."))
    return found


def _collect_refs(node, schemas, refs: set[str]) -> None:
    if isinstance(node, list):
        for value in node:
            _collect_refs(value, schemas, refs)
        return
    if not isinstance(node, dict):
        return
    name = node.get("$ref", "").removeprefix("#/components/schemas/")
    if name and name not in refs:
        refs.add(name)
        _collect_refs(schemas[name], schemas, refs)
    for value in node.values():
        _collect_refs(value, schemas, refs)


def _request_schemas():
    spec = _spec()
    refs: set[str] = set()
    for methods in spec["paths"].values():
        for op in methods.values():
            if isinstance(op, dict):
                _collect_refs(op.get("requestBody"), spec["components"]["schemas"], refs)
    return sorted(refs)


_STRICT_MODEL_SCHEMA = (
    "also the strict json_schema handed to the model, which takes no limits; _over_cap checks them"
)
_STRICT_MODEL_SCHEMAS = {
    "GeneratedSkill": _STRICT_MODEL_SCHEMA,
    "GeneratedFile": _STRICT_MODEL_SCHEMA,
}


_NAMED_DIFFERENTLY_HERE = {
    "CreationChallengeCase": "ChallengeCase",
    "CreationDiagramInterpretation": "ConfirmedDiagramInterpretation",
    "CreationDiagramUncertainty": "DiagramUncertainty",
}


@pytest.mark.parametrize("name", [n for n in _request_schemas() if n not in _STRICT_MODEL_SCHEMAS])
def test_every_limit_the_contract_puts_on_a_request_is_enforced_here(name: str) -> None:
    """A request the contract forbids must be refused here too; this side may only be stricter."""
    runtime_schemas = app.openapi()["components"]["schemas"]
    here = _NAMED_DIFFERENTLY_HERE.get(name, name)
    assert here in runtime_schemas, f"{name} has no model here; name it in _NAMED_DIFFERENTLY_HERE"
    contract = getattr(generated, name).model_json_schema()
    contract_schemas = contract.get("$defs", {})
    looser = []
    for field, declared in contract.get("properties", {}).items():
        runtime = runtime_schemas[here].get("properties", {}).get(field, {})
        enforced = _limits(runtime, runtime_schemas)
        for key, value in _limits(declared, contract_schemas).items():
            if enforced.get(key) != value:
                looser.append(f"{field}.{key}: contract {value}, here {enforced.get(key)}")
    assert looser == [], f"{name} accepts what the contract refuses: {looser}"
