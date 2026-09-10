import pathlib

import pytest
import yaml
from skillhub_api_stub.generated import models as generated
from skillhub_api_stub.generated.models import EnrichSkillRequest

from skillhub_llm.app import app


def test_generated_enrich_request_validates_the_internal_contract() -> None:
    request = EnrichSkillRequest.model_validate(
        {
            "skill_name": "example",
            "skill_md": "---\nname: example\n---\n",
            "file_tree": ["SKILL.md"],
            "language": "zh-Hant",
        }
    )

    assert request.skill_name == "example"
    assert request.file_tree == ["SKILL.md"]


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


def test_every_endpoint_that_can_fail_on_the_gateway_declares_both_ways_it_can():
    """502 and 503 come in a pair; both must be declared together."""
    missing = [
        name for name, responses in _operations() if "502" in responses and "503" not in responses
    ]
    assert missing == [], f"declare a 503 for: {missing}"


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
