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
  - resource: "apps/platform/internal/skill/admission/model_adapter.go#L82-84"
    excerpt_sha256: "sha256:2c79547d31a02acc5cf2dfd02ae75a7a4dd7b34a31ce96d00a61db5e8dfac790"
  - resource: "apps/platform/internal/skill/admission/http.go#L393-394"
    excerpt_sha256: "sha256:749bff0e81b0ef459ff4e1e4dd2d5d86110ef132d6103bd616461881b8e8fbe8"
  - resource: "apps/platform/internal/trial/design/criteria_adapter.go#L30-32"
    excerpt_sha256: "sha256:40a04ab23f5aed9b9dfd8652ab06d501a6430c195b096b04d0b3ef9fc10cf49e"
  - resource: "apps/platform/internal/trial/design/http.go#L77-78"
    excerpt_sha256: "sha256:9051d592b78f5025d1db9bbaceb04f6c90a1e11f0b6abcf706cacca0460b70f7"
  - resource: "apps/platform/internal/trial/improvement/llmadapter/model_adapter.go#L46-48"
    excerpt_sha256: "sha256:c08d035469a4b5302ffd2b8d650b3d6b8abe88756eb56628750b6b8611b5e32c"
  - resource: "apps/llm/tests/test_gateway_failure_log.py#L59-78"
    excerpt_sha256: "sha256:bf6c53c98cccf55730090efbd011ef52cb2e520cb838892a70fac73273d21834"
  - resource: "apps/platform/internal/trial/improvement/eval.go#L489-504"
    excerpt_sha256: "sha256:9a71aaf15dd1c793942d148cb307a51ae86c5eb0270b67cbb37f6f386a0ec06a"
  - resource: "apps/platform/internal/entrypoint/worker/evaluation_execute.go#L35-44"
    excerpt_sha256: "sha256:56cec5cb8f7d53384bca8cd2953816344b9df952aba28088e2eb67198ec1efee"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_integration_test.go#L810-852"
    excerpt_sha256: "sha256:62521314a041b91bae1db39c100f64c4177c5fbfd5f146b68d99ad28e9d35eea"
---
When the model gateway refuses a call because its platform-wide budget for the period is spent, the refusal is named as such rather than reported as a broken gateway, and every such refusal is counted for alerting. A creation step refused this way is not counted against the session, the session waits for the person to send again, and only the cost actually incurred is settled; a skill generation or a criteria suggestion refused this way answers 503 saying nothing was charged and that retrying now is refused too; a judgement refused this way settles as failed with a summary naming the spent budget, that it was not charged and that it will be judged again; the evaluation job is snoozed hourly without spending its attempts, and once the budget recovers a new judgement supersedes the failed one, giving up after a bounded number of snoozes. A refusal of one key's own budget is not this case and stays an ordinary gateway failure.

# Relations
- Contexts: [creation](/contexts/creation)
- Contexts: [ingest](/contexts/ingest)
- Contexts: [testlab](/contexts/testlab)
- Contexts: [eval](/contexts/eval)

# Examples
- The gateway answers 429 budget_exceeded without naming a key: the capability service answers 503 "model gateway platform budget exhausted" and the creation session returns to waiting for input with its step count unchanged.
- The gateway answers 429 budget_exceeded naming the step's key: the capability service answers 502 and the step fails as before.
- A skill generation refused this way answers 503 and creates no version.
