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
#   I-02  Pin by digest. The FROM below carries both tag and digest: the tag is
#         for a human reading the file, the digest is what actually resolves.
#         SKILLHUB_SANDBOX_IMAGE should likewise be a digest reference, never a
#         tag — a tag is a moving target and the run record would stop saying
#         what actually ran. Digest update procedure: infra/images/README.md.
#   I-03/I-04  SBOM and vulnerability scanning run in the `runtime-image` CI job
#         (.github/workflows/runtime-image.yml). Thresholds are still a proposal
#         awaiting sign-off (SEC-002 Q18), so SBX-002 stays unchecked.
#
# Build:
#   docker build -t skillhub/runtime-agent-sdk:2026.08-1 infra/images/runtime-agent-sdk
FROM node:22-bookworm-slim@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436

# The SDK version is pinned, not floated: it is the runtime_version the
# provider declares in ProviderCapability and the one recorded against every
# historical Run (I-05). Bumping it is a new image tag, never an in-place edit.
ARG CLAUDE_AGENT_SDK_VERSION=0.3.233

# Non-root from the image on up (C-02). The provider also passes --user, but an
# image whose only account is root is one bad flag away from running as root.
RUN groupadd --gid 65532 sandbox \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin sandbox

# Skill packages are stored as zip archives (ingest writes packages/<hash>.zip),
# and the sandbox is where they are unpacked — never the control plane, which
# must not open untrusted bytes (iron rule 1). node:22-bookworm-slim carries tar
# but not unzip, so this is the one added package.
RUN apt-get update \
    && apt-get install -y --no-install-recommends unzip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/skillhub
RUN npm install --omit=dev --no-audit --no-fund \
    "@anthropic-ai/claude-agent-sdk@${CLAUDE_AGENT_SDK_VERSION}" \
    && npm cache clean --force \
    # npm and corepack are build-time only: the entrypoint is `node run.mjs` and
    # the only address a sandbox can reach is the model gateway (SBX-007), so
    # nothing here can install a package anyway.
    # They are dropped because every fixable Critical/High the scan finds in this
    # image comes from npm's own bundled node_modules (tar, brace-expansion,
    # picomatch, sigstore) — packages nothing at run time loads. A package
    # manager sitting inside a sandbox that runs untrusted code is a liability
    # even when its dependencies happen to be clean.
    && rm -rf /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/corepack \
       /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack
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
