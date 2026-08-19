# SkillHub Dev Container

This is the recommended clean-machine path for ADR-030. It builds the pinned
`infra/images/devtools` image, initializes `.env` without overwriting it, and
downloads language dependencies.

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
belongs exclusively to `apps/sandbox` and its ADR-005/015 isolation. If
organizational policy forbids privileged containers, use the native toolchain;
`task doctor` reports whether its Docker daemon is reachable.

The default UID/GID is 1000. Dev Container tooling updates it to the local user
where supported; custom image builds can override `USER_UID` and `USER_GID`.
