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

FROM node:${NODE_VERSION}-bookworm-slim AS dsh
ARG DSH_VERSION=0.1.1-rc.2
ARG MULTI_MODEL_VERSION=0.1.0-rc.19
ARG PNPM_VERSION=11.22.0
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates curl fuse3 git jq procps python3 ripgrep socat tini \
    && npm install --global --registry=https://registry.npmjs.org/ \
       "@deepseek-ai/dsh@${DSH_VERSION}" "dsh-multi-model-provider@${MULTI_MODEL_VERSION}" \
       "pnpm@${PNPM_VERSION}" \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-builder /out/kc /out/kcfs /usr/local/bin/
COPY --from=dsh-plugin-builder /opt/dsh-loom /opt/dsh-loom
COPY .data/data-warehouse/docker/gpt-web.patch.yml /opt/dsh-config/gpt-web.patch.yml
COPY --chmod=0755 .data/data-warehouse/docker/dsh.sh /usr/local/bin/kc-compose-dsh
COPY --chmod=0755 .data/data-warehouse/docker/smoke.sh /usr/local/bin/kc-compose-smoke
ENV DSH_MULTI_MODEL_VERSION=${MULTI_MODEL_VERSION} \
    KCFS_BIN=/usr/local/bin/kcfs
WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/kc-compose-dsh"]
