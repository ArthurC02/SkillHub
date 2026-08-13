# Skill Hub

A search engine and testing lab for Agent Skills: find a candidate Skill, try it
in an isolated sandbox with your own prompts and data, and download a portable
package.

## Layout

| Path | What lives there |
| --- | --- |
| `apps/web/` | React + TypeScript SPA (Vite) |
| `services/platform/` | Go control plane: `cmd/api` HTTP server, `cmd/worker` queue consumer |
| `services/llm/` | Python FastAPI service for LLM workloads (uv) |
| `contracts/openapi/` | OpenAPI specs — the single source of truth for cross-language interfaces |
| `db/` | PostgreSQL migrations, sqlc queries and config |
| `infra/compose/` | Local and single-host deployment stack |
| `plans/`, `adr/` | Product plan and architecture decision records (Traditional Chinese) |
| `spikes/` | M0 exploration code; never imported by product code, never built in CI |

## Development

Prerequisites: Go, Node.js, [uv](https://docs.astral.sh/uv/), Docker, and
[Task](https://taskfile.dev/).

```bash
task dev                 # start Postgres
task test                # run every test suite
task lint                # lint every service
```

Each service also works with its own native toolchain — `go test ./...`,
`uv run pytest`, `npm test` — from its own directory.

## Before you write code

Read [AGENTS.md](AGENTS.md) for the implementation rules, and
[ADR-019](adr/ADR-019-monorepo-structure-and-cicd.md) for why the repository is
laid out this way and what CI enforces.
