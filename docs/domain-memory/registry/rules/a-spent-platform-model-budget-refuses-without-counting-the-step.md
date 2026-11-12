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
  - resource: "apps/platform/internal/creator/creation/job.go#L638-643"
    excerpt_sha256: "sha256:0669a162edf1da62860f8f3f84153e74b9e4a8db97611e278c2df5833decbdf9"
  - resource: "apps/platform/internal/creator/creation/attempt_test.go#L96-96"
    excerpt_sha256: "sha256:e0ff0b608b99b32b0f8374190f33aa34ab8a15263047081b2820b36abc635e28"
  - resource: "apps/platform/internal/skill/admission/model_adapter.go#L82-84"
    excerpt_sha256: "sha256:2c79547d31a02acc5cf2dfd02ae75a7a4dd7b34a31ce96d00a61db5e8dfac790"
  - resource: "apps/platform/internal/skill/admission/http.go#L394-395"
    excerpt_sha256: "sha256:2dd124cccc31184279bd7eeb148d32195e5166c4e0a7a11557469067be0a6ef5"
  - resource: "apps/platform/internal/trial/design/criteria_adapter.go#L30-32"
    excerpt_sha256: "sha256:40a04ab23f5aed9b9dfd8652ab06d501a6430c195b096b04d0b3ef9fc10cf49e"
  - resource: "apps/platform/internal/trial/design/http.go#L77-78"
    excerpt_sha256: "sha256:9051d592b78f5025d1db9bbaceb04f6c90a1e11f0b6abcf706cacca0460b70f7"
  - resource: "apps/platform/internal/trial/improvement/llmadapter/model_adapter.go#L46-48"
    excerpt_sha256: "sha256:c08d035469a4b5302ffd2b8d650b3d6b8abe88756eb56628750b6b8611b5e32c"
  - resource: "apps/llm/tests/test_gateway_failure_log.py#L59-78"
    excerpt_sha256: "sha256:bf6c53c98cccf55730090efbd011ef52cb2e520cb838892a70fac73273d21834"
  - resource: "apps/platform/internal/trial/improvement/eval.go#L509-523"
    excerpt_sha256: "sha256:4605c464c7a7fed2ddbb747639bc4b9b980f83b5dfe4c8eb9ca1e152a393ed03"
  - resource: "apps/platform/internal/entrypoint/worker/evaluation_execute.go#L35-44"
    excerpt_sha256: "sha256:56cec5cb8f7d53384bca8cd2953816344b9df952aba28088e2eb67198ec1efee"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/evaluation_integration_test.go#L810-852"
    excerpt_sha256: "sha256:68606224f0f630ed60ed847ff0132cd820f7af5c6e04ff1ac2035f7b865f42c1"
---
When the model gateway refuses a call because its platform-wide budget for the period is spent, the refusal is named as such rather than reported as a broken gateway, and every such refusal is counted for alerting. A creation step refused this way is not counted against the session, the session waits for the person to send again, and only the cost actually incurred is settled; a skill generation or a criteria suggestion refused this way answers 503 saying nothing was charged and that retrying now is refused too; a judgement refused this way settles as failed with a summary naming the spent budget, that it was not charged and that it will be judged again; the evaluation job is snoozed hourly without spending its attempts, and once the budget recovers a new judgement supersedes the failed one, giving up after a bounded number of snoozes. Every such refusal tells the person the budget recovers at 08:00 Taipei time (UTC midnight) each day, which holds only while the gateway's budget period is one day. A refusal of one key's own budget is not this case and stays an ordinary gateway failure.

# Relations
- Contexts: [creation](/contexts/creation)
- Contexts: [ingest](/contexts/ingest)
- Contexts: [testlab](/contexts/testlab)
- Contexts: [eval](/contexts/eval)

# Examples
- The gateway answers 429 budget_exceeded without naming a key: the capability service answers 503 "model gateway platform budget exhausted" and the creation session returns to waiting for input with its step count unchanged.
- The gateway answers 429 budget_exceeded naming the step's key: the capability service answers 502 and the step fails as before.
- A skill generation refused this way answers 503 and creates no version.
