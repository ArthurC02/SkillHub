# Skill Hub Runtime Image — Claude Agent SDK on Node.js 22 LTS (SBX-002).
#
# PDM-003 picked the SDK over the CLI and over a hand-written agent loop, and
# ADR-015's decision record records Node.js 22 LTS with it. The image is
# deliberately small and boring: a reviewer has to be able to read it and say
# what runs inside a sandbox, because everything it executes afterwards is
# untrusted (iron rule 1).
#
# Baseline notes (SEC-002 §4.5):
#   I-01  Pre-built and versioned. The provider names the image; a RunRequest
#         cannot choose one.
#   I-02  Pin by digest at deploy time. The FROM tag below is resolved to a
#         digest by the publishing pipeline, and SKILLHUB_SANDBOX_IMAGE should
#         be a digest reference, never a tag — a tag is a moving target and the
#         run record would stop saying what actually ran.
#   I-03/I-04  SBOM and vulnerability scanning are publish-time gates and are
#         not wired yet; until they are, this image is not publishable and
#         SBX-002 stays unchecked.
#
# Build:
#   docker build -t skillhub/runtime-agent-sdk:2026.08-1 infra/images/runtime-agent-sdk
FROM node:22-bookworm-slim

# The SDK version is pinned, not floated: it is the runtime_version the
# provider declares in ProviderCapability and the one recorded against every
# historical Run (I-05). Bumping it is a new image tag, never an in-place edit.
ARG CLAUDE_AGENT_SDK_VERSION=0.3.233

# Non-root from the image on up (C-02). The provider also passes --user, but an
# image whose only account is root is one bad flag away from running as root.
RUN groupadd --gid 65532 sandbox \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin sandbox

WORKDIR /opt/skillhub
RUN npm install --omit=dev --no-audit --no-fund \
    "@anthropic-ai/claude-agent-sdk@${CLAUDE_AGENT_SDK_VERSION}" \
    && npm cache clean --force
COPY run.mjs /opt/skillhub/run.mjs

# Recorded so a Run's own report can state what it ran under without asking the
# registry.
ENV SKILLHUB_RUNTIME=claude_agent_sdk \
    SKILLHUB_RUNTIME_VERSION=${CLAUDE_AGENT_SDK_VERSION} \
    NODE_ENV=production \
    NPM_CONFIG_UPDATE_NOTIFIER=false

# /work and /out are mounted by the provider as per-run tmpfs, owned by this
# uid. Nothing here writes outside them: the rootfs is read-only at run time.
USER 65532:65532
WORKDIR /work
ENTRYPOINT ["node", "/opt/skillhub/run.mjs"]
