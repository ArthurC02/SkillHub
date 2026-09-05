# Skill Hub

Skill Hub 的產品核心是 Catalog、輕鬆創建與私人訂製／公開散布。互動式創作已依 [ADR-067](docs/adr/ADR-067-interactive-skill-creation-with-langgraph.md) 接上 Python LangGraph、Go 會話與 Web 三種入口；[設定與驗證](docs/development/interactive-creation.md) 列出免費證據及待量測項目。功能預設關閉，尚未解封 M5 曝光或付費實測。

An Agent Skill platform for discovery, creation, private customization and public
distribution. Existing search, sandbox trials and portable packages support this
journey; conversational creation is accepted planning, not an implemented feature.

## Layout

| Path | What lives there |
| --- | --- |
| `apps/web/` | React + TypeScript SPA (Vite) |
| `apps/platform/` | Go control plane: `cmd/api` HTTP server, `cmd/worker` queue consumer |
| `apps/llm/` | Python FastAPI service for LLM workloads (uv) |
| `apps/sandbox/` | Go execution-plane provider (`sandboxd`) |
| `packages/` | Importable libraries: committed generated TS client and Python transport models; generated subdirectories are never hand-edited |
| `contracts/` | Cross-process interface sources of truth: OpenAPI, event and packaging schemas |
| `db/` | PostgreSQL persistence sources: migrations, sqlc queries/config and DB tests |
| `infra/` | Deployment, runtime image, network, node and observability configuration |
| `tools/` | Developer, CI, data-maintenance and operations commands with their tightly coupled fixtures |
| `docs/` | Narrative documentation and historical records, never product imports (ADR-031) |
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
task gen:openapi         # regenerate Go/TypeScript/Python contract outputs
task gen:check           # verify generated output without changing tracked files
task automation:check    # verify Agent docs, task descriptions and ownership markers
task ci                  # run the deterministic, secret-free local CI sequence
task test                # run every test suite
task lint                # lint every application
```

On a new machine without Task, use `go -C tools/devctl run . doctor` and
`go -C tools/devctl run . env-init` as the bootstrap escape hatch. Never fill
real credentials into [`.env.example`](.env.example); only the ignored `.env`
may hold local secrets.

Coding Agents must read [`AGENTS.md`](AGENTS.md); detailed cross-machine setup,
generation ownership, shared-worktree rules and troubleshooting live in
[`docs/development/automation.md`](docs/development/automation.md).

Each application also works with its own native toolchain — `go test ./...`,
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

That arrangement has a second half, on the web side, and it is a file rather
than a flag: [`apps/web/.env.development`](apps/web/.env.development) points the
client at `http://localhost:8080` for `npm run dev` only. `vite build` runs in
production mode and never reads it, so a built bundle calls same-origin paths —
the shape a production deployment and the clean test mode both serve. The
default lives that way round because `API_BASE_URL` is resolved at BUILD time:
whichever value it takes when nobody sets one is the one every artifact carries.
`npm run build` refuses a bundle that talks to an absolute origin
([`scripts/check-bundle-origins.mjs`](apps/web/scripts/check-bundle-origins.mjs));
deleting the `.env.development` file breaks `npm run dev` and nothing else.

## Before you write code

Read [AGENTS.md](AGENTS.md) for the implementation rules, and
[ADR-019](docs/adr/ADR-019-monorepo-structure-and-cicd.md) plus
[ADR-031](docs/adr/ADR-031-artifact-role-repository-layout.md) for why the repository
is laid out this way and what CI enforces.
