# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS build
WORKDIR /src
COPY apps/sandbox/go.mod apps/sandbox/go.sum ./apps/sandbox/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd apps/sandbox && go mod download
COPY apps/sandbox ./apps/sandbox
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd apps/sandbox && mkdir -p /out \
    && go build -ldflags="-s -w" -o /out/sandboxd ./cmd/sandboxd

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/sandboxd /usr/local/bin/sandboxd
ENTRYPOINT ["/usr/local/bin/sandboxd"]
