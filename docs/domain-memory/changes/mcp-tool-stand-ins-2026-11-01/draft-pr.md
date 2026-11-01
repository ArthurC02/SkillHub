# Draft PR: answer a trial's listed MCP tools with stand-ins

## Business intent

A creator whose Skill declares a listed MCP tool can try it: the trial offers the tool as a stand-in and says which step needs it, without reaching any MCP server.

## Domain impact

run owns the rule a-trial-answers-each-listed-mcp-tool-its-version-declares-with-a-stand-in. ingest supplies the unretired mcp_tools rows through the composition root (interaction ingest-run-mcp-stand-ins, synchronous); run never imports ingest. registry-run-facts-v1 gains the stand-ins additively.

## Implementation handoff

The intersection is taken at dispatch because the list changes and versions are immutable; rejected: storing it with the version, sending the whole list, a new Service field. Proof: the two dispatch integration tests and the mutation of the dispatch line, the wiring assignment and the filter, each red then restored.

## Proposal and approvals

mcp-tool-stand-ins-2026-11-01, one developer approval.

## Contract impact

contracts/openapi/sandbox-provider.yaml RunRequest.mcp_tools (MCPToolStandIn: reference, purpose, parameters), additive and optional.

## Verification

rule-a-trial-answers-each-listed-mcp-tool-its-version-declares-with-a-stand-in → the two integration tests; evidence-current → verify-evidence on every cited file.

## Residual risks

The runtime part (run.mjs answers the stand-in) is outside the confirmed sources and is recorded in the specification and ADR, not here.
