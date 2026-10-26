---
type: Business Rule
status: draft
sources:
  - resource: "apps/llm/src/skillhub_llm/gateway.py#L132-142"
    excerpt_sha256: "sha256:e4f664fcb36dac0b715a00bf143027be1deac0a4beaa8dd3c7949c170b17c432"
  - resource: "apps/platform/internal/foundation/integration/llmclient/client.go#L21-27"
    excerpt_sha256: "sha256:3487ca8b5862a4806cd468d6b17ecddc107f07505d01a962c7c977255d56dde4"
  - resource: "apps/platform/internal/creator/creation/model_adapter.go#L46-48"
    excerpt_sha256: "sha256:40a04ab23f5aed9b9dfd8652ab06d501a6430c195b096b04d0b3ef9fc10cf49e"
  - resource: "apps/platform/internal/creator/creation/job.go#L637-642"
    excerpt_sha256: "sha256:af2dc45c971fdfc2a74a1d096328a05bd13a585f2e1ae185148d3553a2427a35"
  - resource: "apps/platform/internal/creator/creation/attempt_test.go#L96-96"
    excerpt_sha256: "sha256:9247368761bf99e1dca8763ef45bfa31ad734b65a47e7b342099d04b3f1b19fb"
  - resource: "apps/llm/tests/test_gateway_failure_log.py#L59-78"
    excerpt_sha256: "sha256:bf6c53c98cccf55730090efbd011ef52cb2e520cb838892a70fac73273d21834"
---
When the model gateway refuses a call because its platform-wide budget for the period is spent, the refusal is named as such rather than reported as a broken gateway, and every such refusal is counted for alerting; a creation step refused this way is not counted against the session, the session waits for the person to send again, and only the cost actually incurred is settled. A refusal of one key's own budget is not this case and stays an ordinary gateway failure.

# Relations
- Contexts: [creation](/contexts/creation)

# Examples
- The gateway answers 429 budget_exceeded without naming a key: the capability service answers 503 "model gateway platform budget exhausted" and the creation session returns to waiting for input with its step count unchanged.
- The gateway answers 429 budget_exceeded naming the step's key: the capability service answers 502 and the step fails as before.
