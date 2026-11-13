# syntax=docker/dockerfile:1.7
FROM redis:8.2.11-bookworm@sha256:b0b01167ff47bc3868e56a967895a26be4230891a1f42a78ab61cdd227accc8d AS redis
FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS tests
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
WORKDIR /build
COPY infra/images/prometheus/summarize-tests.py /tmp/summarize-tests.py
RUN python3 /tmp/summarize-tests.py --self-test
COPY --from=redis /usr/local/bin/redis-server /usr/local/bin/redis-cli /usr/local/bin/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod init skillhub.internal/seaweedfs-build \
    && go mod edit -replace=github.com/tyler-smith/go-bip39=github.com/cosmos/go-bip39@v1.0.0 \
    && go get github.com/seaweedfs/seaweedfs/weed@v0.0.0-20261102112406-984eac6a790b \
        golang.org/x/net@v0.62.0 github.com/apache/thrift@v0.25.0 \
    && go mod download -json github.com/seaweedfs/seaweedfs@v0.0.0-20261102112406-984eac6a790b > /tmp/source.json \
    && grep -Fq '"Hash": "984eac6a790be89e2a11de175a7413e1e3e32dd3"' /tmp/source.json \
    && grep -Fq '"Sum": "h1:zIGjbVHBYe0zsKjRfN0QVjR8pIlHHpJ8aMS/Iorqp9I="' /tmp/source.json
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go mod download && go mod verify \
    && source_dir="$(go list -m -f '{{.Dir}}' github.com/seaweedfs/seaweedfs)" \
    && cp -a "$source_dir" /tmp/seaweed-tests && chmod -R u+w /tmp/seaweed-tests \
    && redis-server --version && redis-cli --version \
    && redis-server --daemonize yes --bind 127.0.0.1 --dir /tmp --logfile /tmp/redis.log \
    && trap 'redis-cli shutdown nosave || true' EXIT \
    && for attempt in {1..100}; do if redis-cli ping 2>/dev/null | grep -qx PONG; then break; fi; sleep 0.1; done \
    && test "$(redis-cli ping)" = PONG \
    && (cd /tmp/seaweed-tests \
        && go get golang.org/x/net@v0.62.0 github.com/apache/thrift@v0.25.0 \
        && { status=0; RUN_REDIS_TESTS=1 go test -mod=mod -tags 'elastic gocdk sqlite ydb tarantool tikv rclone' -json -count=1 -p 2 ./weed/... >/tmp/upstream-tests.jsonl || status=$?; \
             printf '%s\n' "$status" >/tmp/upstream-tests.exit; })
FROM tests AS builder
RUN python3 /tmp/summarize-tests.py /tmp/upstream-tests.jsonl "$(cat /tmp/upstream-tests.exit)"
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go mod download && go mod verify && CGO_ENABLED=0 go build -mod=readonly -p 2 -trimpath -buildvcs=false \
        -ldflags="-s -w -X github.com/seaweedfs/seaweedfs/weed/util/version.COMMIT=984eac6a7" \
        -o /out/weed github.com/seaweedfs/seaweedfs/weed
FROM chrislusf/seaweedfs:4.52@sha256:57877e32bb90073b37dc109b438ba66e84fd5ed9ced5f5c04f56444ef243ce59
RUN apk add --no-cache --upgrade 'zlib>=1.4-r0'
COPY --from=builder /out/weed /usr/bin/weed
