# syntax=docker/dockerfile:1.7

FROM golang:1.27.3-bookworm@sha256:5c3e9b43cfd3bb95c7937462e29d87ada57b481a96fa23ad0eb61778d7a00110 AS wal-g

ARG WALG_COMMIT=3e493188db28335fb93a45bd680c6ff8fc3028c1
WORKDIR /src
RUN git init -q . \
    && git fetch -q --depth 1 https://github.com/wal-g/wal-g "$WALG_COMMIT" \
    && git checkout -q FETCH_HEAD
RUN --mount=type=cache,target=/go/pkg/mod \
    go get google.golang.org/grpc@v1.83.2 golang.org/x/crypto@v0.57.0 golang.org/x/net@v0.60.0 \
        go.opentelemetry.io/otel/sdk@v1.45.0 \
    && go mod tidy \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/wal-g ./main/pg

ARG GOSU_COMMIT=6456aaa0f3c854d199d0f037f068eb97515b7513
WORKDIR /gosu
RUN git init -q .     && git fetch -q --depth 1 https://github.com/tianon/gosu "$GOSU_COMMIT"     && git checkout -q FETCH_HEAD
RUN --mount=type=cache,target=/go/pkg/mod \
    go get github.com/moby/sys/user@v0.4.1 golang.org/x/sys@v0.44.0 \
    && go mod tidy \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/gosu .

FROM pgvector/pgvector:pg18@sha256:2358fcba361ed2233a5ed81b5fe4ca779ccb304120ce531a3bf51c0ed7e2bc11

RUN apt-get update \
    && apt-get install -y --no-install-recommends --only-upgrade libpcre2-8-0 openssl libssl3 \
        perl-base perl libperl5.36 perl-modules-5.36 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=wal-g /out/wal-g /out/gosu /usr/local/bin/
RUN wal-g --version && gosu --version && gosu postgres true
COPY --chmod=0755 skillhub-backup skillhub-restore-drill /usr/local/bin/
