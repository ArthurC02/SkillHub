from skillhub_api_stub.generated.models import EnrichSkillRequest


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
