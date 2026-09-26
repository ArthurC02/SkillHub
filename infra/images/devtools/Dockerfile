# syntax=docker/dockerfile:1.7
FROM golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b

ARG NODE_VERSION=24.21.0
ARG TASK_VERSION=3.53.1
ARG UV_VERSION=0.12.13
ARG UV_X86_64_UNKNOWN_LINUX_GNU_SHA256=745765a3b6e360ad76743599ae5c42e9278c7edf8bbff9fc76d05bf2623a04dd
ARG UV_AARCH64_UNKNOWN_LINUX_GNU_SHA256=2eaa5d94f5db7b3a1a092156b9420459e42ab0217d917fe74a876309cef9b5e9
ARG GOLANGCI_LINT_VERSION=2.13.2
ARG DOCKER_VERSION=29.8.0
ARG COMPOSE_VERSION=5.5.1
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
    && case "$arch" in \
        amd64) \
            node_arch=x64; docker_arch=x86_64; uv_arch=x86_64; uv_sha256="${UV_X86_64_UNKNOWN_LINUX_GNU_SHA256}" ;; \
        arm64) \
            node_arch=arm64; docker_arch=aarch64; uv_arch=aarch64; uv_sha256="${UV_AARCH64_UNKNOWN_LINUX_GNU_SHA256}" ;; \
        *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
    esac \
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
    && curl -fsSL "https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/uv-${uv_arch}-unknown-linux-gnu.tar.gz" -o /tmp/uv.tar.gz \
    && echo "${uv_sha256}  /tmp/uv.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/uv.tar.gz -C /tmp \
    && install -m 0755 "/tmp/uv-${uv_arch}-unknown-linux-gnu/uv" /usr/local/bin/uv \
    && install -m 0755 "/tmp/uv-${uv_arch}-unknown-linux-gnu/uvx" /usr/local/bin/uvx \
    && rm -rf /tmp/uv.tar.gz "/tmp/uv-${uv_arch}-unknown-linux-gnu" \
    && uv python install 3.14 \
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
