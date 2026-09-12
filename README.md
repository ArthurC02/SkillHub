<h1 align="center">Skill Hub</h1>

<p align="center">
  An open platform for finding, building, trialling and sharing Agent Skills.
</p>

<p align="center">
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <a href="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml/badge.svg"></a>
  <a href="README.zh-TW.md">繁體中文</a>
</p>

## What it does

- **Catalog and search** — keyword and semantic search over published skills, with the two combined into one ranking.
- **Sandboxed trials** — every run executes in an isolated sandbox, and the run keeps its trace and its cost.
- **Evaluation** — test cases and judged results per skill version, so "it got better" is a measurement rather than an impression.
- **Immutable versions and portable packages** — a published version never changes, and a skill exports as a package that carries its licence, its provenance and, if you want, its test cases.
- **Guided creation** — a conversational flow that drafts a skill from a plain request. Off by default.
- **Clean test mode** — the same system, running on a machine that cannot install software or reach the internet. See [below](#clean-test-mode).

## Quick start

The fastest way to see the product running is **clean test mode**: one command, no Docker, no API keys, no cost.

```bash
task bootstrap                    # Go, npm and uv dependencies
npm ci --prefix tools/pglite      # the embedded PostgreSQL carrier
npm --prefix apps/web run build   # clean mode serves this build itself
task clean-mode                   # starts everything, then prints the URL
```

`task clean-mode` starts three processes and prints `open http://127.0.0.1:8080/`. It refuses to start with a named reason and the exact command to fix it when something is missing, so you can also just run it first and follow what it says.

The app itself works straight away; the bundled demo skills only reach the catalog once the model-facing service is running too, because a package stays unindexed until it has been enriched.

Without [Task](https://taskfile.dev/), every command has a plain equivalent: `go -C tools/devctl run . bootstrap` and `node tools/cleanmode/start.mjs --seed`.

## Requirements

| Tool | Needed for |
| --- | --- |
| Go | the control plane and the sandbox provider |
| Node.js | the web app, the clean-mode launcher and the database carrier |
| [uv](https://docs.astral.sh/uv/) | the Python service and its interpreter |
| Docker | the development database, object storage and model gateway |
| [Task](https://taskfile.dev/) | optional; every task has a plain command equivalent |

Exact versions live in the files the tooling actually reads — `go.mod`, `.node-version`, `apps/llm/.python-version` and [`tools/toolchain.yaml`](tools/toolchain.yaml) — never in prose. `task doctor` compares your machine against them and names whatever is off.

A [Dev Container](.devcontainer/README.md) is available with everything pinned and installed; clean test mode needs only Node and Go.

## Running the full system

Clean test mode swaps three pieces out. To run the real thing, start the infrastructure and then the four services:

```bash
task doctor      # versions and prerequisites
task env:init    # create .env from .env.example; never overwrites an existing one
task bootstrap   # dependencies; also points git's hooksPath at .githooks
task dev         # Postgres and SeaweedFS containers, no secrets, no cost
```

| Service | Command | Port |
| --- | --- | --- |
| Control plane API (Go) | `go -C apps/platform run ./cmd/api` | 8080 |
| Queue worker (Go) | `go -C apps/platform run ./cmd/worker` | — |
| Model-facing service (Python) | `uv run uvicorn skillhub_llm.app:app` in `apps/llm` | 8000 |
| Sandbox provider (Go) | `go -C apps/sandbox run ./cmd/sandboxd` | 9000 |
| Web app (React) | `npm --prefix apps/web run dev` | 5173 |

The API needs `DATABASE_URL`; the sandbox provider refuses to start without `SKILLHUB_SANDBOX_TOKEN`. [`.env.example`](.env.example) lists every variable — fill values into the ignored `.env`, never into the example.

Model calls go through a gateway that is **off by default**. `task dev:model` starts it and checks that the required keys are present; anything that reaches a provider after that costs money. Everything else in this section is free.

Running the SPA against a local API also needs `DEV_CORS_ORIGIN=http://localhost:5173` on the API process: in development the two are separate origins, in production they are not, so the allowance is opt-in per process.

## Clean test mode

Skill Hub normally needs a real database, real object storage and a real isolated sandbox. Clean test mode is the **same program** with those three swapped for stand-ins that run in-process: PostgreSQL compiled to WebAssembly, an in-memory object store, and a local-process execution driver. It exists because some demonstrations happen on machines that cannot install software and have no general network access.

```bash
task clean-mode
```

> [!WARNING]
> This mode is weaker than production on purpose, and it says so on screen: **the sandbox provides no isolation**, presigned object URLs are **not verified**, and the database serves **a single connection**, so concurrency does not behave the way production does. Never point it at untrusted skills or real data.

Setting no flag changes nothing: without `SKILLHUB_CLEAN_MODE` the program builds exactly the production wiring.

## Repository layout

| Path | Contents |
| --- | --- |
| `apps/` | The four deployable programs: `web`, `platform`, `llm`, `sandbox` |
| `packages/` | Libraries other programs import, including generated API clients |
| `contracts/` | The source of truth for every cross-process interface (OpenAPI, events, packaging) |
| `db/` | Migrations, queries and database tests |
| `infra/` | Compose files, runtime images, networking and observability |
| `tools/` | Developer, CI and operations commands |
| `docs/` | Architecture decisions, plans and runbooks |

## Documentation

- [Architecture decisions](docs/adr/README.md) — why the system is shaped the way it is.
- [Development handbook](docs/development/automation.md) — setup, code generation, CI and troubleshooting.
- [Runbooks](docs/runbooks/) — what to do when something breaks.
- [`AGENTS.md`](AGENTS.md) — the conventions and hard rules, written for coding agents and equally binding on people.

## Contributing

Issues and pull requests are welcome — start with [CONTRIBUTING.md](CONTRIBUTING.md), which covers what to run before opening one and the few rules that catch most review comments. Found a vulnerability? Report it privately: see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
