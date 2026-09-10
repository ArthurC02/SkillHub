FROM node:22-bookworm-slim@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436

ARG CLAUDE_AGENT_SDK_VERSION=0.3.233

ARG IMAGE_VERSION=2026.08-9

# Non-root from the image on up: the provider also passes --user, but an image
# whose only account is root is one bad flag away from running as root.
RUN groupadd --gid 65532 sandbox \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin sandbox

# unzip is intentionally absent: run.mjs unpacks packages itself with node:zlib
# so the same parser (and its path-traversal refusals) runs on every platform
# this image's workload must also run on.
RUN apt-get update \
    # Upgrades this one package to pick up a fixed CVE without floating the
    # pinned base digest or doing a drifting whole-image apt-get upgrade.
    && apt-get install -y --no-install-recommends --only-upgrade libpcre2-8-0 \
    && apt-get install -y --no-install-recommends python3 python3-pip \
    && rm -rf /var/lib/apt/lists/* \
    # Skills invoke `python`, but Debian ships only the versioned `python3` name.
    && ln -s /usr/bin/python3 /usr/local/bin/python

# The sandbox has no network, so dependencies must be baked in here, limited
# to pure-Python wheels (no compiled extension, no system binary) since a
# native parser is memory-unsafe surface isolation is meant to remove.
COPY constraints.txt /tmp/constraints.txt
RUN pip3 install --break-system-packages --no-cache-dir -c /tmp/constraints.txt \
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
    && rm -rf /var/lib/apt/lists/* /root/.cache /tmp/constraints.txt

WORKDIR /opt/skillhub
# Fails the build if package.json's pin disagrees with ARG
# CLAUDE_AGENT_SDK_VERSION, rather than silently installing whichever npm ci
# picks up.
COPY package.json package-lock.json /opt/skillhub/
RUN node -e "const v=require('./package.json').dependencies['@anthropic-ai/claude-agent-sdk']; \
    if (v !== process.env.CLAUDE_AGENT_SDK_VERSION) { \
      console.error('package.json pins '+v+' but ARG CLAUDE_AGENT_SDK_VERSION='+process.env.CLAUDE_AGENT_SDK_VERSION); \
      process.exit(1); \
    } console.log('package.json matches ARG CLAUDE_AGENT_SDK_VERSION:', v);"
RUN npm ci --omit=dev --no-audit --no-fund \
    && npm cache clean --force \
    # npm and corepack are build-time only and dropped afterward: a package
    # manager inside a sandbox that runs untrusted code is a liability even
    # when its own dependencies are clean.
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
