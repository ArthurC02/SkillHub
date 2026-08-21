# syntax=docker/dockerfile:1.7
# Canonical developer environment for ADR-030. The multi-arch index digest is
# pinned so another computer cannot silently receive a different base image.
FROM golang:1.27.0-bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466

ARG NODE_VERSION=22.14.0
ARG TASK_VERSION=3.45.4
ARG UV_VERSION=0.11.3
ARG UV_INSTALLER_SHA256=b953b3f2a2764cbf860eee4578a5949fa90ed010644c6be1006f29010bada946
ARG GOLANGCI_LINT_VERSION=2.13.1
ARG DOCKER_VERSION=29.0.1
ARG COMPOSE_VERSION=2.40.3
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000

ENV DEBIAN_FRONTEND=noninteractive \
    GOBIN=/usr/local/bin \
    UV_INSTALL_DIR=/usr/local/bin \
    UV_LINK_MODE=copy \
    UV_PYTHON_INSTALL_DIR=/opt/uv/python

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git iptables sudo xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && arch="$(dpkg --print-architecture)" \
    && case "$arch" in amd64) node_arch=x64; docker_arch=x86_64 ;; arm64) node_arch=arm64; docker_arch=aarch64 ;; *) echo "unsupported architecture: $arch" >&2; exit 1 ;; esac \
    && curl -fsSLO "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" \
    && curl -fsSLO "https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt" \
    && grep " node-v${NODE_VERSION}-linux-${node_arch}.tar.xz$" SHASUMS256.txt | sha256sum -c - \
    && tar -xJf "node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" -C /usr/local --strip-components=1 \
    && rm "node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" SHASUMS256.txt \
    && curl -fsSLO "https://download.docker.com/linux/static/stable/${docker_arch}/docker-${DOCKER_VERSION}.tgz" \
    && tar -xzf "docker-${DOCKER_VERSION}.tgz" --strip-components=1 -C /usr/local/bin \
    && rm "docker-${DOCKER_VERSION}.tgz" \
    && mkdir -p /usr/local/lib/docker/cli-plugins \
    && curl -fsSL "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-${docker_arch}" -o /usr/local/lib/docker/cli-plugins/docker-compose \
    && chmod 0755 /usr/local/lib/docker/cli-plugins/docker-compose \
    && go install "github.com/go-task/task/v3/cmd/task@v${TASK_VERSION}" \
    && go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${GOLANGCI_LINT_VERSION}" \
    && curl -LsSf "https://astral.sh/uv/${UV_VERSION}/install.sh" -o /tmp/uv-install.sh \
    && echo "${UV_INSTALLER_SHA256}  /tmp/uv-install.sh" | sha256sum -c - \
    && sh /tmp/uv-install.sh \
    && rm /tmp/uv-install.sh \
    && uv python install 3.12 \
    && groupadd --gid "${USER_GID}" "${USERNAME}" \
    && useradd --uid "${USER_UID}" --gid "${USER_GID}" -m -s /bin/bash "${USERNAME}" \
    && groupadd --force docker \
    && usermod -aG docker "${USERNAME}" \
    && echo "${USERNAME} ALL=(root) NOPASSWD:ALL" > "/etc/sudoers.d/${USERNAME}" \
    && chmod 0440 "/etc/sudoers.d/${USERNAME}" \
    && printf '%s\n' 'export PATH=/usr/local/go/bin:/usr/local/bin:$PATH' > /etc/profile.d/skillhub-path.sh \
    && mkdir -p /workspace /go/pkg/mod /home/${USERNAME}/.cache \
    && chown -R "${USERNAME}:${USERNAME}" /workspace /go /opt/uv /home/${USERNAME}

USER ${USERNAME}
WORKDIR /workspace
