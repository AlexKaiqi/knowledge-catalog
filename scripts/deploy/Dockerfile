# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/kc ./cmd/kc \
    && CGO_ENABLED=0 go build -trimpath -o /out/fixture-deployment ./scripts/fixture-deployment

FROM debian:bookworm-slim AS kc
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
COPY --from=go-builder /out/kc /out/fixture-deployment /usr/local/bin/
COPY --chmod=0755 scripts/deploy/bootstrap.sh /usr/local/bin/kc-deploy-bootstrap
COPY --chmod=0755 scripts/deploy/server.sh /usr/local/bin/kc-deploy-server
WORKDIR /var/lib/kc
ENTRYPOINT ["/usr/local/bin/kc"]

FROM kc AS cli
ARG TARGETARCH=amd64
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
COPY --chmod=0755 scripts/deploy/cli.sh /usr/local/bin/kc-deploy-cli
COPY --chmod=0644 scripts/deploy/cli.profile.sh /etc/profile.d/kc-cli.sh
WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/kc-deploy-cli"]
