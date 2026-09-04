# Skill Hub LLM service image (ADR-019 CI job 5 / 04 丙-158).
#
# apps/llm is a Python FastAPI service (ADR-016): the Go platform's only path
# to a model call (iron rule 6/8 — Python holds no business rules, and every
# call still goes through LiteLLM). This image runs it under uvicorn.
#
# Build context is apps/llm itself: the one local path dependency in
# pyproject.toml (`skillhub-api-stub`, packages/api-stub-py) lives only in the
# `dev` dependency group, and the runtime sync below is `--no-dev`.
#
# Build:
#   docker build -f infra/images/llm/Dockerfile -t skillhub/llm:local apps/llm

# syntax=docker/dockerfile:1.7

FROM python:3.12-slim-bookworm@sha256:782412e85d0f0984994c290652577d4018aff08145c85b262bb63dc0c7522254 AS build

# uv pinned to tools/toolchain.yaml's version, verified the same way
# infra/images/devtools/Dockerfile verifies it (installer script sha256, not
# TOFU on astral.sh at build time).
ARG UV_VERSION=0.11.3
ARG UV_INSTALLER_SHA256=b953b3f2a2764cbf860eee4578a5949fa90ed010644c6be1006f29010bada946
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
# --no-editable: plain `uv sync` installs the project itself as an editable
# .pth pointing back at /src/src, which only resolves inside this build
# stage. The runtime stage copies /opt/venv alone, so the package must land
# as real files in site-packages.
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-editable

# Runtime: same base family as the build stage (not distroless) because the
# venv this service runs from still needs a working `python3` on PATH plus its
# shared libs — slim is the minimal Debian base that provides that without
# reintroducing the full toolchain the build stage used to make the venv.
FROM python:3.12-slim-bookworm@sha256:782412e85d0f0984994c290652577d4018aff08145c85b262bb63dc0c7522254 AS runtime

RUN groupadd --gid 65532 skillhub \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin skillhub

COPY --from=build /opt/venv /opt/venv
WORKDIR /app

ENV PATH=/opt/venv/bin:$PATH \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

USER 65532:65532

# Runtime environment this entrypoint reads (names only — no values, no
# secrets in this file per iron rule 11): LLM_SERVICE_TOKEN (shared secret
# with apps/platform, checked on every route but /healthz),
# LITELLM_BASE_URL, LITELLM_API_KEY (a virtual key — never the LiteLLM master
# key, see .env.example), LITELLM_MASTER_KEY (only to refuse LITELLM_API_KEY
# if it is that value), and the per-endpoint model overrides ENRICH_MODEL,
# JUDGE_MODEL, GENERATE_SKILL_MODEL, MATCH_REASON_MODEL,
# SUGGEST_CRITERIA_MODEL (all optional; each has a code default).
EXPOSE 8000

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/healthz', timeout=2)"]

# contracts/openapi/llm-internal.yaml's `servers[0]` is http://llm:8000 —
# 0.0.0.0 so the container's published/compose-network address is that port,
# not just loopback inside the container.
CMD ["uvicorn", "skillhub_llm.app:app", "--host", "0.0.0.0", "--port", "8000"]
