# syntax=docker/dockerfile:1.7
FROM ghcr.io/astral-sh/uv:0.13.7@sha256:96f40d3904aad8d0bd010539001d36c8e2188b981e1334471a90c4a1d93bb490 AS uv
FROM python:3.14.7-slim-bookworm@sha256:82bc3c539b8813ada9d68c63b40158fa002f7f33de9bf3312a3dfdc0620dff56

ENV UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_COMPILE_BYTECODE=1

COPY --from=uv /uv /usr/local/bin/uv
WORKDIR /opt/codegen
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-install-project

ENTRYPOINT ["/opt/codegen/.venv/bin/datamodel-codegen"]
