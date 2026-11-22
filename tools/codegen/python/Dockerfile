# syntax=docker/dockerfile:1.7
FROM ghcr.io/astral-sh/uv:0.13.9@sha256:d3b601f0c7b118bc1753febf7c41cc80981df678445e0254284eecf84db79a7d AS uv
FROM python:3.15.2-slim-bookworm@sha256:96599df822d0e1c6bb700014a05c7f3c8cabc6e0f7ff3c4f9f69cee033b4b5d9

ENV UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_COMPILE_BYTECODE=1

COPY --from=uv /uv /usr/local/bin/uv
WORKDIR /opt/codegen
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-install-project

ENTRYPOINT ["/opt/codegen/.venv/bin/datamodel-codegen"]
