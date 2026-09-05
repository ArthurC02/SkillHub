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
        # M5. Absent until 2026-08-23, which is how a `metadata` property that
        # the contract carried and the runtime DTO could not express under
        # strict `json_schema` went unnoticed: this guard covers every other
        # endpoint, and the new one was the one it did not.
        "GenerateSkillRequest",
        "GenerateSkillResponse",
        # GEN-005/GEN-006: the two new nested shapes, same drift risk.
        "GenerateDiagram",
        "GenerateReference",
        "CreationMessage",
        "CreationToolIntent",
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
    """502 and 503 come in a pair, and only one of them was being declared.

    /v1/generate-skill borrows evaluate's client, so it answers 503 when
    LITELLM_BASE_URL or LITELLM_API_KEY is missing - it has done since that guard
    landed. The contract stopped at 502. Nothing caught it because `devctl gen
    --check` compares schema shapes and never looks at status codes, and the six
    older endpoints all happened to be right (adversarial review, 2026-08-24).

    The invariant, stated so the next endpoint inherits it: an operation that can
    fail because the gateway is broken can also fail because this process was
    never told where the gateway is.
    """
    missing = [
        name for name, responses in _operations() if "502" in responses and "503" not in responses
    ]
    assert missing == [], f"declare a 503 for: {missing}"


def test_the_503_says_both_of_the_things_it_can_mean():
    """A 503 here has two causes and they need different handling.

    `LLM_SERVICE_TOKEN` unset is a deployment that lost a variable both sides
    know about - Go fails closed on the same one at startup. `LITELLM_BASE_URL`
    or `LITELLM_API_KEY` unset is an error only this process can see, and its
    symptom on the Go side is search degrading to FTS-only, quietly, for as long
    as it lasts (gateway.py's whole docstring is that story, M1 audit).

    The contract used to state one cause: five endpoints referenced a response
    called `AuthUnavailable` describing only the workload token, and two others
    spelled out only the gateway. The test above passed on all seven, because it
    only ever counted status codes. This one reads the words.
    """
    spec = _spec()
    for name, responses in _operations():
        if "502" not in responses:
            continue
        text = _described(spec, responses["503"]).lower()
        assert "workload token" in text, f"{name}: the 503 does not mention the workload token"
        assert "gateway" in text, f"{name}: the 503 does not mention the gateway"
