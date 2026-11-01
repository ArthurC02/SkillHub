---
type: Business Rule
status: stable
verified:
  - by: "human:human:ArthurC"
    at: "2026-11-01T19:08:29.954044Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/checks/rule_graders_test.go#L133-140"
    excerpt_sha256: "sha256:eccaa335afd7e34555eb1a95f3139756a2a49cb5f9a7dc97ddcc617cab4a8c09"
  - resource: "apps/platform/internal/trial/improvement/facts/facts.go#L114-120"
    excerpt_sha256: "sha256:671193b623512b1132e5af3173444c47df0aa6545012a620be869b89e91f1045"
  - resource: "apps/platform/internal/trial/improvement/checks/rule_graders.go#L17-32"
    excerpt_sha256: "sha256:eecd17f96f0890cd40fa55a08755526189a1386030e88a755653e75bd8454fe3"
  - resource: "apps/platform/internal/trial/improvement/checks/measured_graders.go#L42-61"
    excerpt_sha256: "sha256:7e4efd5b3a18c0bf4b6cf15cd513f69a7cb0d44c5fc2de4ab57df374d219ea29"
  - resource: "apps/platform/internal/trial/improvement/checks/rule_graders_test.go#L265-272"
    excerpt_sha256: "sha256:d169c96c8834d09bc2514cd629868b121c70c97fb4517dd8af60c8919b6323f4"
  - resource: "apps/platform/internal/trial/improvement/judging/judge_test.go#L803-813"
    excerpt_sha256: "sha256:95de7fa587dcc01cd023a056755a8c63dc9b276dedf59e757967886d8500f564"
---
When the runtime cut the final output, a check that reads it records undetermined, not failed, for a value it cannot find, and undetermined, not passed, for a length within the limit; a value found in the kept part still passes and a kept part already over the limit still fails.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A cut answer that does not mention 2026-10-03 is undetermined, with the reason that the final output was cut off at 10 characters.
- A cut answer of three characters is undetermined against a three-character limit and fails against a two-character limit.
