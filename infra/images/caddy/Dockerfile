# syntax=docker/dockerfile:1.7

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

ARG CADDY_VERSION=2.11.9
ARG CADDY_COMMIT=0d98caea58f794f88e709b1bef2880996a471bca
ARG CADDY_SUM=h1:bhZtcRKQOq0fuLvEgCdPYvUxXyGn9kzjG4lh2332DTE=
WORKDIR /build
COPY listener_fixture.go prepare-tests.py /fixture/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod init skillhub.internal/caddy-build \
    && go get github.com/caddyserver/caddy/v2/cmd/caddy@v$CADDY_VERSION \
        golang.org/x/net@v0.62.0 github.com/klauspost/compress@v1.20.2 \
    && go mod download -json github.com/caddyserver/caddy/v2@v$CADDY_VERSION > /tmp/source.json \
    && grep -Fq '"Hash": "'"$CADDY_COMMIT"'"' /tmp/source.json \
    && grep -Fq '"Sum": "'"$CADDY_SUM"'"' /tmp/source.json \
    && source_dir="$(go list -m -f '{{.Dir}}' github.com/caddyserver/caddy/v2)" \
    && cp -a "$source_dir" /tmp/caddy-tests \
    && chmod -R u+w /tmp/caddy-tests \
    && python3 /fixture/prepare-tests.py \
    && gofmt -w /tmp/caddy-tests/listener_fixture.go /tmp/caddy-tests/modules/caddyhttp/app.go /tmp/caddy-tests/caddytest/caddytest.go \
    && test_path=/tmp/caddy-tests/caddytest/integration/reverseproxy_test.go \
    && test "$(grep -Fc 'localhost example.com {' "$test_path")" -eq 1 \
    && openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 \
        -subj /CN=localhost -addext 'subjectAltName=DNS:localhost,DNS:example.com' \
        -keyout /tmp/sni-key.pem -out /tmp/sni-certificate.pem \
    && sed '/localhost example.com {/a\        tls /tmp/sni-certificate.pem /tmp/sni-key.pem' "$test_path" > /tmp/reverseproxy_test.go \
    && cp /tmp/reverseproxy_test.go "$test_path" \
    && (cd /tmp/caddy-tests \
        && go get golang.org/x/net@v0.62.0 github.com/klauspost/compress@v1.20.2 \
        && go test -mod=mod -v -count=1 -p 2 ./...) \
    && go mod verify \
    && cp "$source_dir/cmd/caddy/main.go" /build/main.go \
    && CGO_ENABLED=0 go build -mod=readonly -p 2 -trimpath -buildvcs=false -ldflags="-s -w" \
        -o /out/caddy .

FROM caddy:2.11.9@sha256:e40b94d0a43b5ff69cb20568d73f10c70703df7b0ffc2298eb3d723cdc9d0bd1

RUN apk add --no-cache --upgrade 'zlib>=1.4-r0'
COPY --from=builder /out/caddy /usr/bin/caddy
