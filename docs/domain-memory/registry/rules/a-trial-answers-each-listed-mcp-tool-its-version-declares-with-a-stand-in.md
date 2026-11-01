---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-01T06:02:14.778167Z"
sources:
  - resource: "apps/platform/internal/trial/execution/schedule.go#L592-592"
    excerpt_sha256: "sha256:04664ebf16d7d50a17fb31fa4df7b5aa70d3cec22e0575d6baa14890d35eecec"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/mcp_tools_integration_test.go#L203-228"
    excerpt_sha256: "sha256:48d47487ae43876a53d6321ac6a25e5e3684b31897f0b84eff02b312ff2e5928"
  - resource: "apps/platform/internal/trial/execution/provider.go#L182-186"
    excerpt_sha256: "sha256:44281042665c3a7e0f846cbc4efeb03bedc6c6d26f42f14cdecac10a1a00060f"
---
A trial run is dispatched with a stand-in for each MCP tool its Skill Version declares in allowed-tools that is on the mcp_tools list and not retired, and for no other mcp__ name; each stand-in carries the tool reference, purpose and parameters and never the server address, so the runtime can answer the call without reaching the tool.

# Relations
- Contexts: [run](/contexts/run)

# Examples
- A version declaring Read, a listed mcp__calendar__create_event, a retired mcp__calendar__delete_event and an unlisted mcp__calendar__send_mail is dispatched with one stand-in, mcp__calendar__create_event.
- A version declaring only Read and Bash is dispatched with no stand-ins.
