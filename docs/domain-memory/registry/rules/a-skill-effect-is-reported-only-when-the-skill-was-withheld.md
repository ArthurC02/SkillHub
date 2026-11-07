---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L99-103"
    excerpt_sha256: "sha256:1a9fa7058aba4c990e2de30bc17d2b9c812b690391749d38db5de306a04acd88"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_scope_integration_test.go#L8-33"
    excerpt_sha256: "sha256:31e162ab9981a55bb6fcd43e06b766753242ecac7f10481eba52fe718dfb40e3"
---
A batch report carries the skill-withheld arm and the skill effect only when the batch was created withholding the skill.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A batch created without withholding reports no skill_withheld and no skill_effect.
