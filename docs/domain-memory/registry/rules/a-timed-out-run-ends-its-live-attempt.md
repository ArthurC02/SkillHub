---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-16T16:32:08.394666Z"
sources:
  - resource: "apps/platform/internal/trial/execution/job.go#L103-109"
    excerpt_sha256: "sha256:3f1b00aec8279e0b970e2732c26c32c251d7e706385577f4ee6f04bacbe15336"
  - resource: "apps/platform/internal/trial/execution/supervisor.go#L107-112"
    excerpt_sha256: "sha256:67183cc4ca98ecbd7fc22fb4e6b0cb51cb14a9b8fd42cba7a81ae10bef96f200"
---
When a Run reaches its deadline while an attempt is still live, the attempt is finished as timed out in the same step that moves the Run to timed_out, whether the deadline is noticed by the Run's own job or by the supervisor; a timed-out Run never leaves a live attempt behind.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A sandbox still running at the deadline: the Run becomes timed_out and its attempt carries the timeout error class.
- A Run whose job died before its deadline: the supervisor times it out and closes the attempt the job left live.
