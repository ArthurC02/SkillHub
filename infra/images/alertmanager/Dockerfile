# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

ARG ALERTMANAGER_VERSION=0.34.2
ARG ALERTMANAGER_COMMIT=e8349048265ccc0871c3d0d668b05eacc68baf50
ARG ALERTMANAGER_SUM=h1:4oOnHs4J41X9n+7MucFjA3I5i5MtasjWTnIY27q5Qkw=
ARG ALERTMANAGER_UI_SHA=3d6b13c5af094724e0bed8f757399e08af44d40d6c6cd87392a1f8ee6ed1e457
WORKDIR /build
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod init skillhub.internal/alertmanager-build \
    && go get github.com/prometheus/alertmanager@v$ALERTMANAGER_VERSION \
        golang.org/x/net@v0.62.0 github.com/klauspost/compress@v1.20.2 \
    && go mod download -json github.com/prometheus/alertmanager@v$ALERTMANAGER_VERSION > /tmp/source.json \
    && grep -Fq '"Hash": "'"$ALERTMANAGER_COMMIT"'"' /tmp/source.json \
    && grep -Fq '"Sum": "'"$ALERTMANAGER_SUM"'"' /tmp/source.json \
    && go mod verify \
    && curl -fLsS "https://github.com/prometheus/alertmanager/releases/download/v$ALERTMANAGER_VERSION/alertmanager-web-ui-$ALERTMANAGER_VERSION.tar.gz" -o /tmp/ui.tar.gz \
    && printf '%s  /tmp/ui.tar.gz\n' "$ALERTMANAGER_UI_SHA" | sha256sum -c - \
    && source_dir="$(go list -m -f '{{.Dir}}' github.com/prometheus/alertmanager)" \
    && test ! -e "$source_dir/ui/app/dist" \
    && test ! -e "$source_dir/alertmanager" \
    && test ! -e "$source_dir/amtool" \
    && trap 'rm -f "$source_dir/alertmanager" "$source_dir/amtool"; rm -rf "$source_dir/ui/app/dist"' EXIT \
    && tar --no-same-owner -xzf /tmp/ui.tar.gz -C "$source_dir/ui/app" \
    && CGO_ENABLED=0 go build -mod=mod -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/prometheus/common/version.Version=$ALERTMANAGER_VERSION -X github.com/prometheus/common/version.Revision=$ALERTMANAGER_COMMIT" \
        -o /out/alertmanager github.com/prometheus/alertmanager/cmd/alertmanager \
    && CGO_ENABLED=0 go build -mod=mod -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/prometheus/common/version.Version=$ALERTMANAGER_VERSION -X github.com/prometheus/common/version.Revision=$ALERTMANAGER_COMMIT" \
        -o /out/amtool github.com/prometheus/alertmanager/cmd/amtool \
    && ln -s /out/alertmanager "$source_dir/alertmanager" \
    && ln -s /out/amtool "$source_dir/amtool" \
    && go test -mod=mod -v -count=1 -p 2 github.com/prometheus/alertmanager/... \
    && rm "$source_dir/alertmanager" "$source_dir/amtool" \
    && rm -rf "$source_dir/ui/app/dist" \
    && trap - EXIT \
    && go mod verify

FROM prom/alertmanager:v0.34.2@sha256:39d8bb3966c694a124428a3a88fbcffd40bd069129ecd47c059f8a3f17c55f3b

COPY --from=builder /out/alertmanager /bin/alertmanager
COPY --from=builder /out/amtool /bin/amtool
