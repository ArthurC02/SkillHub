---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-06T19:38:45.654816Z"
sources:
  - resource: "apps/platform/internal/trial/improvement/evaluation_test.go#L209-255"
    excerpt_sha256: "sha256:f7916b8d47b08f9157c7f24573e676ad5feef8643653e0ad00a99355a101bf7d"
  - resource: "apps/platform/internal/trial/improvement/evaluation.go#L278-279"
    excerpt_sha256: "sha256:00f4885c3c736f5cc30fce59ad00bb6007ce49f3b9720a625fd37adef39d40de"
---
Once a suggestion has gone into a Skill version its acceptance cannot be withdrawn; rejecting it is refused as acceptance_is_final.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- A suggestion applied to version 9 cannot be flipped to rejected.
