<h1 align="center">Skill Hub</h1>

<p align="center">
  An open platform for discovering, creating, trialling and distributing Agent Skills with evidence, provenance and controlled execution.
</p>

<p align="center">
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <a href="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/ArthurC02/SkillHub/actions/workflows/ci.yml/badge.svg"></a>
  <a href="README.zh-TW.md">繁體中文</a>
</p>

> [!IMPORTANT]
> Skill Hub is pre-release software. The repository contains working product paths, but public exposure, paid model use and production deployment remain governed by explicit configuration and verification. Read the [current plan and milestone status](docs/plans/01-goals-and-plan.md) before treating a capability as released.

## Why Skill Hub

Agent Skills are useful only when people can answer practical questions: where did this come from, what can it access, will it work for my task, what did it cost, and can I take the result elsewhere? Skill Hub makes those questions part of the product rather than leaving them to convention.

- **Discover** Skills through keyword and semantic search, with source, licence, dependency, permission and compatibility context.
- **Create and improve** Skills from a task description or guided conversation, while preserving an auditable version history.
- **Trial** a frozen Skill Version with an explicit prompt, test data, acceptance criteria, cost boundary and trace.
- **Evaluate** results as met, partially met, unmet or undetermined, then turn approved improvements into a new immutable version.
- **Package and distribute** portable Skill packages that retain provenance, licence and selected test material.

The product deliberately does not use a global quality leaderboard: evidence is meaningful only in the context of the task and acceptance criteria that produced it.

## Architecture at a glance

```text
Browser
  │
  ▼
React web ───────────────► Go control plane API ───► PostgreSQL + S3-compatible object storage
                                  │                         │
                                  │                         └── transactional outbox
                                  ▼
                           Go Worker (only queue consumer)
                              │                    │
                 internal HTTP│                    │internal HTTP
                              ▼                    ▼
                     Python LLM capability     sandboxd on isolated nodes
                              │                    │
                              ▼                    ▼
                        LiteLLM gateway       gVisor runtime image
                              │
                              ▼
                        model provider
```

The Go control plane owns authorization, Workspace scope, domain rules, Run state and all core data. The Python LLM service and Sandbox are capability providers: they accept structured requests and return structured results, but never access the core database. Every model call goes through LiteLLM; provider credentials stay at the gateway. A Run receives a short-lived Virtual Key, and its state transition plus outward event are recorded transactionally.

The platform code is organized into reviewed bounded contexts for creator, product, skill and trial work. External systems sit behind ports and adapters, while package ownership, query ownership and cross-context dependencies are checked mechanically. See the [architecture identity](apps/platform/architecture-identity.yaml), [bounded-context model](docs/adr/README.md#platform-bounded-context-與-context-map) and [architecture decisions](docs/adr/README.md).

## Security model

- Untrusted Skills, scripts and uploaded data never run in the web or API process.
- The execution plane cannot connect to the core database.
- User data is Workspace-scoped from the authenticated session, not a client-supplied workspace id.
- Skill Versions, Test Case snapshots and historical Runs are immutable; adopting an improvement creates a new version.
- Secrets must not appear in packages, logs, traces or analytics. Traces are masked before storage.
- Production Sandbox nodes use gVisor with default-deny egress controls and are replaced rather than repaired.

Clean test mode is intentionally **not** a security boundary. It uses in-process stand-ins so the product can be demonstrated without Docker, keys or network access; do not use it for untrusted Skills or real data.

## Quick start: clean test mode

The shortest path to a running product is a cost-free demonstration mode. It needs Go and Node.js, but not Docker or model credentials.

```bash
task doctor
task bootstrap
npm ci --prefix tools/pglite
npm --prefix apps/web run build
task clean-mode
```

The launcher prints a local URL and explains any unmet prerequisite. Without [Task](https://taskfile.dev/), use `go -C tools/devctl run . doctor`, `go -C tools/devctl run . bootstrap`, and `node tools/cleanmode/start.mjs --seed`.

Clean mode substitutes an embedded database, in-memory object storage and a local-process driver. It demonstrates the browser-to-API journey, not production isolation, presigned object URLs, concurrency, object storage or paid model capability.

## Local full-stack development

Start with the portable diagnostics; exact tool versions are owned by `go.mod`, `.node-version`, `apps/llm/.python-version` and [`tools/toolchain.yaml`](tools/toolchain.yaml), not this README.

```bash
task doctor
task env:init
task bootstrap
task gen:check
task dev
```

`task dev` starts local PostgreSQL and SeaweedFS without model cost. Then run the product processes in separate terminals:

| Component | Command | Responsibility |
| --- | --- | --- |
| API | `go -C apps/platform run ./cmd/api` | HTTP, authentication, authorization and domain commands |
| Worker | `go -C apps/platform run ./cmd/worker` | Run dispatch, cleanup, outbox and periodic work |
| LLM capability | `cd apps/llm && uv run uvicorn skillhub_llm.app:app` | structured model-backed capabilities |
| Sandbox provider | `go -C apps/sandbox run ./cmd/sandboxd` | local execution-provider boundary |
| Web | `npm --prefix apps/web run dev` | React development UI |

For a local SPA, set `DEV_CORS_ORIGIN=http://localhost:5173` on the API process. For a real Run, API and Worker must share the same database, object-storage, Sandbox, model-gateway and Trace settings; the full dependency and verification sequence is in the [Provision guide](docs/runbooks/provisioning.md).

### Optional model capability and cost

Model capability is off by default.

```bash
task dev:model
task dev:llm
```

The first command starts LiteLLM after checking required secrets. The second mints a budget-limited Virtual Key for `apps/llm`; it does not give that service the gateway master key. A model request can incur cost, so paid live tests are opt-in and never part of the default test command.

## Test and verify

```bash
task gen:check     # generated contracts and SQL output match their sources
task test          # normal test suites; paid E2E remains opt-in
task ci            # deterministic, secret-free local CI sequence
task preflight     # checks unpushed commits against CI-facing rules
```

A green local check is evidence about this machine, not proof of hosted CI or production readiness. [CI diagnostics](docs/runbooks/ci-red.md) explain how to distinguish a skipped, opaque or genuinely failing workflow result. When fixing behaviour, tests are expected to prove that the un-fixed behaviour fails before claiming a fix.

## Repository map

| Path | Purpose |
| --- | --- |
| `apps/web` | React and TypeScript user interface |
| `apps/platform` | Go control plane, Worker and bounded contexts |
| `apps/llm` | Python FastAPI capability provider |
| `apps/sandbox` | Go Sandbox provider |
| `packages` | Reusable libraries and generated API clients |
| `contracts` | Source of truth for cross-process OpenAPI, events and packaging contracts |
| `db` | Migrations, queries, SQL ownership and sqlc configuration |
| `infra` | Compose, deployment, runtime images, egress and observability |
| `tools` | Development, CI, data-maintenance and operations commands |
| `docs` | Product plans, architecture decisions, design guidance and runbooks |

## Documentation

- [Product plan and current milestone status](docs/plans/01-goals-and-plan.md)
- [Specifications and acceptance criteria](docs/plans/02-specifications-and-acceptance-criteria.md)
- [Architecture decisions](docs/adr/README.md)
- [Developer automation](docs/development/automation.md)
- [Platform DDD practices](docs/development/platform-ddd-practices.md)
- [Provision and operations runbooks](docs/runbooks/README.md)
- [`AGENTS.md`](AGENTS.md), the repository rules for people and coding agents

## Contributing

Contributions are welcome. Start with [CONTRIBUTING.md](CONTRIBUTING.md), keep generated files generated, define cross-process interfaces in `contracts/` first, and run the checks appropriate to the change. Do not commit `.env`, credentials, paid-test outputs or secrets.

For a security issue, do not open a public issue. Use the private process in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
