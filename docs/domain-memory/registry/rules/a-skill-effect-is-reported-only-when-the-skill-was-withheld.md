---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-18T20:31:55.890891Z"
sources:
  - resource: "apps/platform/internal/trial/batch/batch_report.go#L99-103"
    excerpt_sha256: "sha256:50ee445b0e048794517e887c09741114744f3b392fc5006fec2826f8658c5a4e"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_batch_scope_integration_test.go#L8-33"
    excerpt_sha256: "sha256:31e162ab9981a55bb6fcd43e06b766753242ecac7f10481eba52fe718dfb40e3"
---
A batch report carries the skill-withheld arm and the skill effect only when the batch was created withholding the skill.

# Relations
- Contexts: [batch](/contexts/batch)

# Examples
- A batch created without withholding reports no skill_withheld and no skill_effect.
