# syntax=docker/dockerfile:1.7

FROM python:3.14-slim-bookworm@sha256:9ab8d9c8514b44f90cf0029dd42fdd7e9e211e639c8b995304cc04568dee900f AS build

ARG UV_VERSION=0.12.13
ARG UV_INSTALLER_SHA256=e7265962d703f3ca66b3e90fae8fcdc6d4e4b587027e9479ccc29b25c3f68d4e
ENV UV_INSTALL_DIR=/usr/local/bin \
    UV_LINK_MODE=copy \
    UV_PROJECT_ENVIRONMENT=/opt/venv
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && curl -LsSf "https://astral.sh/uv/${UV_VERSION}/install.sh" -o /tmp/uv-install.sh \
    && echo "${UV_INSTALLER_SHA256}  /tmp/uv-install.sh" | sha256sum -c - \
    && sh /tmp/uv-install.sh \
    && rm /tmp/uv-install.sh

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

FROM python:3.14-slim-bookworm@sha256:9ab8d9c8514b44f90cf0029dd42fdd7e9e211e639c8b995304cc04568dee900f AS runtime

RUN apt-get update \
    # Upgrades this one package to pick up a fixed CVE without floating the
    # pinned base digest or doing a drifting whole-image apt-get upgrade.
    && apt-get install -y --no-install-recommends --only-upgrade libpcre2-8-0 \
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
