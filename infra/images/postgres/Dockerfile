# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS wal-g

ARG WALG_COMMIT=3e493188db28335fb93a45bd680c6ff8fc3028c1
WORKDIR /src
RUN git init -q . \
    && git fetch -q --depth 1 https://github.com/wal-g/wal-g "$WALG_COMMIT" \
    && git checkout -q FETCH_HEAD
RUN --mount=type=cache,target=/go/pkg/mod \
    go get google.golang.org/grpc@v1.83.2 golang.org/x/crypto@v0.59.0 golang.org/x/net@v0.62.0 \
        go.opentelemetry.io/otel/sdk@v1.45.0 github.com/go-jose/go-jose/v4@v4.1.6 github.com/klauspost/compress@v1.20.2 \
    && go mod tidy \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/wal-g ./main/pg

ARG GOSU_COMMIT=6456aaa0f3c854d199d0f037f068eb97515b7513
WORKDIR /gosu
RUN git init -q .     && git fetch -q --depth 1 https://github.com/tianon/gosu "$GOSU_COMMIT"     && git checkout -q FETCH_HEAD
RUN --mount=type=cache,target=/go/pkg/mod \
    go get github.com/moby/sys/user@v0.4.1 golang.org/x/sys@v0.44.0 \
    && go mod tidy \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/gosu .

FROM pgvector/pgvector:pg18@sha256:c29f30688a9ad961691bdf0ec1650717cc132a5814619d80f80463d6e4c42e47

RUN apt-get update \
    && apt-get install -y --no-install-recommends --only-upgrade libpcre2-8-0 openssl libssl3 \
        perl-base perl libperl5.36 perl-modules-5.36 libsasl2-2 libsasl2-modules-db apt libapt-pkg6.0 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=wal-g /out/wal-g /out/gosu /usr/local/bin/
RUN wal-g --version && gosu --version && gosu postgres true
COPY --chmod=0755 skillhub-backup skillhub-restore-drill /usr/local/bin/
