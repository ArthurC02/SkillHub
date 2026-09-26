# syntax=docker/dockerfile:1.7
FROM ghcr.io/astral-sh/uv:0.12.15@sha256:62f8c047d0a0e9ece6b53fc63df902585a67a47a7f318ddec4a37db586edc8e3 AS uv
FROM python:3.14.7-slim-bookworm@sha256:9ab8d9c8514b44f90cf0029dd42fdd7e9e211e639c8b995304cc04568dee900f

ENV UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_COMPILE_BYTECODE=1

COPY --from=uv /uv /usr/local/bin/uv
WORKDIR /opt/codegen
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-install-project

ENTRYPOINT ["/opt/codegen/.venv/bin/datamodel-codegen"]
