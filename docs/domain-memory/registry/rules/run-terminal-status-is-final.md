---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T05:14:37.495704Z"
sources:
  - resource: "apps/platform/internal/trial/execution/statemachine.go#L58-79"
    excerpt_sha256: "sha256:2b24d1bfc40e17508f9b41163d986c73853f889a335b9e2b5a679853382d0504"
  - resource: "apps/platform/internal/trial/execution/statemachine.go#L102-105"
    excerpt_sha256: "sha256:6dd7b2e7a94fc3753f72114d807cf544fd975da385086d33b97fd39ed6f2d5cf"
  - resource: "apps/platform/internal/trial/execution/statemachine.go#L177-179"
    excerpt_sha256: "sha256:403523b7886f9a7fe5d0410a2c5d1fd06e6ef99d834e335ca5ff384f6bcab486"
  - resource: "apps/platform/internal/trial/execution/statemachine_test.go#L100-118"
    excerpt_sha256: "sha256:587351aa75fd051a0f1e2172d889d9fb455b955927abd7820d707187308779a6"
---
A Run that has reached a terminal status never transitions again. succeeded, failed, cancelled and timed_out have no successors, and a command asking a Run to leave one of them is refused as an illegal transition.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A cancel arriving after the Run already succeeded is refused, not applied.
- A late provider callback reporting failure on a Run that already timed out is refused, so the recorded outcome does not change after the fact.
- A Run in evaluating may still move to succeeded, failed, cancelled or timed_out, because evaluating is not terminal.
