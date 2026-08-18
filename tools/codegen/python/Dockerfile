# syntax=docker/dockerfile:1.7
FROM ghcr.io/astral-sh/uv:0.11.3@sha256:90bbb3c16635e9627f49eec6539f956d70746c409209041800a0280b93152823 AS uv
FROM python:3.12.13-slim-bookworm@sha256:4766d8b510c428e595d74b9cc5bbb2fae8e26316fffb4adc89908d79aacd58a2

ENV UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_COMPILE_BYTECODE=1

COPY --from=uv /uv /usr/local/bin/uv
WORKDIR /opt/codegen
COPY tools/codegen/python/pyproject.toml tools/codegen/python/uv.lock ./
RUN uv sync --frozen --no-install-project

ENTRYPOINT ["/opt/codegen/.venv/bin/datamodel-codegen"]
