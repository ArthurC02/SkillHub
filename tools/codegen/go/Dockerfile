# syntax=docker/dockerfile:1.7
FROM golang:1.25.0-bookworm@sha256:81dc45d05a7444ead8c92a389621fafabc8e40f8fd1a19d7e5df14e61e98bc1a AS build
WORKDIR /src
COPY tools/codegen/go/go.mod tools/codegen/go/go.sum ./
RUN go mod download
RUN go build -trimpath -o /usr/local/bin/ogen github.com/ogen-go/ogen/cmd/ogen
ENTRYPOINT ["/usr/local/bin/ogen"]
