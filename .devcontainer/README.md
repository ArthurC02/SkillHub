# SkillHub Dev Container

This is the recommended clean-machine path for ADR-030. It builds the pinned
`infra/images/devtools` image, initializes `.env` without overwriting it, and
downloads language dependencies.

## Docker socket trust boundary

The container mounts `/var/run/docker.sock` so `task dev` and Docker-backed
integration tests can use the host Docker daemon. Possession of that socket is
equivalent to administrator/root control of the Docker host. Open this Dev
Container only for the trusted SkillHub repository; never run an imported Skill,
Dataset, generated artifact, or other untrusted workload in the devtools
container. Untrusted execution belongs exclusively to `services/sandbox` and
its ADR-005/015 isolation.

Docker Desktop exposes the Linux socket through its VM on Windows and macOS. On
native Linux, `postStartCommand` grants the container's `vscode` group access to
the mounted socket. If organizational policy forbids socket mounting, use the
native toolchain and point Docker CLI at an approved remote context instead;
`task doctor` will report whether it is reachable.

The default UID/GID is 1000. Dev Container tooling updates it to the local user
where supported; custom image builds can override `USER_UID` and `USER_GID`.
