---
type: Business Rule
status: draft
sources:
  - resource: "apps/platform/internal/trial/execution/gateway.go#L404-423"
    excerpt_sha256: "sha256:d14bf87b0636fa6be9a637513f768e19a935ed64f8dbb1723fdff9cd69b8abd9"
  - resource: "apps/platform/internal/trial/execution/gateway.go#L432-435"
    excerpt_sha256: "sha256:e6b96ebf6e808a06ea34a1bcd9b1acab178b8220be35f2d01b8c35fc28ed948e"
  - resource: "apps/platform/internal/trial/execution/platform_budget_test.go#L5-25"
    excerpt_sha256: "sha256:15e575447b93abe0447a81019651051bc5c3b1ea54f63c356922b9558c18294b"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_platform_budget_integration_test.go#L28-57"
    excerpt_sha256: "sha256:645bd48301897fbfb25bb96c158d059b866a2262ffbd4900a1d4d1696a4ee5de"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/run_platform_budget_integration_test.go#L71-81"
    excerpt_sha256: "sha256:507e2c746c0fa5899f619522bc8e838ee82fc6d1c5b34671754b6a05f376dc58"
  - resource: "apps/platform/internal/trial/execution/service.go#L491-493"
    excerpt_sha256: "sha256:cb90fb7494dd7e3e89828372ba0128bb20d70c0cb4a5db8d868ba413b3b24f02"
  - resource: "apps/platform/internal/trial/execution/preflight.go#L152-154"
    excerpt_sha256: "sha256:0ba24f9e4748cf7b51c8067e1d56949fa328cfc824e60b23b8894c950d74e510"
---
A trial Run is admitted only when the gateway's platform-wide model budget, less what is already spent and less the ceiling of every unfinished Run, still covers this Run's ceiling. Otherwise the preflight summary is blocked with platform_model_budget and creating the Run is refused for the same reason: no Run is created and nothing is charged, and the refusal says the budget recovers at 08:00 Taipei time. A budget or count that cannot be read admits the Run, because the gateway still enforces its own budget.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- Limit 50, spent 49.75, nothing in flight, ceiling 0.25: admitted.
- Limit 50, spent 49.5, two Runs in flight, ceiling 0.25: refused.
- The gateway's budget endpoint answers 500: the preflight shows nothing blocked.
