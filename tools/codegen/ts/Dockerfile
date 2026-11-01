# syntax=docker/dockerfile:1.7
FROM rust:1.98-bookworm@sha256:93ce27a88655056a51dbdd8f5f2d7ddc071c7b0070fb288a37b5a285fc83971e AS build
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release --locked && cp target/release/openapi-ts /usr/local/bin/openapi-ts
ENTRYPOINT ["/usr/local/bin/openapi-ts"]
