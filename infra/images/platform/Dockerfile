# syntax=docker/dockerfile:1.7

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

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime

COPY --from=build /out/api /out/worker /out/maintenance /out/reindex /usr/local/bin/

COPY contracts/packaging/profiles /contracts/packaging/profiles
ENV PACKAGING_PROFILES_DIR=/contracts/packaging/profiles

EXPOSE 8080

# No ENTRYPOINT: distroless/static's PATH covers /usr/local/bin, so
# `docker run <image>` runs `api` and `docker run <image> worker|...` swaps in
# another binary by name (CMD is replaced wholesale, not appended to).
CMD ["api"]
