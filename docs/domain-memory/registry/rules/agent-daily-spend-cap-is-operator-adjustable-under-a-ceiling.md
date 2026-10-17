---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-10-17T05:42:18.112047Z"
sources:
  - resource: "apps/platform/internal/product/operations/agents.go#L38-45"
    excerpt_sha256: "sha256:6211e42ac2359b958cd10d4857a002f3bc66b51d2ba7611f690b9dba4df51f66"
  - resource: "apps/platform/internal/product/operations/agents.go#L146-151"
    excerpt_sha256: "sha256:340bcb7471e9f24e68d0ab5b19f3262013d77d69a40ce07f12ba96908a86e518"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/platform_settings_integration_test.go#L44-84"
    excerpt_sha256: "sha256:9da17d1845d4d40b7dced044969315d2916def066862fa0dc46684329f677b27"
---
A platform agent's effective daily spend cap is the operator's override when one is set, otherwise the default its definition declares. An override must be above zero and at most US$5 a day, and re-registering the definitions replaces only the default, never the override.

# Relations
- Contexts: [operations](/contexts/operations)

# Examples
- An operator raising an agent's cap to 5000000 micros takes effect at that agent's next run; 5000001 micros is refused.
- Clearing the override returns the agent to the default its definition declares.
- A worker restart re-registers every definition and the operator's override survives it.
