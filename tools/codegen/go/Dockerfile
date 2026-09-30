# syntax=docker/dockerfile:1.7
FROM golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go build -trimpath -o /usr/local/bin/ogen github.com/ogen-go/ogen/cmd/ogen
ENTRYPOINT ["/usr/local/bin/ogen"]
