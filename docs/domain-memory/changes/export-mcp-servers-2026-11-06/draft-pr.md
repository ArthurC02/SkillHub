# Draft PR: an export carries the listed MCP servers its Skills declare

## Business intent

Someone installing an exported Skill or plugin no longer has to find out on their own which MCP servers it calls. A plugin is wired to the listed servers its members declare, and both shapes name what is left for the reader to set up.

## Domain impact

packaging gains the rule an-export-carries-the-listed-mcp-servers-its-skills-declare. The rules a-plugin-package-is-blocked-by-any-blocked-member and a-plugin-member-version-belongs-to-its-skill are unchanged; their citations move with the code. packaging reads the offered MCP tools with the Skill Version, as the trial run already does; no new dependency policy.

## Implementation handoff

The version read that packaging is given also returns the offered MCP tools for the version's allowed-tools. mcpNeedsOf groups the declared MCP references by server: a listed one with one https address free of credentials and fragments becomes a server; anything else is a gap. A plugin writes mcp.json (Agent Plugins, streamable-http) and .mcp.json (Claude Code, http) and lists servers and gaps in its README; a single Skill package lists them in INSTALL.md. Rejected: writing stdio commands (a local process the platform would be choosing to run) and guessing a server for an unlisted tool. Proof: removing the line that fills a version's MCP tools fails TestExportsCarryTheListedMCPServersTheirSkillsDeclare against the database; writing an address with credentials fails the grouping table.

## Proposal and approvals

export-mcp-servers-2026-11-06, one developer approval; the direction was approved by the user.

## Contract impact

None. The plugin zip gains README.md, mcp.json and .mcp.json; plugin.json gains author and, when shared, license.

## Verification

rule-an-export-carries-the-listed-mcp-servers-its-skills-declare maps to the delivery MCP tests; plugin-member-rules-still-hold maps to the plugin member tests.

## Residual risks

The platform records an address but not a transport; servers are declared as Streamable HTTP, the current MCP transport. A server that only speaks the deprecated HTTP+SSE transport fails to connect in the client and is reported there.
