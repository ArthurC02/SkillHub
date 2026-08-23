import pytest
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
    ],
)
def test_runtime_transport_shape_matches_generated_contract(name: str) -> None:
    """Hand-written FastAPI DTOs may not drift from the generated boundary."""
    runtime = app.openapi()["components"]["schemas"][name]
    contract = getattr(generated, name).model_json_schema()

    assert set(runtime.get("properties", {})) == set(contract.get("properties", {}))
    assert set(runtime.get("required", [])) == set(contract.get("required", []))
