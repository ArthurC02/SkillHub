---
type: Business Rule
status: draft
owner: skill-packaging
---
An export names every MCP server its Skills declare in allowed-tools that is on the operator's MCP tool list: a plugin declares each one as a remote Streamable HTTP server in mcp.json and .mcp.json, a single Skill package lists them in INSTALL.md; a tool not on the list, a server listed under two addresses, or an address with credentials or a fragment is named for the reader to set up and never written, and an export whose tool list cannot be read is not produced.

# Relations
- Contexts: [packaging](/contexts/packaging)

# Examples
- A member declares mcp__docs__search and docs is listed at https://docs.example.com/mcp: mcp.json declares docs as streamable-http at that address and .mcp.json as http.
- A member declares mcp__crm__lookup, which is not on the list: no server is written for it and the README names it for the reader to set up.
- The MCP tool list cannot be read: no plugin is built.
