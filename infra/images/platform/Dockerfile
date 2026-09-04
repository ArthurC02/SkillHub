# Skill Hub platform service image (ADR-019 CI job 5 / 04 丙-158).
#
# apps/platform is one Go module (no go.work, no replace directives) that
# produces four binaries under ./cmd — api, worker, maintenance, reindex
# (ADR-010 deployment units 2/3, plus the two operator-invoked commands). One
# image holds all four; the entrypoint's first argument picks which runs, so
# the four Dockerfile-less `docker run skillhub/platform api|worker|...`
# invocations stay a one-line difference in infra/compose/docker-compose.yml
# instead of four near-identical images.
#
# Build context MUST be the repo root, not apps/platform: the module itself is
# self-contained, but the runtime stage also copies contracts/packaging/profiles
# (PACKAGING_PROFILES_DIR, read by cmd/api — see envx.Or call in cmd/api/main.go)
# which lives outside the module tree.
#
# Build:
#   docker build -f infra/images/platform/Dockerfile -t skillhub/platform:local .

# syntax=docker/dockerfile:1.7

# Pinned to the same digest as infra/images/devtools/Dockerfile's golang stage
# (that file is the source of truth for the Go toolchain version; apps/platform/
# go.mod requires go 1.27.0). Digest, not tag, per I-02's convention (ADR-022).
FROM golang:1.27.0-bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466 AS build

WORKDIR /src

# Dependency layer first so an application-code-only change does not re-download
# the module graph.
COPY apps/platform/go.mod apps/platform/go.sum ./apps/platform/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd apps/platform && go mod download

COPY apps/platform ./apps/platform

# CGO_ENABLED=0: every binary below must run in the distroless static runtime
# stage, which ships no libc. None of the four commands need cgo (pgx is the
# pure-Go Postgres driver already in use).
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd apps/platform && mkdir -p /out \
    && go build -ldflags="-s -w" -o /out/api ./cmd/api \
    && go build -ldflags="-s -w" -o /out/worker ./cmd/worker \
    && go build -ldflags="-s -w" -o /out/maintenance ./cmd/maintenance \
    && go build -ldflags="-s -w" -o /out/reindex ./cmd/reindex

# Runtime: distroless static, not Alpine. These binaries are static
# (CGO_ENABLED=0) so musl buys nothing, and this is the control plane process
# iron rule 2 says untrusted content must never reach — an image with no shell
# and no package manager is less for that promise to lean on, not more. It
# ships ca-certificates and tzdata, which is everything four outbound-HTTPS-only
# Go binaries need from a base image.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime

# distroless/static's "nonroot" variant is uid/gid 65532 — same convention
# infra/images/runtime-agent-sdk/Dockerfile uses for the same reason (C-02:
# non-root from the image on up), already the default user of this base.

COPY --from=build /out/api /out/worker /out/maintenance /out/reindex /usr/local/bin/

# PACKAGING_PROFILES_DIR (default set in cmd/api/main.go via envx.Or) points
# here so the packaging targets ship inside the image rather than requiring a
# volume mount for a handful of static JSON files.
COPY contracts/packaging/profiles /contracts/packaging/profiles
ENV PACKAGING_PROFILES_DIR=/contracts/packaging/profiles

# Runtime environment this entrypoint reads (names only — no values, no
# secrets in this file per iron rule 11). Every process reads DATABASE_URL.
# api additionally reads: API_ADDR, METRICS_ADDR, OBJSTORE_* (or
# SKILLHUB_CLEAN_MODE for the in-process stand-in), LLM_SERVICE_URL,
# LLM_SERVICE_TOKEN, SKILLHUB_TRACE_INGEST_URL, SKILLHUB_TRACE_INGEST_SECRET,
# COOKIE_INSECURE, DEV_LOGIN, GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET,
# OAUTH_REDIRECT_URL, APP_URL, OPERATOR_USER_IDS, BETA_ALLOWLIST,
# DEV_CORS_ORIGIN, IMPORT_ALLOW_INSECURE, IMPORT_EXTRA_HOSTS,
# DOWNLOAD_ARTIFACT_RETENTION, RUN_QUOTA, GENERATE_QUOTA, RATE_LIMIT,
# GENERATE_SKILL_EXPOSED, FEEDBACK_RETENTION, ANALYTICS_RETENTION.
# worker additionally reads: METRICS_ADDR, OBJSTORE_*, LLM_SERVICE_URL,
# LLM_SERVICE_TOKEN, SKILLHUB_TRACE_INGEST_URL, SKILLHUB_TRACE_INGEST_SECRET,
# and refuses to start under SKILLHUB_CLEAN_MODE=1 (that flag is cmd/api-only).
# maintenance additionally reads (subcommand-dependent): AUDIT_RETENTION,
# FEEDBACK_RETENTION, TRACE_RETENTION, SKILL_DELETION_GRACE, OBJSTORE_*.
# reindex additionally reads: LLM_SERVICE_URL, REINDEX_BATCH, OBJSTORE_*.
# Full list and defaults: .env.example at the repo root.

# api and worker serve/scrape HTTP; the other two commands exit after one run.
EXPOSE 8080

# No ENTRYPOINT on purpose: distroless/static's PATH already covers
# /usr/local/bin, so `docker run <image>` runs `api` (this default) and
# `docker run <image> worker|maintenance|reindex ...` swaps in one of the
# other three by name — a shell-less dispatcher would need a shell this base
# does not have. `docker run <image> api --help` still works: CMD is replaced
# wholesale, not appended to.
CMD ["api"]
