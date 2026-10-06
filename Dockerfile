# syntax=docker/dockerfile:1
ARG UPSTREAM_PROVIDERS_IMAGE=ghcr.io/obot-platform/providers@sha256:7139edce4c47a031149821f29201b7d0389d8e9c2f3401ccf5bd859ffeed3bae
ARG LOGINGOV_BUILDER_IMAGE=docker.io/library/golang:1.26.2-bookworm@sha256:47ce5636e9936b2c5cbf708925578ef386b4f8872aec74a67bd13a627d242b19
FROM cgr.dev/chainguard/wolfi-base AS base

RUN apk upgrade --no-cache && apk add --no-cache go-1.26 make git nodejs npm pnpm curl python-3.13 py3.13-pip

FROM base AS tools
WORKDIR /obot-tools/tools
COPY . /obot-tools/tools
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/uv \
    --mount=type=cache,target=/root/go/pkg/mod \
    UV_LINK_MODE=copy BIN_DIR=/bin make package-tools

FROM base AS providers
WORKDIR /obot-tools
COPY ./Makefile /obot-tools/
COPY ./scripts/package-providers.sh /obot-tools/scripts/

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    BIN_DIR=/bin make package-providers

# Preserve Obot's stock public providers and add the GSA login.gov provider as
# a separate registry. Pinning the upstream image makes this layer reproducible.
FROM ${UPSTREAM_PROVIDERS_IMAGE} AS upstream-public-providers

FROM ${LOGINGOV_BUILDER_IMAGE} AS logingov-provider-builder
WORKDIR /src/login.gov-auth-provider
COPY auth-providers-common /src/auth-providers-common
COPY login.gov-auth-provider/go.mod login.gov-auth-provider/go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download
COPY login.gov-auth-provider .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /login.gov-auth-provider .

FROM upstream-public-providers AS public-providers
COPY --chmod=755 packaging/providers/.envrc.providers.gsa-tools /obot-providers/.envrc.providers.gsa-tools
COPY packaging/providers/gsa-tools/auth-providers /obot-providers/gsa-tools/auth-providers
COPY --chmod=755 packaging/providers/login.gov-auth-provider-wrapper /obot-providers/gsa-tools/bin/login.gov-auth-provider
COPY --from=logingov-provider-builder /login.gov-auth-provider /obot-providers/gsa-tools/bin/login.gov-auth-provider.bin
COPY auth-providers-common/templates /obot-providers/gsa-tools/auth-providers-common/templates
COPY login.gov-auth-provider/tool.gpt /obot-providers/gsa-tools/login.gov-auth-provider/tool.gpt
