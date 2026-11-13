# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

ENV GOWORK=off

ARG PROMETHEUS_VERSION=3.16.0
ARG PROMETHEUS_MODULE_VERSION=0.316.0
ARG PROMETHEUS_COMMIT=e2e28adeee2c5883d000fc82aca6008cfbf7e96d
ARG PROMETHEUS_SUM=h1:f1GRFerhTMKQma+awCoC9T820ecWeGSdF+xAlMHzZzM=
ARG PROMETHEUS_UI_SHA=000a45231f6dd22a5703218aad3ee11da125c8a44d1824a27670416c07531001
WORKDIR /build
COPY summarize-tests.py /tmp/summarize-tests.py
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    python3 /tmp/summarize-tests.py --self-test \
    && go mod init skillhub.internal/prometheus-build \
    && go mod edit -require=github.com/prometheus/prometheus@v$PROMETHEUS_MODULE_VERSION \
    && go mod download -json github.com/prometheus/prometheus@v$PROMETHEUS_MODULE_VERSION > /tmp/source.json \
    && grep -Fq '"Hash": "'"$PROMETHEUS_COMMIT"'"' /tmp/source.json \
    && grep -Fq '"Sum": "'"$PROMETHEUS_SUM"'"' /tmp/source.json \
    && go mod verify \
    && source_dir="$(go list -m -f '{{.Dir}}' github.com/prometheus/prometheus)" \
    && test ! -e "$source_dir/web/ui/static" \
    && test ! -e "$source_dir/web/ui/embed.go" \
    && trap 'rm -f "$source_dir/web/ui/embed.go"; rm -rf "$source_dir/web/ui/static"' EXIT \
    && curl -fLsS "https://github.com/prometheus/prometheus/releases/download/v$PROMETHEUS_VERSION/prometheus-web-ui-$PROMETHEUS_VERSION.tar.gz" -o /tmp/ui.tar.gz \
    && printf '%s  /tmp/ui.tar.gz\n' "$PROMETHEUS_UI_SHA" | sha256sum -c - \
    && tar --no-same-owner -xzf /tmp/ui.tar.gz -C "$source_dir/web/ui" \
    && (cd "$source_dir" && bash scripts/compress_assets.sh) \
    && test -s "$source_dir/web/ui/embed.go" \
    && go get github.com/prometheus/prometheus@v$PROMETHEUS_MODULE_VERSION golang.org/x/net@v0.62.0 \
    && CGO_ENABLED=0 go build -mod=mod -tags=netgo,builtinassets -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/prometheus/common/version.Version=$PROMETHEUS_VERSION -X github.com/prometheus/common/version.Revision=$PROMETHEUS_COMMIT -X github.com/prometheus/common/version.Branch=HEAD" \
        -o /out/prometheus github.com/prometheus/prometheus/cmd/prometheus \
    && CGO_ENABLED=0 go build -mod=mod -tags=netgo,builtinassets -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/prometheus/common/version.Version=$PROMETHEUS_VERSION -X github.com/prometheus/common/version.Revision=$PROMETHEUS_COMMIT -X github.com/prometheus/common/version.Branch=HEAD" \
        -o /out/promtool github.com/prometheus/prometheus/cmd/promtool \
    && cp -a "$source_dir" /tmp/prometheus-tests \
    && chmod -R u+w /tmp/prometheus-tests \
    && (cd /tmp/prometheus-tests \
        && go get golang.org/x/net@v0.62.0 \
        && { status=0; env -u OTEL_EXPORTER_OTLP_TRACES_ENDPOINT go test -mod=mod -tags=netgo,builtinassets -json -count=1 -p 2 ./... >/tmp/test-results.jsonl 2>/tmp/test-stderr.log || status=$?; \
            cat /tmp/test-stderr.log >&2; python3 /tmp/summarize-tests.py /tmp/test-results.jsonl "$status"; }) \
    && rm "$source_dir/web/ui/embed.go" \
    && rm -rf "$source_dir/web/ui/static" \
    && trap - EXIT \
    && go mod verify

FROM prom/prometheus:v3.16.0@sha256:e04987641b57704f408e31f6373ba4138ecf27cb4a165d24aabb6c38fddd0f06

COPY --from=builder /out/prometheus /bin/prometheus
COPY --from=builder /out/promtool /bin/promtool
