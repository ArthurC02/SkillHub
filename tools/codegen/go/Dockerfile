# syntax=docker/dockerfile:1.7
FROM golang:1.27.0-bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go build -trimpath -o /usr/local/bin/ogen github.com/ogen-go/ogen/cmd/ogen
ENTRYPOINT ["/usr/local/bin/ogen"]
