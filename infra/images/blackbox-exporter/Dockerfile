# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

ARG EXPORTER_VERSION=0.28.0
ARG EXPORTER_COMMIT=5a059bee8d8ffa4e75947c5055fb0abeefc582e6
ARG EXPORTER_SUM=h1:5n59xN6d4OtnwANA9nQIoBhwfG90b50cMByshvc/+zA=
WORKDIR /build
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod init skillhub.internal/blackbox-exporter-build \
    && go get github.com/prometheus/blackbox_exporter@v$EXPORTER_VERSION \
        golang.org/x/net@v0.62.0 golang.org/x/crypto@v0.59.0 golang.org/x/text@v0.44.0 golang.org/x/sys@v0.50.0 \
        google.golang.org/grpc@v1.83.2 github.com/andybalholm/brotli@v1.2.7 \
    && go mod download -json github.com/prometheus/blackbox_exporter@v$EXPORTER_VERSION > /tmp/source.json \
    && grep -Fq '"Hash": "'"$EXPORTER_COMMIT"'"' /tmp/source.json \
    && grep -Fq '"Sum": "'"$EXPORTER_SUM"'"' /tmp/source.json \
    && go test -mod=mod -v -count=1 -p 2 github.com/prometheus/blackbox_exporter/... \
    && CGO_ENABLED=0 go build -mod=readonly -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/prometheus/common/version.Version=$EXPORTER_VERSION -X github.com/prometheus/common/version.Revision=$EXPORTER_COMMIT" \
        -o /out/blackbox_exporter github.com/prometheus/blackbox_exporter

FROM prom/blackbox-exporter:v0.28.0@sha256:e753ff9f3fc458d02cca5eddab5a77e1c175eee484a8925ac7d524f04366c2fc

COPY --from=builder /out/blackbox_exporter /bin/blackbox_exporter
