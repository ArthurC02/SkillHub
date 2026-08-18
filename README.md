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
| `packages/` | Committed generated TS client and Python transport models; generated subdirectories are never hand-edited |
| `db/` | PostgreSQL migrations, sqlc queries and config |
| `infra/compose/` | Local and single-host deployment stack |
| `tools/` | Repo-side scripts: CI assertions, content import, golden-set evaluation |
| `docs/` | Everything that is read rather than run (ADR-024) |
| `docs/plans/`, `docs/adr/` | Product plan and architecture decision records (Traditional Chinese) |
| `docs/spikes/` | Tombstone only — the M0 exploration code was deleted once its conclusions landed in the M0 reports, ADR-013/023 and `tools/goldenset/` ([details](docs/spikes/README.md)) |

## Development

The recommended cross-machine path is the repository's Dev Container. Native
development needs Go, Node.js, [uv](https://docs.astral.sh/uv/), Docker, and
[Task](https://taskfile.dev/); exact versions come from native version files and
[`tools/toolchain.yaml`](tools/toolchain.yaml), not from this prose.

```bash
task doctor              # diagnose versions and missing prerequisites
task env:init            # create .env without overwriting an existing one
task bootstrap           # download Go, npm and uv dependencies
task dev                 # start secret-free Postgres and SeaweedFS
task dev:model           # opt in to LiteLLM; requires secrets and may spend money
task gen                 # regenerate committed outputs atomically
task gen:openapi         # regenerate TypeScript/Python contract outputs
task gen:check           # verify generated output without changing tracked files
task test                # run every test suite
task lint                # lint every service
```

On a new machine without Task, use `go -C tools/devctl run . doctor` and
`go -C tools/devctl run . env-init` as the bootstrap escape hatch. Never fill
real credentials into [`.env.example`](.env.example); only the ignored `.env`
may hold local secrets.

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
[ADR-019](docs/adr/ADR-019-monorepo-structure-and-cicd.md) plus
[ADR-024](docs/adr/ADR-024-top-level-repository-layout.md) for why the repository
is laid out this way and what CI enforces.
