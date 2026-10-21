# syntax=docker/dockerfile:1.7
FROM golang:1.27.3-bookworm@sha256:5c3e9b43cfd3bb95c7937462e29d87ada57b481a96fa23ad0eb61778d7a00110 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go build -trimpath -o /usr/local/bin/ogen github.com/ogen-go/ogen/cmd/ogen
ENTRYPOINT ["/usr/local/bin/ogen"]
