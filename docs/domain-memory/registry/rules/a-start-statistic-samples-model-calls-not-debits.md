---
type: Business Rule
status: draft
sources:
  - resource: "db/migrations/0150_reflection_candidates.sql#L56-67"
    excerpt_sha256: "sha256:3a4716ec55b384493b73181778115e5c9b67aef14ca04bd59289d7650e3734f2"
  - resource: "db/queries/cost.sql#L14-24"
    excerpt_sha256: "sha256:b5cb92e68210ce403effe39ee882ddbe19460f5b15a1ceceffaced8b6748139f"
  - resource: "apps/platform/internal/creator/credit/service.go#L520-535"
    excerpt_sha256: "sha256:9bd35ddad68e8dc8f4191de3a65f2f8e7f60a67f15772630ade65bb0ef5d04af"
  - resource: "apps/platform/internal/creator/credit/postgres_integration_test.go#L306-343"
    excerpt_sha256: "sha256:f99edf8496eeab0636c1aff7ded3449c616c997aa0872001eff140bcf0ed328f"
---
The start statistics of a cost kind are taken over its model calls, not over its debits: a gateway-measured event that lists its calls in call_usd_micros contributes one sample per call, and any other gateway-measured event contributes one sample. A list, when present, holds two or more non-negative costs that sum to the event's usd_micros, and only on a gateway-measured event; a charge that would break this is refused as invalid.

# Relations
- Contexts: [credit](/contexts/credit)

# Examples
- One batch reflection debit of 6000 micros listing calls of 1000, 2000 and 3000, beside a single-call event of 4000, is four samples with p50 2500 and p95 3850.
- Four single-call creation_step events of 100, 200, 300 and 400 micros are four samples with p95 385, the same as before calls were listed.
- A list of one call, calls summing to 29 of a 30-micro debit, or calls on an estimated event are refused.
