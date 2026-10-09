# syntax=docker/dockerfile:1.7
FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go build -trimpath -o /usr/local/bin/ogen github.com/ogen-go/ogen/cmd/ogen
ENTRYPOINT ["/usr/local/bin/ogen"]
