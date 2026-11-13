# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

ARG EXPORTER_VERSION=0.20.1
ARG EXPORTER_COMMIT=867fbcac31cd18c143e244190ea9168cca069827
ARG EXPORTER_SUM=h1:MahiuLbx8SHpGu9I96O93wdA7GBhU3PcsMUeLg6EsqU=
WORKDIR /build
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod init skillhub.internal/postgres-exporter-build \
    && go get github.com/prometheus-community/postgres_exporter/cmd/postgres_exporter@v$EXPORTER_VERSION \
        golang.org/x/net@v0.62.0 golang.org/x/crypto@v0.59.0 golang.org/x/text@v0.44.0 golang.org/x/sys@v0.50.0 \
    && go mod download -json github.com/prometheus-community/postgres_exporter@v$EXPORTER_VERSION > /tmp/source.json \
    && grep -Fq '"Hash": "'"$EXPORTER_COMMIT"'"' /tmp/source.json \
    && grep -Fq '"Sum": "'"$EXPORTER_SUM"'"' /tmp/source.json \
    && go test -mod=mod -v -count=1 -p 2 github.com/prometheus-community/postgres_exporter/... \
    && CGO_ENABLED=0 go build -mod=readonly -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/prometheus/common/version.Version=$EXPORTER_VERSION -X github.com/prometheus/common/version.Revision=$EXPORTER_COMMIT" \
        -o /out/postgres_exporter github.com/prometheus-community/postgres_exporter/cmd/postgres_exporter

FROM prometheuscommunity/postgres-exporter:v0.20.1@sha256:ac5ec343104fae0e2d84a27bb8d69b38430a11910c5382cad85d478d2bab713e

COPY --from=builder /out/postgres_exporter /bin/postgres_exporter
