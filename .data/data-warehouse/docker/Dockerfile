# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25
ARG NODE_VERSION=24.20.0

FROM golang:${GO_VERSION}-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/kc ./cmd/kc \
    && CGO_ENABLED=0 go build -trimpath -o /out/kcfs ./cmd/kcfs \
    && CGO_ENABLED=0 go build -trimpath -o /out/connector-preview ./.data/data-warehouse/connector/preview

FROM node:${NODE_VERSION}-bookworm-slim AS dsh-plugin-builder
WORKDIR /opt/dsh-loom
COPY dsh-plugin/package.json dsh-plugin/package-lock.json ./
RUN npm ci --ignore-scripts --legacy-peer-deps
COPY dsh-plugin/ ./
RUN npm run build

FROM debian:bookworm-slim AS kc
ARG TARGETARCH
ARG DOLT_VERSION=2.3.1
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates curl default-mysql-client git jq python3 tar \
    && case "${TARGETARCH}" in amd64|arm64) ;; *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; esac \
    && curl -fsSL "https://github.com/dolthub/dolt/releases/download/v${DOLT_VERSION}/dolt-linux-${TARGETARCH}.tar.gz" \
       | tar -xz -C /tmp \
    && install -m 0755 "/tmp/dolt-linux-${TARGETARCH}/bin/dolt" /usr/local/bin/dolt \
    && rm -rf /tmp/dolt-linux-* /var/lib/apt/lists/*
COPY --from=go-builder /out/kc /out/kcfs /out/connector-preview /usr/local/bin/
COPY .data/data-warehouse/connector /opt/data-warehouse/connector
COPY .data/data-warehouse/knowledge /opt/data-warehouse/knowledge
COPY --chmod=0755 .data/data-warehouse/docker/bootstrap.sh /usr/local/bin/kc-compose-bootstrap
COPY --chmod=0755 .data/data-warehouse/docker/server.sh /usr/local/bin/kc-compose-server
ENV PYTHONDONTWRITEBYTECODE=1 \
    KC_DOLT_BIN=/usr/local/bin/dolt
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/kc"]

FROM kc AS cli
ARG TARGETARCH
ARG TTYD_VERSION=1.7.7
RUN apt-get update \
    && apt-get install -y --no-install-recommends tini \
    && case "${TARGETARCH}" in \
         amd64) ttyd_arch=x86_64 ;; \
         arm64) ttyd_arch=aarch64 ;; \
         *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
       esac \
    && curl -fsSL "https://github.com/tsl0922/ttyd/releases/download/${TTYD_VERSION}/ttyd.${ttyd_arch}" \
         -o /usr/local/bin/ttyd \
    && chmod 0755 /usr/local/bin/ttyd \
    && ttyd --version \
    && rm -rf /var/lib/apt/lists/*
COPY --chmod=0755 .data/data-warehouse/docker/cli.sh /usr/local/bin/kc-compose-cli
COPY --chmod=0755 .data/data-warehouse/docker/cli-smoke.sh /usr/local/bin/kc-compose-cli-smoke
COPY --chmod=0644 .data/data-warehouse/docker/cli.profile.sh /etc/profile.d/kc-cli.sh
WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/kc-compose-cli"]

FROM node:${NODE_VERSION}-bookworm-slim AS dsh
ARG DSH_VERSION=0.1.1-rc.2
ARG MULTI_MODEL_VERSION=0.1.0-rc.19
ARG PNPM_VERSION=11.22.0
ARG DSH_PROFILE_LOCK_SHA256=737e938fce37dd6c248f37ea58ae0d2263aafc195e0b8df0ae4c31616c654b71
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates chromium curl fuse3 git jq procps python3 ripgrep socat tini \
    && npm install --global --registry=https://registry.npmjs.org/ \
       "@deepseek-ai/dsh@${DSH_VERSION}" "pnpm@${PNPM_VERSION}" \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-builder /out/kc /out/kcfs /usr/local/bin/
COPY --from=dsh-plugin-builder /opt/dsh-loom /opt/dsh-loom
COPY .data/data-warehouse/docker/gpt-web.patch.yml /opt/dsh-config/gpt-web.patch.yml
COPY .data/data-warehouse/docker/dsh_profile.py /usr/local/lib/kc/dsh_profile.py
RUN DSH_HOME=/run/dsh-home dsh plugin --profile kc-compose-web add "dsh-multi-model-provider@${MULTI_MODEL_VERSION}" \
    && DSH_HOME=/run/dsh-home dsh plugin --profile kc-compose-web add file:/opt/dsh-loom \
    && npm --prefix /run/dsh-home/profiles/kc-compose-web pkg set \
       'dsh.profile.bundles[0]=@deepseek-ai/dsh-base' \
       'dsh.profile.bundles[1]=@deepseek-ai/dsh-web-app' \
       'dsh.profile.bundles[2]=dsh-multi-model-provider' \
       'dsh.profile.bundles[3]=dsh-loom' \
    && actual_lock_sha256="$(sha256sum /run/dsh-home/profiles/kc-compose-web/pnpm-lock.yaml | cut -d' ' -f1)" \
    && echo "DSH profile lock: ${actual_lock_sha256}" \
    && test "${actual_lock_sha256}" = "${DSH_PROFILE_LOCK_SHA256}" \
    && mkdir -p /opt/dsh-profile-seed/profiles \
    && mv /run/dsh-home/profiles/kc-compose-web /opt/dsh-profile-seed/profiles/ \
    && rm -rf /opt/dsh-profile-seed/profiles/kc-compose-web/node_modules /run/dsh-home
COPY --chmod=0755 .data/data-warehouse/docker/dsh.sh /usr/local/bin/kc-compose-dsh
COPY --chmod=0755 .data/data-warehouse/docker/smoke.sh /usr/local/bin/kc-compose-smoke
COPY --chmod=0755 .data/data-warehouse/docker/web-smoke.sh /usr/local/bin/kc-compose-web-smoke
ENV DSH_VERSION=${DSH_VERSION} \
    DSH_MULTI_MODEL_VERSION=${MULTI_MODEL_VERSION} \
    DSH_PROFILE_LOCK_SHA256=${DSH_PROFILE_LOCK_SHA256} \
    DSH_HOME=/run/dsh-home \
    KCFS_BIN=/usr/local/bin/kcfs
WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/kc-compose-dsh"]
