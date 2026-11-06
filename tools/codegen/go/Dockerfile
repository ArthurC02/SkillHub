# syntax=docker/dockerfile:1.7
FROM golang:1.27.3-bookworm@sha256:5c3e9b43cfd3bb95c7937462e29d87ada57b481a96fa23ad0eb61778d7a00110 AS build
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
