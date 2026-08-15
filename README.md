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

Line endings are LF everywhere, enforced by [`.gitattributes`](.gitattributes),
which overrides `core.autocrlf`. No per-machine git config is needed on Windows
or anywhere else — leave `core.autocrlf` at whatever it is and clone normally.
Without this, a CRLF checkout makes `gofmt`, `prettier --check` and
`golangci-lint fmt --diff` report format errors on files you never touched,
because they read the working tree directly rather than through git, while the
same checks pass on CI's Linux runner. A clone that predates the file can be
refreshed once with `git rm --cached -r . && git reset --hard` (commit your work
first — this rewrites the working tree).

Running the SPA against a local API needs `DEV_CORS_ORIGIN=http://localhost:5173`
on `cmd/api`: the two are separate origins in development and same-origin in
production, so the allowance is opt-in per process and unset everywhere else.
`httpx.DevCORS` explains why this is not a Vite dev-server proxy.

## Before you write code

Read [AGENTS.md](AGENTS.md) for the implementation rules, and
[ADR-019](adr/ADR-019-monorepo-structure-and-cicd.md) for why the repository is
laid out this way and what CI enforces.
