# SkillHub Dev Container

This is the recommended clean-machine path documented in [開發自動化與依賴治理](../docs/adr/README.md#開發自動化與依賴治理). It builds the pinned
`infra/images/devtools` image, initializes `.env` without overwriting it, and
downloads language dependencies.

## Startup and initialization flow

- `postStartCommand` starts a nested Docker daemon (`dockerd`) and waits until
  `docker info` succeeds.
- `postCreateCommand` runs `.devcontainer/post-create.sh`, which checks required
  tool binaries, verifies required `.env` keys, and runs dependency bootstrap.
- Bootstrap is cached by lockfile hash and skipped when the dependency graph is
  unchanged.

If you need a clean baseline, delete `.devcontainer/.bootstrap.stamp` and run:

```bash
bash .devcontainer/post-create.sh
```

Use `.devcontainer/.env.remote.example` as an optional starter template when
preparing a remote-only `.env`.

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
