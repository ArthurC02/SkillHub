# syntax=docker/dockerfile:1.7

FROM golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b AS wal-g

ARG WALG_COMMIT=3e493188db28335fb93a45bd680c6ff8fc3028c1
WORKDIR /src
RUN git init -q . \
    && git fetch -q --depth 1 https://github.com/wal-g/wal-g "$WALG_COMMIT" \
    && git checkout -q FETCH_HEAD
RUN --mount=type=cache,target=/go/pkg/mod \
    go get google.golang.org/grpc@v1.83.2 golang.org/x/crypto@v0.56.0 \
    && go mod tidy \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/wal-g ./main/pg

ARG GOSU_COMMIT=6456aaa0f3c854d199d0f037f068eb97515b7513
WORKDIR /gosu
RUN git init -q .     && git fetch -q --depth 1 https://github.com/tianon/gosu "$GOSU_COMMIT"     && git checkout -q FETCH_HEAD
RUN --mount=type=cache,target=/go/pkg/mod     CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/gosu .

FROM pgvector/pgvector:pg18@sha256:2ba9ca5f2e7daa0f0e7723cba1ee9167bab54efd3640516a44ac1a928dd67e7a

RUN apt-get update \
    && apt-get install -y --no-install-recommends --only-upgrade libpcre2-8-0 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=wal-g /out/wal-g /out/gosu /usr/local/bin/
RUN wal-g --version && gosu --version && gosu postgres true
COPY --chmod=0755 skillhub-backup skillhub-restore-drill /usr/local/bin/
