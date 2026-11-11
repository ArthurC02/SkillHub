# syntax=docker/dockerfile:1.7
FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath -o /out/ogen github.com/ogen-go/ogen/cmd/ogen

FROM gcr.io/distroless/static-debian12@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2
COPY --from=build /usr/local/go /usr/local/go
COPY --from=build /out/ogen /usr/local/bin/ogen
ENV PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
    GOTOOLCHAIN=local \
    GOPATH=/tmp/go \
    GOCACHE=/tmp/go-build
ENTRYPOINT ["/usr/local/bin/ogen"]
