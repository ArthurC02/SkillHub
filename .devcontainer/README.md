# SkillHub Dev Container

This is the recommended clean-machine path documented in [開發自動化與依賴治理](../docs/adr/README.md#開發自動化與依賴治理). It builds the pinned
`infra/images/devtools` image, initializes `.env` without overwriting it, and
downloads language dependencies.

## Remote-development quick start

- **GitHub Codespaces**: create a Codespace from the repository and wait for the
  post-create step to finish.
- **VS Code Dev Containers**: install the Dev Containers extension, clone the
  repository locally, then run **Dev Containers: Reopen in Container**.

The container opens `README.md` and this file by default. `postCreateCommand`
runs `.devcontainer/post-create.sh`, which calls `go -C tools/devctl run . env-init`
and `go -C tools/devctl run . bootstrap`. This gives the container a local
`.env`, Go modules, Node packages, Python virtual environment, and repo hooks
without overwriting an existing `.env`.

`postStartCommand` runs `.devcontainer/post-start.sh`, which starts the nested
Docker daemon on demand and waits until `docker info` succeeds. That keeps the
first window usable even before you start local infrastructure with `task dev`.

### Common ports

- `5173`: Web dev server (auto-opens as a preview)
- `8080`: Platform API
- `4000`: LiteLLM gateway
- `8333`: SeaweedFS S3 API
- `5432`: PostgreSQL

## Daily development flow

1. Wait for the Dev Container to finish `postCreateCommand`.
2. Run `task doctor` to verify the pinned tools inside the container.
3. Run `task dev` to start PostgreSQL and SeaweedFS.
4. Start the product processes you need:
   - `go -C apps/platform run ./cmd/api`
   - `go -C apps/platform run ./cmd/worker`
   - `cd apps/llm && uv run uvicorn skillhub_llm.app:app`
   - `go -C apps/sandbox run ./cmd/sandboxd`
   - `npm --prefix apps/web run dev`

For model-backed work, opt in separately with `task dev:model` and
`task dev:llm` after populating the ignored `.env` file with real secrets.

## Docker-in-Docker trust boundary

The Dev Container runs privileged so it can start its own Docker daemon, backed
by the `skillhub-devcontainer-docker` volume. This is intentional: mounting the
host socket makes a nested generator interpret `/workspace` as a path on the
physical host, which is a different path on Windows/macOS and fails clean-clone
generation. DinD gives the CLI and daemon the same `/workspace` namespace and
keeps its images/volumes separate from the host daemon.

Privileged mode still grants broad kernel capabilities. Open this Dev Container
only for the trusted SkillHub repository; never run an imported Skill, Dataset,
generated artifact, or other untrusted workload in devtools. Untrusted execution
belongs exclusively to `apps/sandbox` and its [Sandbox 隔離與執行安全](../docs/adr/README.md#sandbox-隔離與執行安全) isolation. If
organizational policy forbids privileged containers, use the native toolchain;
`task doctor` reports whether its Docker daemon is reachable.

The default UID/GID is 1000. Dev Container tooling updates it to the local user
where supported; custom image builds can override `USER_UID` and `USER_GID`.
