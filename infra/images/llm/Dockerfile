# syntax=docker/dockerfile:1.7

FROM ghcr.io/astral-sh/uv:0.13.7@sha256:96f40d3904aad8d0bd010539001d36c8e2188b981e1334471a90c4a1d93bb490 AS uv
FROM python:3.14.10-slim-bookworm@sha256:516b51cb9db2ef061e465c803e50f64361b82cefd91e525b889b967b83e83527 AS build

ENV UV_LINK_MODE=copy \
    UV_PROJECT_ENVIRONMENT=/opt/venv
COPY --from=uv /uv /usr/local/bin/uv

WORKDIR /src
# Lockfile-only layer first so an application-code-only change does not redo
# dependency resolution/download.
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project
COPY src ./src
COPY README.md ./
# --no-editable: a plain `uv sync` installs this project as an editable .pth
# pointing back at /src, which only resolves inside this build stage.
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable

FROM python:3.14.10-slim-bookworm@sha256:516b51cb9db2ef061e465c803e50f64361b82cefd91e525b889b967b83e83527 AS runtime

RUN apt-get update \
    # Upgrades this one package to pick up a fixed CVE without floating the
    # pinned base digest or doing a drifting whole-image apt-get upgrade.
    && apt-get install -y --no-install-recommends --only-upgrade libpcre2-8-0 openssl libssl3 perl-base apt libapt-pkg6.0 \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --gid 65532 skillhub \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin skillhub

COPY --from=build /opt/venv /opt/venv
WORKDIR /app

ENV PATH=/opt/venv/bin:$PATH \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

USER 65532:65532

EXPOSE 8000

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/healthz', timeout=2)"]

CMD ["uvicorn", "skillhub_llm.app:app", "--host", "0.0.0.0", "--port", "8000"]
