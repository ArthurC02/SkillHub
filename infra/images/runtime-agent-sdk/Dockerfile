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
#         (.github/workflows/runtime-image.yml), and are published as
#         attestations on the GHCR digest so node admission can query them
#         (SBX-011). Thresholds are settled in ADR-022 §2.
#
# Build:
#   docker build -t skillhub/runtime-agent-sdk:2026.08-3 infra/images/runtime-agent-sdk
FROM node:22-bookworm-slim@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436

# The SDK version is pinned, not floated: it is the runtime_version the
# provider declares in ProviderCapability and the one recorded against every
# historical Run (I-05). Bumping it is a new image tag, never an in-place edit.
ARG CLAUDE_AGENT_SDK_VERSION=0.3.233

# The human-readable version of this image. Contents change => new version, never
# an in-place edit of an existing one (I-05, README "Digest 更新程序"). The
# runtime-image workflow reads it straight out of this file so the tag it pushes
# to GHCR and the tag written here cannot drift apart.
# 2026.08-3 closes the gap between the catalog's `deps` field and what the
# packages actually import (content-baseline-report.md §13.4).
ARG IMAGE_VERSION=2026.08-3

# Non-root from the image on up (C-02). The provider also passes --user, but an
# image whose only account is root is one bad flag away from running as root.
RUN groupadd --gid 65532 sandbox \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin sandbox

# Skill packages are stored as zip archives (ingest writes packages/<hash>.zip),
# and the sandbox is where they are unpacked — never the control plane, which
# must not open untrusted bytes (iron rule 1). node:22-bookworm-slim carries tar
# but not unzip, so unzip is added here.
#
# python3 is the second addition, and it is not a convenience: 33 of the 45
# catalog skills declare their worked examples in Python, and the M2 baseline
# runs recorded `python3: command not found` in the sandbox. What ran instead
# was the model's re-implementation of the script, not the script — a different
# product with no field in the catalog saying so
# (plans/mvp/m2/content-baseline-report.md §6.3).
RUN apt-get update \
    && apt-get install -y --no-install-recommends unzip python3 python3-pip \
    && rm -rf /var/lib/apt/lists/* \
    # Skills write `python foo.py` as often as `python3 foo.py`; Debian ships
    # only the versioned name. One symlink, or a whole class of skills fails on
    # the interpreter name rather than on anything real.
    && ln -s /usr/bin/python3 /usr/local/bin/python

# The sandbox has no network (SBX-007: the only reachable address is the model
# gateway), so a dependency that is not baked in here cannot be installed at run
# time — pre-installing is the only path that exists.
#
# The set is the union of `deps` declared by the 45 catalog skills in
# tools/content/seed-skills.json, minus what cannot go in. "The catalog declared
# it" is a rule that can be re-derived from data; "it seemed useful" is not, and
# would grow this image forever.
#
# 2026.08-2 read that union off a `deps` field that had been transcribed from an
# incomplete read of each SKILL.md: 13 of the 45 skills under-declared, and the
# image inherited every gap. The field is now corrected against a static scan of
# the pinned package trees, which added 8 installable packages to the union and
# also — this is what forced the second half of the rule — a set of things that
# must not be installed at all:
#
#   ADMISSION BAR: pure-Python wheel, no compiled extension, no system binary.
#
# A sandbox exists to parse untrusted data. Native parsers are the memory-unsafe
# surface that isolation is compensating for, so pulling a rendering or OCR stack
# in to satisfy a `deps` entry trades away the thing the sandbox is for. The bar
# is a property of the package, not a judgement call per package, and every
# rejection it produces is named with its reason in infra/images/README.md —
# a rejected dependency is a disclosed limitation, never a silent one.
#
# Each direct dependency is pinned to the newest release the I-06 gate accepts:
# the first pass pinned a conservative older set and grype found fixable High
# findings in lxml, pypdf and pdfminer.six, which have no waiver path (ADR-022).
# Transitive dependencies are left to the resolver — the image digest and the
# SBOM published beside it (SBX-011) are what make a build reproducible and
# auditable, and a hand-maintained lock file would be a second, drifting answer
# to a question the SBOM already answers exactly.
#
# pip goes the way npm went below — a package manager inside a sandbox that runs
# untrusted code is a liability, and with no network it cannot even do its job.
RUN pip3 install --break-system-packages --no-cache-dir \
      pandas==3.0.5 \
      numpy==2.4.6 \
      openpyxl==3.1.5 \
      lxml==6.1.1 \
      python-dateutil==2.9.0.post0 \
      python-docx==1.2.0 \
      python-pptx==1.0.2 \
      pypdf==6.16.1 \
      pdfplumber==0.11.10 \
      pycountry==26.2.16 \
      chardet==7.6.0 \
      defusedxml==0.7.1 \
      ftfy==6.3.1 \
      confusable-homoglyphs==3.3.1 \
      pytz==2026.3.post1 \
      phonenumbers==9.0.37 \
      python-stdnum==2.2 \
    && apt-get purge -y python3-pip \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/* /root/.cache

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
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    # The rootfs is read-only at run time, so every .pyc write would fail and be
    # retried on the next import. The bytecode baked in at build time is still
    # used; this only stops the pointless attempts.
    PYTHONDONTWRITEBYTECODE=1

LABEL org.opencontainers.image.version="${IMAGE_VERSION}" \
      org.opencontainers.image.title="skillhub-runtime-agent-sdk" \
      org.opencontainers.image.source="https://github.com/ArthurC02/skillhub"

# /work and /out are mounted by the provider as per-run tmpfs, owned by this
# uid. Nothing here writes outside them: the rootfs is read-only at run time.
USER 65532:65532
WORKDIR /work
ENTRYPOINT ["node", "/opt/skillhub/run.mjs"]
